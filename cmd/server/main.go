package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/lobo235/nomad-gateway/internal/api"
	"github.com/lobo235/nomad-gateway/internal/config"
	"github.com/lobo235/nomad-gateway/internal/nomad"
)

// version is set at build time via -ldflags "-X main.version=<value>".
var version = "dev"

func main() {
	// Handle --version before any config loading so the binary stays usable
	// even when env vars are absent.
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" || arg == "-v" {
			fmt.Printf("nomad-gateway version %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
			return
		}
	}

	// Bootstrap logger at INFO so we can log config errors before cfg is loaded.
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "error", err)
		os.Exit(1)
	}

	// Re-create logger at the configured level.
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	log.Info("starting nomad-gateway", "version", version, "log_level", cfg.LogLevel)

	nomadClient, err := nomad.NewClient(cfg.NomadAddr, cfg.NomadToken, log)
	if err != nil {
		log.Error("failed to create nomad client", "error", err)
		os.Exit(1)
	}

	srv := api.NewServer(nomadClient, cfg.GatewayAPIKey, version, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := ":" + cfg.Port
	if err := srv.Run(ctx, addr); err != nil {
		log.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
