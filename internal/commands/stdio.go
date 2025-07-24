package commands

import (
	"context"

	"github.com/buildkite/buildkite-mcp-server/pkg/config"
	"github.com/buildkite/buildkite-mcp-server/pkg/server"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type StdioCmd struct {
	config.Config // embed configuration options for stdio server
}

func (c *StdioCmd) Run(ctx context.Context, globals *Globals) error {
	s := server.NewMCPServer(globals.Version, globals.Client)

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
