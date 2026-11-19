// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	// failedArchiveGrace is how long a failed run keeps its server archive for a retry upload.
	failedArchiveGrace = 7 * 24 * time.Hour
	// orphanRunDirGrace keeps a run folder with no run row alone long enough for any in-flight write to settle.
	orphanRunDirGrace   = 24 * time.Hour
	localCopySweepLimit = 1000
)

// runDirNamePattern matches an xid, which is all a managed run folder is ever named.
var runDirNamePattern = regexp.MustCompile(`^[0-9a-v]{20}$`)

// managedRunFiles are the only files a managed run folder holds; a folder with anything else is not ours to delete.
var managedRunFiles = map[string]bool{"mcharbor.tar": true, "mcharbor.tar.fetch": true, "mcharbor.tar.move": true}

// localCopySweepResult is what one pass over the server-side archive directory freed.
type localCopySweepResult struct {
	removed int
	bytes   int64
}

func (r *localCopySweepResult) add(n int64) {
	r.removed++
	r.bytes += n
}

// sweepLocalCopies frees the server-side archives nothing needs any more: copies every destination
// holds whose plan does not keep one (or that have no plan), failed runs past failedArchiveGrace, and
// run folders with no run row. Paths are derived from the run id, never read from the row.
func sweepLocalCopies(ctx context.Context, db *sql.DB, dataDir string, now time.Time) (localCopySweepResult, error) {
	var res localCopySweepResult
	root := LocalBackupDir(dataDir)
	errs := []error{
		sweepRedundantCopies(ctx, db, root, now, &res),
		sweepFailedArchives(ctx, db, root, now, &res),
		sweepOrphanRunDirs(ctx, db, root, now, &res),
	}
	return res, errors.Join(errs...)
}

// managedRunIDs runs query, which selects (id, archive_path), and keeps the runs whose archive path is
// the managed one under root: a run recorded anywhere else is not this sweep's to touch.
func managedRunIDs(ctx context.Context, db *sql.DB, root, query string, args ...any) ([]string, error) {
	qctx, cancel := context.WithTimeout(ctx, tempSettingQueryTimeout)
	defer cancel()
	rows, err := db.QueryContext(qctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs for the local copy sweep: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Debug("closing local copy sweep rows", "error", cerr)
		}
	}()
	var ids []string
	for rows.Next() {
		var id, archivePath string
		if err := rows.Scan(&id, &archivePath); err != nil {
			return ids, fmt.Errorf("reading a backup run for the local copy sweep: %w", err)
		}
		abs, absErr := filepath.Abs(archivePath)
		if absErr != nil || abs != filepath.Join(root, id, "mcharbor.tar") {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return ids, fmt.Errorf("reading backup runs for the local copy sweep: %w", err)
	}
	return ids, nil
}

// sweepRedundantCopies drops the server copy of each successful run that every destination holds and
// that no plan asks to keep. This also clears copies left behind before the default changed.
func sweepRedundantCopies(ctx context.Context, db *sql.DB, root string, now time.Time, res *localCopySweepResult) error {
	ids, err := managedRunIDs(ctx, db, root, `
		SELECT r.id, r.archive_path FROM container_backup_runs r
		LEFT JOIN container_backup_plans p ON p.id = r.plan_id
		WHERE r.status = 'success' AND COALESCE(r.operation, 'backup') = 'backup'
		  AND r.local_copy_dropped = 0 AND COALESCE(r.archive_path, '') <> ''
		  AND COALESCE(r.origin, '') <> 'upload'
		  AND COALESCE(p.keep_local_copy, 0) = 0
		  AND EXISTS (SELECT 1 FROM container_backup_run_destinations d WHERE d.run_id = r.id)
		  AND NOT EXISTS (SELECT 1 FROM container_backup_run_destinations d
		                  WHERE d.run_id = r.id AND (d.status <> 'success' OR COALESCE(d.remote_path, '') = ''))
		LIMIT ?`, localCopySweepLimit)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	stamp := now.UTC().Format(time.RFC3339)
	for _, id := range ids {
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		freed, removeErr := removeRunDir(root, id, func() error {
			_, execErr := db.ExecContext(ctx,
				`UPDATE container_backup_runs SET local_copy_dropped = 1, updated_at = ? WHERE id = ? AND local_copy_dropped = 0`, stamp, id)
			return execErr
		})
		if removeErr != nil {
			errs = append(errs, removeErr)
			continue
		}
		if freed >= 0 {
			res.add(freed)
		}
	}
	return errors.Join(errs...)
}

