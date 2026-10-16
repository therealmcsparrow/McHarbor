// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/xid"
)

// FrameworkAccessReview is the attestation framework for a sign-off on the
// access review, which has no pack run behind it and so carries an empty run ID.
const FrameworkAccessReview = "access-review"

// Mapping-only frameworks an attestation may name. Their controls are described in
// the documentation rather than run, so a sign-off on them never names a run.
const (
	FrameworkSOC2     = "soc2"
	FrameworkISO27001 = "iso-27001"
)

// maxAttestationNote bounds the note so one request cannot store a document; a
// longer justification belongs in the system of record the note points to.
const maxAttestationNote = 4000

// maxAttestationScope bounds the scope, which is an environment ID or a short label.
const maxAttestationScope = 128

// Attestation errors, mapped to 400/409 by the handler.
var (
	// ErrAttestationFramework names a framework nothing can be signed off against.
	ErrAttestationFramework = errors.New("unknown attestation framework")
	// ErrAttestationNote is an empty or oversized note; a sign-off without a word
	// of justification is not evidence of a review.
	ErrAttestationNote = errors.New("attestation note is required")
	// ErrAttestationScope is an oversized scope.
	ErrAttestationScope = errors.New("attestation scope is too long")
	// ErrAttestationActor is a create without a user; an anonymous sign-off is worthless.
	ErrAttestationActor = errors.New("attestation actor is required")
	// ErrAttestationRun is a run ID that does not exist or belongs to another framework.
	ErrAttestationRun = errors.New("attestation run does not match the framework")
	// ErrAttestationRunOpen is a run that is still running, whose result is not yet a snapshot.
	ErrAttestationRunOpen = errors.New("attestation run has not finished")
)

// Attestation is one sign-off. There is no update and no delete: a newer
// attestation supersedes an older one, and the older one still reads back.
type Attestation struct {
	ID             string `json:"id"`
	Framework      string `json:"framework"`
	RunID          string `json:"runId"`
	Scope          string `json:"scope"`
	AttestedBy     string `json:"attestedBy"`
	AttestedByName string `json:"attestedByName"`
	AttestedAt     string `json:"attestedAt"`
	Note           string `json:"note"`
}

// AttestationInput is what a caller asks to record; the actor is never taken
// from the body, only from the authenticated request.
type AttestationInput struct {
	Framework string `json:"framework"`
	RunID     string `json:"runId"`
	Scope     string `json:"scope"`
	Note      string `json:"note"`
}

// AttestableFrameworks returns every framework key an attestation may name: each
// pack, the access review, and the mapping-only frameworks.
func AttestableFrameworks() []string {
	out := make([]string, 0, len(Frameworks())+3)
	for _, f := range Frameworks() {
		out = append(out, f.Key)
	}
	return append(out, FrameworkAccessReview, FrameworkSOC2, FrameworkISO27001)
}

func attestable(framework string) bool {
	for _, k := range AttestableFrameworks() {
		if k == framework {
			return true
		}
	}
	return false
}

// Attest validates and records a sign-off by actor. A named run must exist, belong
// to the same framework and be finished, so the attestation points at a fixed snapshot.
func (s *Service) Attest(ctx context.Context, actor string, in AttestationInput) (Attestation, error) {
	in.Framework = strings.TrimSpace(in.Framework)
	in.RunID = strings.TrimSpace(in.RunID)
	in.Scope = strings.TrimSpace(in.Scope)
	in.Note = strings.TrimSpace(in.Note)
	switch {
	case strings.TrimSpace(actor) == "":
		return Attestation{}, ErrAttestationActor
	case !attestable(in.Framework):
		return Attestation{}, ErrAttestationFramework
	case in.Note == "" || utf8.RuneCountInString(in.Note) > maxAttestationNote:
		return Attestation{}, ErrAttestationNote
	case utf8.RuneCountInString(in.Scope) > maxAttestationScope:
		return Attestation{}, ErrAttestationScope
	}
	if in.RunID != "" {
		var framework, status string
		err := s.db.QueryRowContext(ctx,
			`SELECT framework, status FROM compliance_pack_runs WHERE id = ?`, in.RunID).Scan(&framework, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return Attestation{}, ErrAttestationRun
		}
		if err != nil {
			return Attestation{}, fmt.Errorf("read attested run: %w", err)
		}
		if framework != in.Framework {
			return Attestation{}, ErrAttestationRun
		}
		if status == RunRunning {
			return Attestation{}, ErrAttestationRunOpen
		}
	}
	stamp := s.now().UTC().Format(time.RFC3339)
	id := xid.New().String()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO compliance_attestations
		 (id, framework, run_id, scope, attested_by, attested_at, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Framework, in.RunID, in.Scope, actor, stamp, in.Note, stamp, stamp); err != nil {
		return Attestation{}, fmt.Errorf("insert compliance attestation: %w", err)
	}
	return s.attestation(ctx, id)
}

func (s *Service) attestation(ctx context.Context, id string) (Attestation, error) {
	row := s.db.QueryRowContext(ctx, attestationSelect+` WHERE a.id = ?`, id)
	return scanAttestation(row)
}

// attestationSelect joins the username so a list reads as people rather than
// xids; a deleted user still reads back by ID because the join is outer.
const attestationSelect = `SELECT a.id, a.framework, a.run_id, a.scope, a.attested_by,
	COALESCE(u.username, ''), a.attested_at, a.note
	FROM compliance_attestations a LEFT JOIN users u ON u.id = a.attested_by`

// ListAttestations returns sign-offs newest first, optionally narrowed to one
// framework, with the total for pagination.
func (s *Service) ListAttestations(ctx context.Context, framework string, limit, offset int) ([]Attestation, int, error) {
	if limit <= 0 || limit > maxRunPage {
		limit = 25
	}
	where, args := "", []any{}
	if framework != "" {
		where, args = " WHERE a.framework = ?", []any{framework}
	}
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM compliance_attestations a`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count compliance attestations: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		attestationSelect+where+` ORDER BY a.attested_at DESC, a.id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("select compliance attestations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("compliance packs: closing rows failed", "error", err)
		}
	}()
	out := []Attestation{}
	for rows.Next() {
		a, err := scanAttestation(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan compliance attestations: %w", err)
	}
	return out, total, nil
}

func scanAttestation(row scanner) (Attestation, error) {
	var a Attestation
	if err := row.Scan(&a.ID, &a.Framework, &a.RunID, &a.Scope, &a.AttestedBy, &a.AttestedByName,
		&a.AttestedAt, &a.Note); err != nil {
		return Attestation{}, fmt.Errorf("scan compliance attestation: %w", err)
	}
	return a, nil
}
