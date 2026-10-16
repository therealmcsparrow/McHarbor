// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Sentinel errors the handler maps onto existing i18n codes plus a data.reason.
var (
	ErrNotFound              = errors.New("approval not found")
	ErrInvalidRule           = errors.New("approval rule is invalid")
	ErrSelfApproval          = errors.New("a requester cannot decide their own request")
	ErrNotPending            = errors.New("approval is no longer pending")
	ErrExpired               = errors.New("approval has expired")
	ErrAlreadyDecided        = errors.New("approver has already decided this request")
	ErrApproverLacksPerm     = errors.New("approver lacks the rule's approver permission")
	ErrNotRequester          = errors.New("only the requester may act on this request")
	ErrNotApproved           = errors.New("approval is not approved")
	ErrAlreadyExecuted       = errors.New("approval was already executed")
	ErrEncryptionUnavailable = errors.New("request body cannot be stored without encryption")
)

// Encrypter is the part of the encryption service the stored body needs.
type Encrypter interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encrypted string) (string, error)
}

// Service stores rules and requests. It never executes anything itself; the
// handler replays a claimed request through the router.
type Service struct {
	db  *sql.DB
	enc Encrypter
	now func() time.Time
}

// NewService returns a Service backed by database; enc keeps the replayable body
// encrypted at rest and may be nil only where no held route carries a body.
func NewService(database *sql.DB, enc Encrypter) *Service {
	return &Service{db: database, enc: enc, now: time.Now}
}

func (s *Service) stamp() string { return s.now().UTC().Format(time.RFC3339) }

const ruleColumns = `id, category, environment_id, enabled, required_approvers, approver_permission,
	expiry_minutes, description, COALESCE(created_by, ''), created_at, updated_at`

func scanRule(row interface{ Scan(...any) error }) (*Rule, error) {
	var r Rule
	var env sql.NullString
	var enabled int
	if err := row.Scan(&r.ID, &r.Category, &env, &enabled, &r.RequiredApprovers, &r.ApproverPermission,
		&r.ExpiryMinutes, &r.Description, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if env.Valid && env.String != "" {
		r.EnvironmentID = &env.String
	}
	r.Enabled = enabled == 1
	return &r, nil
}

// ListRules returns every rule, categories first so the builder groups them.
func (s *Service) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+ruleColumns+` FROM approval_rules ORDER BY category, created_at LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing approval rules: %w", err)
	}
	defer func() { db.CloseRows(rows) }()
	out := []Rule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning approval rule: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// validateRule normalises in and rejects values the gate could not enforce.
func validateRule(in *RuleInput) error {
	in.Category = strings.TrimSpace(in.Category)
	if !KnownCategory(in.Category) {
		return ErrInvalidRule
	}
	if in.RequiredApprovers == 0 {
		in.RequiredApprovers = 1
	}
	if in.RequiredApprovers < 1 || in.RequiredApprovers > 5 {
		return ErrInvalidRule
	}
	if in.ExpiryMinutes == 0 {
		in.ExpiryMinutes = 1440
	}
	if in.ExpiryMinutes < 5 || in.ExpiryMinutes > 10080 {
		return ErrInvalidRule
	}
	in.ApproverPermission = strings.TrimSpace(in.ApproverPermission)
	if in.ApproverPermission == "" {
		in.ApproverPermission = string(rbac.PermApprovalsDecide)
	}
	known := false
	for _, e := range rbac.CatalogSeed() {
		if string(e.Key) == in.ApproverPermission {
			known = true
			break
		}
	}
	if !known {
		return ErrInvalidRule
	}
	if in.EnvironmentID != nil && strings.TrimSpace(*in.EnvironmentID) == "" {
		in.EnvironmentID = nil
	}
	in.Description = strings.TrimSpace(in.Description)
	return nil
}

// CreateRule validates and stores a rule; a new rule is enabled unless told otherwise.
func (s *Service) CreateRule(ctx context.Context, in RuleInput, actorID string) (*Rule, error) {
	if err := validateRule(&in); err != nil {
		return nil, err
	}
	enabled := in.Enabled == nil || *in.Enabled
	id := xid.New().String()
	now := s.stamp()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO approval_rules (id, category, environment_id, enabled, required_approvers,
		approver_permission, expiry_minutes, description, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Category, in.EnvironmentID, boolInt(enabled), in.RequiredApprovers, in.ApproverPermission,
		in.ExpiryMinutes, in.Description, actorID, now, now); err != nil {
		return nil, fmt.Errorf("creating approval rule: %w", err)
	}
	return s.Rule(ctx, id)
}

// UpdateRule replaces a rule's settings. Requests already held keep the terms
// they were created under; only new requests see the change.
func (s *Service) UpdateRule(ctx context.Context, id string, in RuleInput) (*Rule, error) {
	if err := validateRule(&in); err != nil {
		return nil, err
	}
	enabled := in.Enabled == nil || *in.Enabled
	res, err := s.db.ExecContext(ctx, `UPDATE approval_rules SET category = ?, environment_id = ?, enabled = ?,
		required_approvers = ?, approver_permission = ?, expiry_minutes = ?, description = ?, updated_at = ?
		WHERE id = ?`,
		in.Category, in.EnvironmentID, boolInt(enabled), in.RequiredApprovers, in.ApproverPermission,
		in.ExpiryMinutes, in.Description, s.stamp(), id)
	if err != nil {
		return nil, fmt.Errorf("updating approval rule: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return nil, ErrNotFound
	}
	return s.Rule(ctx, id)
}

// DeleteRule removes a rule, which returns its category to ungated behaviour.
func (s *Service) DeleteRule(ctx context.Context, id string) (*Rule, error) {
	rule, err := s.Rule(ctx, id)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("deleting approval rule: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return nil, ErrNotFound
	}
	return rule, nil
}

// Rule returns one rule by id.
func (s *Service) Rule(ctx context.Context, id string) (*Rule, error) {
	r, err := scanRule(s.db.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM approval_rules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading approval rule: %w", err)
	}
	return r, nil
}

// RuleFor returns the enabled rule that governs category in envID, preferring an
// environment-specific rule over a global one; nil means the action is ungated.
func (s *Service) RuleFor(ctx context.Context, category, envID string) (*Rule, error) {
	r, err := scanRule(s.db.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM approval_rules
		WHERE category = ? AND enabled = 1 AND (environment_id IS NULL OR environment_id = '' OR environment_id = ?)
		ORDER BY CASE WHEN environment_id IS NULL OR environment_id = '' THEN 1 ELSE 0 END, created_at
		LIMIT 1`, category, envID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolving approval rule: %w", err)
	}
	return r, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
