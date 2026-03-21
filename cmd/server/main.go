package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lobo235/nomad-gateway/internal/api"
	"github.com/lobo235/nomad-gateway/internal/config"
	"github.com/lobo235/nomad-gateway/internal/nomad"
)

// version is set at build time via -ldflags "-X main.version=<value>".
var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "error", err)
		os.Exit(1)
	}

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
