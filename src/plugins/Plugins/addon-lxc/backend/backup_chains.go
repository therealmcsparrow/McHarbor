// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
)

// listingPrefix names every incremental listing McHarbor creates.
//
// Prefixed so a listing an operator made by hand is never removed or relied on
// by this code: what McHarbor did not create, it does not manage.
const listingPrefix = "mcharbor-ct-"

// ErrChainIncomplete means a backup's chain is missing a link it needs.
var ErrChainIncomplete = errors.New("this backup's chain is incomplete")

// chainFor returns a container's chain, creating one when it has none.
func (s *Service) chainFor(ctx context.Context, containerID, planID string) (BackupChain, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupChainColumns+`
		  FROM plugin_addon_lxc_backup_chains WHERE container_id = ?`, containerID)
	if err != nil {
		return BackupChain{}, fmt.Errorf("reading the backup chain: %w", err)
	}
	found := rows.Next()
	var chain BackupChain
	if found {
		chain, err = scanBackupChain(rows)
	}
	_ = rows.Close()
	if err != nil {
		return BackupChain{}, err
	}
	if found {
		return chain, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	chain = BackupChain{
		ID:          xid.New().String(),
		ContainerID: containerID,
		PlanID:      planID,
	}
	// Derived from the chain id, so two chains on one container can never
	// collide and a listing always names the chain it belongs to.
	chain.ListingName = listingPrefix + chain.ID
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_chains
		       (id, container_id, plan_id, listing_name, invalidated, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		chain.ID, chain.ContainerID, nullIfEmpty(planID), chain.ListingName,
		now, now); err != nil {
		return BackupChain{}, fmt.Errorf("starting a backup chain: %w", err)
	}
	return chain, nil
}

