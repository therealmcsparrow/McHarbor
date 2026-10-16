// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
)

// bitmapPrefix names every dirty bitmap McHarbor creates.
//
// Prefixed so a bitmap an operator made by hand is never removed or relied on
// by this code: what McHarbor did not create, it does not manage.
const bitmapPrefix = "mcharbor-"

// chainColumns is the select list every chain read shares.
const chainColumns = `
	id, vm_id, disk_id, COALESCE(plan_id, ''), bitmap_name,
	COALESCE(granularity, 0), COALESCE(last_run_id, ''),
	COALESCE(last_full_at, ''), COALESCE(last_run_at, ''),
	invalidated, COALESCE(invalid_why, '')`

// chainFor returns a disk's chain, creating one when it has none.
func (s *Service) chainFor(ctx context.Context, vmID, diskID, planID string) (BackupChain, error) {
	rows, err := s.db.Query(ctx, `SELECT `+chainColumns+`
		  FROM plugin_addon_kvm_backup_chains WHERE disk_id = ?`, diskID)
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
		ID:     xid.New().String(),
		VMID:   vmID,
		DiskID: diskID,
		PlanID: planID,
	}
	// Derived from the chain id, so two chains on one disk can never collide
	// and a bitmap always names the chain it belongs to.
	chain.BitmapName = bitmapPrefix + chain.ID
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_backup_chains
		       (id, vm_id, disk_id, bitmap_name, plan_id, invalidated, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		chain.ID, chain.VMID, chain.DiskID, chain.BitmapName,
		nullIfEmpty(planID), now, now); err != nil {
		return BackupChain{}, fmt.Errorf("starting a backup chain: %w", err)
	}
	return chain, nil
}

// InvalidateChain marks a disk's chain unusable, with the reason.
//
// Called from every path that breaks the relationship between a bitmap and
// the disk under it: a resize, a snapshot revert, a restore, a disk removal.
// An incremental taken after any of those restores to a disk that looks
// intact and is not, so the chain is broken here rather than hoping the agent
// notices.
func (s *Service) InvalidateChain(ctx context.Context, diskID, why string) error {
	if strings.TrimSpace(diskID) == "" {
		return nil
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_chains
		   SET invalidated = 1, invalid_why = ?, updated_at = ?
		 WHERE disk_id = ? AND invalidated = 0`,
		why, time.Now().UTC().Format(time.RFC3339), diskID); err != nil {
		return fmt.Errorf("invalidating the backup chain: %w", err)
	}
	return nil
}

// InvalidateChainsForVM breaks every chain on a machine.
//
// Used where the change is not attributable to one disk — a revert to a
// snapshot taken before the chain existed, or a restore of the whole machine.
func (s *Service) InvalidateChainsForVM(ctx context.Context, vmID, why string) error {
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_chains
		   SET invalidated = 1, invalid_why = ?, updated_at = ?
		 WHERE vm_id = ? AND invalidated = 0`,
		why, time.Now().UTC().Format(time.RFC3339), vmID); err != nil {
		return fmt.Errorf("invalidating the machine's backup chains: %w", err)
	}
	return nil
}

