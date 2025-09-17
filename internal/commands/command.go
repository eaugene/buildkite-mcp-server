package commands

import (
	"fmt"
	"runtime"

	buildkitelogs "github.com/buildkite/buildkite-logs"
	gobuildkite "github.com/buildkite/go-buildkite/v4"
)

type Globals struct {
	Client              *gobuildkite.Client
	BuildkiteLogsClient *buildkitelogs.Client
	Version             string
}

func UserAgent(version string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	return fmt.Sprintf("buildkite-mcp-server/%s (%s; %s)", version, os, arch)
}
