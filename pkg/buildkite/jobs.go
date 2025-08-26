package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/buildkite/buildkite-mcp-server/pkg/buildkite/joblogs"
	"github.com/buildkite/buildkite-mcp-server/pkg/config"
	"github.com/buildkite/buildkite-mcp-server/pkg/tokens"
	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
)

type JobsClient interface {
	GetJobLog(ctx context.Context, org string, pipeline string, buildNumber string, jobID string) (buildkite.JobLog, *buildkite.Response, error)
	UnblockJob(ctx context.Context, org string, pipeline string, buildNumber string, jobID string, opt *buildkite.JobUnblockOptions) (buildkite.Job, *buildkite.Response, error)
}

// GetJobsArgs struct for typed parameters
type GetJobsArgs struct {
	OrgSlug      string `json:"org_slug"`
	PipelineSlug string `json:"pipeline_slug"`
	BuildNumber  string `json:"build_number"`
	JobState     string `json:"job_state"`
	IncludeAgent bool   `json:"include_agent"`
	Page         int    `json:"page"`
	PerPage      int    `json:"perPage"`
}

// GetJobLogsArgs struct for typed parameters
type GetJobLogsArgs struct {
	OrgSlug      string `json:"org_slug"`
	PipelineSlug string `json:"pipeline_slug"`
	BuildNumber  string `json:"build_number"`
	JobUUID      string `json:"job_uuid"`
}

// UnblockJobArgs struct for typed parameters
type UnblockJobArgs struct {
	OrgSlug      string            `json:"org_slug"`
	PipelineSlug string            `json:"pipeline_slug"`
	BuildNumber  string            `json:"build_number"`
	JobID        string            `json:"job_id"`
	Fields       map[string]string `json:"fields,omitempty"`
}

