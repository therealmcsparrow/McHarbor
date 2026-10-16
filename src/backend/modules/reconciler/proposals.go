// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Proposal statuses. Pending waits for a person; the rest are final.
const (
	ProposalPending    = "pending"
	ProposalApplying   = "applying"
	ProposalApplied    = "applied"
	ProposalFailed     = "failed"
	ProposalRejected   = "rejected"
	ProposalSuperseded = "superseded"
)

// Proposal origins: a person or the propose mode asked for it, or enforcement
// acted and recorded what it did.
const (
	OriginPropose = "propose"
	OriginEnforce = "enforce"
)

// ErrProposalNotPending is returned when a proposal was already resolved.
var ErrProposalNotPending = errors.New("reconciler: proposal is no longer pending")

// Proposal is a reviewable plan to restore one desired state, and afterwards
// the record of what applying it did.
type Proposal struct {
	ID             string       `json:"id"`
	DesiredStateID string       `json:"desiredStateId"`
	DesiredName    string       `json:"desiredName"`
	Kind           string       `json:"kind"`
	EnvID          string       `json:"envId"`
	Origin         string       `json:"origin"`
	Status         string       `json:"status"`
	Fingerprint    string       `json:"fingerprint"`
	Report         *DriftReport `json:"report"`
	Steps          []Step       `json:"steps"`
	Results        []StepResult `json:"results"`
	Error          string       `json:"error,omitempty"`
	Note           string       `json:"note,omitempty"`
	ResolvedBy     string       `json:"resolvedBy,omitempty"`
	ResolvedAt     string       `json:"resolvedAt,omitempty"`
	CreatedAt      string       `json:"createdAt"`
}

// ProposalStore persists proposals.
type ProposalStore struct {
	db *sql.DB
}

// NewProposalStore returns a store over db.
func NewProposalStore(database *sql.DB) *ProposalStore {
	return &ProposalStore{db: database}
}

const proposalColumns = `p.id, p.desired_state_id, COALESCE(d.name, ''), COALESCE(d.kind, ''), p.env_id, p.origin, p.status,
	p.fingerprint, p.report, p.steps, p.result, p.error, p.note, p.resolved_by, p.resolved_at, p.created_at`

func scanProposal(row rowScanner) (Proposal, error) {
	var p Proposal
	var envID, resolvedAt sql.NullString
	var report, steps, result string
	if err := row.Scan(&p.ID, &p.DesiredStateID, &p.DesiredName, &p.Kind, &envID, &p.Origin, &p.Status,
		&p.Fingerprint, &report, &steps, &result, &p.Error, &p.Note, &p.ResolvedBy, &resolvedAt, &p.CreatedAt); err != nil {
		return p, err
	}
	p.EnvID, p.ResolvedAt = envID.String, resolvedAt.String
	p.Steps, p.Results = []Step{}, []StepResult{}
	if report != "" {
		var r DriftReport
		if json.Unmarshal([]byte(report), &r) == nil {
			p.Report = &r
		}
	}
	if steps != "" {
		if err := json.Unmarshal([]byte(steps), &p.Steps); err != nil {
			return p, fmt.Errorf("decoding proposal steps: %w", err)
		}
		// A row written before steps were redacted on write still carries live env values.
		p.Steps = redactSteps(p.Steps)
	}
	if result != "" {
		if err := json.Unmarshal([]byte(result), &p.Results); err != nil {
			return p, fmt.Errorf("decoding proposal results: %w", err)
		}
	}
	return p, nil
}

// Create writes a pending proposal and supersedes any earlier pending one for
// the same desired state, so only the plan for the current drift can be approved.
func (s *ProposalStore) Create(ctx context.Context, ds *DesiredState, origin string, report *DriftReport, steps []Step) (_ *Proposal, err error) {
	rawReport, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("encoding report: %w", err)
	}
	rawSteps, err := json.Marshal(redactSteps(steps))
	if err != nil {
		return nil, fmt.Errorf("encoding steps: %w", err)
	}
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	var envID any
	if ds.EnvID != "" {
		envID = ds.EnvID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting proposal transaction: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, rbErr)
		}
	}()
	if _, err = tx.ExecContext(ctx,
		`UPDATE reconcile_proposals SET status = ?, note = ?, resolved_at = ?, updated_at = ?
		 WHERE desired_state_id = ? AND status = ?`,
		ProposalSuperseded, "replaced by a newer plan", now, now, ds.ID, ProposalPending); err != nil {
		return nil, fmt.Errorf("superseding earlier proposals: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO reconcile_proposals (id, desired_state_id, env_id, origin, status, fingerprint, report, steps,
		 created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, ds.ID, envID, origin, ProposalPending, Fingerprint(report), string(rawReport), string(rawSteps), now, now); err != nil {
		return nil, fmt.Errorf("inserting proposal: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing proposal: %w", err)
	}
	return s.Get(ctx, id)
}

