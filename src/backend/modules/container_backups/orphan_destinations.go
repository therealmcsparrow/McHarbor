// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// cleanupOrphanDestinations reconciles destination rows whose parent
// backup run no longer exists. The orphan state arises when a
// `DELETE FROM container_backup_runs` runs on a SQLite connection
// whose `PRAGMA foreign_keys` is OFF (FK enforcement is per-connection
// in SQLite and survives connection-pool reuse). Migration 058
// historically had to set `PRAGMA foreign_keys = OFF` to rebuild the
// destinations table; that connection occasionally leaks into the
// pool afterwards and silently disables CASCADE for follow-up
// DELETE statements. The result is destination rows with no parent
// run, pointing at files that nobody will ever delete through the
// normal path.
//
// For each orphan, we:
//   1. Try to delete the cloud file using the destination's stored
//      path + its location's credentials. Failure here is logged but
//      does NOT block the row from being removed — we still want
//      the local DB consistent so the orphan doesn't recur.
//   2. Delete the destination row.
//
// The function deliberately separates run_id presence and status
// checks. We only attempt a cloud delete for `success` destinations
// (anything else has nothing on the cloud to delete). `failure`,
// `uploading`, etc. get their rows dropped without a remote call.
//
// Returns the number of destinations reconciled (cloud + DB), and
// any error from the discovery query. Errors during individual
// cleanup steps are logged but do not abort the loop.
func (s *Service) cleanupOrphanDestinations(ctx context.Context, logger *slog.Logger) (int, error) {
	locationsByID, err := s.orphanLocationsByID(ctx)
	if err != nil {
		return 0, fmt.Errorf("loading storage locations for orphan cleanup: %w", err)
	}

	destinations, err := s.orphanDestinations(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing orphan backup destinations: %w", err)
	}
	if len(destinations) == 0 {
		return 0, nil
	}

	reconciled := 0
	for _, dest := range destinations {
		var location *backupStorageDestination
		if strings.TrimSpace(dest.StorageLocationID) != "" {
			location = locationsByID[dest.StorageLocationID]
		}
		if dest.Status == "success" && strings.TrimSpace(dest.Path) != "" {
			if err := s.deleteDestinationFile(ctx, dest, location, logger); err != nil && logger != nil {
				logger.Warn(
					"orphan backup destination cloud delete failed; removing DB row anyway",
					"run", dest.RunID, "destination", dest.ID,
					"type", dest.LocationType, "name", dest.StorageLocationName,
					"path", dest.Path, "error", err,
				)
			}
		}
		if _, err := s.db.ExecContext(ctx, "DELETE FROM container_backup_run_destinations WHERE id = ?", dest.ID); err != nil {
			if logger != nil {
				logger.Warn(
					"orphan backup destination row delete failed",
					"destination", dest.ID, "run", dest.RunID, "error", err,
				)
			}
			continue
		}
		reconciled++
	}
	if logger != nil && reconciled > 0 {
		logger.Info(
			"orphan backup destinations reconciled",
			"count", reconciled, "scanned", len(destinations),
		)
	}
	return reconciled, nil
}

// orphanLocationsByID returns the storage locations referenced by
// orphan destinations, keyed by ID. Locations whose ID is missing
// are silently skipped — the credential lookup would fail later,
// and the orphan sweeper logs the failure rather than blocking.
func (s *Service) orphanLocationsByID(ctx context.Context) (map[string]*backupStorageDestination, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT d.storage_location_id
		FROM container_backup_run_destinations d
		LEFT JOIN container_backup_runs r ON r.id = d.run_id
		WHERE d.storage_location_id IS NOT NULL
		  AND d.storage_location_id <> ''
		  AND r.id IS NULL
		LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*backupStorageDestination{}
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		location, loadErr := s.backupStorageDestination(ctx, id)
		if loadErr != nil || location == nil {
			continue
		}
		out[id] = location
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// reconcileOrphansOnStartup runs the orphan destination sweep once,
// after a short grace period to let migrations and connection-pool
// warmup finish. It is launched as a background goroutine from
// Mount and never blocks startup. Errors are logged but never
// returned — a failed sweep just means we keep the orphan rows
// around for the next delete cycle, which itself now reconciles
// destinations explicitly.
func (s *Service) reconcileOrphansOnStartup(logger *slog.Logger) {
	if logger == nil {
		return
	}
	timer := time.NewTimer(orphanSweepGracePeriod)
	defer timer.Stop()
	<-timer.C
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := s.cleanupOrphanDestinations(ctx, logger); err != nil {
		logger.Warn("startup orphan destination sweep failed", "error", err)
	}
}

// orphanDestinations lists destination rows whose run_id no longer
// points at a parent backup run. Results are returned in the order
// they appear in the table; limit 1000 keeps the cleanup bounded.
func (s *Service) orphanDestinations(ctx context.Context) ([]BackupRunDestination, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.run_id, COALESCE(d.storage_location_id, ''),
		       d.storage_location_name, d.location_type,
		       d.status, d.remote_path, COALESCE(d.error, ''),
		       COALESCE(d.uploaded_at, ''),
		       d.bytes_uploaded, d.bytes_total,
		       d.created_at, d.updated_at
		FROM container_backup_run_destinations d
		LEFT JOIN container_backup_runs r ON r.id = d.run_id
		WHERE r.id IS NULL
		ORDER BY d.created_at ASC
		LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackupRunDestination
	for rows.Next() {
		var dest BackupRunDestination
		var storageID sql.NullString
		if err := rows.Scan(
			&dest.ID, &dest.RunID, &storageID, &dest.StorageLocationName, &dest.LocationType,
			&dest.Status, &dest.Path, &dest.Error, &dest.UploadedAt,
			&dest.BytesUploaded, &dest.BytesTotal,
			&dest.CreatedAt, &dest.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if storageID.Valid {
			dest.StorageLocationID = storageID.String
		}
		out = append(out, dest)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
