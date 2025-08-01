package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"regexp"
	"time"

	buildkitelogs "github.com/buildkite/buildkite-logs"
	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel/attribute"
)

// BuildkiteLogsClient interface for dependency injection (matches upstream library interface)
type BuildkiteLogsClient interface {
	DownloadAndCache(ctx context.Context, org, pipeline, build, job string, cacheTTL time.Duration, forceRefresh bool) (string, error)
}

// Verify that upstream BuildkiteLogsClient implements our interface
var _ BuildkiteLogsClient = (*buildkitelogs.Client)(nil)

// Common parameter structures for log tools
type JobLogsBaseParams struct {
	Org          string `json:"org"`
	Pipeline     string `json:"pipeline"`
	Build        string `json:"build"`
	Job          string `json:"job"`
	Format       string `json:"format"`
	Raw          bool   `json:"raw"`
	PreserveANSI bool   `json:"preserve_ansi"`
	CacheTTL     string `json:"cache_ttl"`
	ForceRefresh bool   `json:"force_refresh"`
}

type SearchLogsParams struct {
	JobLogsBaseParams
	Pattern       string `json:"pattern"`
	Context       int    `json:"context"`
	BeforeContext int    `json:"before_context"`
	AfterContext  int    `json:"after_context"`
	CaseSensitive bool   `json:"case_sensitive"`
	InvertMatch   bool   `json:"invert_match"`
	Reverse       bool   `json:"reverse"`
	SeekStart     int    `json:"seek_start"`
	Limit         int    `json:"limit"`
}

type TailLogsParams struct {
	JobLogsBaseParams
	Tail int `json:"tail"`
}

type ReadLogsParams struct {
	JobLogsBaseParams
	Seek  int `json:"seek"`
	Limit int `json:"limit"`
}

// Response structures following the spec formats
type LogEntry struct {
	Timestamp int64  `json:"timestamp,omitempty"`
	Group     string `json:"group,omitempty"`
	Content   string `json:"content"`
	Command   bool   `json:"command,omitempty"`
	RowNumber int64  `json:"row_number,omitempty"`
}

type TerseLogEntry struct {
	TS  int64  `json:"ts,omitempty"`
	G   string `json:"g,omitempty"`
	C   string `json:"c"`
	CMD bool   `json:"cmd,omitempty"`
	RN  int64  `json:"rn,omitempty"`
}

// Use the library's types
type SearchResult = buildkitelogs.SearchResult
type FileInfo struct {
	buildkitelogs.ParquetFileInfo
	CacheFile string `json:"cache_file"`
}

type LogResponse struct {
	Results     interface{} `json:"results,omitempty"`
	Entries     interface{} `json:"entries,omitempty"`
	FileInfo    *FileInfo   `json:"file_info,omitempty"`
	MatchCount  int         `json:"match_count,omitempty"`
	TotalRows   int64       `json:"total_rows,omitempty"`
	QueryTimeMS int64       `json:"query_time_ms"`
}

// Use the library's SearchOptions
type SearchOptions = buildkitelogs.SearchOptions

// Real implementation using buildkite-logs-parquet library with injected client
func newParquetReader(ctx context.Context, client BuildkiteLogsClient, params JobLogsBaseParams) (*buildkitelogs.ParquetReader, error) {
	// Parse cache TTL
	ttl := parseCacheTTL(params.CacheTTL)

	// Download and cache the logs using injected client
	cacheFilePath, err := client.DownloadAndCache(ctx, params.Org, params.Pipeline, params.Build, params.Job, ttl, params.ForceRefresh)
	if err != nil {
		return nil, fmt.Errorf("failed to download/cache logs: %w", err)
	}

	// Create parquet reader from cached file
	reader := buildkitelogs.NewParquetReader(cacheFilePath)
	return reader, nil
}

func parseCacheTTL(ttlStr string) time.Duration {
	if ttlStr == "" {
		return 30 * time.Second
	}
	duration, err := time.ParseDuration(ttlStr)
	if err != nil {
		return 30 * time.Second
	}
	return duration
}

func validateSearchPattern(pattern string) error {
	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	return nil
}

