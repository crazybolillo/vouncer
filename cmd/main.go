package main

import (
	"context"
	"github.com/caarlos0/env/v11"
	"log/slog"
	"os"
	"vouncer/internal/serve"
)

func main() {
	os.Exit(run(context.Background()))
}

func run(ctx context.Context) int {
	cfg, err := env.ParseAs[serve.Config]()
	if err != nil {
		slog.Error("Failed to parse configuration", slog.String("reason", err.Error()))
		return 1
	}

	if cfg.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	return serve.Start(ctx, cfg)
}
