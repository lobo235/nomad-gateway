package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	NomadAddr     string
	NomadToken    string
	GatewayAPIKey string
	Port          string
}

func Load() (*Config, error) {
	// Load .env if present — ignore error if file doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		NomadAddr:     os.Getenv("NOMAD_ADDR"),
		NomadToken:    os.Getenv("NOMAD_TOKEN"),
		GatewayAPIKey: os.Getenv("GATEWAY_API_KEY"),
		Port:          os.Getenv("PORT"),
	}

	if cfg.NomadAddr == "" {
		return nil, fmt.Errorf("NOMAD_ADDR is required")
	}
	if cfg.NomadToken == "" {
		return nil, fmt.Errorf("NOMAD_TOKEN is required")
	}
	if cfg.GatewayAPIKey == "" {
		return nil, fmt.Errorf("GATEWAY_API_KEY is required")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	return cfg, nil
}