func GetJobs(client BuildsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[GetJobsArgs]) {
	return mcp.NewTool("get_jobs",
			mcp.WithDescription("Get all jobs for a specific build including their state, timing, commands, and execution details"),
			mcp.WithString("org_slug",
				mcp.Required(),
			),
			mcp.WithString("pipeline_slug",
				mcp.Required(),
			),
			mcp.WithString("build_number",
				mcp.Required(),
			),
			mcp.WithString("job_state",
				mcp.Description("Filter jobs by state. Supports actual states (scheduled, running, passed, failed, canceled, skipped, etc.)"),
			),
			mcp.WithBoolean("include_agent",
				mcp.Description("Include detailed agent information in the response. When false (default), only agent ID is included to reduce response size."),
			),
			mcp.WithNumber("page",
				mcp.Description("Page number for pagination (min 1)"),
				mcp.Min(1),
			),
			mcp.WithNumber("perPage",
				mcp.Description("Results per page for pagination (min 1, max 50)"),
				mcp.Min(1),
				mcp.Max(50),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Get Jobs",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, args GetJobsArgs) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.GetJobs")
			defer span.End()

			// Validate required parameters
			if args.OrgSlug == "" {
				return mcp.NewToolResultError("org_slug parameter is required"), nil
			}
			if args.PipelineSlug == "" {
				return mcp.NewToolResultError("pipeline_slug parameter is required"), nil
			}
			if args.BuildNumber == "" {
				return mcp.NewToolResultError("build_number parameter is required"), nil
			}

			// Set defaults for pagination
			page := args.Page
			if page == 0 {
				page = 1
			}
			perPage := args.PerPage
			if perPage == 0 {
				perPage = 30
			}

			paginationParams := ClientSidePaginationParams{
				Page:    page,
				PerPage: perPage,
			}

			span.SetAttributes(
				attribute.String("org_slug", args.OrgSlug),
				attribute.String("pipeline_slug", args.PipelineSlug),
				attribute.String("build_number", args.BuildNumber),
				attribute.String("job_state", args.JobState),
				attribute.Bool("include_agent", args.IncludeAgent),
				attribute.Int("page", paginationParams.Page),
				attribute.Int("per_page", paginationParams.PerPage),
			)

			build, resp, err := client.Get(ctx, args.OrgSlug, args.PipelineSlug, args.BuildNumber, &buildkite.BuildGetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get build: %s", string(body))), nil
			}

			jobs := build.Jobs

			// Filter jobs by state if specified
			if args.JobState != "" {
				filteredJobs := make([]buildkite.Job, 0)
				for _, job := range build.Jobs {
					if job.State == args.JobState {
						filteredJobs = append(filteredJobs, job)
					}
				}
				jobs = filteredJobs
			}

			// Remove agent details if not requested to reduce response size, but keep agent ID
			if !args.IncludeAgent {
				jobsWithoutAgent := make([]buildkite.Job, len(jobs))
				for i, job := range jobs {
					jobCopy := job
					// Keep only the agent ID, remove all other verbose agent details
					jobCopy.Agent = buildkite.Agent{ID: job.Agent.ID}
					jobsWithoutAgent[i] = jobCopy
				}
				jobs = jobsWithoutAgent
			}

			// Always apply client-side pagination
			result := applyClientSidePagination(jobs, paginationParams)
			r, err := json.Marshal(&result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal jobs: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func GetJobLogs(client *buildkite.Client) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[GetJobLogsArgs]) {
	return mcp.NewTool("get_job_logs",
			mcp.WithDescription("Get the log output and metadata for a specific job, including content, size, and header timestamps. Automatically saves to file for large logs to avoid token limits."),
			mcp.WithString("org_slug",
				mcp.Required(),
			),
			mcp.WithString("pipeline_slug",
				mcp.Required(),
			),
			mcp.WithString("build_number",
				mcp.Required(),
			),
			mcp.WithString("job_uuid",
				mcp.Required(),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Get Job Logs",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, args GetJobLogsArgs) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.GetJobLogs")
			defer span.End()

			// Validate required parameters
			if args.OrgSlug == "" {
				return mcp.NewToolResultError("org_slug parameter is required"), nil
			}
			if args.PipelineSlug == "" {
				return mcp.NewToolResultError("pipeline_slug parameter is required"), nil
			}
			if args.BuildNumber == "" {
				return mcp.NewToolResultError("build_number parameter is required"), nil
			}
			if args.JobUUID == "" {
				return mcp.NewToolResultError("job_uuid parameter is required"), nil
			}

			span.SetAttributes(
				attribute.String("org_slug", args.OrgSlug),
				attribute.String("pipeline_slug", args.PipelineSlug),
				attribute.String("build_number", args.BuildNumber),
				attribute.String("job_uuid", args.JobUUID),
			)

			// Get job logs from API
			joblog, resp, err := client.Jobs.GetJobLog(ctx, args.OrgSlug, args.PipelineSlug, args.BuildNumber, args.JobUUID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get job logs: %s", string(body))), nil
			}

			// the default logs that come from the API can be pretty dense with ANSI codes or HTML
			// so we can strip that out before returning it to the LLM
			processedLog, err := joblogs.Process(joblog)
			if err != nil {
				return nil, fmt.Errorf("failed to process job log: %w", err)
			}

			// Estimate tokens to decide delivery mode
			tokenCount := tokens.EstimateTokens(processedLog)
			span.SetAttributes(
				attribute.Int("token_count", tokenCount),
			)

			cfg := config.FromContext(ctx)

			// if a threshold is set, we can use it to determine if we should switch to file mode
			// this allows us to handle large logs more efficiently
			// and avoid hitting token limits in the LLM
			if threshold := cfg.JobLogTokenThreshold; threshold > 0 {
				span.SetAttributes(attribute.Int("token_threshold", threshold))

				// Smart switching: use file mode for large logs
				if tokenCount > threshold {

					log.Info().Int("token_count", tokenCount).Msg("Job log exceeds token threshold, switching to file mode")

					return handleLargeLogFile(ctx, processedLog, JobLogsResponse{
						TokenCount:  tokenCount,
						JobUUID:     args.JobUUID,
						BuildNumber: args.BuildNumber,
						Reason:      fmt.Sprintf("Log exceeded %d token threshold", threshold),
					}, threshold)
				}
			}

			// Inline mode for small logs
			span.SetAttributes(attribute.String("delivery_mode", "inline"))

			response := JobLogsResponse{
				DeliveryMode: "inline",
				Content:      processedLog,
				TokenCount:   tokenCount,
				JobUUID:      args.JobUUID,
				BuildNumber:  args.BuildNumber,
			}

			r, err := json.Marshal(&response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// JobLogsResponse represents the unified response for job logs
type JobLogsResponse struct {
	DeliveryMode  string `json:"delivery_mode"`             // "inline" | "file"
	Content       string `json:"content,omitempty"`         // Only for inline
	FilePath      string `json:"file_path,omitempty"`       // Only for file
	FileSizeBytes int64  `json:"file_size_bytes,omitempty"` // Only for file
	TokenCount    int    `json:"token_count"`               // Always included
	JobUUID       string `json:"job_uuid"`                  // Always included
	BuildNumber   string `json:"build_number"`              // Always included
	Reason        string `json:"reason,omitempty"`          // Why file mode was chosen
}

// handleLargeLogFile handles saving large logs to file
func handleLargeLogFile(ctx context.Context, processedLog string, response JobLogsResponse, threshold int) (*mcp.CallToolResult, error) {
	_, span := trace.Start(ctx, "buildkite.handleLargeLogFile")
	defer span.End()

	outputDir, err := os.MkdirTemp("", "buildkite-job-logs-") // Ensure the directory exists
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary output directory: %w", err)
	}

	filePath := filepath.Join(outputDir, "job.log")

	// Create and write file
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(processedLog)
	if err != nil {
		return nil, fmt.Errorf("failed to write log file: %w", err)
	}

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Get absolute path for response
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute file path: %w", err)
	}

	span.SetAttributes(
		attribute.String("file_path", absPath),
		attribute.Int64("file_size_bytes", fileInfo.Size()),
	)

	response.DeliveryMode = "file"
	response.FilePath = absPath
	response.FileSizeBytes = fileInfo.Size()
	response.Reason = fmt.Sprintf("Log exceeded %d token threshold, saved to file", threshold)

	r, err := json.Marshal(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return mcp.NewToolResultText(string(r)), nil
}

func UnblockJob(client JobsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[UnblockJobArgs]) {
	return mcp.NewTool("unblock_job",
			mcp.WithDescription("Unblock a blocked job in a Buildkite build to allow it to continue execution"),
			mcp.WithString("org_slug",
				mcp.Required(),
			),
			mcp.WithString("pipeline_slug",
				mcp.Required(),
			),
			mcp.WithString("build_number",
				mcp.Required(),
			),
			mcp.WithString("job_id",
				mcp.Required(),
			),
			mcp.WithObject("fields",
				mcp.Description("JSON object containing string values for block step fields"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Unblock Job",
				ReadOnlyHint: mcp.ToBoolPtr(false),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, args UnblockJobArgs) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.UnblockJob")
			defer span.End()

			// Validate required parameters
			if args.OrgSlug == "" {
				return mcp.NewToolResultError("org_slug parameter is required"), nil
			}
			if args.PipelineSlug == "" {
				return mcp.NewToolResultError("pipeline_slug parameter is required"), nil
			}
			if args.BuildNumber == "" {
				return mcp.NewToolResultError("build_number parameter is required"), nil
			}
			if args.JobID == "" {
				return mcp.NewToolResultError("job_id parameter is required"), nil
			}

			span.SetAttributes(
				attribute.String("org_slug", args.OrgSlug),
				attribute.String("pipeline_slug", args.PipelineSlug),
				attribute.String("build_number", args.BuildNumber),
				attribute.String("job_id", args.JobID),
			)

			// Prepare unblock options
			unblockOptions := buildkite.JobUnblockOptions{}
			if len(args.Fields) > 0 {
				unblockOptions.Fields = args.Fields
			}

			// Unblock the job
			job, _, err := client.UnblockJob(ctx, args.OrgSlug, args.PipelineSlug, args.BuildNumber, args.JobID, &unblockOptions)
			if err != nil {
				var errResp *buildkite.ErrorResponse
				if errors.As(err, &errResp) {
					if errResp.RawBody != nil {
						return mcp.NewToolResultError(string(errResp.RawBody)), nil
					}
				}

				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcpTextResult(span, &job)
		}
}
