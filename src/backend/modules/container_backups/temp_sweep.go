// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// tempOlderThanLimit is the age past which a leftover counts for the "older_than_2h" success metric.
	tempOlderThanLimit = 2 * time.Hour
	// tempWalkLimit bounds the size walk of one entry so a huge leftover cannot stall a sweep.
	tempWalkLimit = 200000
	// tempLocationLimit bounds the local storage location read.
	tempLocationLimit = 1000
)

// tempSweepMu serialises the hourly sweep and the manual cleanup, so they never remove the same entry twice.
var tempSweepMu sync.Mutex

// tempSweepResult is what one hourly sweep removed and kept.
type tempSweepResult struct {
	Removed     int
	Bytes       int64
	KeptOwned   int
	OlderThan2h int
}

// BackupTempCleanupResult is the answer of POST /container-backups/temp/cleanup.
type BackupTempCleanupResult struct {
	RemovedEntries int   `json:"removedEntries"`
	RemovedBytes   int64 `json:"removedBytes"`
	KeptOwned      int   `json:"keptOwned"`
	KeptRecent     int   `json:"keptRecent"`
	ActiveRuns     int   `json:"activeRuns"`
}

// rootPass is the outcome of one pass over a single temp root.
type rootPass struct {
	removed     int
	bytes       int64
	keptOwned   int
	keptRecent  int
	olderThan2h int
	remaining   int
	missing     bool
}

// CleanupTemp removes the McHarbor-named entries in the current and retired temp roots that no run owns; while a run is
// active it keeps anything touched within the hour, and it never touches destination partials.
func (s *Service) CleanupTemp(ctx context.Context) (BackupTempCleanupResult, error) {
	return cleanupTemp(ctx, s.db, s.dataDir, s.getenv, time.Now())
}

// RunTempSweep runs one sweep with the process environment and logs the "backup temp sweep" line the success metric reads.
func RunTempSweep(ctx context.Context, db *sql.DB, dataDir string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	res, err := sweepTemp(ctx, db, dataDir, os.Getenv, time.Now())
	if err != nil {
		logger.Warn("backup temp sweep incomplete", "error", err)
	}
	logger.Info("backup temp sweep", "removed", res.Removed, "bytes", res.Bytes, "kept_owned", res.KeptOwned, "older_than_2h", res.OlderThan2h)
}

// sweepTemp removes every unowned McHarbor-named entry older than an hour from the current and retired roots, and stale
// partial files from the local destinations. A retired root that holds no McHarbor entry afterwards leaves the list.
func sweepTemp(ctx context.Context, db *sql.DB, dataDir string, getenv func(string) string, now time.Time) (tempSweepResult, error) {
	tempSweepMu.Lock()
	defer tempSweepMu.Unlock()
	var res tempSweepResult
	var errs []error

	current, _, _ := resolveTempRoot(ctx, db, dataDir, getenv)
	retired, err := retiredTempRoots(ctx, db)
	if err != nil {
		errs = append(errs, err)
	}
	retired = safeRetiredRoots(retired, dataDir)
	passes, dropped, passErrs := sweepRoots(ctx, current, retired, now, tempSweepMaxAge)
	errs = append(errs, passErrs...)
	for _, p := range passes {
		res.Removed += p.removed
		res.Bytes += p.bytes
		res.KeptOwned += p.keptOwned
		res.OlderThan2h += p.olderThan2h
	}
	if err := dropRetiredRoots(ctx, db, dropped); err != nil {
		errs = append(errs, err)
	}

	dirs, err := destinationDirs(ctx, db, dataDir)
	if err != nil {
		errs = append(errs, err)
	}
	for _, dir := range dirs {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		n, b, k := sweepPartials(dir, now)
		res.Removed += n
		res.Bytes += b
		res.KeptOwned += k
	}
	return res, errors.Join(errs...)
}

// cleanupTemp is the manual cleanup: with no run active every unowned McHarbor entry goes at any age; with one active,
// only entries older than an hour do.
func cleanupTemp(ctx context.Context, db *sql.DB, dataDir string, getenv func(string) string, now time.Time) (BackupTempCleanupResult, error) {
	tempSweepMu.Lock()
	defer tempSweepMu.Unlock()
	res := BackupTempCleanupResult{ActiveRuns: activeTempOwners()}
	var minAge time.Duration
	if res.ActiveRuns > 0 {
		minAge = tempSweepMaxAge
	}
	current, _, _ := resolveTempRoot(ctx, db, dataDir, getenv)
	retired, err := retiredTempRoots(ctx, db)
	if err != nil {
		return res, err
	}
	retired = safeRetiredRoots(retired, dataDir)
	passes, dropped, errs := sweepRoots(ctx, current, retired, now, minAge)
	for _, p := range passes {
		res.RemovedEntries += p.removed
		res.RemovedBytes += p.bytes
		res.KeptOwned += p.keptOwned
		res.KeptRecent += p.keptRecent
	}
	if err := dropRetiredRoots(ctx, db, dropped); err != nil {
		errs = append(errs, err)
	}
	return res, errors.Join(errs...)
}

