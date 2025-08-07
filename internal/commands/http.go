package commands

import (
	"context"
	"fmt"

	"github.com/buildkite/buildkite-mcp-server/pkg/server"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
)

type HTTPCmd struct {
	Listen string `help:"The address to listen on." default:"localhost:3000"`
	UseSSE bool   `help:"Use deprecated SSS transport instead of Streamable HTTP." default:"false"`
}

func (c *HTTPCmd) Run(ctx context.Context, globals *Globals) error {
	mcpServer := server.NewMCPServer(globals.Version, globals.Client, globals.BuildkiteLogsClient)
	logEvent := log.Ctx(ctx).Info().Str("address", c.Listen)

	if c.UseSSE {
		httpServer := mcpserver.NewSSEServer(mcpServer)
		endpoint := fmt.Sprintf("http://%s%s", c.Listen, httpServer.CompleteSsePath())
		logEvent.Str("transport", "sse").Str("endpoint", endpoint).Msg("Starting SSE HTTP server")

		return httpServer.Start(c.Listen)
	} else {
		httpServer := mcpserver.NewStreamableHTTPServer(mcpServer)
		endpoint := fmt.Sprintf("http://%s/mcp", c.Listen)
		logEvent.Str("transport", "streamable-http").Str("endpoint", endpoint).Msg("Starting streamable HTTP server")

		return httpServer.Start(c.Listen)
	}
}
