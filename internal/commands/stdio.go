package commands

import (
	"context"

	"github.com/buildkite/buildkite-mcp-server/internal/toolsets"
	"github.com/buildkite/buildkite-mcp-server/pkg/config"
	"github.com/buildkite/buildkite-mcp-server/pkg/server"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type StdioCmd struct {
	EnabledToolsets []string `help:"Comma-separated list of toolsets to enable (e.g., 'pipelines,builds,clusters'). Use 'all' to enable all toolsets." default:"all" env:"BUILDKITE_TOOLSETS"`
	ReadOnly        bool     `help:"Enable read-only mode, which filters out write operations from all toolsets." default:"false" env:"BUILDKITE_READ_ONLY"`
	config.Config            // embed configuration options for stdio server
}

func (c *StdioCmd) Run(ctx context.Context, globals *Globals) error {
	// Validate the enabled toolsets
	if err := toolsets.ValidateToolsets(c.EnabledToolsets); err != nil {
		return err
	}

	s := server.NewMCPServer(globals.Version, globals.Client, globals.BuildkiteLogsClient,
		server.WithReadOnly(c.ReadOnly), server.WithToolsets(c.EnabledToolsets...))

	return mcpserver.ServeStdio(s,
		mcpserver.WithStdioContextFunc(
			setupContext(&c.Config, globals),
		),
	)
}

func setupContext(cfg *config.Config, globals *Globals) mcpserver.StdioContextFunc {
	return func(ctx context.Context) context.Context {

		// add the logger to the context
		ctx = globals.Logger.WithContext(ctx)

		// Here you can modify the context based on environment variables or other logic
		return config.WithConfig(ctx, cfg)
	}
}
