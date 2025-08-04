package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel/attribute"
)

type BuildsClient interface {
	Get(ctx context.Context, org, pipelineSlug, buildNumber string, options *buildkite.BuildGetOptions) (buildkite.Build, *buildkite.Response, error)
	ListByPipeline(ctx context.Context, org, pipelineSlug string, options *buildkite.BuildsListOptions) ([]buildkite.Build, *buildkite.Response, error)
	Create(ctx context.Context, org string, pipeline string, b buildkite.CreateBuild) (buildkite.Build, *buildkite.Response, error)
}

// JobSummary represents a summary of jobs grouped by state, with finished jobs classified as passed/failed
type JobSummary struct {
	Total   int            `json:"total"`
	ByState map[string]int `json:"by_state"`
}

// BuildSummary - Essential fields (~85% token reduction)
type BuildSummary struct {
	ID        string               `json:"id"`
	Number    int                  `json:"number"`
	State     string               `json:"state"`
	Branch    string               `json:"branch"`
	Commit    string               `json:"commit"`
	Message   string               `json:"message"`
	WebURL    string               `json:"web_url"`
	CreatedAt *buildkite.Timestamp `json:"created_at"`
	JobsTotal int                  `json:"jobs_total"`
}

// BuildDetail - Medium detail (~60% token reduction)
type BuildDetail struct {
	BuildSummary                      // Embed summary fields
	Source       string               `json:"source"`
	Author       buildkite.Author     `json:"author"`
	StartedAt    *buildkite.Timestamp `json:"started_at"`
	FinishedAt   *buildkite.Timestamp `json:"finished_at"`
	JobSummary   *JobSummary          `json:"job_summary"`
	// Exclude: Jobs[], Env{}, MetaData{}, Pipeline{}, TestEngine{}
}

// BuildWithSummary represents a build with job summary and optionally full job details
type BuildWithSummary struct {
	buildkite.Build
	JobSummary *JobSummary `json:"job_summary"`
}

// ListBuildsArgs struct with enhanced filtering
type ListBuildsArgs struct {
	Org          string `json:"org"`
	PipelineSlug string `json:"pipeline_slug"`
	Branch       string `json:"branch"`       // existing
	State        string `json:"state"`        // NEW: running, passed, failed, etc.
	Commit       string `json:"commit"`       // NEW: specific commit SHA
	Creator      string `json:"creator"`      // NEW: filter by build creator
	DetailLevel  string `json:"detail_level"` // summary, detailed, full
	Page         int    `json:"page"`
	PerPage      int    `json:"per_page"`
}

// GetBuildArgs struct
type GetBuildArgs struct {
	Org          string `json:"org"`
	PipelineSlug string `json:"pipeline_slug"`
	BuildNumber  string `json:"build_number"`
	DetailLevel  string `json:"detail_level"` // summary, detailed, full
}

// GetBuildTestEngineRunsArgs struct
type GetBuildTestEngineRunsArgs struct {
	Org          string `json:"org"`
	PipelineSlug string `json:"pipeline_slug"`
	BuildNumber  string `json:"build_number"`
}

// Helper functions for build conversion

// summarizeBuild converts a buildkite.Build to BuildSummary
func summarizeBuild(build buildkite.Build) BuildSummary {
	return BuildSummary{
		ID:        build.ID,
		Number:    build.Number,
		State:     build.State,
		Branch:    build.Branch,
		Commit:    build.Commit,
		Message:   build.Message,
		WebURL:    build.WebURL,
		CreatedAt: build.CreatedAt,
		JobsTotal: len(build.Jobs),
	}
}

// detailBuild converts a buildkite.Build to BuildDetail with job summary
func detailBuild(build buildkite.Build) BuildDetail {
	summary := summarizeBuild(build)

	// Create job summary
	jobSummary := &JobSummary{
		Total:   len(build.Jobs),
		ByState: make(map[string]int),
	}

	for _, job := range build.Jobs {
		if job.State == "" {
			continue
		}
		jobSummary.ByState[job.State]++
	}

	return BuildDetail{
		BuildSummary: summary,
		Source:       build.Source,
		Author:       build.Author,
		StartedAt:    build.StartedAt,
		FinishedAt:   build.FinishedAt,
		JobSummary:   jobSummary,
	}
}

