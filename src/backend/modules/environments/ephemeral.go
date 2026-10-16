// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// The ephemeral seam.
//
// A preview environment has to be distinguishable from a production one, or a
// reaper working from "some table points at this id" will eventually delete the
// wrong thing. The provenance lives on the environment row — `ephemeral`,
// `preview_id` and `expires_at` — and every destructive call below refuses to
// touch a row that is not marked.
//
// Every signature here is primitives. GitOps reaches these methods through the
// service registry, which hands back `any`, and modules never import one another.

// CreateEphemeralEnvironment creates an environment that belongs to a preview and
// stamps its provenance and expiry in the same statement. The caller has already
// checked environments.manage for actorUserID; this records who it was created
// for so a teardown is attributable.
func (s *Service) CreateEphemeralEnvironment(ctx context.Context, actorUserID, name, previewID, expiresAt string) (string, error) {
	if name == "" {
		return "", errors.New("environments: a preview environment needs a name")
	}
	if previewID == "" {
		return "", errors.New("environments: a preview environment needs the preview it belongs to")
	}

	env, _, err := s.Create(ctx, CreateRequest{Name: name, ConnectionType: "socket"})
	if err != nil {
		return "", fmt.Errorf("environments: creating the preview environment: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE environments SET ephemeral = 1, preview_id = ?, expires_at = ?, updated_at = ? WHERE id = ?`,
		previewID, nullableTime(expiresAt), time.Now().UTC().Format(time.RFC3339), env.ID,
	); err != nil {
		// An environment created without its provenance is exactly the row the
		// reaper must never delete, so it is removed here rather than left behind
		// looking like production.
		if delErr := s.Delete(ctx, env.ID); delErr != nil {
			slog.Error("environments: could not remove a preview environment that failed to be marked ephemeral",
				"environment", env.ID, "error", delErr)
		}
		return "", fmt.Errorf("environments: marking the preview environment ephemeral: %w", err)
	}

	slog.Info("environments: preview environment created",
		"environment", env.ID, "preview", previewID, "for", actorUserID, "expires", expiresAt)
	return env.ID, nil
}

// DestroyEphemeralEnvironment removes a preview's environment. It is idempotent —
// an environment that is already gone is success — and it refuses any row that is
// not marked as this preview's, which is the single most important safety
// property in the preview lifecycle.
func (s *Service) DestroyEphemeralEnvironment(ctx context.Context, envID, previewID string) error {
	owner, ephemeral, err := s.EphemeralPreviewOf(ctx, envID)
	if err != nil {
		return err
	}
	if !ephemeral {
		return fmt.Errorf("environments: refusing to delete %s, it is not marked as a preview environment", envID)
	}
	if previewID != "" && owner != previewID {
		return fmt.Errorf("environments: refusing to delete %s, it belongs to preview %s and not %s", envID, owner, previewID)
	}
	if err := s.Delete(ctx, envID); err != nil {
		// Delete reports a missing row as an error; for a teardown that is the
		// outcome being asked for, so it is success.
		if err.Error() == "environment not found" {
			return nil
		}
		return fmt.Errorf("environments: deleting the preview environment: %w", err)
	}
	return nil
}

// EphemeralPreviewOf reports which preview an environment belongs to and whether
// it is marked ephemeral at all. An environment that no longer exists reports
// (empty, false, nil), so a teardown treats it as already done.
func (s *Service) EphemeralPreviewOf(ctx context.Context, envID string) (string, bool, error) {
	var previewID sql.NullString
	var ephemeral sql.NullBool
	err := s.db.QueryRowContext(ctx,
		`SELECT preview_id, ephemeral FROM environments WHERE id = ?`, envID,
	).Scan(&previewID, &ephemeral)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("environments: reading preview provenance for %s: %w", envID, err)
	}
	return previewID.String, ephemeral.Bool, nil
}

// ReapExpiredEphemeral deletes preview environments whose stamped expiry has
// passed. It is how an orphan is collected: when a pipeline is deleted its
// preview rows cascade away, and nothing would otherwise point at the environment
// they created. The expiry on the environment itself is what gives it an owner.
func (s *Service) ReapExpiredEphemeral(ctx context.Context) (int, error) {
	ids, err := s.expiredEphemeralIDs(ctx, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}

	reaped := 0
	for _, id := range ids {
		if err := s.DestroyEphemeralEnvironment(ctx, id, ""); err != nil {
			slog.Warn("environments: reaping an expired preview environment failed", "environment", id, "error", err)
			continue
		}
		reaped++
	}
	return reaped, nil
}

// ClearPreviewExpiry removes the expiry from an ephemeral environment so the
// reaper stops considering it. Used when an operator adopts a preview environment
// rather than letting it be collected.
func (s *Service) ClearPreviewExpiry(ctx context.Context, envID string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE environments SET ephemeral = 0, preview_id = '', expires_at = NULL, updated_at = ?
		 WHERE id = ? AND ephemeral = 1`,
		time.Now().UTC().Format(time.RFC3339), envID)
	if err != nil {
		return fmt.Errorf("environments: adopting preview environment %s: %w", envID, err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("environments: %s is not an adoptable preview environment", envID)
	}
	return nil
}

// nullableTime writes NULL rather than an empty string, so an expiry that was
// never set cannot compare as "already expired".
func nullableTime(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// expiredEphemeralIDs returns up to 50 preview environments whose expiry is at or before now, closing the cursor before any destroy runs.
func (s *Service) expiredEphemeralIDs(ctx context.Context, now string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM environments
		 WHERE ephemeral = 1 AND expires_at IS NOT NULL AND expires_at != '' AND expires_at <= ?
		 ORDER BY expires_at ASC LIMIT 50`, now)
	if err != nil {
		return nil, fmt.Errorf("environments: selecting expired preview environments: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("environments: scanning expired preview environment: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("environments: reading expired preview environments: %w", err)
	}
	return ids, nil
}
