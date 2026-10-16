// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// lowDiskBytes is where the startup report starts warning: below it a SQLite database,
// the backup staging area or a plugin install can fail part way through.
const lowDiskBytes = 1 << 30

// StartupReport is what this node found at boot. It holds no secrets and no paths
// beyond the data directory the operator configured, so it is safe to show cluster.view.
type StartupReport struct {
	NodeID         string    `json:"nodeId"`
	CheckedAt      time.Time `json:"checkedAt"`
	DatabaseDriver string    `json:"databaseDriver"`
	DatabaseOK     bool      `json:"databaseReachable"`
	DataDirOK      bool      `json:"dataDirWritable"`
	DataDirFree    int64     `json:"dataDirFreeBytes"`
	LowDisk        bool      `json:"lowDisk"`
	KeySource      string    `json:"encryptionKeySource"`
	TLSMode        string    `json:"tlsMode"`
	PluginStore    string    `json:"pluginStore"`
	HAMode         string    `json:"haMode"`
	HAModeRefused  bool      `json:"haModeRefused"`
}

// StartupInputs is what main knows at boot that the report cannot discover itself.
type StartupInputs struct {
	NodeID        string
	Driver        string
	DataDir       string
	KeyFromEnv    bool
	TLSEnabled    bool
	TLSForceHTTPS bool
	PluginStore   string
	HAMode        string
	HAModeRefused bool
}

// BuildStartupReport probes the database and the data directory once.
func BuildStartupReport(ctx context.Context, database *sql.DB, in StartupInputs) StartupReport {
	r := StartupReport{
		NodeID: in.NodeID, CheckedAt: time.Now().UTC(), DatabaseDriver: in.Driver,
		PluginStore: in.PluginStore, HAMode: in.HAMode, HAModeRefused: in.HAModeRefused,
		KeySource: "file", TLSMode: "off",
	}
	if r.HAMode == "" {
		r.HAMode = RolePeer
	}
	if in.KeyFromEnv {
		r.KeySource = "env"
	}
	switch {
	case in.TLSEnabled && in.TLSForceHTTPS:
		r.TLSMode = "https-only"
	case in.TLSEnabled:
		r.TLSMode = "https"
	}
	pingCtx, cancel := context.WithTimeout(ctx, clusterCallTimeout)
	defer cancel()
	r.DatabaseOK = database != nil && database.PingContext(pingCtx) == nil
	r.DataDirOK = dirWritable(in.DataDir) == nil
	if free, err := diskFree(in.DataDir); err == nil {
		r.DataDirFree = free
		r.LowDisk = free < lowDiskBytes
	}
	return r
}

func dirWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".mcharbor-write-probe-*")
	if err != nil {
		return fmt.Errorf("creating probe file: %w", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing probe file: %w", err)
	}
	return os.Remove(filepath.Clean(name))
}

// Log writes the report as one structured line, at warning level when anything an
// operator must fix was found.
func (r StartupReport) Log(logger *slog.Logger) {
	level := slog.LevelInfo
	if !r.DatabaseOK || !r.DataDirOK || r.LowDisk || r.HAModeRefused {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, "startup health",
		"db_driver", r.DatabaseDriver, "db_reachable", r.DatabaseOK,
		"data_dir_writable", r.DataDirOK, "data_dir_free_bytes", r.DataDirFree, "low_disk", r.LowDisk,
		"encryption_key_source", r.KeySource, "tls", r.TLSMode, "plugin_store", r.PluginStore,
		"ha_mode", r.HAMode, "ha_mode_refused", r.HAModeRefused)
}

// SetStartupReport keeps the boot report for the HA page.
func (s *Service) SetStartupReport(r StartupReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startup = &r
}

// StartupReport returns the boot report, or false before main has built it.
func (s *Service) StartupReport() (StartupReport, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startup == nil {
		return StartupReport{}, false
	}
	return *s.startup, true
}
