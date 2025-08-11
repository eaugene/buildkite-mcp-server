package buildkite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/buildkite/buildkite-mcp-server/pkg/trace"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type OrganizationsClient interface {
	List(ctx context.Context, options *buildkite.OrganizationListOptions) ([]buildkite.Organization, *buildkite.Response, error)
}

func UserTokenOrganization(client OrganizationsClient) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("user_token_organization",
			mcp.WithDescription("Get the organization associated with the user token used for this request"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        "Get Organization for User Token",
				ReadOnlyHint: mcp.ToBoolPtr(true),
			}),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, span := trace.Start(ctx, "buildkite.UserTokenOrganization")
			defer span.End()

			orgs, resp, err := client.List(ctx, &buildkite.OrganizationListOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if resp.StatusCode != 200 {
				return mcp.NewToolResultError("failed to get current user organizations"), nil
			}

			if len(orgs) == 0 {
				return mcp.NewToolResultError("no organization found for the current user token"), nil
			}

			r, err := json.Marshal(&orgs[0])
			if err != nil {
				return nil, fmt.Errorf("failed to marshal user organizations: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func HandleUserTokenOrganizationPrompt(
	ctx context.Context,
	request mcp.GetPromptRequest,
) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "When asked for detail of a users pipelines start by looking up the user's token organization",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: "When asked for detail of a users pipelines start by looking up the user's token organization",
				},
			},
		},
	}, nil
}
