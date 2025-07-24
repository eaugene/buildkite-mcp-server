package commands

import (
	"context"

	"github.com/buildkite/buildkite-mcp-server/pkg/server"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
)

type HTTPCmd struct {
	Listen string `help:"The address to listen on." default:"localhost:3000"`
}

func (c *HTTPCmd) Run(ctx context.Context, globals *Globals) error {

	mcpServer := server.NewMCPServer(globals.Version, globals.Client)

	httpServer := mcpserver.NewSSEServer(mcpServer)

	log.Ctx(ctx).Info().Str("address", c.Listen).Msg("Starting HTTP server")

	return httpServer.Start(c.Listen)
}
