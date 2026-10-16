// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package instancebackup runs, verifies, schedules and restores encrypted
// instance backups, and copies them to configured storage locations.
package instancebackup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// Destination statuses. The column's CHECK accepts only these three.
const (
	DestUploading = "uploading"
	DestSuccess   = "success"
	DestFailure   = "failure"
)

// ExportDestination is one place a finished export was sent.
//
// The location's name and type are copied onto the row rather than joined: a
// location deleted a year later must not erase the record of where a backup that
// still exists was actually sent.
type ExportDestination struct {
	ID            string  `json:"id"`
	LocationID    *string `json:"locationId,omitempty"`
	LocationName  string  `json:"locationName"`
	LocationType  string  `json:"locationType"`
	Status        string  `json:"status"`
	RemoteKey     string  `json:"remoteKey"`
	Error         string  `json:"error"`
	UploadedAt    *string `json:"uploadedAt,omitempty"`
	BytesUploaded int64   `json:"bytesUploaded"`
	BytesTotal    int64   `json:"bytesTotal"`
}

// scheduleLocationIDs returns the storage locations a schedule copies to, in a
// stable order so the editor does not reshuffle the list on every open.
func (s *Service) scheduleLocationIDs(ctx context.Context, scheduleID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT storage_location_id
		FROM instance_backup_schedule_storage_locations
		WHERE schedule_id = ? ORDER BY created_at, storage_location_id`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("reading schedule destinations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("instance backup: closing query rows", "error", err)
		}
	}()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning schedule destination: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// setScheduleLocations replaces a schedule's destinations.
//
// Replace rather than merge: the editor sends the whole set, so a location the
// operator removed has to disappear. Unknown ids are dropped instead of failing
// the save — a location deleted between opening the form and saving it should not
// cost the operator every other edit they made.
func (s *Service) setScheduleLocations(ctx context.Context, scheduleID string, ids []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning schedule destinations update: %w", err)
	}
	defer coredb.RollbackTx(tx)
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM instance_backup_schedule_storage_locations WHERE schedule_id = ?`, scheduleID); err != nil {
		return fmt.Errorf("clearing schedule destinations: %w", err)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO instance_backup_schedule_storage_locations
			(schedule_id, storage_location_id, created_at)
			SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM storage_locations WHERE id = ?)`,
			scheduleID, id, now(), id); err != nil {
			return fmt.Errorf("linking schedule destination %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing schedule destinations: %w", err)
	}
	return nil
}

// exportDestinations returns where one export was sent.
func (s *Service) exportDestinations(ctx context.Context, exportID string) ([]ExportDestination, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, storage_location_id, storage_location_name, location_type,
		status, remote_key, error, uploaded_at, bytes_uploaded, bytes_total
		FROM instance_backup_export_destinations WHERE export_id = ?
		ORDER BY storage_location_name, id`, exportID)
	if err != nil {
		return nil, fmt.Errorf("reading export destinations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("instance backup: closing query rows", "error", err)
		}
	}()
	var out []ExportDestination
	for rows.Next() {
		var d ExportDestination
		if err := rows.Scan(&d.ID, &d.LocationID, &d.LocationName, &d.LocationType, &d.Status,
			&d.RemoteKey, &d.Error, &d.UploadedAt, &d.BytesUploaded, &d.BytesTotal); err != nil {
			return nil, fmt.Errorf("scanning export destination: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// locationLabel reads a location's display name and type for the destination row.
// A missing location is not an error: the export still happened, and the row
// records what it can.
func (s *Service) locationLabel(ctx context.Context, id string) (string, string) {
	var name, kind string
	err := s.db.QueryRowContext(ctx,
		`SELECT name, location_type FROM storage_locations WHERE id = ?`, id).Scan(&name, &kind)
	if err != nil && err != sql.ErrNoRows {
		s.logger.Warn("instance backup: reading a storage location's label", "id", id, "error", err)
	}
	return name, kind
}

// copyToLocations sends a finished export to every destination and records one
// row per destination.
//
// Every destination is attempted even after one fails, and a failure is recorded
// rather than returned: the local export is already complete, and the reason an
// operator asked for two destinations is that one of them may be unreachable. It
// reports whether at least one upload succeeded, and the reasons the others did
// not — a caller that only learns "none worked" cannot tell a wrong credential
// from a full disk, and that is the sentence the operator needs.
func (s *Service) copyToLocations(ctx context.Context, e *Export, path string, ids []string, uploader Uploader) (anyOK bool, problems []string) {
	if uploader == nil || len(ids) == 0 {
		return false, nil
	}
	// The id stays in the key: two exports can share a second, and retention must
	// never delete an object a kept export still points at.
	key := "mcharbor-instance/" + e.FileName

	// bounded by the storage locations one export targets; each upload is a remote effect no transaction can span
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		name, kind := s.locationLabel(ctx, id)
		status, message := DestSuccess, ""

		f, err := os.Open(path)
		if err != nil {
			status, message = DestFailure, err.Error()
		} else {
			_, upErr := uploader.Upload(ctx, id, key, f, e.SizeBytes)
			if closeErr := f.Close(); closeErr != nil && upErr == nil {
				upErr = closeErr
			}
			if upErr != nil {
				status, message = DestFailure, upErr.Error()
			}
		}
		if status == DestSuccess {
			anyOK = true
		} else {
			label := name
			if label == "" {
				label = id
			}
			problems = append(problems, label+": "+message)
		}

		uploaded := any(nil)
		bytesUp := int64(0)
		if status == DestSuccess {
			uploaded, bytesUp = now(), e.SizeBytes
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO instance_backup_export_destinations
			(id, export_id, storage_location_id, storage_location_name, location_type, status,
			 remote_key, error, uploaded_at, created_at, updated_at, bytes_uploaded, bytes_total)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			xid.New().String(), e.ID, id, name, kind, status, key, message, uploaded,
			now(), now(), bytesUp, e.SizeBytes); err != nil {
			s.logger.Error("instance backup: recording a destination", "export", e.ID, "location", id, "error", err)
		}
	}

	// The legacy single-location columns stay in step, because a pre-migration
	// restore and the reporting plugin's readiness report both still read them.
	s.syncLegacyLocationColumns(ctx, e.ID)
	return anyOK, problems
}

// syncLegacyLocationColumns mirrors the destination rows onto the old
// location_* columns: the first success if there is one, otherwise the first
// failure, so an older reader sees a truthful summary rather than nothing.
func (s *Service) syncLegacyLocationColumns(ctx context.Context, exportID string) {
	var id, key, status, errText string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(storage_location_id, ''), remote_key, status, error
		FROM instance_backup_export_destinations WHERE export_id = ?
		ORDER BY CASE status WHEN 'success' THEN 0 ELSE 1 END, id LIMIT 1`, exportID).
		Scan(&id, &key, &status, &errText)
	if err == sql.ErrNoRows {
		return
	}
	if err != nil {
		s.logger.Warn("instance backup: summarising destinations", "export", exportID, "error", err)
		return
	}
	legacy := "uploaded"
	if status == DestFailure {
		legacy = "failed"
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE instance_backup_exports
		SET location_id = ?, location_key = ?, location_status = ?, location_error = ?, updated_at = ?
		WHERE id = ?`, nullable(id), key, legacy, errText, now(), exportID); err != nil {
		s.logger.Warn("instance backup: writing the legacy location summary", "export", exportID, "error", err)
	}
}

// removeFromLocations deletes an export's remote copies. Every destination is
// attempted: one unreachable location must not leave the others orphaned.
func (s *Service) removeFromLocations(ctx context.Context, exportID string, uploader Uploader) {
	if uploader == nil {
		return
	}
	dests, err := s.exportDestinations(ctx, exportID)
	if err != nil {
		s.logger.Warn("instance backup: reading destinations before delete", "export", exportID, "error", err)
		return
	}
	for _, d := range dests {
		if d.LocationID == nil || d.RemoteKey == "" || d.Status != DestSuccess {
			continue
		}
		if err := uploader.Delete(ctx, *d.LocationID, d.RemoteKey); err != nil {
			s.logger.Warn("instance backup: removing a deleted export from storage",
				"export", exportID, "location", *d.LocationID, "key", d.RemoteKey, "error", err)
		}
	}
}
