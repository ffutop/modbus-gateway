package main

import (
	"log/slog"
	"os"
)

var logLevelMap = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

func main() {
	// Load configuration from command line and config file
	config, err := LoadConfig()
	if err != nil {
		slog.Error("load config failed", "err", err)
	}

	// Set json log handler
	level := logLevelMap[config.LogLevel]
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: level <= slog.LevelDebug,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Create and run gateway
	gateway := NewGateway(config)
	err = gateway.Run()
	if err != nil {
		slog.Error("gateway run failed", "err", err)
	}
}
