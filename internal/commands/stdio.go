package commands

import (
	"context"

	"github.com/mark3labs/mcp-go/server"
)

type StdioCmd struct{}

func (c *StdioCmd) Run(ctx context.Context, globals *Globals) error {
	s := NewMCPServer(globals)

	return server.ServeStdio(s,
		server.WithStdioContextFunc(
			setupContext(globals),
		),
	)
}

// NewMCPServer creates a new MCP server instance with the provided globals.
func setupContext(globals *Globals) server.StdioContextFunc {
	return func(ctx context.Context) context.Context {

		// add the logger to the context
		return globals.Logger.WithContext(ctx)
	}
}
