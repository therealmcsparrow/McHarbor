// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"
)

const msgVMBackupRestore = "vm_backup_restore"

// ErrRestoreNotConfirmed means the caller did not say they understand a
// restore replaces the machine's current disks.
var ErrRestoreNotConfirmed = errors.New("a restore has to be confirmed")

// StartRestore rebuilds a machine's disks from one backup.
//
// Every refusal below is checked before anything is downloaded, because the
// expensive half of a restore is the transfer and a restore that fails at the
// end has already overwritten nothing but wasted an hour.
func (s *Service) StartRestore(ctx context.Context, sourceRunID string, req RestoreBackupInput) (BackupRun, error) {
	if !req.Confirm {
		return BackupRun{}, ErrRestoreNotConfirmed
	}

	source, err := s.BackupRunByID(ctx, sourceRunID)
	if err != nil {
		return BackupRun{}, err
	}
	if source.Operation != BackupOpBackup || source.Status != BackupStatusSuccess {
		return BackupRun{}, fmt.Errorf("%w: only a backup that finished successfully can be restored",
			ErrBackupNotRestorable)
	}

	vm, err := s.Get(ctx, source.VMID)
	if err != nil {
		return BackupRun{}, err
	}
	// A restore writes the machine's disks out from under it. QEMU has them
	// open while it runs, so this is a refusal rather than a warning.
	if vm.State != StateStopped {
		return BackupRun{}, ErrRunning
	}

	// The whole chain has to be present before a single byte moves. A gap
	// means the delta cannot be applied, and applying what is there anyway
	// produces a disk that looks intact and is not.
	chain, err := s.chainAncestors(ctx, source)
	if err != nil {
		return BackupRun{}, err
	}
	artifacts, err := s.resolveChainArtifacts(ctx, chain, req.DiskIDs)
	if err != nil {
		return BackupRun{}, err
	}

	run, err := s.createRestoreRun(ctx, vm, source, req.DiskIDs)
	if err != nil {
		return BackupRun{}, err
	}

	s.logBackupEvent(ctx, run.ID, "started", "starting", "info",
		"started from the backup taken at "+source.StartedAt.Format(time.RFC3339)+".")

	go s.executeRestore(context.WithoutCancel(ctx), vm, run, artifacts, req.DiskIDs)
	return run, nil
}

// resolveChainArtifacts turns a chain into the ordered list the agent applies.
//
// Ordering is explicit rather than implied: applying an incremental before the
// full it builds on, or skipping a link, produces a plausible-looking disk
// with someone else's data in it.
func (s *Service) resolveChainArtifacts(ctx context.Context, chain []BackupRun, diskIDs []string) ([]agentRestoreArtifact, error) {
	wanted := map[string]bool{}
	for _, id := range diskIDs {
		wanted[id] = true
	}

	var out []agentRestoreArtifact
	for sequence, run := range chain {
		destinations, err := s.runDestinations(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		for _, dest := range destinations {
			if dest.Status != DestinationSuccess || dest.RemotePath == "" {
				continue
			}
			if len(wanted) > 0 && !wanted[dest.DiskID] {
				continue
			}
			grantID, token, err := s.backupGrants.issue(run.ID, run.VMID)
			if err != nil {
				return nil, err
			}
			s.registerDownload(grantID, backupDownload{
				locationID: dest.LocationID,
				remotePath: dest.RemotePath,
			})
			out = append(out, agentRestoreArtifact{
				DiskID:    dest.DiskID,
				Kind:      run.Kind,
				URL:       backupDownloadPath + grantID,
				Token:     token,
				Sequence:  sequence,
				SizeBytes: dest.BytesUploaded,
			})
			// One copy per disk per link is enough; the others are the same
			// bytes at a second destination.
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: none of this backup's objects are still at a destination",
			ErrChainIncomplete)
	}
	return out, nil
}

// backupDownloadPath is where an agent pulls a chain's artifacts from.
const backupDownloadPath = "/api/vms/backups/internal/download/"

// backupDownload is one artifact an agent may pull.
type backupDownload struct {
	locationID string
	remotePath string
}

// registerDownload records what a download grant authorises.
func (s *Service) registerDownload(grantID string, download backupDownload) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	if s.backupDownloads == nil {
		s.backupDownloads = map[string]backupDownload{}
	}
	s.backupDownloads[grantID] = download
}

