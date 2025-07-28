package commands

import (
	"fmt"
	"runtime"

	gobuildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/rs/zerolog"
	buildkitelogs "github.com/wolfeidau/buildkite-logs-parquet"
)

type Globals struct {
	Client              *gobuildkite.Client
	BuildkiteLogsClient *buildkitelogs.Client
	Version             string
	Logger              zerolog.Logger
}

func UserAgent(version string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	return fmt.Sprintf("buildkite-mcp-server/%s (%s; %s)", version, os, arch)
}