// InvalidateChain marks a container's chain unusable, with the reason.
//
// Called from every path that breaks the relationship between the listing and
// the tree under it: a snapshot revert, a restore, a clone over the rootfs. An
// incremental taken after any of those restores a tree missing everything
// changed since, so the chain is broken here rather than hoping the agent
// notices.
func (s *Service) InvalidateChain(ctx context.Context, containerID, why string) error {
	if strings.TrimSpace(containerID) == "" {
		return nil
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_backup_chains
		   SET invalidated = 1, invalid_why = ?, updated_at = ?
		 WHERE container_id = ? AND invalidated = 0`,
		why, time.Now().UTC().Format(time.RFC3339), containerID); err != nil {
		return fmt.Errorf("invalidating the backup chain: %w", err)
	}
	return nil
}

// restartChain replaces an invalidated chain with a fresh one headed by a full
// copy that was just taken.
//
// A new row rather than clearing the flag, so the old chain's runs stay
// restorable among themselves. The listing name carries over: the agent wrote a
// level-0 listing under it for the full copy that heads the new chain, and a new
// name would find no listing and force a second full copy for nothing.
func (s *Service) restartChain(ctx context.Context, chain BackupChain) (BackupChain, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	fresh := BackupChain{
		ID:          xid.New().String(),
		ContainerID: chain.ContainerID,
		PlanID:      chain.PlanID,
		ListingName: chain.ListingName,
	}
	if fresh.ListingName == "" {
		fresh.ListingName = listingPrefix + fresh.ID
	}

	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_lxc_backup_chains WHERE id = ?`, chain.ID); err != nil {
		return BackupChain{}, fmt.Errorf("retiring the old backup chain: %w", err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_chains
		       (id, container_id, plan_id, listing_name, invalidated, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		fresh.ID, fresh.ContainerID, nullIfEmpty(fresh.PlanID), fresh.ListingName,
		now, now); err != nil {
		return BackupChain{}, fmt.Errorf("starting a backup chain: %w", err)
	}
	return fresh, nil
}

// decideRunKind settles full or incremental before a run starts.
//
// Conservative by construction: every answer this cannot prove is safe comes
// back full. The agent applies the same test again against the listing on disk
// and may downgrade further — that second check is the authoritative one,
// because only the agent can see the file.
func (s *Service) decideRunKind(ctx context.Context, plan BackupPlan, chain BackupChain, requested string) (string, string) {
	if requested == BackupKindFull {
		return BackupKindFull, "a full copy was asked for"
	}
	if chain.Invalidated {
		reason := chain.InvalidWhy
		if reason == "" {
			reason = "this container's backup chain was marked unusable"
		}
		return BackupKindFull, reason
	}
	if chain.LastRunID == "" || chain.LastFullAt == nil {
		return BackupKindFull, "this is the first backup of this container"
	}

	if plan.FullEvery > 0 {
		since, err := s.runsSinceFull(ctx, chain.ID)
		if err != nil {
			// A chain whose history cannot be read is a chain whose age is
			// unknown, and the safe reading of unknown is "due".
			return BackupKindFull, "the host could not read this chain's history"
		}
		if since >= plan.FullEvery {
			return BackupKindFull, fmt.Sprintf("a full copy is taken every %d runs", plan.FullEvery)
		}
	}

	lastOK, err := s.lastRunSucceeded(ctx, chain.ID)
	if err != nil || !lastOK {
		// Building on top of a run that failed means building on a delta whose
		// base may never have reached its destination.
		return BackupKindFull, "the previous backup of this container did not finish"
	}
	return BackupKindIncremental, ""
}

// runsSinceFull counts successful runs since this chain's last full copy.
func (s *Service) runsSinceFull(ctx context.Context, chainID string) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT COUNT(*) FROM plugin_addon_lxc_backup_runs
		 WHERE chain_id = ? AND operation = 'backup' AND status = 'success'
		   AND started_at > COALESCE(
		       (SELECT MAX(started_at) FROM plugin_addon_lxc_backup_runs
		         WHERE chain_id = ? AND operation = 'backup'
		           AND status = 'success' AND kind = 'full'), '')`,
		chainID, chainID)
	if err != nil {
		return 0, fmt.Errorf("counting runs since the last full backup: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return 0, rows.Err()
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, fmt.Errorf("counting runs since the last full backup: %w", err)
	}
	return count, rows.Err()
}

// lastRunSucceeded reports whether this chain's most recent backup finished.
func (s *Service) lastRunSucceeded(ctx context.Context, chainID string) (bool, error) {
	rows, err := s.db.Query(ctx, `
		SELECT status FROM plugin_addon_lxc_backup_runs
		 WHERE chain_id = ? AND operation = 'backup'
		 ORDER BY started_at DESC LIMIT 1`, chainID)
	if err != nil {
		return false, fmt.Errorf("reading the chain's last run: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return false, rows.Err()
	}
	var status string
	if err := rows.Scan(&status); err != nil {
		return false, fmt.Errorf("reading the chain's last run: %w", err)
	}
	return status == BackupStatusSuccess, rows.Err()
}

// recordChainRun advances a chain after a run, honouring the agent's verdict.
//
// This is the point the whole incremental design turns on. If the agent reported
// that it fell back to a full copy, the chain is restarted and the run is
// recorded as full — the database never claims an incremental that was not
// taken, because a restore would then apply a delta on top of a base that does
// not contain what it assumes.
func (s *Service) recordChainRun(
	ctx context.Context,
	chain BackupChain,
	runID, takenKind string,
	fellBack bool,
	reason string,
) (BackupChain, error) {
	// A full copy taken because the chain could not continue — the agent fell
	// back, or the chain was already invalidated — heads a new chain. The run is
	// moved onto it, so the next incremental's ancestry starts at this full copy
	// rather than walking back into history the listing no longer describes.
	if fellBack || (takenKind == BackupKindFull && chain.Invalidated) {
		if fellBack {
			if err := s.InvalidateChain(ctx, chain.ContainerID, reason); err != nil {
				return chain, err
			}
		}
		refreshed, err := s.restartChain(ctx, chain)
		if err != nil {
			return chain, err
		}
		chain = refreshed
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_lxc_backup_runs SET chain_id = ?, updated_at = ? WHERE id = ?`,
			chain.ID, time.Now().UTC().Format(time.RFC3339), runID); err != nil {
			return chain, fmt.Errorf("moving the run onto its new backup chain: %w", err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		UPDATE plugin_addon_lxc_backup_chains
		   SET last_run_id = ?, last_run_at = ?, updated_at = ?
		 WHERE id = ?`
	args := []any{runID, now, now, chain.ID}
	if takenKind == BackupKindFull {
		query = `
			UPDATE plugin_addon_lxc_backup_chains
			   SET last_run_id = ?, last_run_at = ?, last_full_at = ?, updated_at = ?
			 WHERE id = ?`
		args = []any{runID, now, now, now, chain.ID}
	}
	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		return chain, fmt.Errorf("advancing the backup chain: %w", err)
	}
	return chain, nil
}

// chainAncestors returns a run's chain back to its full copy, oldest first.
//
// A restore needs every link. A gap — a run pruned, a run that failed — means
// the delta on top of it cannot be applied, so the caller refuses rather than
// rebuilding a tree that looks complete.
func (s *Service) chainAncestors(ctx context.Context, run BackupRun) ([]BackupRun, error) {
	if run.Kind == BackupKindFull {
		return []BackupRun{run}, nil
	}
	if run.ChainID == "" {
		return nil, fmt.Errorf("%w: this backup records no chain, so what it builds on is unknown",
			ErrChainIncomplete)
	}

	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_lxc_backup_runs
		 WHERE chain_id = ? AND operation = 'backup' AND status = 'success'
		   AND started_at <= ?
		 ORDER BY started_at LIMIT ?`,
		run.ChainID, run.StartedAt.UTC().Format(time.RFC3339), maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading the backup chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var all []BackupRun
	for rows.Next() {
		entry, err := scanBackupRun(rows)
		if err != nil {
			return nil, err
		}
		all = append(all, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the backup chain: %w", err)
	}

	// Walk back to the most recent full copy and return from there. Anything
	// before it is a different baseline and must not be applied.
	start := -1
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Kind == BackupKindFull {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("%w: the full copy this backup builds on is no longer available",
			ErrChainIncomplete)
	}
	return all[start:], nil
}

// ErrChainUnrestorable means a chain's links exist but cannot be applied, so a
// restore of it would fail half way through; the message names the full copy
// that can be restored instead.
var ErrChainUnrestorable = errors.New("this backup cannot be restored")

// checkChainRestorable refuses a chain whose incrementals were read from a dir
// pool's tree copy, before anything is downloaded or written.
//
// Such a copy gets fresh inode numbers on every run and GNU tar matches
// directories by inode, so the incremental records renames that never happened
// and fails replaying them. Agents from 2026.09.11 take those runs as full
// copies; this catches the chains stored before that. poolBackend is the source
// container's pool backend when it is still known, "" otherwise: a live copy is
// recognised either way, an operator-frozen dir copy (recorded as a snapshot)
// only when the backend is known.
func checkChainRestorable(chain []BackupRun, poolBackend string) error {
	if len(chain) < 2 {
		return nil
	}
	for _, link := range chain {
		copied := link.Consistency == BackupConsistencyLiveCopy ||
			(poolBackend == "dir" && link.Consistency == BackupConsistencySnapshot)
		if !copied {
			continue
		}
		full := chain[0]
		return fmt.Errorf("%w: its incremental backups were read from a live copy of a dir-pool rootfs, "+
			"whose inode numbers change on every run, so GNU tar cannot replay them. Nothing was changed. "+
			"Restore the full backup taken at %s (run %s) instead",
			ErrChainUnrestorable, full.StartedAt.UTC().Format(time.RFC3339), full.ID)
	}
	return nil
}

// restorableChain resolves a run's chain and refuses one that cannot be applied,
// so both a restore and a recreate stop before a byte moves.
func (s *Service) restorableChain(ctx context.Context, source BackupRun) ([]BackupRun, error) {
	chain, err := s.chainAncestors(ctx, source)
	if err != nil {
		return nil, err
	}
	backend := ""
	if origin, err := s.Get(ctx, source.ContainerID); err == nil {
		backend = origin.PoolBackend
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err := checkChainRestorable(chain, backend); err != nil {
		return nil, err
	}
	return chain, nil
}

// ChainForContainer returns a container's chain, or a zero chain when it has
// none. Read-only, for the protection report and the detail panel.
func (s *Service) ChainForContainer(ctx context.Context, containerID string) (BackupChain, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupChainColumns+`
		  FROM plugin_addon_lxc_backup_chains WHERE container_id = ?`, containerID)
	if err != nil {
		return BackupChain{}, fmt.Errorf("reading the backup chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return BackupChain{}, rows.Err()
	}
	return scanBackupChain(rows)
}
