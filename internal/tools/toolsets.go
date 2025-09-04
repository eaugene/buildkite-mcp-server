package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	buildkitelogs "github.com/buildkite/buildkite-logs"
	"github.com/buildkite/buildkite-mcp-server/pkg/buildkite"
	gobuildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolDefinition wraps an MCP tool with additional metadata
type ToolDefinition struct {
	Tool           mcp.Tool
	Handler        server.ToolHandlerFunc
	RequiredScopes []string // Buildkite API token scopes required for this tool
}

// IsReadOnly returns true if the tool is read-only
func (td ToolDefinition) IsReadOnly() bool {
	if td.Tool.Annotations.ReadOnlyHint == nil {
		return false
	}
	return *td.Tool.Annotations.ReadOnlyHint
}

// Toolset represents a logical grouping of related tools
type Toolset struct {
	Name        string
	Description string
	Tools       []ToolDefinition
}

// GetReadOnlyTools returns only the read-only tools from this toolset
func (ts Toolset) GetReadOnlyTools() []ToolDefinition {
	var readOnlyTools []ToolDefinition
	for _, tool := range ts.Tools {
		if tool.IsReadOnly() {
			readOnlyTools = append(readOnlyTools, tool)
		}
	}
	return readOnlyTools
}

// GetAllTools returns all tools from this toolset
func (ts Toolset) GetAllTools() []ToolDefinition {
	return ts.Tools
}

// GetRequiredScopes returns all unique scopes required by tools in this toolset
func (ts Toolset) GetRequiredScopes() []string {
	scopeMap := make(map[string]bool)
	for _, tool := range ts.Tools {
		for _, scope := range tool.RequiredScopes {
			scopeMap[scope] = true
		}
	}

	scopes := make([]string, 0, len(scopeMap))
	for scope := range scopeMap {
		scopes = append(scopes, scope)
	}
	slices.Sort(scopes)
	return scopes
}

// ToolsetRegistry manages the registration and discovery of toolsets
type ToolsetRegistry struct {
	toolsets map[string]Toolset
}

// NewToolsetRegistry creates a new toolset registry
func NewToolsetRegistry() *ToolsetRegistry {
	return &ToolsetRegistry{
		toolsets: make(map[string]Toolset),
	}
}

// Register adds a toolset to the registry
func (tr *ToolsetRegistry) Register(name string, toolset Toolset) {
	tr.toolsets[name] = toolset
}

// Get retrieves a toolset by name
func (tr *ToolsetRegistry) Get(name string) (Toolset, bool) {
	toolset, exists := tr.toolsets[name]
	return toolset, exists
}

// List returns all registered toolset names
func (tr *ToolsetRegistry) List() []string {
	names := make([]string, 0, len(tr.toolsets))
	for name := range tr.toolsets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// GetEnabledTools returns tools from enabled toolsets, optionally filtering for read-only
func (tr *ToolsetRegistry) GetEnabledTools(enabledToolsets []string, readOnlyMode bool) []ToolDefinition {
	var tools []ToolDefinition

	// If "all" is specified, enable all toolsets
	if slices.Contains(enabledToolsets, "all") {
		enabledToolsets = tr.List()
	}

	for _, toolsetName := range enabledToolsets {
		if toolset, exists := tr.toolsets[toolsetName]; exists {
			if readOnlyMode {
				tools = append(tools, toolset.GetReadOnlyTools()...)
			} else {
				tools = append(tools, toolset.GetAllTools()...)
			}
		}
	}

	return tools
}

// ToolsetMetadata provides information about a toolset for introspection
type ToolsetMetadata struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	ToolCount     int    `json:"tool_count"`
	ReadOnlyCount int    `json:"read_only_count"`
}

