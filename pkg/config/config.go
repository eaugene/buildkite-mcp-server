package config

import (
	"context"
	"slices"
	"strings"
)

type Config struct {
	JobLogTokenThreshold int      `help:"The threshold for logging tokens. Default is 0, which means no tokens are logged." default:"0" env:"JOB_LOG_TOKEN_THRESHOLD"`
	EnabledToolsets      []string `help:"Comma-separated list of toolsets to enable (e.g., 'pipelines,builds,clusters'). Use 'all' to enable all toolsets." default:"all" env:"BUILDKITE_TOOLSETS"`
	ReadOnly             bool     `help:"Enable read-only mode, which filters out write operations from all toolsets." default:"false" env:"BUILDKITE_READ_ONLY"`
	DynamicToolsets      bool     `help:"Enable dynamic toolset discovery for optimized AI agent interactions." default:"false" env:"BUILDKITE_DYNAMIC_TOOLSETS"`
}

// IsToolsetEnabled checks if a toolset is enabled based on the configuration
func (c *Config) IsToolsetEnabled(name string) bool {
	if slices.Contains(c.EnabledToolsets, "all") {
		return true
	}
	return slices.Contains(c.EnabledToolsets, name)
}

// ParseToolsets parses a comma-separated string of toolset names
func (c *Config) ParseToolsets(toolsetsStr string) {
	if toolsetsStr == "" {
		c.EnabledToolsets = []string{"all"}
		return
	}

	toolsets := strings.Split(toolsetsStr, ",")
	for i, toolset := range toolsets {
		toolsets[i] = strings.TrimSpace(toolset)
	}
	c.EnabledToolsets = toolsets
}

// configContextKey is used as a key for storing Config in context
type configContextKey struct{}

func FromContext(ctx context.Context) *Config {
	if config, ok := ctx.Value(configContextKey{}).(*Config); ok {
		return config
	}
	return &Config{
		JobLogTokenThreshold: 0, // Default value
		EnabledToolsets:      []string{"all"},
		ReadOnly:             false,
		DynamicToolsets:      false,
	}
}

func WithConfig(ctx context.Context, config *Config) context.Context {
	return context.WithValue(ctx, configContextKey{}, config)
}
