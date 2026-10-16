// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// reapBatch bounds one tick's work so a backlog of previews cannot hold the
// leader's scheduler goroutine for minutes.
const reapBatch = 50

// ReapPreviews finishes every teardown that is owed, and is the one place a
// preview is destroyed without a pull-request event. It runs from the existing
// leader-gated GitOps scheduler tick, so there is no second singleton role and
// no second goroutine.
//
// Everything it acts on is derived from a persisted column, because the
// scheduler keeps no state across a restart. Four survival cases, in the order
// they are handled, followed by the forge probe (probe.go):
//
//  1. a teardown interrupted mid-flight ('demolishing') is finished;
//  2. a preview past its persisted expiry is destroyed;
//  3. a preview whose creation half-failed — no environment past the grace
//     window, or a recorded failure — is cleaned up;
//  4. a preview whose repository has been deleted is destroyed, because
//     "I cannot find it" means destroy it, not raise an error.
//
// An ephemeral environment whose preview row has been cascade-deleted with its
// pipeline is reaped by its own stamped expiry, through the environments
// service, so an orphan has an owner even when nothing points at it.
func (s *Service) ReapPreviews(ctx context.Context) error {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// 1. Interrupted teardowns. Finishing one is the same idempotent call that
	//    started it, which is the whole point of the 'demolishing' state.
	note(s.reapWhere(ctx, "interrupted-teardown", previewDemolished,
		`SELECT id FROM gitops_pr_previews WHERE status = ? ORDER BY updated_at ASC LIMIT ?`,
		previewDemolishing, reapBatch))

	// 2. Expiry.
	note(s.reapWhere(ctx, "expired", previewExpired,
		`SELECT id FROM gitops_pr_previews
		 WHERE status = ? AND expires_at IS NOT NULL AND expires_at != '' AND expires_at <= ?
		 ORDER BY expires_at ASC LIMIT ?`,
		previewActive, nowStr, reapBatch))

	// 3. Creation that half-failed. The row is written before the environment
	//    exists, so this state is reachable by a crash in between.
	grace := now.Add(-previewCreationGrace).Format(time.RFC3339)
	note(s.reapWhere(ctx, "creation-incomplete", previewFailed,
		`SELECT id FROM gitops_pr_previews
		 WHERE status = ? AND (preview_environment_id IS NULL OR preview_environment_id = '')
		   AND opened_at <= ?
		 ORDER BY opened_at ASC LIMIT ?`,
		previewActive, grace, reapBatch))
	note(s.reapWhere(ctx, "failed-provisioning", previewFailed,
		`SELECT id FROM gitops_pr_previews
		 WHERE status = ? AND preview_environment_id IS NOT NULL AND preview_environment_id != ''
		   AND updated_at <= ?
		 ORDER BY updated_at ASC LIMIT ?`,
		previewFailed, grace, reapBatch))

	// 4. Repository gone. The preview cannot be reconciled against anything any
	//    more, so it is destroyed rather than kept for a forge that no longer
	//    knows about it.
	note(s.reapWhere(ctx, "repository-deleted", previewDemolished,
		`SELECT p.id FROM gitops_pr_previews p
		 LEFT JOIN git_repos r ON r.id = p.repo_id
		 WHERE p.status IN (?, ?) AND r.id IS NULL
		 ORDER BY p.opened_at ASC LIMIT ?`,
		previewActive, previewDemolishing, reapBatch))

	if prov, ok := s.environments(); ok {
		reaped, err := prov.ReapExpiredEphemeral(ctx)
		if err != nil {
			note(fmt.Errorf("gitops: reaping orphaned preview environments: %w", err))
		} else if reaped > 0 {
			slog.Info("gitops: reaped orphaned preview environments", "count", reaped)
		}
	}

	// 5. A pull request closed or deleted while its webhook was missed. The
	//    probe destroys only on a definite answer from an authenticated request,
	//    so a forge outage or an expired token can never delete a live preview.
	note(s.probePreviews(ctx, now))

	// A promotion whose runner died with the process would read 'pending'
	// forever; it is recorded as interrupted instead.
	note(s.failInterruptedPromotions(ctx, now))
	return firstErr
}

// reapWhere tears down every preview the query selects. One failure is logged
// and the sweep continues, because a wedged environment must not stop the rest
// of the backlog from being cleared.
func (s *Service) reapWhere(ctx context.Context, reason, finalStatus, query string, args ...any) error {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("gitops: selecting previews to reap (%s): %w", reason, err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("gitops: closing rows failed", "error", cerr)
		}
	}()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("gitops: scanning preview to reap (%s): %w", reason, err)
		}
		ids = append(ids, id)
	}
	closeErr := rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("gitops: reading previews to reap (%s): %w", reason, err)
	}
	if closeErr != nil {
		return fmt.Errorf("gitops: closing preview reap cursor (%s): %w", reason, closeErr)
	}

	var firstErr error
	for _, id := range ids {
		if err := s.TeardownPreview(ctx, id, reason, "", finalStatus); err != nil {
			slog.Warn("gitops: reaping preview failed", "preview", id, "reason", reason, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		slog.Info("gitops: preview reaped", "preview", id, "reason", reason)
	}
	return firstErr
}
