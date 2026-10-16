// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package config

import (
	"crypto/tls"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLoadUsesDefaultsAndEnvironment(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ALLOWED_ORIGINS", "https://one.test, https://two.test ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Port != 9999 || cfg.Host != "127.0.0.1" || cfg.LogLevel != "debug" {
		t.Fatalf("loaded config = %#v", cfg)
	}
	if cfg.DatabasePath == "" || cfg.DataDir == "" {
		t.Fatalf("defaults should be populated: %#v", cfg)
	}
}

func TestLoadWrapsParseError(t *testing.T) {
	t.Setenv("PORT", "not-a-port")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "parsing config") {
		t.Fatalf("Load error = %v, want parsing config", err)
	}
}

func TestAddr(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: 5474}
	if got := cfg.Addr(); got != "127.0.0.1:5474" {
		t.Fatalf("Addr() = %q", got)
	}
}

func TestAllowedOriginList(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		cfg := &Config{AllowedOrigins: " https://one.test,https://two.test "}
		want := []string{"https://one.test", "https://two.test"}
		if got := cfg.AllowedOriginList(); !reflect.DeepEqual(got, want) {
			t.Fatalf("AllowedOriginList() = %#v, want %#v", got, want)
		}
	})

	t.Run("defaults", func(t *testing.T) {
		cfg := &Config{Port: 5474}
		want := []string{
			"http://localhost:8173", "https://localhost:8173",
			"http://localhost:5474", "https://localhost:5474",
			"http://localhost:8705", "https://localhost:8705",
		}
		if got := cfg.AllowedOriginList(); !reflect.DeepEqual(got, want) {
			t.Fatalf("AllowedOriginList() = %#v, want %#v", got, want)
		}
	})
}

func TestLogSlogLevel(t *testing.T) {
	tests := []struct {
		level string
		want  slog.Level
	}{
		{level: "debug", want: slog.LevelDebug},
		{level: "warn", want: slog.LevelWarn},
		{level: "error", want: slog.LevelError},
		{level: "info", want: slog.LevelInfo},
		{level: "unknown", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			cfg := &Config{LogLevel: tt.level}
			if got := cfg.LogSlogLevel(); got != tt.want {
				t.Fatalf("LogSlogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTLSMinVersionDefaultsToTLS13(t *testing.T) {
	t.Setenv("TLS_MIN_VERSION", "")
	if err := os.Unsetenv("TLS_MIN_VERSION"); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.TLSMinVersion != TLSMinVersion(tls.VersionTLS13) {
		t.Fatalf("default TLSMinVersion = 0x%04x, want TLS 1.3 (0x%04x)",
			uint16(cfg.TLSMinVersion), tls.VersionTLS13)
	}
}

func TestTLSMinVersionAcceptsTLS12Downgrade(t *testing.T) {
	t.Setenv("TLS_MIN_VERSION", "1.2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.TLSMinVersion != TLSMinVersion(tls.VersionTLS12) {
		t.Fatalf("TLSMinVersion = 0x%04x, want TLS 1.2 (0x%04x)",
			uint16(cfg.TLSMinVersion), tls.VersionTLS12)
	}
}

func TestTLSMinVersionRefusesBelowTLS12(t *testing.T) {
	for _, value := range []string{"1.0", "1.1", "0x0301"} {
		t.Setenv("TLS_MIN_VERSION", value)
		if _, err := Load(); err == nil {
			t.Fatalf("TLS_MIN_VERSION=%q was accepted, want refusal", value)
		}
	}
}

func TestDBConfigDrivers(t *testing.T) {
	tests := []struct {
		driver     string
		wantDriver string
		wantTarget string
	}{
		{driver: "", wantDriver: "sqlite", wantTarget: "/data/mcharbor.db"},
		{driver: "sqlite", wantDriver: "sqlite", wantTarget: "/data/mcharbor.db"},
		{driver: "postgres", wantDriver: "postgres", wantTarget: "dsn"},
		{driver: "PostgreSQL", wantDriver: "postgres", wantTarget: "dsn"},
		{driver: "pgsql", wantDriver: "postgres", wantTarget: "dsn"},
		{driver: "sqlite3", wantDriver: "sqlite", wantTarget: "/data/mcharbor.db"},
		{driver: "mysql", wantDriver: "mysql", wantTarget: "dsn"},
		{driver: "mariadb", wantDriver: "mysql", wantTarget: "dsn"},
		{driver: "mssql", wantDriver: "mssql", wantTarget: "dsn"},
		{driver: " SQLServer ", wantDriver: "mssql", wantTarget: "dsn"},
		{driver: "azuresql", wantDriver: "mssql", wantTarget: "dsn"},
		{driver: "oracle", wantDriver: "sqlite", wantTarget: "/data/mcharbor.db"},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			cfg := &Config{DBDriver: tt.driver, DBDSN: " dsn ", DatabasePath: "/data/mcharbor.db"}
			gotDriver, gotTarget := cfg.DBConfig()
			if gotDriver != tt.wantDriver || gotTarget != tt.wantTarget {
				t.Fatalf("DBConfig() for %q = (%q, %q), want (%q, %q)",
					tt.driver, gotDriver, gotTarget, tt.wantDriver, tt.wantTarget)
			}
		})
	}
}
