// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"strings"
	"time"
)

// Bulk actions and their per-user outcomes.
const (
	BulkDisable = "disable"
	BulkEnable  = "enable"
	BulkUnlock  = "unlock"
	BulkDelete  = "delete"

	OutcomeWouldApply = "would_apply"
	OutcomeApplied    = "applied"
	OutcomeSkipped    = "skipped"
	OutcomeFailed     = "failed"

	// MaxBulkUsers bounds one request so a preview stays readable and an apply
	// cannot hold the write lock for long.
	MaxBulkUsers = 200
)

// ErrBulkInvalid is returned for an unknown action or an empty or oversized selection.
var ErrBulkInvalid = errors.New("bulk user action is invalid")

// BulkInput is the body of both bulk routes. DryRun reports what would happen
// without changing anything, using the same rules the apply path enforces.
type BulkInput struct {
	Action  string   `json:"action"`
	UserIDs []string `json:"userIds"`
	DryRun  bool     `json:"dryRun"`
	Reason  string   `json:"reason"`
}

// BulkItem is the outcome for one selected user.
type BulkItem struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason,omitempty"`
}

// BulkSummary counts outcomes, and is what the audit summary records.
type BulkSummary struct {
	Total   int `json:"total"`
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// BulkResult is the preview or the report of an apply.
type BulkResult struct {
	Action  string      `json:"action"`
	DryRun  bool        `json:"dryRun"`
	Items   []BulkItem  `json:"items"`
	Summary BulkSummary `json:"summary"`
}

type bulkTarget struct {
	id, username string
	exists       bool
	activeAdmin  bool
	disabled     bool
	idpManaged   bool
	locked       bool
}

func normalizeBulk(in *BulkInput) error {
	in.Action = strings.TrimSpace(strings.ToLower(in.Action))
	switch in.Action {
	case BulkDisable, BulkEnable, BulkUnlock, BulkDelete:
	default:
		return ErrBulkInvalid
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(in.UserIDs))
	for _, id := range in.UserIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > MaxBulkUsers {
		return ErrBulkInvalid
	}
	in.UserIDs = ids
	return nil
}

func (s *Service) loadBulkTargets(ctx context.Context, ids []string) (map[string]*bulkTarget, error) {
	marks := make([]string, len(ids))
	args := []any{time.Now().UTC().Format(time.RFC3339)}
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	//nolint:gosec // G202: the concatenated part is a list of "?" markers, every value is bound
	query := `SELECT id, username,
		(is_active = 1 AND role = 'admin' AND (disabled_at IS NULL OR disabled_at = '')),
		(disabled_at IS NOT NULL AND disabled_at != ''),
		COALESCE(auth_provider, 'local') != 'local',
		(password_locked_until IS NOT NULL AND password_locked_until > ?)
		FROM users WHERE id IN (` + strings.Join(marks, ",") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("loading bulk targets: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	out := map[string]*bulkTarget{}
	for rows.Next() {
		t := &bulkTarget{exists: true}
		if err := rows.Scan(&t.id, &t.username, &t.activeAdmin, &t.disabled, &t.idpManaged, &t.locked); err != nil {
			return nil, fmt.Errorf("scanning bulk target: %w", err)
		}
		out[t.id] = t
	}
	return out, rows.Err()
}

// precheck applies the rules every action shares, and the ones specific to it,
// so a preview and an apply refuse exactly the same users for the same reasons.
// remainingAdmins is decremented as the batch would remove admins, which is how
// a batch that selects every admin is caught on its last one.
func precheck(action, actorID string, t *bulkTarget, remainingAdmins *int) string {
	switch {
	case t == nil || !t.exists:
		return "not_found"
	case t.id == actorID && (action == BulkDisable || action == BulkDelete):
		return "self"
	case t.idpManaged && action == BulkDisable:
		return "idp_managed"
	}
	switch action {
	case BulkDisable:
		if t.disabled {
			return "already_disabled"
		}
	case BulkEnable:
		if !t.disabled {
			return "not_disabled"
		}
	case BulkUnlock:
		if !t.locked {
			return "not_locked"
		}
	}
	if t.activeAdmin && (action == BulkDisable || action == BulkDelete) {
		if *remainingAdmins <= 1 {
			return "last_admin"
		}
		*remainingAdmins--
	}
	return ""
}

// Bulk previews or applies one action over a selection.
func (s *Service) Bulk(ctx context.Context, in BulkInput, actorID string) (*BulkResult, error) {
	if err := normalizeBulk(&in); err != nil {
		return nil, err
	}
	targets, err := s.loadBulkTargets(ctx, in.UserIDs)
	if err != nil {
		return nil, err
	}
	admins, err := s.countActiveAdmins(ctx, "")
	if err != nil {
		return nil, err
	}
	res := &BulkResult{Action: in.Action, DryRun: in.DryRun, Items: make([]BulkItem, 0, len(in.UserIDs))}
	for _, id := range in.UserIDs {
		t := targets[id]
		item := BulkItem{UserID: id}
		if t != nil {
			item.Username = t.username
		}
		if reason := precheck(in.Action, actorID, t, &admins); reason != "" {
			item.Outcome, item.Reason = OutcomeSkipped, reason
		} else if in.DryRun {
			item.Outcome = OutcomeWouldApply
		} else if err := s.applyOne(ctx, in, id, actorID); err != nil {
			item.Outcome, item.Reason = OutcomeFailed, bulkErrorReason(err)
		} else {
			item.Outcome = OutcomeApplied
		}
		res.Items = append(res.Items, item)
		switch item.Outcome {
		case OutcomeApplied, OutcomeWouldApply:
			res.Summary.Applied++
		case OutcomeSkipped:
			res.Summary.Skipped++
		default:
			res.Summary.Failed++
		}
	}
	res.Summary.Total = len(res.Items)
	return res, nil
}

func (s *Service) applyOne(ctx context.Context, in BulkInput, id, actorID string) error {
	switch in.Action {
	case BulkDisable:
		return s.Disable(ctx, id, actorID, in.Reason)
	case BulkEnable:
		return s.Enable(ctx, id)
	case BulkUnlock:
		return s.Unlock(ctx, id, actorID)
	case BulkDelete:
		return s.Delete(ctx, id)
	}
	return ErrBulkInvalid
}

func bulkErrorReason(err error) string {
	switch {
	case errors.Is(err, ErrUserNotFound), errors.Is(err, sql.ErrNoRows):
		return "not_found"
	case errors.Is(err, ErrLastAdmin):
		return "last_admin"
	case errors.Is(err, ErrUserAlreadyDisabled):
		return "already_disabled"
	case errors.Is(err, ErrUserNotDisabled):
		return "not_disabled"
	case errors.Is(err, ErrUserIdentityProviderLocked):
		return "idp_managed"
	default:
		return "error"
	}
}
