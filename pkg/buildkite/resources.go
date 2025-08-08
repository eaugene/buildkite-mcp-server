package buildkite

import (
	"context"
	"embed"

	"github.com/mark3labs/mcp-go/mcp"
)

//go:embed resources/*.md
var resourcesFS embed.FS

func HandleDebugLogsGuideResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	content, err := resourcesFS.ReadFile("resources/debug-logs-guide.md")
	if err != nil {
		return nil, err
	}

	return []mcp.ResourceContents{
		&mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/markdown",
			Text:     string(content),
		},
	}, nil
}
