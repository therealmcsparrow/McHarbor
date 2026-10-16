// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// maxDirectoryRows bounds every reconcile read; a directory past it blocks the
// plan instead of silently reconciling against a truncated account list.
const maxDirectoryRows = 50000

// Sync run triggers recorded in scim_sync_log.run_trigger.
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
	TriggerPreview  = "preview"
)

// ErrPlanBlocked is returned when a plan refuses to apply, e.g. because the
// directory came back empty while accounts are still linked.
var ErrPlanBlocked = errors.New("scim sync plan is blocked")

// SyncRun is one entry for scim_sync_log.
type SyncRun struct {
	Status  string
	Error   string
	Summary SyncSummary
	DryRun  bool
	Trigger string
	ActorID string
	Notes   []string
}

// PlanSync pulls the remote directory and computes what a sync would change,
// without writing anything.
func (s *Service) PlanSync(ctx context.Context, puller *Puller, endpointID string) (*SyncPlan, error) {
	ep, err := s.EndpointByID(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	token, err := s.DecryptToken(ctx, endpointID)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}
	users, groups, err := puller.Pull(ctx, ep, token)
	if err != nil {
		return nil, err
	}
	return s.planFrom(ctx, endpointID, users, groups)
}

func (s *Service) planFrom(ctx context.Context, endpointID string, users []User, groups []Group) (*SyncPlan, error) {
	mappings, err := s.loadMappings(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	locals, err := s.loadLocalUsers(ctx)
	if err != nil {
		return nil, err
	}
	localGroups, err := s.loadLocalGroups(ctx)
	if err != nil {
		return nil, err
	}
	plan := computePlan(users, groups, mappings, locals, localGroups)
	plan.EndpointID = endpointID
	plan.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if len(locals) >= maxDirectoryRows || len(mappings) >= maxDirectoryRows {
		plan.Blocked = true
		plan.BlockedReason = "directory_too_large"
	}
	return &plan, nil
}

// ApplyPlan writes a plan in one transaction: mapping rows, profile updates,
// and disables with session revocation. Skipped rows are left untouched.
func (s *Service) ApplyPlan(ctx context.Context, endpointID string, plan *SyncPlan) error {
	if plan.Blocked {
		return ErrPlanBlocked
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		for _, change := range plan.Users {
			if err := applyChange(ctx, tx, endpointID, change, now); err != nil {
				return fmt.Errorf("applying %s for %s: %w", change.Action, change.ScimID, err)
			}
		}
		for _, scimID := range plan.SeenIDs {
			if _, err := tx.ExecContext(ctx,
				`UPDATE scim_user_mappings SET last_seen_at = ? WHERE endpoint_id = ? AND scim_user_id = ?`,
				now, endpointID, scimID); err != nil {
				return fmt.Errorf("touching mapping %s: %w", scimID, err)
			}
		}
		return nil
	})
}

func applyChange(ctx context.Context, tx *sql.Tx, endpointID string, change UserChange, now string) error {
	switch change.Action {
	case ActionSkip:
		return nil
	case ActionUnmatched:
		return upsertMapping(ctx, tx, endpointID, change, "")
	case ActionLink:
		return upsertMapping(ctx, tx, endpointID, change, change.LocalUserID)
	case ActionUpdate:
		if err := upsertMapping(ctx, tx, endpointID, change, change.LocalUserID); err != nil {
			return err
		}
		return updateProfile(ctx, tx, change, now)
	case ActionDisable:
		if change.Link {
			if err := upsertMapping(ctx, tx, endpointID, change, change.LocalUserID); err != nil {
				return err
			}
		}
		return disableUser(ctx, tx, endpointID, change, now)
	}
	return fmt.Errorf("unknown action %q", change.Action)
}

