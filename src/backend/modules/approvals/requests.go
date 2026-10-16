// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// HoldInput is everything the gate captured about a request it is holding.
type HoldInput struct {
	Rule          *Rule
	Category      string
	RequesterID   string
	RequesterName string
	EnvironmentID string
	Method        string
	Path          string
	RoutePattern  string
	ResourceName  string
	Body          []byte
	APIKeyID      string
}

const requestColumns = `id, COALESCE(rule_id, ''), category, requester_id, requester_name, environment_id, method, path,
	route_pattern, resource_name, body_redacted, body_encrypted, COALESCE(api_key_id, ''), status, required_approvers,
	approvals_count, approver_permission, expires_at, decided_at, executed_at, result_status, result_code,
	created_at, updated_at`

func scanRequest(row interface{ Scan(...any) error }) (*Request, error) {
	var r Request
	var env, decided, executed sql.NullString
	var result sql.NullInt64
	if err := row.Scan(&r.ID, &r.RuleID, &r.Category, &r.RequesterID, &r.RequesterName, &env, &r.Method, &r.Path,
		&r.RoutePattern, &r.ResourceName, &r.BodyRedacted, &r.bodyEncrypted, &r.apiKeyID, &r.Status, &r.RequiredApprovers,
		&r.ApprovalsCount, &r.ApproverPermission, &r.ExpiresAt, &decided, &executed, &result, &r.ResultCode,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if env.Valid && env.String != "" {
		r.EnvironmentID = &env.String
	}
	if decided.Valid {
		r.DecidedAt = &decided.String
	}
	if executed.Valid {
		r.ExecutedAt = &executed.String
	}
	if result.Valid {
		v := int(result.Int64)
		r.ResultStatus = &v
	}
	r.Decisions = []Decision{}
	return &r, nil
}

// Hold stores a held request, or returns the caller's still-open request for the
// same method and path so a double click queues one approval, not two.
func (s *Service) Hold(ctx context.Context, in HoldInput) (*Request, bool, error) {
	if in.Rule == nil {
		return nil, false, ErrInvalidRule
	}
	now := s.stamp()
	existing, err := scanRequest(s.db.QueryRowContext(ctx, `SELECT `+requestColumns+` FROM approval_requests
		WHERE requester_id = ? AND method = ? AND path = ? AND status IN (?, ?) AND expires_at > ?
		ORDER BY created_at DESC LIMIT 1`, in.RequesterID, in.Method, in.Path, StatusPending, StatusApproved, now))
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("checking open approvals: %w", err)
	}

	encrypted := ""
	if len(in.Body) > 0 {
		if s.enc == nil {
			return nil, false, ErrEncryptionUnavailable
		}
		if encrypted, err = s.enc.Encrypt(string(in.Body)); err != nil {
			return nil, false, fmt.Errorf("encrypting held body: %w", err)
		}
	}
	var env any
	if in.EnvironmentID != "" {
		env = in.EnvironmentID
	}
	var keyID any
	if in.APIKeyID != "" {
		keyID = in.APIKeyID
	}
	id := xid.New().String()
	expires := s.now().UTC().Add(time.Duration(in.Rule.ExpiryMinutes) * time.Minute).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO approval_requests (id, rule_id, category, requester_id, requester_name,
		environment_id, method, path, route_pattern, resource_name, body_redacted, body_encrypted, api_key_id, status,
		required_approvers, approvals_count, approver_permission, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		id, in.Rule.ID, in.Category, in.RequesterID, in.RequesterName, env, in.Method, in.Path, in.RoutePattern,
		in.ResourceName, redactBody(in.Body), encrypted, keyID, StatusPending, in.Rule.RequiredApprovers,
		in.Rule.ApproverPermission, expires, now, now); err != nil {
		return nil, false, fmt.Errorf("holding request: %w", err)
	}
	req, err := s.Get(ctx, id)
	return req, true, err
}

