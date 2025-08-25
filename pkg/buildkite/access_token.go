package buildkite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/buildkite/buildkite-mcp-server/pkg/tokens"
	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/attribute"
)

type AccessTokenClient interface {
	Get(ctx context.Context) (buildkite.AccessToken, *buildkite.Response, error)
}

func AccessToken(client AccessTokenClient) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("access_token",
			mcp.WithDescription("Get information about the current API access token including its scopes and UUID"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Get Access Token",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.AccessToken")
			defer span.End()

			token, _, err := client.Get(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			r, err := json.Marshal(&token)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal token: %w", err)
			}

			span.SetAttributes(
				attribute.Int("estimated_tokens", tokens.EstimateTokens(string(r))),
			)

			return mcp.NewToolResultText(string(r)), nil
		}
}
