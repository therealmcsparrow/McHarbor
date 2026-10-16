// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package elevations lets a user request time-boxed access to a permission they lack and lets an approver grant or deny it.
package elevations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/permcatalog"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Service is the data-access layer for elevation requests. Phase 6
// of the RBAC roadmap.
type Service struct {
	db *sql.DB
}

// NewService constructs an elevations service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ErrNotFound is returned when an elevation request is missing.
var ErrNotFound = errors.New("elevation request not found")

// ErrNotPending is returned when a decide call is made against a row
// that is no longer in the "pending" state.
var ErrNotPending = errors.New("elevation request is not pending")

// ErrNotElevatable is returned when the requested permission is
// flagged as not-elevatable in the catalog.
var ErrNotElevatable = errors.New("permission is not elevatable")

// ErrUnknownPermission is returned for a key no catalog defines.
var ErrUnknownPermission = errors.New("unknown permission")

// maxPendingPerRequester bounds one user's open requests. Each request notifies
// every approver, so an unbounded queue is an inbox-flooding tool.
const maxPendingPerRequester = 5

// ErrTooManyPending refuses a request from a user already at the pending cap.
var ErrTooManyPending = errors.New("too many pending elevation requests")

// ErrAlreadyPending refuses a second pending request for the same permission and scope.
var ErrAlreadyPending = errors.New("an elevation request for this permission is already pending")

// ErrSelfApproval stops a requester granting their own elevation, which would
// turn an approval workflow into a self-service permission switch.
var ErrSelfApproval = errors.New("an elevation cannot be approved by its requester")

// Create inserts a new pending request. Auto-rejects if the
// permission is not flagged as elevatable in the catalog.
func (s *Service) Create(ctx context.Context, requesterID string, in CreateRequest) (ElevationRequest, error) {
	elevatable, found, err := rbac.IsElevatable(ctx, s.db, in.PermissionKey)
	if err != nil {
		return ElevationRequest{}, err
	}
	if !found {
		return ElevationRequest{}, fmt.Errorf("%w: %q", ErrUnknownPermission, in.PermissionKey)
	}
	if !elevatable {
		return ElevationRequest{}, ErrNotElevatable
	}
	// elevation_requests references permission_catalog, which only the host seeds,
	// so a plugin key is mirrored in before its first request can be stored.
	if err := s.mirrorPluginPermission(ctx, in.PermissionKey); err != nil {
		return ElevationRequest{}, fmt.Errorf("mirroring plugin permission %s: %w", in.PermissionKey, err)
	}

	envID := ""
	if in.EnvironmentID != nil {
		envID = *in.EnvironmentID
	}
	var pending, same int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN permission_key = ? AND COALESCE(environment_id, '') = ? THEN 1 ELSE 0 END), 0)
		   FROM elevation_requests WHERE requester_id = ? AND status = 'pending'`,
		in.PermissionKey, envID, requesterID).Scan(&pending, &same); err != nil {
		return ElevationRequest{}, fmt.Errorf("counting pending elevation requests: %w", err)
	}
	if same > 0 {
		return ElevationRequest{}, ErrAlreadyPending
	}
	if pending >= maxPendingPerRequester {
		return ElevationRequest{}, ErrTooManyPending
	}

	if in.RequestedMinutes <= 0 {
		in.RequestedMinutes = 60
	}
	if in.RequestedMinutes > 24*60 {
		in.RequestedMinutes = 24 * 60
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO elevation_requests
		 (id, requester_id, permission_key, environment_id, resource_type, resource_id,
		  justification, requested_minutes, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
		id, requesterID, in.PermissionKey, in.EnvironmentID, in.ResourceType, in.ResourceID,
		in.Justification, in.RequestedMinutes, now,
	)
	if err != nil {
		return ElevationRequest{}, err
	}

	return ElevationRequest{
		ID:               id,
		RequesterID:      requesterID,
		PermissionKey:    in.PermissionKey,
		EnvironmentID:    in.EnvironmentID,
		ResourceType:     in.ResourceType,
		ResourceID:       in.ResourceID,
		Justification:    in.Justification,
		RequestedMinutes: in.RequestedMinutes,
		Status:           StatusPending,
		CreatedAt:        now,
	}, nil
}

// ListMine returns the requester's own requests (newest first).
func (s *Service) ListMine(ctx context.Context, requesterID string) ([]ElevationRequest, error) {
	return s.query(ctx,
		"WHERE e.requester_id = ? ORDER BY e.created_at DESC LIMIT 100",
		requesterID,
	)
}

// ListPending returns the admin inbox (status=pending, newest first).
func (s *Service) ListPending(ctx context.Context) ([]ElevationRequest, error) {
	return s.query(ctx, "WHERE e.status = 'pending' ORDER BY e.created_at ASC LIMIT 100")
}

// ListAll returns every request regardless of status. Admin only.
func (s *Service) ListAll(ctx context.Context) ([]ElevationRequest, error) {
	return s.query(ctx, "ORDER BY e.created_at DESC LIMIT 200")
}

// ByID returns a single request.
func (s *Service) ByID(ctx context.Context, id string) (ElevationRequest, error) {
	rows, err := s.query(ctx, "WHERE e.id = ? LIMIT 1", id)
	if err != nil {
		return ElevationRequest{}, err
	}
	if len(rows) == 0 {
		return ElevationRequest{}, ErrNotFound
	}
	return rows[0], nil
}

// Decide approves or denies a pending request. Updates the row,
// writes an elevation_history entry, invalidates the user's effective
// permissions by writing nothing directly — the next EffectivePermissions
// call re-reads the table.
func (s *Service) Decide(ctx context.Context, id, approverID string, approve bool, note string) (ElevationRequest, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Read existing so we know the requester + minutes.
	existing, err := s.ByID(ctx, id)
	if err != nil {
		return ElevationRequest{}, err
	}
	if existing.Status != StatusPending {
		return ElevationRequest{}, ErrNotPending
	}
	if approve && approverID == existing.RequesterID {
		return ElevationRequest{}, ErrSelfApproval
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ElevationRequest{}, err
	}
	defer db.RollbackTx(tx)

	newStatus := StatusDenied
	var approvedUntil *string
	if approve {
		newStatus = StatusApproved
		until := time.Now().UTC().Add(time.Duration(existing.RequestedMinutes) * time.Minute).Format(time.RFC3339)
		approvedUntil = &until
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE elevation_requests
		 SET status = ?, approver_id = ?, approver_note = ?, approved_until = ?, decided_at = ?
		 WHERE id = ? AND status = 'pending'`,
		newStatus, approverID, note, approvedUntil, now, id,
	)
	if err != nil {
		return ElevationRequest{}, err
	}

	action := "deny"
	if approve {
		action = "approve"
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO elevation_history (id, request_id, actor_id, action, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		xid.New().String(), id, approverID, action, note, now,
	)
	if err != nil {
		return ElevationRequest{}, err
	}

	if err := tx.Commit(); err != nil {
		return ElevationRequest{}, err
	}

	// Re-read with the new state.
	return s.ByID(ctx, id)
}

