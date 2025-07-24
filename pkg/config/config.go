package config

import "context"

type Config struct {
	JobLogTokenThreshold int `help:"The threshold for logging tokens. Default is 0, which means no tokens are logged." default:"0" env:"JOB_LOG_TOKEN_THRESHOLD"`
}

func FromContext(ctx context.Context) *Config {
	if config, ok := ctx.Value(Config{}).(*Config); ok {
		return config
	}
	return &Config{
		JobLogTokenThreshold: 0, // Default value
	}
}

func WithConfig(ctx context.Context, config *Config) context.Context {
	return context.WithValue(ctx, Config{}, config)
}
