// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const runTempPrefix = "mcharbor-run-"

type tempCtxKey struct{}

// tempConfig is what the package-level workspace helpers need to resolve the current root; the handler and the
// scheduler hold separate Service instances, so it is shared like activeBackupRuns.
type tempConfig struct {
	db      *sql.DB
	dataDir string
	getenv  func(string) string
}

var tempCfg atomic.Pointer[tempConfig]

var (
	tempOwnersMu sync.Mutex
	tempOwners   = map[string]int{}
)

// configureTempRoot records the database, data directory and env reader the workspace helpers resolve the root from.
func configureTempRoot(db *sql.DB, dataDir string, getenv func(string) string) {
	tempCfg.Store(&tempConfig{db: db, dataDir: dataDir, getenv: getenv})
}

// currentTempRoot resolves the root new runs use at this moment; a rejected env value falls back to the default.
func currentTempRoot(ctx context.Context) string {
	cfg := tempCfg.Load()
	if cfg == nil {
		cfg = &tempConfig{}
	}
	getenv := cfg.getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if cfg.db != nil {
		root, _, _ := resolveTempRoot(ctx, cfg.db, cfg.dataDir, getenv)
		return root
	}
	if raw := getenv(tempDirEnv); raw != "" {
		clean, err := validateTempRoot(raw, cfg.dataDir)
		if err == nil {
			return clean
		}
		logEnvInvalidOnce(raw, err)
	}
	return defaultTempRoot(cfg.dataDir)
}

// validRunTempID is an allow-list: a run ID becomes a folder name, so only letters, digits, '-' and '_' pass.
func validRunTempID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// acquireRunTemp creates <root>/mcharbor-run-<runID> on the root resolved now, registers it as owned and carries it in
// ctx; release removes only that folder and is safe to call twice. An existing folder is an error, never reused.
func acquireRunTemp(ctx context.Context, runID string) (context.Context, string, func(), error) {
	if !validRunTempID(runID) {
		return ctx, "", nil, fmt.Errorf("invalid backup run id for temp folder")
	}
	root := currentTempRoot(ctx)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return ctx, "", nil, fmt.Errorf("creating backup temp root: %w", err)
	}
	dir := filepath.Join(root, runTempPrefix+runID)
	registerOwned(dir)
	if err := os.Mkdir(dir, 0o700); err != nil {
		deregisterOwned(dir)
		return ctx, "", nil, fmt.Errorf("creating backup run folder: %w", err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if err := os.RemoveAll(dir); err != nil {
				slog.Warn("removing backup run folder failed", "path", dir, "error", err)
			}
			deregisterOwned(dir)
		})
	}
	return context.WithValue(ctx, tempCtxKey{}, dir), dir, release, nil
}

// runTempDir returns the run workspace carried in ctx, else the current root.
func runTempDir(ctx context.Context) string {
	if dir, ok := ctx.Value(tempCtxKey{}).(string); ok && dir != "" {
		return dir
	}
	return currentTempRoot(ctx)
}

// ownTempPath marks a partial file as in use so the sweep skips it until the returned release runs.
func ownTempPath(path string) func() {
	clean := filepath.Clean(path)
	registerOwned(clean)
	var once sync.Once
	return func() { once.Do(func() { deregisterOwned(clean) }) }
}

func registerOwned(path string) {
	tempOwnersMu.Lock()
	defer tempOwnersMu.Unlock()
	tempOwners[path]++
}

func deregisterOwned(path string) {
	tempOwnersMu.Lock()
	defer tempOwnersMu.Unlock()
	if tempOwners[path] <= 1 {
		delete(tempOwners, path)
		return
	}
	tempOwners[path]--
}

// tempPathOwned reports whether path is a registered path or lies beneath one.
func tempPathOwned(path string) bool {
	clean := filepath.Clean(path)
	tempOwnersMu.Lock()
	defer tempOwnersMu.Unlock()
	for owned := range tempOwners {
		if pathWithin(owned, clean) {
			return true
		}
	}
	return false
}

// activeTempOwners counts the registered run folders and partial files.
func activeTempOwners() int {
	tempOwnersMu.Lock()
	defer tempOwnersMu.Unlock()
	return len(tempOwners)
}