// query is a small helper that runs the SELECT and returns parsed
// ElevationRequest rows.
func (s *Service) query(ctx context.Context, where string, args ...any) ([]ElevationRequest, error) {
	// bounded by the LIMIT every caller puts in where (100, 200 or 1)
	q := "SELECT e.id, e.requester_id, e.permission_key, e.environment_id, e.resource_type, e.resource_id, " + // SQL identifier: where is one of four fixed clauses from this file's callers, never user input
		"e.justification, e.requested_minutes, e.status, e.approver_id, e.approver_note, e.approved_until, " +
		"e.decided_at, e.created_at, COALESCE(pc.description, pp.description, '') FROM elevation_requests e " +
		"LEFT JOIN permission_catalog pc ON pc.key = e.permission_key " +
		"LEFT JOIN (SELECT permission_key, MIN(description) AS description FROM plugin_permissions GROUP BY permission_key) pp " +
		"ON pp.permission_key = e.permission_key " + where
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { db.CloseRows(rows) }()
	var items []ElevationRequest
	for rows.Next() {
		var r ElevationRequest
		var envID, resType, resID, approverID, approverNote, approvedUntil, decidedAt sql.NullString
		if err := rows.Scan(
			&r.ID, &r.RequesterID, &r.PermissionKey, &envID, &resType, &resID,
			&r.Justification, &r.RequestedMinutes, &r.Status, &approverID, &approverNote, &approvedUntil,
			&decidedAt, &r.CreatedAt, &r.PermissionLabel,
		); err != nil {
			return nil, err
		}
		if envID.Valid {
			s := envID.String
			r.EnvironmentID = &s
		}
		if resType.Valid {
			s := resType.String
			r.ResourceType = &s
		}
		if resID.Valid {
			s := resID.String
			r.ResourceID = &s
		}
		if approverID.Valid {
			s := approverID.String
			r.ApproverID = &s
		}
		if approverNote.Valid {
			s := approverNote.String
			r.ApproverNote = &s
		}
		if approvedUntil.Valid {
			s := approvedUntil.String
			r.ApprovedUntil = &s
		}
		if decidedAt.Valid {
			s := decidedAt.String
			r.DecidedAt = &s
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []ElevationRequest{}
	}
	return items, nil
}

// Elevatable lists every permission with whether it can be requested.
func (s *Service) Elevatable(ctx context.Context) ([]rbac.ElevatableEntry, error) {
	return rbac.ElevatableCatalog(ctx, s.db)
}

// SetElevatable records an admin's decision for one key, refusing a key no
// host or plugin catalog defines so a typo cannot leave an orphan override.
func (s *Service) SetElevatable(ctx context.Context, key string, elevatable bool, actorID string) error {
	if _, found, err := rbac.IsElevatable(ctx, s.db, key); err != nil {
		return err
	} else if !found {
		return ErrUnknownPermission
	}
	return rbac.SetElevatable(ctx, s.db, key, elevatable, actorID)
}

// SweepExpired marks every approved request whose approved_until is in
// the past as expired. Phase 6 of the RBAC roadmap. Called from
// the request handler as a cheap pre-read so the UI sees the right
// state without a background worker. Idempotent and cheap.
func (s *Service) SweepExpired(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE elevation_requests
		 SET status = 'expired'
		 WHERE status = 'approved' AND approved_until IS NOT NULL AND approved_until < ?`,
		now,
	)
	return err
}

// mirrorPluginPermission copies a plugin key into permission_catalog with its
// scope translated to the catalog vocabulary, because the table's CHECK rejects
// the manifest's "env". A key the plugins table does not know is a no-op.
func (s *Service) mirrorPluginPermission(ctx context.Context, key string) error {
	var origin, description, scope string
	err := s.db.QueryRowContext(ctx,
		`SELECT origin, description, scope FROM plugin_permissions WHERE permission_key = ? LIMIT 1`, key,
	).Scan(&origin, &description, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO permission_catalog (key, category, action, description, scope, is_system, added_at)
		 VALUES (?, ?, '', ?, ?, 0, CURRENT_TIMESTAMP)
		 ON CONFLICT (key) DO NOTHING`, key, origin, description, permcatalog.CatalogScope(scope))
	return err
}
