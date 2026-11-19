// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// localCopyRefetchGrace is how long an archive fetched back from a destination stays on disk after
// its last use, so browsing a restore and then running it downloads the archive once, not per click.
const localCopyRefetchGrace = time.Hour

// localCopyLocks serialises the drop, fetch and sweep of one run's archive, so two restores of the
// same run do not download it twice and a sweep never removes a file a fetch is still writing.
var localCopyLocks sync.Map

func localCopyLock(runID string) *sync.Mutex {
	lock, _ := localCopyLocks.LoadOrStore(runID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// managedArchivePath is where the server keeps a run's own archive. Every path this file removes or
// writes is derived here rather than read from the row, so a corrupt row cannot point it elsewhere.
func (s *Service) managedArchivePath(runID string) string {
	return filepath.Join(s.backupDir(), safeArchiveName.ReplaceAllString(runID, "-"), "mcharbor.tar")
}

// isManagedArchive reports whether a stored archive path is the managed path for runID.
func (s *Service) isManagedArchive(runID, archivePath string) bool {
	abs, err := filepath.Abs(strings.TrimSpace(archivePath))
	return err == nil && abs == s.managedArchivePath(runID)
}

// storedOnEveryDestination reports whether a run's archive reached at least one destination and
// none failed: only then is the server's copy redundant.
func storedOnEveryDestination(destinations []BackupRunDestination) bool {
	if len(destinations) == 0 {
		return false
	}
	for _, dest := range destinations {
		if dest.Status != "success" || strings.TrimSpace(dest.Path) == "" {
			return false
		}
	}
	return true
}

// stagedArchivePath is where a run builds its archive: inside its own backup temp folder, so the
// operator's temp location rather than the data directory takes the write.
func stagedArchivePath(ctx context.Context) string {
	return filepath.Join(runTempDir(ctx), "mcharbor.tar")
}

// planKeepsLocalCopy reports whether a run's plan asked for a server copy. A run with no plan (an
// ad-hoc backup) or whose plan was deleted follows the default, which is not to keep one.
func (s *Service) planKeepsLocalCopy(ctx context.Context, planID string) (bool, error) {
	if strings.TrimSpace(planID) == "" {
		return false, nil
	}
	var keep bool
	err := s.db.QueryRowContext(ctx, "SELECT keep_local_copy FROM container_backup_plans WHERE id = ?", planID).Scan(&keep)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the plan's local copy setting: %w", err)
	}
	return keep, nil
}

// storedEverywhere reports whether every destination of runID already holds its archive.
func (s *Service) storedEverywhere(ctx context.Context, runID string) (bool, error) {
	run := &BackupRun{ID: runID}
	if err := s.hydrateRunDestinations(ctx, run); err != nil {
		return false, err
	}
	return storedOnEveryDestination(run.Destinations), nil
}

// settleArchive moves a run's archive out of its temp folder before releaseTemp deletes that folder.
// An incomplete or cancelled archive is discarded; one every destination holds and the plan does not
// keep is never copied to the server's disk at all. Anything else lands on the managed path, where
// download, restore and retry upload expect it.
func (s *Service) settleArchive(ctx context.Context, planID, runID string, archive archiveResult, complete bool) (archiveResult, error) {
	staged := strings.TrimSpace(archive.path)
	managed := s.managedArchivePath(runID)
	if staged == "" || staged == managed {
		return archive, nil
	}
	if !complete || ctx.Err() != nil {
		removeLogged(staged)
		return archiveResult{}, nil
	}
	// The run's own ctx may be at its deadline; the settle must still finish, and the move is bounded by
	// the archive's size.
	settleCtx := context.WithoutCancel(ctx)
	queryCtx, cancel := context.WithTimeout(settleCtx, backupRunFinalizeTimeout)
	defer cancel()
	everywhere, err := s.storedEverywhere(queryCtx, runID)
	if err != nil && s.logger != nil {
		s.logger.Warn("container backup: listing destinations before settling the archive failed", "run", runID, "error", err)
	}
	keep, err := s.planKeepsLocalCopy(queryCtx, planID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("container backup: reading the plan's local copy setting failed", "run", runID, "plan", planID, "error", err)
		}
		keep = true
	}
	if everywhere && !keep {
		removeLogged(staged)
		archive.path, archive.dropped = managed, true
		return archive, nil
	}
	lock := localCopyLock(runID)
	lock.Lock()
	defer lock.Unlock()
	if err := moveArchive(settleCtx, staged, managed); err != nil {
		if everywhere {
			// The destinations hold it, so the run stays usable: restore fetches it back.
			if s.logger != nil {
				s.logger.Warn("container backup: keeping a server copy failed, it will be fetched back on demand", "run", runID, "error", err)
			}
			archive.path, archive.dropped = managed, true
			return archive, nil
		}
		return archiveResult{}, fmt.Errorf("keeping the backup archive on this server: %w", err)
	}
	archive.path = managed
	return archive, nil
}