func upsertMapping(ctx context.Context, tx *sql.Tx, endpointID string, change UserChange, localID string) error {
	var local any
	if localID != "" {
		local = localID
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE scim_user_mappings
		   SET mcharbor_user_id = ?, scim_username = ?, scim_email = ?, last_seen_at = ?
		 WHERE endpoint_id = ? AND scim_user_id = ?`,
		local, change.UserName, change.Email, time.Now().UTC().Format(time.RFC3339), endpointID, change.ScimID)
	if err != nil {
		return err
	}
	if db.RowsAffected(res) > 0 {
		return nil
	}
	id, err := newID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO scim_user_mappings (id, endpoint_id, scim_user_id, mcharbor_user_id, scim_username, scim_email)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, endpointID, change.ScimID, local, change.UserName, change.Email)
	return err
}

func updateProfile(ctx context.Context, tx *sql.Tx, change UserChange, now string) error {
	var sets []string
	var args []any
	for _, field := range change.Fields {
		switch field {
		case "email":
			sets = append(sets, "email = ?")
			args = append(args, change.Email)
		case "displayName":
			sets = append(sets, "display_name = ?")
			args = append(args, change.DisplayName)
		}
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, now, change.LocalUserID)
	_, err := tx.ExecContext(ctx, "UPDATE users SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...) // SQL identifier: sets holds only the fixed "column = ?" fragments appended above
	return err
}

func disableUser(ctx context.Context, tx *sql.Tx, endpointID string, change UserChange, now string) error {
	by := "scim:" + endpointID
	reason := "SCIM: " + change.Reason
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		   SET is_active = 0, disabled_at = ?, disabled_by = ?, disabled_reason = ?, updated_at = ?
		 WHERE id = ? AND (disabled_at IS NULL OR disabled_at = '')`,
		now, by, reason, now, change.LocalUserID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ?, revoked_by = ?
		 WHERE user_id = ? AND (revoked_at IS NULL OR revoked_at = '')`,
		now, by, change.LocalUserID)
	return err
}

// RecordRun appends a sync-log row. Real runs also refresh the endpoint's
// last-sync columns and absolute counts; dry runs leave the endpoint alone.
func (s *Service) RecordRun(ctx context.Context, endpointID string, run SyncRun) error {
	logID, err := newID()
	if err != nil {
		return err
	}
	notes := ""
	if len(run.Notes) > 0 {
		encoded, err := json.Marshal(run.Notes)
		if err != nil {
			return fmt.Errorf("encoding sync notes: %w", err)
		}
		notes = string(encoded)
	}
	trigger := run.Trigger
	if trigger == "" {
		trigger = TriggerSchedule
	}
	var actor any
	if run.ActorID != "" {
		actor = run.ActorID
	}
	dryRun := 0
	if run.DryRun {
		dryRun = 1
	}
	summary := run.Summary
	return db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scim_sync_log
			    (id, endpoint_id, direction, status, finished_at,
			     users_created, users_updated, users_deleted,
			     groups_created, groups_updated, groups_deleted, error,
			     users_seen, users_linked, users_unmatched, users_disabled, users_skipped,
			     groups_seen, dry_run, run_trigger, actor_id, item_errors)
			VALUES (?, ?, 'pull', ?, CURRENT_TIMESTAMP, ?, ?, 0, 0, 0, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			logID, endpointID, run.Status,
			summary.Linked+summary.Unmatched, summary.Updated, run.Error,
			summary.UsersSeen, summary.Linked, summary.Unmatched, summary.Disabled, summary.Skipped,
			summary.GroupsSeen, dryRun, trigger, actor, notes,
		); err != nil {
			return fmt.Errorf("inserting sync log: %w", err)
		}
		if run.DryRun {
			return nil
		}
		if run.Status != "success" {
			_, err := tx.ExecContext(ctx, `
				UPDATE scim_endpoints
				   SET last_sync_at = CURRENT_TIMESTAMP, last_sync_status = ?, last_sync_error = ?,
				       updated_at = CURRENT_TIMESTAMP
				 WHERE id = ?`, run.Status, run.Error, endpointID)
			return err
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE scim_endpoints
			   SET last_sync_at = CURRENT_TIMESTAMP, last_sync_status = 'success', last_sync_error = '',
			       users_count = ?, groups_count = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`, summary.UsersSeen, summary.GroupsSeen, endpointID)
		return err
	})
}

// PlanNotes turns the skipped rows of a plan into the operator-facing notes
// stored with a run, so the history says which accounts were left alone and why.
func PlanNotes(plan *SyncPlan) []string {
	var notes []string
	if plan.Blocked {
		notes = append(notes, "blocked: "+plan.BlockedReason)
	}
	for _, change := range plan.Users {
		if change.Action == ActionSkip {
			notes = append(notes, fmt.Sprintf("skipped %s (%s): %s", change.UserName, change.LocalUsername, change.Reason))
		}
	}
	return notes
}

func (s *Service) loadMappings(ctx context.Context, endpointID string) ([]mappingRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, scim_user_id, COALESCE(mcharbor_user_id, ''), scim_username, scim_email
		  FROM scim_user_mappings
		 WHERE endpoint_id = ?
		 LIMIT ?`, endpointID, maxDirectoryRows)
	if err != nil {
		return nil, fmt.Errorf("loading scim mappings: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []mappingRow
	for rows.Next() {
		var m mappingRow
		if err := rows.Scan(&m.ID, &m.ScimUserID, &m.McharborUserID, &m.ScimUsername, &m.ScimEmail); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) loadLocalUsers(ctx context.Context) ([]localUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, COALESCE(u.email, ''), COALESCE(u.display_name, ''),
		       CASE WHEN u.is_active = 1 AND (u.disabled_at IS NULL OR u.disabled_at = '') THEN 1 ELSE 0 END,
		       CASE WHEN EXISTS (
		                SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		                 WHERE ur.user_id = u.id AND r.permissions LIKE '%"*"%')
		              OR EXISTS (
		                SELECT 1 FROM group_members gm
		                  JOIN group_roles gr ON gr.group_id = gm.group_id
		                  JOIN roles r ON r.id = gr.role_id
		                 WHERE gm.user_id = u.id AND r.permissions LIKE '%"*"%')
		            THEN 1 ELSE 0 END
		  FROM users u
		 WHERE u.id NOT IN ('system', '__legacy_owner__')
		 LIMIT ?`, maxDirectoryRows)
	if err != nil {
		return nil, fmt.Errorf("loading local users: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []localUser
	for rows.Next() {
		var u localUser
		var active, protected int
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &active, &protected); err != nil {
			return nil, err
		}
		u.Active = active == 1
		u.Protected = protected == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) loadLocalGroups(ctx context.Context) ([]localGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM groups LIMIT ?`, maxDirectoryRows)
	if err != nil {
		return nil, fmt.Errorf("loading local groups: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []localGroup
	for rows.Next() {
		var g localGroup
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
