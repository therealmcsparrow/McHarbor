// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// UpdateSnapshot is what an upgrade must be able to put back if it is cut short:
// the version it replaces, the enabled flag, and every role grant of the
// plugin's keys. It is written before the uninstall and cleared after the
// restore, so a row that survives a restart marks an upgrade that never finished.
type UpdateSnapshot struct {
	Previous PreviousInstall
	Enabled  bool
	Grants   map[string][]string
}

// installedState reads the parts of an installed row a rollback must reproduce.
// ok is false when slug is not installed.
func (s *Service) installedState(ctx context.Context, slug string) (prev PreviousInstall, enabled, ok bool, err error) {
	var signed, en int
	err = s.db.QueryRowContext(ctx,
		`SELECT version, COALESCE(build, ''), built_for, signed, enabled FROM appstore_plugins WHERE slug = ?`,
		slug).Scan(&prev.Version, &prev.Build, &prev.BuiltFor, &signed, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return PreviousInstall{}, false, false, nil
	}
	if err != nil {
		return PreviousInstall{}, false, false, fmt.Errorf("reading installed row for %s: %w", slug, err)
	}
	prev.Slug, prev.Signed = slug, signed == 1
	return prev, en == 1, true, nil
}

// SaveUpdateSnapshot persists snap, replacing any earlier one for the slug.
func (s *Service) SaveUpdateSnapshot(ctx context.Context, snap UpdateSnapshot) error {
	grants := snap.Grants
	if grants == nil {
		grants = map[string][]string{}
	}
	encoded, err := json.Marshal(grants)
	if err != nil {
		return fmt.Errorf("encoding role grants: %w", err)
	}
	signed, enabled := 0, 0
	if snap.Previous.Signed {
		signed = 1
	}
	if snap.Enabled {
		enabled = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return withTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM appstore_plugin_update_snapshots WHERE slug = ?`, snap.Previous.Slug); err != nil {
			return fmt.Errorf("replacing update snapshot: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO appstore_plugin_update_snapshots
				(id, slug, version, build, built_for, signed, enabled, grants, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), snap.Previous.Slug, snap.Previous.Version, snap.Previous.Build,
			snap.Previous.BuiltFor, signed, enabled, string(encoded), now, now); err != nil {
			return fmt.Errorf("writing update snapshot: %w", err)
		}
		return nil
	})
}

// ClearUpdateSnapshot forgets the snapshot for slug once its upgrade finished
// or was rolled back.
func (s *Service) ClearUpdateSnapshot(ctx context.Context, slug string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM appstore_plugin_update_snapshots WHERE slug = ?`, slug); err != nil {
		return fmt.Errorf("clearing update snapshot for %s: %w", slug, err)
	}
	return nil
}

// UpdateSnapshotFor returns the pending snapshot for slug, or nil when none is.
func (s *Service) UpdateSnapshotFor(ctx context.Context, slug string) (*UpdateSnapshot, error) {
	snaps, err := s.querySnapshots(ctx,
		`SELECT slug, version, build, built_for, signed, enabled, grants
		 FROM appstore_plugin_update_snapshots WHERE slug = ?`, slug)
	if err != nil || len(snaps) == 0 {
		return nil, err
	}
	return &snaps[0], nil
}

// PendingUpdateSnapshots lists every upgrade that did not finish.
func (s *Service) PendingUpdateSnapshots(ctx context.Context) ([]UpdateSnapshot, error) {
	return s.querySnapshots(ctx,
		`SELECT slug, version, build, built_for, signed, enabled, grants
		 FROM appstore_plugin_update_snapshots ORDER BY slug LIMIT 1000`)
}

func (s *Service) querySnapshots(ctx context.Context, query string, args ...any) ([]UpdateSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading update snapshots: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin service: closing snapshot rows failed", "error", err)
		}
	}()
	var out []UpdateSnapshot
	for rows.Next() {
		var snap UpdateSnapshot
		var signed, enabled int
		var raw string
		if err := rows.Scan(&snap.Previous.Slug, &snap.Previous.Version, &snap.Previous.Build,
			&snap.Previous.BuiltFor, &signed, &enabled, &raw); err != nil {
			return nil, fmt.Errorf("scanning update snapshot: %w", err)
		}
		snap.Previous.Signed, snap.Enabled = signed == 1, enabled == 1
		if err := json.Unmarshal([]byte(raw), &snap.Grants); err != nil {
			return nil, fmt.Errorf("decoding role grants for %s: %w", snap.Previous.Slug, err)
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}