// moveArchive renames src to dst, and copies through a temporary file when the two sit on different
// filesystems, so dst is never a partial archive.
func moveArchive(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("creating the backup run directory: %w", err)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening the staged archive: %w", err)
	}
	defer closeLogged(in, "staged backup archive")
	tmp := dst + ".move"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("creating the archive copy: %w", err)
	}
	_, copyErr := copyWithContext(ctx, out, in)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil {
		copyErr = os.Rename(tmp, dst)
	}
	if copyErr != nil {
		removeLogged(tmp)
		return fmt.Errorf("copying the archive to the backup directory: %w", copyErr)
	}
	removeLogged(src)
	return nil
}

// dropLocalCopyAfterRun removes a successful run's server-side archive when its plan asked not to keep
// it, or when it has no plan. The flag is written before the file goes, so a failed removal leaves a
// usable archive.
func (s *Service) dropLocalCopyAfterRun(ctx context.Context, run *BackupRun) {
	s.sweepRefetchedCopies(ctx)
	if run == nil || run.Operation != "backup" || run.Status != "success" || run.LocalCopyDropped ||
		!s.isManagedArchive(run.ID, run.ArchivePath) {
		return
	}
	keep, err := s.planKeepsLocalCopy(ctx, run.PlanID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("container backup: reading the plan's local copy setting failed", "run", run.ID, "plan", run.PlanID, "error", err)
		}
		return
	}
	if keep {
		return
	}
	if err := s.hydrateRunDestinations(ctx, run); err != nil {
		if s.logger != nil {
			s.logger.Warn("container backup: listing destinations before dropping the local copy failed", "run", run.ID, "error", err)
		}
		return
	}
	if !storedOnEveryDestination(run.Destinations) {
		return
	}
	lock := localCopyLock(run.ID)
	lock.Lock()
	defer lock.Unlock()
	if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_runs SET local_copy_dropped = 1, updated_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339), run.ID); err != nil {
		if s.logger != nil {
			s.logger.Warn("container backup: marking the local copy dropped failed", "run", run.ID, "error", err)
		}
		return
	}
	run.LocalCopyDropped = true
	if err := os.RemoveAll(filepath.Dir(s.managedArchivePath(run.ID))); err != nil && s.logger != nil {
		s.logger.Warn("container backup: removing the local copy failed", "run", run.ID, "error", err)
	}
}

// ensureDroppedArchive is ensureLocalArchive for a loaded run: one that never dropped its copy
// costs nothing.
func (s *Service) ensureDroppedArchive(ctx context.Context, run *BackupRun) error {
	if run == nil || !run.LocalCopyDropped {
		return nil
	}
	return s.ensureLocalArchive(ctx, run.ID, run.ArchivePath)
}