// GetMetadata returns metadata for all registered toolsets
func (tr *ToolsetRegistry) GetMetadata() []ToolsetMetadata {
	metadata := make([]ToolsetMetadata, 0, len(tr.toolsets))

	for name, toolset := range tr.toolsets {
		readOnlyCount := len(toolset.GetReadOnlyTools())
		metadata = append(metadata, ToolsetMetadata{
			Name:          name,
			Description:   toolset.Description,
			ToolCount:     len(toolset.Tools),
			ReadOnlyCount: readOnlyCount,
		})
	}

	// Sort by name for consistency
	slices.SortFunc(metadata, func(a, b ToolsetMetadata) int {
		if a.Name < b.Name {
			return -1
		} else if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return metadata
}

// GetRequiredScopes returns all unique scopes required by enabled toolsets
func (tr *ToolsetRegistry) GetRequiredScopes(enabledToolsets []string, readOnlyMode bool) []string {
	scopeMap := make(map[string]bool)

	// If "all" is specified, enable all toolsets
	if slices.Contains(enabledToolsets, "all") {
		enabledToolsets = tr.List()
	}

	for _, toolsetName := range enabledToolsets {
		if toolset, exists := tr.toolsets[toolsetName]; exists {
			var tools []ToolDefinition
			if readOnlyMode {
				tools = toolset.GetReadOnlyTools()
			} else {
				tools = toolset.GetAllTools()
			}

			for _, tool := range tools {
				for _, scope := range tool.RequiredScopes {
					scopeMap[scope] = true
				}
			}
		}
	}

	scopes := make([]string, 0, len(scopeMap))
	for scope := range scopeMap {
		scopes = append(scopes, scope)
	}
	slices.Sort(scopes)
	return scopes
}

// NewTool creates a new tool definition with annotations based on access level
func NewTool(tool mcp.Tool, handler server.ToolHandlerFunc, scopes []string) ToolDefinition {
	return ToolDefinition{
		Tool:           tool,
		Handler:        handler,
		RequiredScopes: scopes,
	}
}

// CreateBuiltinToolsets creates the default toolsets with all available tools
func CreateBuiltinToolsets(client *gobuildkite.Client, buildkiteLogsClient *buildkitelogs.Client) map[string]Toolset {
	// Create a client adapter for artifact tools
	clientAdapter := &buildkite.BuildkiteClientAdapter{Client: client}

	return map[string]Toolset{
		"clusters": {
			Name:        "Cluster Management",
			Description: "Tools for managing Buildkite clusters and cluster queues",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.GetCluster(client.Clusters) }),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.ListClusters(client.Clusters) }),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					return buildkite.GetClusterQueue(client.ClusterQueues)
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					return buildkite.ListClusterQueues(client.ClusterQueues)
				}),
			},
		},
		"pipelines": {
			Name:        "Pipeline Management",
			Description: "Tools for managing Buildkite pipelines",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.GetPipeline(client.Pipelines)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.ListPipelines(client.Pipelines)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.CreatePipeline(client.Pipelines)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.UpdatePipeline(client.Pipelines)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
			},
		},
		"builds": {
			Name:        "Build Operations",
			Description: "Tools for managing builds and jobs",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.ListBuilds(client.Builds)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.GetBuild(client.Builds)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.GetBuildTestEngineRuns(client.Builds)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.CreateBuild(client.Builds)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.WaitForBuild(client.Builds)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.GetJobs(client.Builds)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.UnblockJob(client.Jobs)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
			},
		},
		"artifacts": {
			Name:        "Artifact Management",
			Description: "Tools for managing build artifacts",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.ListArtifacts(clientAdapter) }),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.GetArtifact(clientAdapter) }),
			},
		},
		"tests": {
			Name:        "Test Engine",
			Description: "Tools for managing test runs and test results",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.ListTestRuns(client.TestRuns) }),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.GetTestRun(client.TestRuns) }),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					return buildkite.GetFailedTestExecutions(client.TestRuns)
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.GetTest(client.Tests) }),
			},
		},
		"logs": {
			Name:        "Log Management",
			Description: "Tools for searching, reading, and analyzing job logs",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.SearchLogs(buildkiteLogsClient)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.TailLogs(buildkiteLogsClient)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.GetLogsInfo(buildkiteLogsClient)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					tool, handler, scopes := buildkite.ReadLogs(buildkiteLogsClient)
					return tool, mcp.NewTypedToolHandler(handler), scopes
				}),
			},
		},
		"annotations": {
			Name:        "Annotation Management",
			Description: "Tools for managing build annotations",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					return buildkite.ListAnnotations(client.Annotations)
				}),
			},
		},
		"user": {
			Name:        "User & Organization",
			Description: "Tools for user and organization information",
			Tools: []ToolDefinition{
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.CurrentUser(client.User) }),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) {
					return buildkite.UserTokenOrganization(client.Organizations)
				}),
				newToolFromFunc(func() (mcp.Tool, server.ToolHandlerFunc, []string) { return buildkite.AccessToken(client.AccessTokens) }),
			},
		},
	}
}

