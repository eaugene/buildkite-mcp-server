package main

import (
	"context"
	"os"

	"github.com/alecthomas/kong"
	"github.com/buildkite/buildkite-mcp-server/internal/commands"
	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/rs/zerolog"
)

var (
	version = "dev"

	cli struct {
		Stdio       commands.StdioCmd `cmd:"" help:"stdio mcp server."`
		HTTP        commands.HTTPCmd  `cmd:"" help:"http mcp server."`
		Tools       commands.ToolsCmd `cmd:"" help:"list available tools." hidden:""`
		APIToken    string            `help:"The Buildkite API token to use." env:"BUILDKITE_API_TOKEN"`
		BaseURL     string            `help:"The base URL of the Buildkite API to use." env:"BUILDKITE_BASE_URL" default:"https://api.buildkite.com/"`
		Debug       bool              `help:"Enable debug mode."`
		HTTPHeaders []string          `help:"Additional HTTP headers to send with every request. Format: 'Key: Value'" name:"http-header" env:"BUILDKITE_HTTP_HEADERS"`
		Version     kong.VersionFlag
	}
)

func main() {
	ctx := context.Background()

	cmd := kong.Parse(&cli,
		kong.Name("buildkite-mcp-server"),
		kong.Description("A server that proxies requests to the Buildkite API."),
		kong.UsageOnError(),
		kong.Vars{
			"version": version,
		},
		kong.BindTo(ctx, (*context.Context)(nil)),
	)

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	if cli.Debug {
		logger = logger.Level(zerolog.DebugLevel).With().Caller().Logger()
	}

	tp, err := trace.NewProvider(ctx, "buildkite-mcp-server", version)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create trace provider")
	}
	defer func() {
		_ = tp.Shutdown(ctx)
	}()

	// Parse additional headers into a map
	headers := commands.ParseHeaders(cli.HTTPHeaders, logger)

	client, err := buildkite.NewOpts(
		buildkite.WithTokenAuth(cli.APIToken),
		buildkite.WithUserAgent(commands.UserAgent(version)),
		buildkite.WithHTTPClient(trace.NewHTTPClientWithHeaders(headers)),
		buildkite.WithBaseURL(cli.BaseURL),
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create buildkite client")
	}

	err = cmd.Run(&commands.Globals{Version: version, Client: client, Logger: logger})
	cmd.FatalIfErrorf(err)
}
