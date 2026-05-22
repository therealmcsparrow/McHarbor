// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package config

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// App
	Port   int    `env:"PORT" envDefault:"5474"`
	Host   string `env:"HOST" envDefault:"0.0.0.0"`
	Secret string `env:"MCHARBOR_SECRET"`

	// Database
	DatabasePath string `env:"DATABASE_PATH" envDefault:"./data/mcharbor.db"`

	// Docker
	DockerHost      string `env:"DOCKER_HOST" envDefault:"unix:///var/run/docker.sock"`
	DockerTLSVerify string `env:"DOCKER_TLS_VERIFY"`
	DockerCertPath  string `env:"DOCKER_CERT_PATH"`

	// Kubernetes
	KubeconfigPath string `env:"KUBECONFIG"`

	// Auth
	AuthDisable        bool `env:"AUTH_DISABLE" envDefault:"false"`
	ForceSecureCookies bool `env:"FORCE_SECURE_COOKIES" envDefault:"false"`

	// Encryption
	EncryptionKey string `env:"ENCRYPTION_KEY"`
	DataDir       string `env:"DATA_DIR" envDefault:"./data"`

	// Logging
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	LogJSON  bool   `env:"LOG_JSON" envDefault:"false"`

	// CORS
	AllowedOrigins string `env:"ALLOWED_ORIGINS" envDefault:""`

	// App Store
	AppStoreCatalogURL string `env:"APPSTORE_CATALOG_URL" envDefault:""`
	AppStoreSyncCron   string `env:"APPSTORE_SYNC_CRON" envDefault:""`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c *Config) AllowedOriginList() []string {
	if c.AllowedOrigins != "" {
		origins := strings.Split(c.AllowedOrigins, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		return origins
	}

	return []string{
		"http://localhost:8173",
		fmt.Sprintf("http://localhost:%d", c.Port),
		"http://localhost:8705",
	}
}

func (c *Config) LogSlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
