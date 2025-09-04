package server

import (
	buildkitelogs "github.com/buildkite/buildkite-logs"
	"github.com/buildkite/buildkite-mcp-server/internal/tools"
	"github.com/buildkite/buildkite-mcp-server/pkg/buildkite"
	"github.com/buildkite/buildkite-mcp-server/pkg/config"
	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	gobuildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
)

func NewMCPServerWithConfig(version string, client *gobuildkite.Client, buildkiteLogsClient *buildkitelogs.Client, cfg *config.Config) *server.MCPServer {
	s := server.NewMCPServer(
		"buildkite-mcp-server",
		version,
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithHooks(trace.NewHooks()),
		server.WithLogging())

	log.Info().Str("version", version).Msg("Starting Buildkite MCP server")

	// Use toolset system with configuration
	if cfg != nil {
		s.AddTools(BuildkiteTools(client, buildkiteLogsClient, WithConfig(cfg))...)
	} else {
		// Use default toolset configuration when no config provided
		s.AddTools(BuildkiteTools(client, buildkiteLogsClient)...)
	}

	s.AddPrompt(mcp.NewPrompt("user_token_organization_prompt",
		mcp.WithPromptDescription("When asked for detail of a users pipelines start by looking up the user's token organization"),
	), buildkite.HandleUserTokenOrganizationPrompt)

	s.AddResource(mcp.NewResource(
		"debug-logs-guide",
		"Debug Logs Guide",
		mcp.WithResourceDescription("Comprehensive guide for debugging Buildkite build failures using logs"),
	), buildkite.HandleDebugLogsGuideResource)

	return s
}

// ToolsetOption configures toolset behavior
type ToolsetOption func(*ToolsetConfig)

// ToolsetConfig holds configuration for toolset selection and behavior
type ToolsetConfig struct {
	EnabledToolsets []string
	ReadOnly        bool
	DynamicToolsets bool
}

// WithToolsets enables specific toolsets
func WithToolsets(toolsets ...string) ToolsetOption {
	return func(cfg *ToolsetConfig) {
		cfg.EnabledToolsets = toolsets
	}
}

// WithReadOnly enables read-only mode which filters out write operations
func WithReadOnly(readOnly bool) ToolsetOption {
	return func(cfg *ToolsetConfig) {
		cfg.ReadOnly = readOnly
	}
}

// WithDynamicToolsets enables dynamic toolset discovery tools
func WithDynamicToolsets(dynamic bool) ToolsetOption {
	return func(cfg *ToolsetConfig) {
		cfg.DynamicToolsets = dynamic
	}
}

// WithConfig applies settings from a config.Config struct
func WithConfig(cfg *config.Config) ToolsetOption {
	return func(toolsetCfg *ToolsetConfig) {
		toolsetCfg.EnabledToolsets = cfg.EnabledToolsets
		toolsetCfg.ReadOnly = cfg.ReadOnly
		toolsetCfg.DynamicToolsets = cfg.DynamicToolsets
	}
}

// BuildkiteTools creates tools using the toolset system with functional options
func BuildkiteTools(client *gobuildkite.Client, buildkiteLogsClient *buildkitelogs.Client, opts ...ToolsetOption) []server.ServerTool {
	// Default configuration
	cfg := &ToolsetConfig{
		EnabledToolsets: []string{"all"},
		ReadOnly:        false,
		DynamicToolsets: false,
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}
	// Create builtin toolsets
	builtinToolsets := tools.CreateBuiltinToolsets(client, buildkiteLogsClient)

	// Create registry and register toolsets
	registry := tools.NewToolsetRegistry()
	for name, toolset := range builtinToolsets {
		registry.Register(name, toolset)
	}

	// Get enabled tools with read-only filtering
	enabledTools := registry.GetEnabledTools(cfg.EnabledToolsets, cfg.ReadOnly)

	// Add introspection tools if dynamic toolsets is enabled
	if cfg.DynamicToolsets {
		introspectionTools := tools.CreateIntrospectionTools(registry)
		enabledTools = append(enabledTools, introspectionTools...)
	}

	// Convert to ServerTool format
	var serverTools []server.ServerTool
	for _, toolDef := range enabledTools {
		serverTools = append(serverTools, server.ServerTool{
			Tool:    toolDef.Tool,
			Handler: toolDef.Handler,
		})
	}

	log.Info().
		Strs("enabled_toolsets", cfg.EnabledToolsets).
		Bool("read_only", cfg.ReadOnly).
		Bool("dynamic_toolsets", cfg.DynamicToolsets).
		Int("tool_count", len(serverTools)).
		Msg("Registered tools from toolsets")

	return serverTools
}