func formatLogEntries(entries []buildkitelogs.ParquetLogEntry, format string, raw bool, preserveANSI bool) interface{} {
	if raw {
		result := make([]string, len(entries))
		for i, entry := range entries {
			if preserveANSI {
				result[i] = entry.Content
			} else {
				result[i] = entry.CleanContent(true)
			}
		}
		return result
	}

	switch format {
	case "json-terse":
		result := make([]TerseLogEntry, len(entries))
		for i, entry := range entries {
			content := entry.Content
			group := entry.Group
			if !preserveANSI {
				content = entry.CleanContent(true)
				group = entry.CleanGroup(true)
			}

			terse := TerseLogEntry{C: content}
			if entry.HasTime() {
				terse.TS = entry.Timestamp
			}
			if group != "" {
				terse.G = group
			}
			if entry.RowNumber > 0 {
				terse.RN = entry.RowNumber
			}
			result[i] = terse
		}
		return result
	case "json":
		result := make([]LogEntry, len(entries))
		for i, entry := range entries {
			content := entry.Content
			group := entry.Group
			if !preserveANSI {
				content = entry.CleanContent(true)
				group = entry.CleanGroup(true)
			}

			result[i] = LogEntry{
				Content: content,
				Group:   group,
			}
			if entry.HasTime() {
				result[i].Timestamp = entry.Timestamp
			}
			if entry.RowNumber > 0 {
				result[i].RowNumber = entry.RowNumber
			}
		}
		return result
	default: // text format
		result := make([]string, len(entries))
		for i, entry := range entries {
			content := entry.Content
			group := entry.Group
			if !preserveANSI {
				content = entry.CleanContent(true)
				group = entry.CleanGroup(true)
			}

			line := ""
			if entry.HasTime() {
				ts := time.UnixMilli(entry.Timestamp)
				line += fmt.Sprintf("[%s] ", ts.Format("2006-01-02 15:04:05.000"))
			}
			if group != "" {
				line += fmt.Sprintf("[%s] ", group)
			}
			line += content
			result[i] = line
		}
		return result
	}
}