// ensureLocalArchive makes a run's archive present on disk before something reads it. A run whose
// local copy was dropped is fetched back from the first destination that still serves it.
func (s *Service) ensureLocalArchive(ctx context.Context, runID, archivePath string) error {
	target := s.managedArchivePath(runID)
	var dropped bool
	var size int64
	err := s.db.QueryRowContext(ctx, "SELECT local_copy_dropped, archive_size FROM container_backup_runs WHERE id = ?", runID).Scan(&dropped, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the backup run's local copy state: %w", err)
	}
	if !dropped {
		return nil
	}
	if !s.isManagedArchive(runID, archivePath) {
		return ErrBackupRunNotDownloadable
	}
	lock := localCopyLock(runID)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(target); err == nil {
		now := time.Now()
		if err := os.Chtimes(target, now, now); err != nil && s.logger != nil {
			s.logger.Warn("container backup: refreshing the fetched archive's grace failed", "run", runID, "error", err)
		}
		return nil
	}
	run := &BackupRun{ID: runID}
	if err := s.hydrateRunDestinations(ctx, run); err != nil {
		return err
	}
	var errs []error
	for _, dest := range run.Destinations {
		if dest.Status != "success" || strings.TrimSpace(dest.Path) == "" {
			continue
		}
		err := s.fetchFromDestination(ctx, dest, target, size)
		if err == nil {
			if s.logger != nil {
				s.logger.Info("container backup: fetched the archive back from a destination", "run", runID, "destination", dest.StorageLocationName)
			}
			return nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", dest.StorageLocationName, err))
	}
	if len(errs) == 0 {
		return fmt.Errorf("%w: no destination holds this backup", ErrBackupRunNotDownloadable)
	}
	return fmt.Errorf("fetching the backup archive from its destinations: %w", errors.Join(errs...))
}

// fetchFromDestination copies one destination's archive to target through a temporary file, so a
// broken transfer never leaves a truncated archive where a restore would read it.
func (s *Service) fetchFromDestination(ctx context.Context, dest BackupRunDestination, target string, size int64) error {
	src, err := s.openDestinationArchive(ctx, dest)
	if err != nil {
		return err
	}
	defer closeLogged(src, "backup destination archive")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("creating the backup run directory: %w", err)
	}
	tmp := target + ".fetch"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating the fetched archive: %w", err)
	}
	written, copyErr := copyWithContext(ctx, out, src)
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil && size > 0 && written != size {
		copyErr = fmt.Errorf("fetched %d bytes, the run recorded %d", written, size)
	}
	if copyErr == nil {
		copyErr = os.Rename(tmp, target)
	}
	if copyErr != nil {
		if rmErr := os.Remove(tmp); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) && s.logger != nil {
			s.logger.Warn("container backup: removing a failed fetch failed", "path", tmp, "error", rmErr)
		}
		return copyErr
	}
	return nil
}

// openDestinationArchive reads a destination's archive the same way it was written: a local
// destination from its file, every other type through the storage adapter that uploaded it.
func (s *Service) openDestinationArchive(ctx context.Context, dest BackupRunDestination) (io.ReadCloser, error) {
	if dest.LocationType == "local" {
		path, err := cleanLocalBackupPath(dest.Path)
		if err != nil {
			return nil, err
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening the local destination archive: %w", err)
		}
		return file, nil
	}
	if strings.TrimSpace(dest.StorageLocationID) == "" {
		return nil, fmt.Errorf("the destination has no storage location")
	}
	location, err := s.backupStorageDestination(ctx, dest.StorageLocationID)
	if err != nil {
		return nil, err
	}
	adapter, _, ok := s.resolveStorageAdapter(dest.LocationType)
	if !ok {
		return nil, fmt.Errorf("no installed plugin handles %s", dest.LocationType)
	}
	reader, err := adapter.Download(ctx, s.buildLocationConfigForUpload(location), dest.Path)
	if err != nil {
		return nil, fmt.Errorf("downloading from %s: %w", dest.LocationType, err)
	}
	return reader, nil
}

// sweepRefetchedCopies removes archives fetched back for dropped runs once they have sat unused for
// localCopyRefetchGrace, so a restore does not quietly turn into a permanent local copy.
func (s *Service) sweepRefetchedCopies(ctx context.Context) {
	// No LIMIT: a sweep must see every dropped run, and the result is a list of ids.
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM container_backup_runs WHERE local_copy_dropped = 1")
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("container backup: listing dropped local copies failed", "error", err)
		}
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil && s.logger != nil {
		s.logger.Warn("container backup: reading dropped local copies failed", "error", err)
	}
	closeLogged(rows, "dropped local copy rows")
	cutoff := time.Now().Add(-localCopyRefetchGrace)
	for _, id := range ids {
		s.sweepRefetchedCopy(id, cutoff)
	}
}

func (s *Service) sweepRefetchedCopy(runID string, cutoff time.Time) {
	lock := localCopyLock(runID)
	if !lock.TryLock() {
		return
	}
	defer lock.Unlock()
	target := s.managedArchivePath(runID)
	info, err := os.Stat(target)
	if err != nil || info.ModTime().After(cutoff) {
		return
	}
	if err := os.RemoveAll(filepath.Dir(target)); err != nil && s.logger != nil {
		s.logger.Warn("container backup: removing a fetched archive failed", "run", runID, "error", err)
	}
}