// SweepExpired marks every open request whose window has passed. It runs before
// each read so a status is never older than the clock, with no janitor needed.
func (s *Service) SweepExpired(ctx context.Context) error {
	now := s.stamp()
	if _, err := s.db.ExecContext(ctx, `UPDATE approval_requests SET status = ?, updated_at = ?
		WHERE status IN (?, ?) AND expires_at <= ?`, StatusExpired, now, StatusPending, StatusApproved, now); err != nil {
		return fmt.Errorf("expiring approvals: %w", err)
	}
	return nil
}

// List returns one page of requests, newest first, filtered by status and,
// when requesterID is set, to that requester's own.
func (s *Service) List(ctx context.Context, status, requesterID string, page, perPage int) ([]Request, int64, error) {
	if err := s.SweepExpired(ctx); err != nil {
		return nil, 0, err
	}
	where := ` WHERE 1 = 1`
	var args []any
	if status != "" {
		where += ` AND status = ?`
		args = append(args, status)
	}
	if requesterID != "" {
		where += ` AND requester_id = ?`
		args = append(args, requesterID)
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_requests`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting approvals: %w", err)
	}
	args = append(args, perPage, (page-1)*perPage)
	rows, err := s.db.QueryContext(ctx, `SELECT `+requestColumns+` FROM approval_requests`+where+
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing approvals: %w", err)
	}
	defer func() { db.CloseRows(rows) }()
	out := []Request{}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning approval: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		if out[i].Decisions, err = s.decisions(ctx, out[i].ID); err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

// Get returns one request with its decisions.
func (s *Service) Get(ctx context.Context, id string) (*Request, error) {
	if err := s.SweepExpired(ctx); err != nil {
		return nil, err
	}
	r, err := scanRequest(s.db.QueryRowContext(ctx, `SELECT `+requestColumns+` FROM approval_requests WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading approval: %w", err)
	}
	if r.Decisions, err = s.decisions(ctx, id); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) decisions(ctx context.Context, id string) ([]Decision, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, approver_id, approver_name, decision, note, created_at
		FROM approval_decisions WHERE request_id = ? ORDER BY created_at LIMIT 100`, id)
	if err != nil {
		return nil, fmt.Errorf("listing decisions: %w", err)
	}
	defer func() { db.CloseRows(rows) }()
	out := []Decision{}
	for rows.Next() {
		var d Decision
		if err := rows.Scan(&d.ID, &d.ApproverID, &d.ApproverName, &d.Decision, &d.Note, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning decision: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PermissionCheck answers whether the approver holds perm in the request's
// environment. It is injected so the service does not re-implement RBAC.
type PermissionCheck func(ctx context.Context, perm, envID string) (bool, error)

// Decide records one approver's vote. The requester can never vote (separation
// of duties), any deny closes the request, and the request is approved only
// once the rule's count of distinct approvers is reached.
func (s *Service) Decide(ctx context.Context, id, approverID, approverName string, in DecideInput, can PermissionCheck) (*Request, error) {
	req, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.RequesterID == approverID {
		return nil, ErrSelfApproval
	}
	switch req.Status {
	case StatusPending:
	case StatusExpired:
		return nil, ErrExpired
	default:
		return nil, ErrNotPending
	}
	env := ""
	if req.EnvironmentID != nil {
		env = *req.EnvironmentID
	}
	if can != nil {
		ok, err := can(ctx, req.ApproverPermission, env)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrApproverLacksPerm
		}
	}
	for _, d := range req.Decisions {
		if d.ApproverID == approverID {
			return nil, ErrAlreadyDecided
		}
	}

	decision := "deny"
	if in.Approve {
		decision = "approve"
	}
	now := s.stamp()
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO approval_decisions (id, request_id, approver_id, approver_name, decision, note, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, xid.New().String(), id, approverID, approverName, decision, in.Note, now, now); err != nil {
			return fmt.Errorf("recording decision: %w", err)
		}
		var res sql.Result
		var err error
		if in.Approve {
			res, err = tx.ExecContext(ctx, `UPDATE approval_requests
				SET approvals_count = approvals_count + 1,
				    status = CASE WHEN approvals_count + 1 >= required_approvers THEN ? ELSE status END,
				    decided_at = CASE WHEN approvals_count + 1 >= required_approvers THEN ? ELSE decided_at END,
				    updated_at = ?
				WHERE id = ? AND status = ? AND expires_at > ?`, StatusApproved, now, now, id, StatusPending, now)
		} else {
			res, err = tx.ExecContext(ctx, `UPDATE approval_requests SET status = ?, decided_at = ?, updated_at = ?
				WHERE id = ? AND status = ? AND expires_at > ?`, StatusDenied, now, now, id, StatusPending, now)
		}
		if err != nil {
			return fmt.Errorf("applying decision: %w", err)
		}
		if db.RowsAffected(res) == 0 {
			return ErrNotPending
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Cancel withdraws the requester's own open request.
func (s *Service) Cancel(ctx context.Context, id, requesterID string) (*Request, error) {
	req, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.RequesterID != requesterID {
		return nil, ErrNotRequester
	}
	now := s.stamp()
	res, err := s.db.ExecContext(ctx, `UPDATE approval_requests SET status = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`, StatusCancelled, now, id, StatusPending, StatusApproved)
	if err != nil {
		return nil, fmt.Errorf("cancelling approval: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return nil, ErrNotPending
	}
	return s.Get(ctx, id)
}

// Claim moves an approved request to executing for its requester, atomically,
// so two concurrent execute calls cannot both replay it. It returns the
// decrypted original body.
func (s *Service) Claim(ctx context.Context, id, requesterID string) (*Request, []byte, error) {
	req, err := s.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if req.RequesterID != requesterID {
		return nil, nil, ErrNotRequester
	}
	now := s.stamp()
	res, err := s.db.ExecContext(ctx, `UPDATE approval_requests SET status = ?, updated_at = ?
		WHERE id = ? AND requester_id = ? AND status = ? AND expires_at > ?`,
		StatusExecuting, now, id, requesterID, StatusApproved, now)
	if err != nil {
		return nil, nil, fmt.Errorf("claiming approval: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		switch req.Status {
		case StatusExpired:
			return nil, nil, ErrExpired
		case StatusExecuted, StatusExecuting:
			return nil, nil, ErrAlreadyExecuted
		default:
			return nil, nil, ErrNotApproved
		}
	}
	var body []byte
	if req.bodyEncrypted != "" {
		if s.enc == nil {
			return nil, nil, ErrEncryptionUnavailable
		}
		plain, err := s.enc.Decrypt(req.bodyEncrypted)
		if err != nil {
			return nil, nil, fmt.Errorf("decrypting held body: %w", err)
		}
		body = []byte(plain)
	}
	req.Status = StatusExecuting
	return req, body, nil
}

// Finish records the replay's outcome. The request is executed whatever the
// status: the one permitted attempt has been spent.
func (s *Service) Finish(ctx context.Context, id string, status int, code string) (*Request, error) {
	now := s.stamp()
	if _, err := s.db.ExecContext(ctx, `UPDATE approval_requests SET status = ?, executed_at = ?, result_status = ?,
		result_code = ?, body_encrypted = '', updated_at = ? WHERE id = ? AND status = ?`,
		StatusExecuted, now, status, code, now, id, StatusExecuting); err != nil {
		return nil, fmt.Errorf("finishing approval: %w", err)
	}
	return s.Get(ctx, id)
}

// inTx runs fn in a transaction bound to ctx, so a cancelled request abandons the decision instead of committing it.
func (s *Service) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		db.RollbackTx(tx)
		return err
	}
	return tx.Commit()
}