// SearchLogs implements the search_logs MCP tool
func SearchLogs(client BuildkiteLogsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[SearchLogsParams]) {
	return mcp.NewTool("search_logs",
			mcp.WithDescription("Search log entries using regex patterns with optional context lines. 💡 For recent failures, try 'tail_logs' first, then use search_logs with patterns like 'error|failed|exception' and limit: 10-20. Default json-terse format: {ts: timestamp_ms, g: group_name, c: content, cmd: is_command, rn: row_number}."),
			mcp.WithString("org",
				mcp.Required(),
				mcp.Description("Buildkite organization slug"),
			),
			mcp.WithString("pipeline",
				mcp.Required(),
				mcp.Description("Pipeline slug"),
			),
			mcp.WithString("build",
				mcp.Required(),
				mcp.Description("Build number or UUID"),
			),
			mcp.WithString("job",
				mcp.Required(),
				mcp.Description("Job ID"),
			),
			mcp.WithString("pattern",
				mcp.Required(),
				mcp.Description("Regex pattern to search for"),
			),
			mcp.WithNumber("context",
				mcp.Description("Show NUM lines before and after each match (default: 0)"),
				mcp.Min(0),
			),
			mcp.WithNumber("before_context",
				mcp.Description("Show NUM lines before each match (default: 0)"),
				mcp.Min(0),
			),
			mcp.WithNumber("after_context",
				mcp.Description("Show NUM lines after each match (default: 0)"),
				mcp.Min(0),
			),
			mcp.WithBoolean("case_sensitive",
				mcp.Description("Case-sensitive search (default: false)"),
			),
			mcp.WithBoolean("invert_match",
				mcp.Description("Show non-matching lines (default: false)"),
			),
			mcp.WithBoolean("reverse",
				mcp.Description("Search backwards from end/seek position (default: false)"),
			),
			mcp.WithNumber("seek_start",
				mcp.Description("Start search from this row number (0-based, useful with reverse: true)"),
				mcp.Min(0),
			),
			mcp.WithNumber("limit",
				mcp.Description("Limit number of matches returned (default: 100, 0 = no limit)"),
				mcp.Min(0),
				mcp.DefaultNumber(100),
			),
			mcp.WithString("format",
				mcp.Description(`Output format - "text", "json", or "json-terse" (default: "json-terse")`),
			),
			mcp.WithBoolean("raw",
				mcp.Description("Output raw log content without timestamps/groups (default: false)"),
			),
			mcp.WithBoolean("preserve_ansi",
				mcp.Description("Preserve ANSI escape codes (default: false)"),
			),
			mcp.WithString("cache_ttl",
				mcp.Description(`Cache TTL for non-terminal jobs (default: "30s")`),
			),
			mcp.WithBoolean("force_refresh",
				mcp.Description("Force refresh cached entry (default: false)"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Search Logs",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, params SearchLogsParams) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.SearchLogs")
			defer span.End()

			startTime := time.Now()

			span.SetAttributes(
				attribute.String("org", params.Org),
				attribute.String("pipeline", params.Pipeline),
				attribute.String("build", params.Build),
				attribute.String("job", params.Job),
				attribute.String("pattern", params.Pattern),
				attribute.Int("context", params.Context),
				attribute.Bool("case_sensitive", params.CaseSensitive),
				attribute.Bool("invert_match", params.InvertMatch),
				attribute.Bool("reverse", params.Reverse),
				attribute.Int("limit", params.Limit),
				attribute.String("format", params.Format),
			)

			// Validate search pattern
			if err := validateSearchPattern(params.Pattern); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Set defaults
			if params.Format == "" {
				params.Format = "json-terse"
			}

			// Create parquet reader
			reader, err := newParquetReader(ctx, client, params.JobLogsBaseParams)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create log reader: %v", err)), nil
			}

			// Build search options
			opts := SearchOptions{
				Pattern:       params.Pattern,
				CaseSensitive: params.CaseSensitive,
				InvertMatch:   params.InvertMatch,
				Reverse:       params.Reverse,
				Context:       params.Context,
				BeforeContext: params.BeforeContext,
				AfterContext:  params.AfterContext,
			}

			// Perform search using iterator
			var results []SearchResult
			count := 0
			for result, err := range reader.SearchEntriesIter(opts) {
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Search error: %v", err)), nil
				}

				results = append(results, result)
				count++

				// Apply limit if specified
				if params.Limit > 0 && count >= params.Limit {
					break
				}
			}

			queryTime := time.Since(startTime)
			response := LogResponse{
				Results:     results,
				MatchCount:  len(results),
				QueryTimeMS: queryTime.Milliseconds(),
			}

			r, err := json.Marshal(&response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal search results: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// TailLogs implements the tail_logs MCP tool
func TailLogs(client BuildkiteLogsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[TailLogsParams]) {
	return mcp.NewTool("tail_logs",
			mcp.WithDescription("Show the last N entries from the log file. 🔥 RECOMMENDED for failure diagnosis - most build failures appear in the final log entries. More token-efficient than read_logs for recent issues. Default json-terse format: {ts: timestamp_ms, g: group_name, c: content, cmd: is_command, rn: row_number}."),
			mcp.WithString("org",
				mcp.Required(),
				mcp.Description("Buildkite organization slug"),
			),
			mcp.WithString("pipeline",
				mcp.Required(),
				mcp.Description("Pipeline slug"),
			),
			mcp.WithString("build",
				mcp.Required(),
				mcp.Description("Build number or UUID"),
			),
			mcp.WithString("job",
				mcp.Required(),
				mcp.Description("Job ID"),
			),
			mcp.WithNumber("tail",
				mcp.Description("Number of lines to show from end (default: 10)"),
				mcp.Min(1),
				mcp.DefaultNumber(10),
			),
			mcp.WithString("format",
				mcp.Description(`Output format - "text", "json", or "json-terse" (default: "json-terse")`),
			),
			mcp.WithBoolean("raw",
				mcp.Description("Output raw log content without timestamps/groups (default: false)"),
			),
			mcp.WithBoolean("preserve_ansi",
				mcp.Description("Preserve ANSI escape codes (default: false)"),
			),
			mcp.WithString("cache_ttl",
				mcp.Description(`Cache TTL for non-terminal jobs (default: "30s")`),
			),
			mcp.WithBoolean("force_refresh",
				mcp.Description("Force refresh cached entry (default: false)"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Tail Logs",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, params TailLogsParams) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.TailLogs")
			defer span.End()

			startTime := time.Now()

			// Set defaults
			if params.Tail <= 0 {
				params.Tail = 10
			}
			if params.Format == "" {
				params.Format = "json-terse"
			}

			span.SetAttributes(
				attribute.String("org", params.Org),
				attribute.String("pipeline", params.Pipeline),
				attribute.String("build", params.Build),
				attribute.String("job", params.Job),
				attribute.Int("tail", params.Tail),
				attribute.String("format", params.Format),
			)

			// Create parquet reader
			reader, err := newParquetReader(ctx, client, params.JobLogsBaseParams)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create log reader: %v", err)), nil
			}

			// Get file info first to calculate tail position
			fileInfo, err := reader.GetFileInfo()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get file info: %v", err)), nil
			}

			// Calculate starting position for tail
			startRow := fileInfo.RowCount - int64(params.Tail)
			if startRow < 0 {
				startRow = 0
			}

			// Get tail entries using SeekToRow
			var entries []buildkitelogs.ParquetLogEntry
			for entry, err := range reader.SeekToRow(startRow) {
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to read tail entries: %v", err)), nil
				}
				entries = append(entries, entry)
			}

			queryTime := time.Since(startTime)
			formattedEntries := formatLogEntries(entries, params.Format, params.Raw, params.PreserveANSI)

			response := LogResponse{
				Entries:     formattedEntries,
				TotalRows:   fileInfo.RowCount,
				QueryTimeMS: queryTime.Milliseconds(),
			}

			r, err := json.Marshal(&response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tail results: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// GetLogsInfo implements the get_logs_info MCP tool
func GetLogsInfo(client BuildkiteLogsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[JobLogsBaseParams]) {
	return mcp.NewTool("get_logs_info",
			mcp.WithDescription("Get metadata and statistics about the Parquet log file. 📊 RECOMMENDED as first step - check file size before reading large logs to plan your approach efficiently."),
			mcp.WithString("org",
				mcp.Required(),
				mcp.Description("Buildkite organization slug"),
			),
			mcp.WithString("pipeline",
				mcp.Required(),
				mcp.Description("Pipeline slug"),
			),
			mcp.WithString("build",
				mcp.Required(),
				mcp.Description("Build number or UUID"),
			),
			mcp.WithString("job",
				mcp.Required(),
				mcp.Description("Job ID"),
			),
			mcp.WithString("format",
				mcp.Description(`Output format - "text", "json", or "json-terse" (default: "json-terse")`),
			),
			mcp.WithString("cache_ttl",
				mcp.Description(`Cache TTL for non-terminal jobs (default: "30s")`),
			),
			mcp.WithBoolean("force_refresh",
				mcp.Description("Force refresh cached entry (default: false)"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Get Logs Info",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, params JobLogsBaseParams) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.GetLogsInfo")
			defer span.End()

			startTime := time.Now()

			// Set defaults
			if params.Format == "" {
				params.Format = "json-terse"
			}

			span.SetAttributes(
				attribute.String("org", params.Org),
				attribute.String("pipeline", params.Pipeline),
				attribute.String("build", params.Build),
				attribute.String("job", params.Job),
				attribute.String("format", params.Format),
			)

			// Create parquet reader
			reader, err := newParquetReader(ctx, client, params)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create log reader: %v", err)), nil
			}

			// Get file info
			libFileInfo, err := reader.GetFileInfo()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get file info: %v", err)), nil
			}

			// Get cache file path
			cacheFile, err := buildkitelogs.GetCacheFilePath(params.Org, params.Pipeline, params.Build, params.Job)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get cache file path: %v", err)), nil
			}

			// Create our response with additional cache file info
			fileInfo := &FileInfo{
				ParquetFileInfo: *libFileInfo,
				CacheFile:       cacheFile,
			}

			queryTime := time.Since(startTime)
			response := LogResponse{
				FileInfo:    fileInfo,
				QueryTimeMS: queryTime.Milliseconds(),
			}

			r, err := json.Marshal(&response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal file info: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ReadLogs implements the read_logs MCP tool
func ReadLogs(client BuildkiteLogsClient) (tool mcp.Tool, handler mcp.TypedToolHandlerFunc[ReadLogsParams]) {
	return mcp.NewTool("read_logs",
			mcp.WithDescription("Read log entries from the file, optionally starting from a specific row number. ⚠️ ALWAYS use 'limit' parameter to avoid excessive tokens. For recent failures, use 'tail_logs' instead. Recommended limits: investigation (100-500), exploration (use seek + small limits). Default json-terse format: {ts: timestamp_ms, g: group_name, c: content, cmd: is_command, rn: row_number}."),
			mcp.WithString("org",
				mcp.Required(),
				mcp.Description("Buildkite organization slug"),
			),
			mcp.WithString("pipeline",
				mcp.Required(),
				mcp.Description("Pipeline slug"),
			),
			mcp.WithString("build",
				mcp.Required(),
				mcp.Description("Build number or UUID"),
			),
			mcp.WithString("job",
				mcp.Required(),
				mcp.Description("Job ID"),
			),
			mcp.WithNumber("seek",
				mcp.Description("Row number to start from (0-based, default: 0)"),
				mcp.Min(0),
			),
			mcp.WithNumber("limit",
				mcp.Description("Limit number of entries returned (default: 100, 0 = no limit)"),
				mcp.Min(0),
				mcp.DefaultNumber(100),
			),
			mcp.WithString("format",
				mcp.Description(`Output format - "text", "json", or "json-terse" (default: "json-terse")`),
			),
			mcp.WithBoolean("raw",
				mcp.Description("Output raw log content without timestamps/groups (default: false)"),
			),
			mcp.WithBoolean("preserve_ansi",
				mcp.Description("Preserve ANSI escape codes (default: false)"),
			),
			mcp.WithString("cache_ttl",
				mcp.Description(`Cache TTL for non-terminal jobs (default: "30s")`),
			),
			mcp.WithBoolean("force_refresh",
				mcp.Description("Force refresh cached entry (default: false)"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Read Logs",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		),
		func(ctx context.Context, request mcp.CallToolRequest, params ReadLogsParams) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.ReadLogs")
			defer span.End()

			startTime := time.Now()

			// Set defaults
			if params.Format == "" {
				params.Format = "json-terse"
			}

			span.SetAttributes(
				attribute.String("org", params.Org),
				attribute.String("pipeline", params.Pipeline),
				attribute.String("build", params.Build),
				attribute.String("job", params.Job),
				attribute.Int("seek", params.Seek),
				attribute.Int("limit", params.Limit),
				attribute.String("format", params.Format),
			)

			// Create parquet reader
			reader, err := newParquetReader(ctx, client, params.JobLogsBaseParams)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create log reader: %v", err)), nil
			}

			// Read entries with seek and limit
			var entries []buildkitelogs.ParquetLogEntry
			count := 0

			// Choose iterator based on seek parameter
			var entryIter iter.Seq2[buildkitelogs.ParquetLogEntry, error]
			if params.Seek > 0 {
				entryIter = reader.SeekToRow(int64(params.Seek))
			} else {
				entryIter = reader.ReadEntriesIter()
			}

			for entry, err := range entryIter {
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to read entries: %v", err)), nil
				}

				entries = append(entries, entry)
				count++

				// Apply limit if specified
				if params.Limit > 0 && count >= params.Limit {
					break
				}
			}

			queryTime := time.Since(startTime)
			formattedEntries := formatLogEntries(entries, params.Format, params.Raw, params.PreserveANSI)

			response := LogResponse{
				Entries:     formattedEntries,
				QueryTimeMS: queryTime.Milliseconds(),
			}

			r, err := json.Marshal(&response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal read results: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}
