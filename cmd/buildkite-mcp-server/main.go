package main

import (
	"context"
	"os"

	"github.com/alecthomas/kong"
	"github.com/buildkite/buildkite-mcp-server/internal/commands"
	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	gobuildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/rs/zerolog"
	buildkitelogs "github.com/wolfeidau/buildkite-logs-parquet"
)

var (
	version = "dev"

	cli struct {
		Stdio       commands.StdioCmd `cmd:"" help:"stdio mcp server."`
		HTTP        commands.HTTPCmd  `cmd:"" help:"http mcp server."`
		Tools       commands.ToolsCmd `cmd:"" help:"list available tools." hidden:""`
		APIToken    string            `help:"The Buildkite API token to use." env:"BUILDKITE_API_TOKEN"`
		BaseURL     string            `help:"The base URL of the Buildkite API to use." env:"BUILDKITE_BASE_URL" default:"https://api.buildkite.com/"`
		CacheURL    string            `help:"The blob storage URL for job logs cache." env:"BKLOG_CACHE_URL"`
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

	client, err := gobuildkite.NewOpts(
		gobuildkite.WithTokenAuth(cli.APIToken),
		gobuildkite.WithUserAgent(commands.UserAgent(version)),
		gobuildkite.WithHTTPClient(trace.NewHTTPClientWithHeaders(headers)),
		gobuildkite.WithBaseURL(cli.BaseURL),
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create buildkite client")
	}

	// Create ParquetClient with cache URL from flag/env (uses upstream library's high-level client)
	parquetClient := buildkitelogs.NewParquetClient(client, cli.CacheURL)

	err = cmd.Run(&commands.Globals{Version: version, Client: client, ParquetClient: parquetClient, Logger: logger})
	cmd.FatalIfErrorf(err)
}