// restartChain replaces an invalidated chain with a fresh one.
//
// A new row and a new bitmap name rather than clearing the flag: the old
// chain's runs stay restorable among themselves, and the new bitmap cannot
// be confused with the one whose history is no longer trustworthy.
func (s *Service) restartChain(ctx context.Context, chain BackupChain) (BackupChain, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	fresh := BackupChain{
		ID:     xid.New().String(),
		VMID:   chain.VMID,
		DiskID: chain.DiskID,
		PlanID: chain.PlanID,
	}
	fresh.BitmapName = bitmapPrefix + fresh.ID

	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_backup_chains WHERE id = ?`, chain.ID); err != nil {
		return BackupChain{}, fmt.Errorf("retiring the old backup chain: %w", err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_backup_chains
		       (id, vm_id, disk_id, bitmap_name, plan_id, invalidated, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		fresh.ID, fresh.VMID, fresh.DiskID, fresh.BitmapName,
		nullIfEmpty(fresh.PlanID), now, now); err != nil {
		return BackupChain{}, fmt.Errorf("starting a backup chain: %w", err)
	}
	return fresh, nil
}

// decideRunKind settles full or incremental before a run starts.
//
// Conservative by construction: every answer this cannot prove is safe comes
// back full. The agent applies the same test again against the live bitmap
// and may downgrade further — that second check is the authoritative one,
// because only the agent can see the bitmap.
func (s *Service) decideRunKind(ctx context.Context, plan BackupPlan, chain BackupChain, requested string) (string, string) {
	if requested == BackupKindFull {
		return BackupKindFull, "a full copy was asked for"
	}
	if chain.Invalidated {
		reason := chain.InvalidWhy
		if reason == "" {
			reason = "this disk's backup chain was marked unusable"
		}
		return BackupKindFull, reason
	}
	if chain.LastRunID == "" || chain.LastFullAt == nil {
		return BackupKindFull, "this is the first backup of this disk"
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
		// Building on top of a run that failed means building on a delta
		// whose base may never have reached its destination.
		return BackupKindFull, "the previous backup of this disk did not finish"
	}
	return BackupKindIncremental, ""
}

// runsSinceFull counts successful runs since this chain's last full copy.
func (s *Service) runsSinceFull(ctx context.Context, chainID string) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT COUNT(*) FROM plugin_addon_kvm_backup_runs
		 WHERE chain_id = ? AND operation = 'backup' AND status = 'success'
		   AND started_at > COALESCE(
		       (SELECT MAX(started_at) FROM plugin_addon_kvm_backup_runs
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
		SELECT status FROM plugin_addon_kvm_backup_runs
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
// This is the point the whole incremental design turns on. If the agent
// reported that it fell back to a full copy, the chain is restarted and the
// run is recorded as full — the database never claims an incremental that was
// not taken, because a restore would then apply a delta on top of a base that
// does not contain what it assumes.
func (s *Service) recordChainRun(
	ctx context.Context,
	chain BackupChain,
	runID string,
	takenKind string,
	fellBack bool,
	reason string,
) (BackupChain, error) {
	if fellBack {
		if err := s.InvalidateChain(ctx, chain.DiskID, reason); err != nil {
			return chain, err
		}
		refreshed, err := s.restartChain(ctx, chain)
		if err != nil {
			return chain, err
		}
		chain = refreshed
	}

	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		UPDATE plugin_addon_kvm_backup_chains
		   SET last_run_id = ?, last_run_at = ?, updated_at = ?
		 WHERE id = ?`
	args := []any{runID, now, now, chain.ID}
	if takenKind == BackupKindFull {
		query = `
			UPDATE plugin_addon_kvm_backup_chains
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
// rebuilding a disk that looks complete.
func (s *Service) chainAncestors(ctx context.Context, run BackupRun) ([]BackupRun, error) {
	if run.Kind == BackupKindFull {
		return []BackupRun{run}, nil
	}
	if run.ChainID == "" {
		return nil, fmt.Errorf("%w: this backup records no chain, so what it builds on is unknown",
			ErrChainIncomplete)
	}

	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_kvm_backup_runs
		 WHERE chain_id = ? AND operation = 'backup' AND status = 'success'
		   AND started_at <= ?
		 ORDER BY started_at LIMIT 1000`,
		run.ChainID, run.StartedAt.Format(time.RFC3339))
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

// chainsForVM lists every backup chain on a machine, one per disk.
func (s *Service) chainsForVM(ctx context.Context, vmID string) ([]BackupChain, error) {
	rows, err := s.db.Query(ctx, `SELECT `+chainColumns+`
		  FROM plugin_addon_kvm_backup_chains
		 WHERE vm_id = ? ORDER BY disk_id LIMIT ?`, vmID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing backup chains: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BackupChain
	for rows.Next() {
		chain, err := scanBackupChain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, chain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing backup chains: %w", err)
	}
	return out, nil
}