// Get returns one proposal, or nil when it does not exist.
func (s *ProposalStore) Get(ctx context.Context, id string) (*Proposal, error) {
	p, err := scanProposal(s.db.QueryRowContext(ctx,
		`SELECT `+proposalColumns+` FROM reconcile_proposals p LEFT JOIN desired_states d ON d.id = p.desired_state_id
		 WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading proposal: %w", err)
	}
	return &p, nil
}

// PendingFor returns the pending proposal of a desired state, or nil.
func (s *ProposalStore) PendingFor(ctx context.Context, desiredStateID string) (*Proposal, error) {
	p, err := scanProposal(s.db.QueryRowContext(ctx,
		`SELECT `+proposalColumns+` FROM reconcile_proposals p LEFT JOIN desired_states d ON d.id = p.desired_state_id
		 WHERE p.desired_state_id = ? AND p.status = ? ORDER BY p.created_at DESC LIMIT 1`, desiredStateID, ProposalPending))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading pending proposal: %w", err)
	}
	return &p, nil
}

// List returns proposals newest first, filtered by status and environment.
func (s *ProposalStore) List(ctx context.Context, status, envID string, page, perPage int) ([]Proposal, int64, error) {
	where, args := " WHERE 1 = 1", []any{}
	if status != "" {
		where += " AND p.status = ?"
		args = append(args, status)
	}
	if envID != "" {
		where += " AND p.env_id = ?"
		args = append(args, envID)
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reconcile_proposals p`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting proposals: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+proposalColumns+` FROM reconcile_proposals p LEFT JOIN desired_states d ON d.id = p.desired_state_id`+ // SQL identifier: where is assembled from fixed literals; every value goes through ?
			where+` ORDER BY p.created_at DESC, p.id DESC LIMIT ? OFFSET ?`, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing proposals: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("reconciler: closing rows failed", "error", cerr)
		}
	}()
	out := []Proposal{}
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// Resolve moves a pending proposal to a final status. It fails with
// ErrProposalNotPending when someone else resolved it first, which is what
// keeps a double-click from applying a plan twice.
func (s *ProposalStore) Resolve(ctx context.Context, id, status, by, note string, results []StepResult, applyErr string) error {
	raw, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("encoding results: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE reconcile_proposals SET status = ?, resolved_by = ?, note = ?, result = ?, error = ?, resolved_at = ?,
		 updated_at = ? WHERE id = ? AND status = ?`,
		status, by, note, string(raw), applyErr, now, now, id, ProposalPending)
	if err != nil {
		return fmt.Errorf("resolving proposal: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrProposalNotPending
	}
	return nil
}

// Claim marks a pending proposal as being applied by moving it out of pending
// before any step runs; only the caller whose update lands may apply it.
func (s *ProposalStore) Claim(ctx context.Context, id, by string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE reconcile_proposals SET status = ?, resolved_by = ?, updated_at = ? WHERE id = ? AND status = ?`,
		ProposalApplying, by, now, id, ProposalPending)
	if err != nil {
		return fmt.Errorf("claiming proposal: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrProposalNotPending
	}
	return nil
}

// Finish records the outcome of a claimed proposal.
func (s *ProposalStore) Finish(ctx context.Context, id, status string, results []StepResult, applyErr string) error {
	raw, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("encoding results: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`UPDATE reconcile_proposals SET status = ?, result = ?, error = ?, resolved_at = ?, updated_at = ? WHERE id = ?`,
		status, string(raw), applyErr, now, now, id)
	if err != nil {
		return fmt.Errorf("finishing proposal: %w", err)
	}
	return nil
}

// SupersedePending closes every pending proposal of a desired state, used when
// its drift resolved on its own or the state was deleted.
func (s *ProposalStore) SupersedePending(ctx context.Context, desiredStateID, note string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE reconcile_proposals SET status = ?, note = ?, resolved_at = ?, updated_at = ?
		 WHERE desired_state_id = ? AND status = ?`,
		ProposalSuperseded, note, now, now, desiredStateID, ProposalPending)
	if err != nil {
		return fmt.Errorf("superseding proposals: %w", err)
	}
	return nil
}