// sweepRoots passes over the current root and each distinct retired root, and names the retired roots that are now
// empty of McHarbor entries or gone.
func sweepRoots(ctx context.Context, current string, retired []string, now time.Time, minAge time.Duration) (passes []rootPass, dropped []string, errs []error) {
	seen := map[string]bool{}
	roots := append([]string{current}, retired...)
	for i, dir := range roots {
		dir = filepath.Clean(dir)
		if seen[dir] || !filepath.IsAbs(dir) {
			continue
		}
		seen[dir] = true
		p, err := sweepTempRoot(ctx, dir, now, minAge)
		passes = append(passes, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if i > 0 && (p.missing || p.remaining == 0) {
			dropped = append(dropped, roots[i])
		}
	}
	return passes, dropped, errs
}

// sweepTempRoot removes the unowned McHarbor-named top-level entries of dir older than minAge. Removal goes through an
// os.Root, so a planted symlink can neither redirect a delete outside dir nor be followed; the link itself is what goes.
func sweepTempRoot(ctx context.Context, dir string, now time.Time, minAge time.Duration) (rootPass, error) {
	var p rootPass
	rt, err := os.OpenRoot(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			p.missing = true
			return p, nil
		}
		return p, fmt.Errorf("opening backup temp folder %s: %w", dir, err)
	}
	defer closeRoot(rt, dir)
	list, err := fs.ReadDir(rt.FS(), ".")
	if err != nil {
		return p, fmt.Errorf("listing backup temp folder %s: %w", dir, err)
	}
	var errs []error
	for _, e := range list {
		name := e.Name()
		if !isTempRootEntry(name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return p, err
		}
		p.remaining++
		info, err := rt.Lstat(name)
		if err != nil {
			continue
		}
		path := filepath.Join(dir, name)
		if tempPathOwned(path) {
			p.keptOwned++
			continue
		}
		age := now.Sub(info.ModTime())
		if age > tempOlderThanLimit {
			p.olderThan2h++
		}
		if minAge > 0 && age <= minAge {
			p.keptRecent++
			continue
		}
		size := entrySize(rt, name, info)
		// The ownership check runs again right before the delete: a run may have registered the path since the first look.
		if tempPathOwned(path) {
			p.keptOwned++
			continue
		}
		if err := rt.RemoveAll(name); err != nil {
			slog.Warn("backup temp entry not removed", "path", path, "error", err)
			errs = append(errs, fmt.Errorf("removing %s: %w", path, err))
			continue
		}
		p.remaining--
		p.removed++
		p.bytes += size
	}
	return p, errors.Join(errs...)
}

// entrySize adds up the bytes of the regular files beneath name without following a symlink, and stops at tempWalkLimit entries.
func entrySize(rt *os.Root, name string, info fs.FileInfo) int64 {
	if !info.IsDir() {
		return info.Size()
	}
	var size int64
	visited := 0
	walkErr := fs.WalkDir(rt.FS(), name, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable leaf only under-counts the bytes reported
		}
		visited++
		if visited > tempWalkLimit {
			return fs.SkipAll
		}
		if !d.IsDir() {
			if fi, ierr := d.Info(); ierr == nil {
				size += fi.Size()
			}
		}
		return nil
	})
	if walkErr != nil {
		slog.Debug("backup temp size walk stopped", "entry", name, "error", walkErr)
	}
	return size
}

// destinationDirs lists the folders that can hold destination partials: the local backup directory and every local
// storage location's base path.
func destinationDirs(ctx context.Context, db *sql.DB, dataDir string) ([]string, error) {
	dirs := []string{LocalBackupDir(dataDir)}
	qctx, cancel := context.WithTimeout(ctx, tempSettingQueryTimeout)
	defer cancel()
	rows, err := db.QueryContext(qctx,
		`SELECT base_path FROM storage_locations WHERE location_type = 'local' AND base_path IS NOT NULL AND base_path <> '' LIMIT ?`, tempLocationLimit)
	if err != nil {
		return dirs, fmt.Errorf("listing local storage locations: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Debug("closing storage location rows", "error", cerr)
		}
	}()
	seen := map[string]bool{filepath.Clean(dirs[0]): true}
	for rows.Next() {
		var base string
		if err := rows.Scan(&base); err != nil {
			return dirs, fmt.Errorf("reading local storage location: %w", err)
		}
		base = filepath.Clean(base)
		if !seen[base] {
			seen[base] = true
			dirs = append(dirs, base)
		}
	}
	if err := rows.Err(); err != nil {
		return dirs, fmt.Errorf("reading local storage locations: %w", err)
	}
	return dirs, nil
}

// dropRetiredRoots removes drop from the saved retired list. It re-reads the list inside the transaction, so a root a
// concurrent save just retired is not lost.
func dropRetiredRoots(ctx context.Context, db *sql.DB, drop []string) error {
	if len(drop) == 0 {
		return nil
	}
	gone := map[string]bool{}
	for _, r := range drop {
		gone[r] = true
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning retired folder update: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Debug("rolling back retired folder update", "error", rbErr)
		}
	}()
	current, err := retiredTempRoots(ctx, tx)
	if err != nil {
		return err
	}
	next := make([]string, 0, len(current))
	for _, r := range current {
		if !gone[r] {
			next = append(next, r)
		}
	}
	if len(next) == len(current) {
		return nil
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("encoding retired backup temp folders: %w", err)
	}
	if err := writeTempSetting(ctx, tx, tempRetiredSetting, string(encoded)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing retired folder update: %w", err)
	}
	return nil
}
