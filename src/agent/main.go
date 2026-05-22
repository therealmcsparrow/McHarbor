// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
)

// Config holds agent configuration loaded from environment variables.
type Config struct {
	McHarborURL string `env:"MCHARBOR_URL,required"`
	AgentToken  string `env:"MCHARBOR_AGENT_TOKEN,required"`
	DockerHost  string `env:"DOCKER_HOST" envDefault:"unix:///var/run/docker.sock"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	Insecure    bool   `env:"MCHARBOR_INSECURE" envDefault:"false"`
}

func main() {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Set up logger
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	logger.Info("starting mcharbor-agent",
		"version", agentVersion,
		"server", cfg.McHarborURL,
		"dockerHost", cfg.DockerHost,
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signals
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-done
		logger.Info("shutting down...")
		cancel()
	}()

	agent := NewAgent(cfg, logger)
	RunWithReconnect(ctx, agent, logger)

	logger.Info("agent stopped")
}