// createPaginatedBuildResult creates a paginated result with the appropriate converter
func createPaginatedBuildResult[T any](builds []buildkite.Build, converter func(buildkite.Build) T, headers map[string]string) PaginatedResult[T] {
	items := make([]T, len(builds))
	for i, build := range builds {
		items[i] = converter(build)
	}

	return PaginatedResult[T]{
		Items:   items,
		Headers: headers,
	}
}

func ListBuilds(client BuildsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[ListBuildsArgs]) {
	return mcp.NewTool("list_builds",
			mcp.WithDescription("List all builds for a pipeline with their status, commit information, and metadata"),
			mcp.WithString("org",
				mcp.Required(),
				mcp.Description("The organization slug for the owner of the pipeline"),
			),
			mcp.WithString("pipeline_slug",
				mcp.Required(),
				mcp.Description("The slug of the pipeline"),
			),
			mcp.WithString("branch",
				mcp.Description("Filter builds by git branch name"),
			),
			mcp.WithString("state",
				mcp.Description("Filter builds by state. Supports actual states (scheduled, running, passed, failed, canceled, skipped, etc.)"),
			),
			mcp.WithString("commit",
				mcp.Description("Filter builds by specific commit SHA"),
			),
			mcp.WithString("creator",
				mcp.Description("Filter builds by build creator"),
			),
			mcp.WithString("detail_level",
				mcp.Description("Response detail level: 'summary' (essential fields), 'detailed' (medium detail), or 'full' (complete build data). Default: 'summary'"),
			),
			mcp.WithNumber("page",
				mcp.Description("Page number for pagination (min 1)"),
			),
			mcp.WithNumber("per_page",
				mcp.Description("Results per page for pagination (min 1, max 100)"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "List Builds",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, args ListBuildsArgs) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.ListBuilds")
			defer span.End()

			// Validate required parameters
			if args.Org == "" {
				return mcp.NewToolResultError("org parameter is required"), nil
			}
			if args.PipelineSlug == "" {
				return mcp.NewToolResultError("pipeline_slug parameter is required"), nil
			}

			span.SetAttributes(
				attribute.String("org", args.Org),
				attribute.String("pipeline_slug", args.PipelineSlug),
				attribute.String("branch", args.Branch),
				attribute.String("state", args.State),
				attribute.String("commit", args.Commit),
				attribute.String("creator", args.Creator),
				attribute.String("detail_level", args.DetailLevel),
				attribute.Int("page", args.Page),
				attribute.Int("per_page", args.PerPage),
			)

			// Set default detail level
			detailLevel := args.DetailLevel
			if detailLevel == "" {
				detailLevel = "summary"
			}

			// Set default pagination
			page := args.Page
			if page == 0 {
				page = 1
			}
			perPage := args.PerPage
			if perPage == 0 {
				perPage = 30
			}

			options := &buildkite.BuildsListOptions{
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
			}

			// Set exclusions based on detail level
			switch detailLevel {
			case "summary":
				options.ExcludeJobs = true
				options.ExcludePipeline = true
			case "detailed":
				options.ExcludeJobs = true
				options.ExcludePipeline = true
			case "full":
				// Include everything
			default:
				return mcp.NewToolResultError("detail_level must be 'summary', 'detailed', or 'full'"), nil
			}

			// Apply filters
			if args.Branch != "" {
				options.Branch = []string{args.Branch}
			}
			if args.State != "" {
				options.State = []string{args.State}
			}
			if args.Commit != "" {
				options.Commit = args.Commit
			}
			if args.Creator != "" {
				options.Creator = args.Creator
			}

			builds, resp, err := client.ListByPipeline(ctx, args.Org, args.PipelineSlug, options)
			if err != nil {
				var errResp *buildkite.ErrorResponse
				if errors.As(err, &errResp) {
					if errResp.RawBody != nil {
						return mcp.NewToolResultError(string(errResp.RawBody)), nil
					}
				}

				return mcp.NewToolResultError(err.Error()), nil
			}

			headers := map[string]string{
				"Link": resp.Header.Get("Link"),
			}

			var result any
			switch detailLevel {
			case "summary":
				result = createPaginatedBuildResult(builds, summarizeBuild, headers)
			case "detailed":
				result = createPaginatedBuildResult(builds, detailBuild, headers)
			case "full":
				result = PaginatedResult[buildkite.Build]{
					Items:   builds,
					Headers: headers,
				}
			}

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal builds: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func GetBuildTestEngineRuns(client BuildsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[GetBuildTestEngineRunsArgs]) {
	return mcp.NewTool("get_build_test_engine_runs",
			mcp.WithDescription("Get test engine runs data for a specific build in Buildkite. This can be used to look up Test Runs."),
			mcp.WithString("org",
				mcp.Required(),
				mcp.Description("The organization slug for the owner of the pipeline"),
			),
			mcp.WithString("pipeline_slug",
				mcp.Required(),
				mcp.Description("The slug of the pipeline"),
			),
			mcp.WithString("build_number",
				mcp.Required(),
				mcp.Description("The number of the build"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Get Build Test Engine Runs",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, args GetBuildTestEngineRunsArgs) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.GetBuildTestEngineRuns")
			defer span.End()

			// Validate required parameters
			if args.Org == "" {
				return mcp.NewToolResultError("org parameter is required"), nil
			}
			if args.PipelineSlug == "" {
				return mcp.NewToolResultError("pipeline_slug parameter is required"), nil
			}
			if args.BuildNumber == "" {
				return mcp.NewToolResultError("build_number parameter is required"), nil
			}

			span.SetAttributes(
				attribute.String("org", args.Org),
				attribute.String("pipeline_slug", args.PipelineSlug),
				attribute.String("build_number", args.BuildNumber),
			)

			build, _, err := client.Get(ctx, args.Org, args.PipelineSlug, args.BuildNumber, &buildkite.BuildGetOptions{
				IncludeTestEngine: true,
			})
			if err != nil {
				var errResp *buildkite.ErrorResponse
				if errors.As(err, &errResp) {
					if errResp.RawBody != nil {
						return mcp.NewToolResultError(string(errResp.RawBody)), nil
					}
				}

				return mcp.NewToolResultError(err.Error()), nil
			}

			// Extract just the test engine runs data
			var testEngineRuns []buildkite.TestEngineRun
			if build.TestEngine != nil {
				testEngineRuns = build.TestEngine.Runs
			}

			r, err := json.Marshal(&testEngineRuns)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal test engine runs: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func GetBuild(client BuildsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[GetBuildArgs]) {
	return mcp.NewTool("get_build",
			mcp.WithDescription("Get detailed information about a specific build including its jobs, timing, and execution details"),
			mcp.WithString("org",
				mcp.Required(),
				mcp.Description("The organization slug for the owner of the pipeline"),
			),
			mcp.WithString("pipeline_slug",
				mcp.Required(),
				mcp.Description("The slug of the pipeline"),
			),
			mcp.WithString("build_number",
				mcp.Required(),
				mcp.Description("The number of the build"),
			),
			mcp.WithString("detail_level",
				mcp.Description("Response detail level: 'summary' (essential fields), 'detailed' (medium detail), or 'full' (complete build data). Default: 'detailed'"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Get Build",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, args GetBuildArgs) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.GetBuild")
			defer span.End()

			// Validate required parameters
			if args.Org == "" {
				return mcp.NewToolResultError("org parameter is required"), nil
			}
			if args.PipelineSlug == "" {
				return mcp.NewToolResultError("pipeline_slug parameter is required"), nil
			}
			if args.BuildNumber == "" {
				return mcp.NewToolResultError("build_number parameter is required"), nil
			}

			span.SetAttributes(
				attribute.String("org", args.Org),
				attribute.String("pipeline_slug", args.PipelineSlug),
				attribute.String("build_number", args.BuildNumber),
				attribute.String("detail_level", args.DetailLevel),
			)

			// Set default detail level
			detailLevel := args.DetailLevel
			if detailLevel == "" {
				detailLevel = "detailed"
			}

			// Configure build get options based on detail level
			options := &buildkite.BuildGetOptions{
				IncludeTestEngine: true,
			}

			build, _, err := client.Get(ctx, args.Org, args.PipelineSlug, args.BuildNumber, options)
			if err != nil {
				var errResp *buildkite.ErrorResponse
				if errors.As(err, &errResp) {
					if errResp.RawBody != nil {
						return mcp.NewToolResultError(string(errResp.RawBody)), nil
					}
				}

				return mcp.NewToolResultError(err.Error()), nil
			}

			var result any
			switch detailLevel {
			case "summary":
				result = summarizeBuild(build)
			case "detailed":
				result = detailBuild(build)
			case "full":
				result = build
			default:
				return mcp.NewToolResultError("detail_level must be 'summary', 'detailed', or 'full'"), nil
			}

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal build: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CreateBuildArgs struct {
	OrgSlug      string  `json:"org_slug"`
	PipelineSlug string  `json:"pipeline_slug"`
	Commit       string  `json:"commit"`
	Branch       string  `json:"branch"`
	Message      string  `json:"message"`
	Environment  []Entry `json:"environment"`
	MetaData     []Entry `json:"metadata"`
}

func CreateBuild(client BuildsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[CreateBuildArgs]) {
	return mcp.NewTool("create_build",
			mcp.WithDescription("Trigger a new build on a Buildkite pipeline for a specific commit and branch, with optional environment variables, metadata, and author information"),
			mcp.WithString("org_slug",
				mcp.Required(),
				mcp.Description("The organization slug for the owner of the pipeline"),
			),
			mcp.WithString("pipeline_slug",
				mcp.Required(),
				mcp.Description("The slug of the pipeline"),
			),
			mcp.WithString("commit",
				mcp.Required(),
				mcp.Description("The commit SHA to build"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("The branch to build"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("The commit message for the build"),
			),
			mcp.WithArray("environment",
				mcp.Items(
					map[string]any{
						"type":     "object",
						"required": []string{"key", "value"},
						"properties": map[string]any{
							"key": map[string]any{
								"type":        "string",
								"description": "The name of the environment variable",
							},
							"value": map[string]any{
								"type":        "string",
								"description": "The value of the environment variable",
							},
						},
					},
				),
				mcp.Description("Environment variables to set for the build")),
			mcp.WithArray("metadata",
				mcp.Items(
					map[string]any{
						"type":     "object",
						"required": []string{"key", "value"},
						"properties": map[string]any{
							"key": map[string]any{
								"type":        "string",
								"description": "The key of the meta-data item",
							},
							"value": map[string]any{
								"type":        "string",
								"description": "The value of the meta-data item",
							},
						},
					},
				),
				mcp.Description("Meta-data values to set for the build")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Create Build",
				ReadOnlyHint: mcp.ToBoolPtr(false),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, args CreateBuildArgs) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.CreateBuild")
			defer span.End()

			createBuild := buildkite.CreateBuild{
				Commit:   args.Commit,
				Branch:   args.Branch,
				Message:  args.Message,
				Env:      convertEntries(args.Environment),
				MetaData: convertEntries(args.MetaData),
			}

			span.SetAttributes(
				attribute.String("org", args.OrgSlug),
				attribute.String("pipeline_slug", args.PipelineSlug),
			)

			build, _, err := client.Create(ctx, args.OrgSlug, args.PipelineSlug, createBuild)
			if err != nil {
				var errResp *buildkite.ErrorResponse
				if errors.As(err, &errResp) {
					if errResp.RawBody != nil {
						return mcp.NewToolResultError(string(errResp.RawBody)), nil
					}
				}

				return mcp.NewToolResultError(err.Error()), nil
			}

			r, err := json.Marshal(&build)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal created build: %w", err)
			}
			return mcp.NewToolResultText(string(r)), nil
		}
}

func convertEntries(entries []Entry) map[string]string {
	if entries == nil {
		return nil
	}

	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		result[entry.Key] = entry.Value
	}
	return result
}