// downloadFor returns what a grant authorises.
func (s *Service) downloadFor(grantID string) (backupDownload, bool) {
	s.backupMu.RLock()
	defer s.backupMu.RUnlock()
	download, ok := s.backupDownloads[grantID]
	return download, ok
}

// executeRestore drives one restore to completion in the background.
func (s *Service) executeRestore(
	ctx context.Context,
	vm VM,
	run BackupRun,
	artifacts []agentRestoreArtifact,
	diskIDs []string,
) {
	ctx, cancel := context.WithTimeout(ctx, backupGrantTTL)
	defer cancel()

	spec := &agentRestoreSpec{RunID: run.ID, Artifacts: artifacts}
	wanted := map[string]bool{}
	for _, id := range diskIDs {
		wanted[id] = true
	}
	for _, disk := range backupableDisks(vm) {
		if len(wanted) > 0 && !wanted[disk.ID] {
			continue
		}
		spec.Disks = append(spec.Disks, agentRestoreDisk{
			DiskID: disk.ID, VolumePath: disk.VolumePath,
		})
	}

	s.updateBackupProgress(ctx, run.ID, "restoring", "")
	if _, err := s.call(ctx, vm.EnvironmentID, msgVMBackupRestore, agentPayload{
		VMID: vm.ID, Restore: spec,
	}); err != nil {
		s.failBackupRun(ctx, run.ID, err)
		return
	}

	// Every restored disk's chain is finished: the disk's contents no longer
	// match what its bitmap has been tracking, so the next backup must be a
	// full copy. Skipping this is how a restore quietly poisons every
	// incremental taken after it.
	for _, disk := range spec.Disks {
		if err := s.InvalidateChain(ctx, disk.DiskID,
			"this disk was restored from a backup, so its change log no longer matches its contents"); err != nil {
			s.logger.Warn("kvm: invalidating a chain after restore",
				"disk", disk.DiskID, "error", err)
		}
	}

	s.finishBackupRun(ctx, run.ID, run.Kind, false, "", run.Consistency, 0)
}

// createRestoreRun records a restore as a run of its own.
//
// Same table as backups, keyed by operation, so history, progress and polling
// are one implementation — the arrangement the container module uses.
func (s *Service) createRestoreRun(ctx context.Context, vm VM, source BackupRun, diskIDs []string) (BackupRun, error) {
	now := time.Now().UTC()
	run := BackupRun{
		ID:            xid.New().String(),
		VMID:          vm.ID,
		EnvironmentID: vm.EnvironmentID,
		Operation:     BackupOpRestore,
		SourceRunID:   source.ID,
		Status:        BackupStatusRunning,
		Kind:          source.Kind,
		ChainID:       source.ChainID,
		Consistency:   source.Consistency,
		IncludedDisks: diskIDs,
		StartedAt:     now,
	}
	if len(run.IncludedDisks) == 0 {
		run.IncludedDisks = source.IncludedDisks
	}

	stamp := now.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_backup_runs
		       (id, plan_id, vm_id, environment_id, operation, source_run_id,
		        status, kind, chain_id, consistency, included_disks,
		        progress_stage, progress_message, started_at, created_at, updated_at)
		VALUES (?, NULL, ?, ?, 'restore', ?, 'running', ?, ?, ?, ?, 'starting', '', ?, ?, ?)`,
		run.ID, run.VMID, run.EnvironmentID, run.SourceRunID, run.Kind,
		nullIfEmpty(run.ChainID), run.Consistency, encodeDiskList(run.IncludedDisks),
		stamp, stamp, stamp); err != nil {
		return BackupRun{}, fmt.Errorf("recording the restore: %w", err)
	}
	return run, nil
}