// newToolFromFunc creates a new ToolDefinition from a function that returns (tool, handler, scopes)
func newToolFromFunc(toolFunc func() (mcp.Tool, server.ToolHandlerFunc, []string)) ToolDefinition {
	tool, handler, scopes := toolFunc()
	return NewTool(tool, handler, scopes)
}

// CreateIntrospectionTools creates tools for toolset discovery and introspection
func CreateIntrospectionTools(registry *ToolsetRegistry) []ToolDefinition {
	return []ToolDefinition{
		NewTool(
			mcp.NewTool("list_toolsets",
				mcp.WithDescription("List all available toolsets with their descriptions and tool counts"),
				mcp.WithToolAnnotation(mcp.ToolAnnotation{
					Title:        "List Toolsets",
					ReadOnlyHint: mcp.ToBoolPtr(true),
				}),
			),
			func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				metadata := registry.GetMetadata()

				result := map[string]any{
					"toolsets":       metadata,
					"total_toolsets": len(metadata),
				}

				jsonBytes, err := json.Marshal(result)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				return mcp.NewToolResultText(string(jsonBytes)), nil
			},
			[]string{},
		),
		NewTool(
			mcp.NewTool("get_toolset_info",
				mcp.WithDescription("Get detailed information about a specific toolset including its tools"),
				mcp.WithString("toolset_name", mcp.Required()),
				mcp.WithToolAnnotation(mcp.ToolAnnotation{
					Title:        "Get Toolset Information",
					ReadOnlyHint: mcp.ToBoolPtr(true),
				}),
			),
			func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				toolsetName, err := request.RequireString("toolset_name")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				toolset, exists := registry.Get(toolsetName)
				if !exists {
					return mcp.NewToolResultError("toolset not found: " + toolsetName), nil
				}

				// Convert tools to a serializable format
				tools := make([]map[string]any, len(toolset.Tools))
				readOnlyCount := 0
				allScopes := make(map[string]bool)
				for i, tool := range toolset.Tools {
					isReadOnly := tool.IsReadOnly()
					if isReadOnly {
						readOnlyCount++
					}
					// Collect all unique scopes for this toolset
					for _, scope := range tool.RequiredScopes {
						allScopes[scope] = true
					}
					tools[i] = map[string]any{
						"name":        tool.Tool.Name,
						"description": tool.Tool.Description,
						"read_only":   isReadOnly,
						"scopes":      tool.RequiredScopes,
					}
				}

				// Convert scope map to sorted slice
				toolsetScopes := make([]string, 0, len(allScopes))
				for scope := range allScopes {
					toolsetScopes = append(toolsetScopes, scope)
				}
				slices.Sort(toolsetScopes)

				result := map[string]any{
					"name":            toolset.Name,
					"description":     toolset.Description,
					"tool_count":      len(toolset.Tools),
					"read_only_count": readOnlyCount,
					"required_scopes": toolsetScopes,
					"tools":           tools,
				}

				jsonBytes, err := json.Marshal(result)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				return mcp.NewToolResultText(string(jsonBytes)), nil
			},
			[]string{},
		),
		NewTool(
			mcp.NewTool("get_required_scopes",
				mcp.WithDescription("Get the minimum Buildkite API token scopes required for a given toolset configuration"),
				mcp.WithString("toolsets"),
				mcp.WithString("read_only"),
				mcp.WithToolAnnotation(mcp.ToolAnnotation{
					Title:        "Get Required Scopes",
					ReadOnlyHint: mcp.ToBoolPtr(true),
				}),
			),
			func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				toolsetsStr, err := request.RequireString("toolsets")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				readOnlyStr, _ := request.RequireString("read_only")
				if readOnlyStr == "" {
					readOnlyStr = "false" // default to false
				}
				readOnly := readOnlyStr == "true"

				// Parse toolsets string
				var enabledToolsets []string
				if toolsetsStr == "all" {
					enabledToolsets = []string{"all"}
				} else {
					enabledToolsets = strings.Split(toolsetsStr, ",")
					for i, toolset := range enabledToolsets {
						enabledToolsets[i] = strings.TrimSpace(toolset)
					}
				}

				// Get required scopes
				requiredScopes := registry.GetRequiredScopes(enabledToolsets, readOnly)

				result := map[string]any{
					"toolsets":        enabledToolsets,
					"read_only_mode":  readOnly,
					"required_scopes": requiredScopes,
					"scope_count":     len(requiredScopes),
				}

				jsonBytes, err := json.Marshal(result)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				return mcp.NewToolResultText(string(jsonBytes)), nil
			},
			[]string{},
		),
	}
}
