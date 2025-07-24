package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/buildkite/buildkite-mcp-server/pkg/server"
	gobuildkite "github.com/buildkite/go-buildkite/v4"
)

type ToolsCmd struct{}

func (c *ToolsCmd) Run(ctx context.Context, globals *Globals) error {

	client := &gobuildkite.Client{}

	// Collect all tools
	tools := server.BuildkiteTools(client)

	for _, tool := range tools {

		buf := new(bytes.Buffer)

		err := json.NewEncoder(buf).Encode(&tool.Tool)
		if err != nil {
			return err
		}

		fmt.Print(buf.String())

	}

	return nil
}
