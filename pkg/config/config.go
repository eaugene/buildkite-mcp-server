package config

import (
	"context"
)

type Config struct {
	JobLogTokenThreshold int `help:"The threshold for logging tokens. Default is 0, which means no tokens are logged." default:"0" env:"JOB_LOG_TOKEN_THRESHOLD"`
}

// configContextKey is used as a key for storing Config in context
type configContextKey struct{}

func FromContext(ctx context.Context) *Config {
	if config, ok := ctx.Value(configContextKey{}).(*Config); ok {
		return config
	}
	return &Config{
		JobLogTokenThreshold: 0, // Default value
	}
}

func WithConfig(ctx context.Context, config *Config) context.Context {
	return context.WithValue(ctx, configContextKey{}, config)
}