// sweepFailedArchives removes the archive a failed or cancelled run kept for retry upload once
// failedArchiveGrace has passed, and clears its archive path so retry says the archive is gone.
func sweepFailedArchives(ctx context.Context, db *sql.DB, root string, now time.Time, res *localCopySweepResult) error {
	cutoff := now.Add(-failedArchiveGrace).UTC().Format(time.RFC3339)
	ids, err := managedRunIDs(ctx, db, root, `
		SELECT id, archive_path FROM container_backup_runs
		WHERE status IN ('failure', 'cancelled') AND COALESCE(operation, 'backup') = 'backup'
		  AND COALESCE(archive_path, '') <> '' AND COALESCE(origin, '') <> 'upload'
		  AND COALESCE(NULLIF(completed_at, ''), started_at) < ?
		LIMIT ?`, cutoff, localCopySweepLimit)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	stamp := now.UTC().Format(time.RFC3339)
	for _, id := range ids {
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		freed, removeErr := removeRunDir(root, id, func() error {
			_, execErr := db.ExecContext(ctx,
				`UPDATE container_backup_runs SET archive_path = NULL, updated_at = ? WHERE id = ?`, stamp, id)
			return execErr
		})
		if removeErr != nil {
			errs = append(errs, removeErr)
			continue
		}
		if freed >= 0 {
			res.add(freed)
		}
	}
	return errors.Join(errs...)
}

// sweepOrphanRunDirs removes run folders whose run row is gone, once they are older than
// orphanRunDirGrace and hold nothing but archive files.
func sweepOrphanRunDirs(ctx context.Context, db *sql.DB, root string, now time.Time, res *localCopySweepResult) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("listing the backup directory: %w", err)
	}
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsDir() || !runDirNamePattern.MatchString(name) {
			continue
		}
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) <= orphanRunDirGrace {
			continue
		}
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM container_backup_runs WHERE id = ?`, name).Scan(&exists); err != nil {
			errs = append(errs, fmt.Errorf("checking backup run %s: %w", name, err))
			continue
		}
		if exists > 0 {
			continue
		}
		freed, removeErr := removeRunDir(root, name, nil)
		if removeErr != nil {
			errs = append(errs, removeErr)
			continue
		}
		if freed >= 0 {
			res.add(freed)
		}
	}
	return errors.Join(errs...)
}

// removeRunDir deletes root/<runID> when it holds only archive files and no run or fetch is using it.
// before runs first and the folder stays when it fails, so the row never claims a copy is gone that is
// still there. It returns the bytes freed, or -1 when nothing was removed.
func removeRunDir(root, runID string, before func() error) (int64, error) {
	if !runDirNamePattern.MatchString(runID) {
		return -1, nil
	}
	if _, active := activeBackupRuns.Load(runID); active {
		return -1, nil
	}
	lock := localCopyLock(runID)
	if !lock.TryLock() {
		return -1, nil
	}
	defer lock.Unlock()
	dir := filepath.Join(root, runID)
	info, statErr := os.Lstat(dir)
	missing := errors.Is(statErr, fs.ErrNotExist)
	if statErr != nil && !missing {
		return -1, fmt.Errorf("checking backup run folder %s: %w", dir, statErr)
	}
	var size int64
	if !missing {
		if !info.IsDir() {
			return -1, nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return -1, fmt.Errorf("listing backup run folder %s: %w", dir, err)
		}
		for _, e := range entries {
			if !e.Type().IsRegular() || !managedRunFiles[e.Name()] {
				return -1, nil
			}
			if fi, ierr := e.Info(); ierr == nil {
				size += fi.Size()
			}
		}
	}
	if before != nil {
		if err := before(); err != nil {
			return -1, fmt.Errorf("updating backup run %s before removing its archive: %w", runID, err)
		}
	}
	if missing {
		return -1, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("backup run folder not removed", "path", dir, "error", err)
		return -1, fmt.Errorf("removing %s: %w", dir, err)
	}
	return size, nil
}
