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

// ErrRestoreNotConfirmed means the caller did not type the container's name.
//
// A restore replaces a whole filesystem and cannot be undone, so the consent is
// the name rather than a tick box: a tick box in a table row is one mis-click.
var ErrRestoreNotConfirmed = errors.New("a restore has to be confirmed by typing the container's name")

// StartRestore rebuilds a container's rootfs from one backup.
//
// Every refusal below is checked before anything is downloaded, because the
// expensive half of a restore is the transfer and a restore that fails at the
// end has already cost the operator the container's downtime.
func (s *Service) StartRestore(ctx context.Context, sourceRunID string, req RestoreBackupInput) (BackupRun, error) {
	source, err := s.BackupRunByID(ctx, sourceRunID)
	if err != nil {
		return BackupRun{}, err
	}
	if source.Operation != BackupOpBackup || source.Status != BackupStatusSuccess {
		return BackupRun{}, invalid(codeBadRequest,
			"only a backup that finished successfully can be restored")
	}

	targetID := strings.TrimSpace(req.TargetContainerID)
	if targetID == "" {
		targetID = source.ContainerID
	}
	target, err := s.Get(ctx, targetID)
	if err != nil {
		return BackupRun{}, err
	}

	// The typed name is checked against the container being written over, not
	// against the one the backup came from. Those differ on a restore into a new
	// container, and the one the operator needs to be sure about is the one that
	// loses its filesystem.
	if strings.TrimSpace(req.ConfirmName) != target.Name {
		return BackupRun{}, ErrRestoreNotConfirmed
	}

	// Writing the rootfs out from under a running init leaves its processes
	// holding inodes that no longer exist, and it does not fail cleanly. A
	// refusal rather than a stop McHarbor performs on the operator's behalf:
	// stopping a container is their decision, not a side effect of a restore.
	if target.State == StateRunning || target.State == StateFrozen {
		return BackupRun{}, ErrRunning
	}
	if target.IsTemplate {
		return BackupRun{}, invalid(codeTemplateBackup,
			"a template cannot be restored over: its rootfs is the backing store for every clone of it")
	}

	// The whole chain has to be present before a single byte moves. A gap means
	// the delta cannot be applied, and applying what is there anyway produces a
	// tree that looks intact and is not.
	chain, err := s.restorableChain(ctx, source)
	if err != nil {
		return BackupRun{}, err
	}
	// Settled before anything is downloaded, like every other refusal here: a
	// chain whose runs disagree on their range cannot be re-owned correctly.
	sourceMap, mapOrigin, err := s.sourceIDMapFor(ctx, source, chain)
	if err != nil {
		return BackupRun{}, err
	}
	artifacts, grants, err := s.resolveChainArtifacts(ctx, chain)
	if err != nil {
		return BackupRun{}, err
	}

	run, err := s.createRestoreRun(ctx, target, source)
	if err != nil {
		s.clearDownloads(grants)
		return BackupRun{}, err
	}

	s.logBackupEvent(ctx, run.ID, "started", "starting", "info",
		"started from the backup taken at "+source.StartedAt.Format(time.RFC3339)+".")

	s.logRestoreIDMap(ctx, run.ID, sourceMap, mapOrigin, idMapOf(target))
	go s.executeRestore(context.WithoutCancel(ctx), target, run, artifacts, grants, sourceMap, restoreFollowUp{})
	return run, nil
}

// resolveChainArtifacts turns a chain into the ordered list the agent applies,
// and the grant ids it minted so they can be released afterwards.
//
// Ordering is explicit rather than implied: applying an incremental before the
// full it builds on, or skipping a link, produces a plausible-looking filesystem
// with someone else's data in it.
func (s *Service) resolveChainArtifacts(ctx context.Context, chain []BackupRun) ([]agentRestoreArtifact, []string, error) {
	var out []agentRestoreArtifact
	var grants []string

	for sequence, run := range chain {
		destinations, err := s.runDestinations(ctx, run.ID)
		if err != nil {
			s.clearDownloads(grants)
			return nil, nil, err
		}
		found := false
		for _, dest := range destinations {
			if dest.Status != DestinationSuccess || dest.RemotePath == "" {
				continue
			}
			// The manifest is not applied to the filesystem, so it is not part of
			// what the agent downloads. It exists for an operator rebuilding a
			// container McHarbor no longer has a row for, which is a separate
			// act from this one.
			if dest.Part != BackupPartRootfs {
				continue
			}
			grantID, token, err := s.backupGrants.issue(run.ID, run.ContainerID)
			if err != nil {
				s.clearDownloads(grants)
				return nil, nil, err
			}
			s.registerDownload(grantID, backupDownload{
				locationID: dest.LocationID,
				remotePath: dest.RemotePath,
			})
			grants = append(grants, grantID)
			out = append(out, agentRestoreArtifact{
				Part:      dest.Part,
				Kind:      run.Kind,
				URL:       backupDownloadPath + grantID,
				Token:     token,
				Sequence:  sequence,
				SizeBytes: dest.BytesUploaded,
			})
			// One copy per link is enough; the others are the same bytes at a
			// second destination.
			found = true
			break
		}
		if !found {
			s.clearDownloads(grants)
			return nil, nil, fmt.Errorf(
				"%w: one link of this chain is no longer at any destination",
				ErrChainIncomplete)
		}
	}
	if len(out) == 0 {
		s.clearDownloads(grants)
		return nil, nil, fmt.Errorf("%w: none of this backup's objects are still at a destination",
			ErrChainIncomplete)
	}
	return out, grants, nil
}

// restoreFollowUp is what a restore does to its target once the rootfs is back.
//
// Zero for an ordinary restore. A recreate sets materialize, because its target
// was only defined — the host builds its runtime state around the restored tree,
// which is the one order in which nothing is unpacked just to be replaced.
type restoreFollowUp struct {
	materialize bool
	start       bool
}

// executeRestore drives one restore to completion in the background.
func (s *Service) executeRestore(
	ctx context.Context,
	target Container,
	run BackupRun,
	artifacts []agentRestoreArtifact,
	grants []string,
	sourceMap *backupIDMap,
	follow restoreFollowUp,
) {
	ctx, cancel := context.WithTimeout(ctx, backupGrantTTL)
	defer cancel()
	// The grants outlive the call that minted them and must not outlive the run:
	// each one is a bearer token for reading a backup out of a destination.
	defer func() {
		for _, id := range grants {
			s.backupGrants.revoke(id)
		}
		s.clearDownloads(grants)
	}()

	s.updateBackupProgress(ctx, run.ID, "downloading", "")
	targetMap := idMapOf(target)
	reply, err := s.call(ctx, target.EnvironmentID, msgCTBackupRestore, &agentPayload{
		ContainerID: target.ID,
		Restore: &agentRestoreSpec{
			RunID:       run.ID,
			TargetPath:  target.RootfsPath,
			PoolID:      target.PoolID,
			PoolBackend: target.PoolBackend,
			Artifacts:   artifacts,
			SourceIDMap: sourceMap,
			TargetIDMap: &targetMap,
		},
	})
	if err == nil && !reply.Success {
		err = agentError(reply)
	}
	if reply != nil && reply.BackupResult != nil && reply.BackupResult.IDShift != "" {
		if recErr := s.recordRunIDShift(ctx, run.ID, reply.BackupResult.IDShift); recErr != nil {
			s.logger.Warn("system containers: recording a restore's ownership outcome",
				"run", run.ID, "error", recErr)
		}
	}
	// An agent that predates re-owning ignores both maps and swaps in a tree
	// owned by the source range, which boots and owns nothing. Its silence is
	// the only sign, so a restore that could move owners needs it said.
	if err == nil && restoreNeedsAck(sourceMap, targetMap) && !ackOK(reply) {
		err = ErrAgentCannotReown
	}
	if err != nil {
		s.failBackupRun(ctx, run.ID, err)
		if follow.materialize {
			s.markCreateFailed(ctx, target.ID,
				fmt.Errorf("the backup could not be restored into this container: %w", err))
		}
		return
	}

	if follow.materialize {
		if err := s.materialize(ctx, target, ""); err != nil {
			cause := fmt.Errorf("the backup was restored, but the host could not set the container up: %w", err)
			s.failBackupRun(ctx, run.ID, cause)
			s.markCreateFailed(ctx, target.ID, cause)
			return
		}
	}

	// The restored container's chain is finished: its contents no longer match
	// what the host's incremental listing has been tracking, so the next backup
	// must be a full copy. Skipping this is how a restore quietly poisons every
	// incremental taken after it.
	if err := s.InvalidateChain(ctx, target.ID,
		"this container was restored from a backup, so its incremental listing no longer "+
			"matches its filesystem"); err != nil {
		s.logger.Warn("system containers: invalidating a chain after a restore",
			"container", target.ID, "error", err)
	}

	s.finishBackupRun(ctx, run.ID, run.Kind, false, "", run.Consistency, restoreBytes(artifacts))

	if follow.start {
		if err := s.Start(ctx, target.ID); err != nil {
			// The recreate succeeded; the container simply did not come up, and
			// its row carries why.
			s.logger.Warn("system containers: recreated but did not start",
				"container", target.ID, "error", err)
		}
	}
}

// restoreBytes is what a restore moved: the stored size of every link the agent
// downloaded. The agent reports no count of its own for a restore.
func restoreBytes(artifacts []agentRestoreArtifact) int64 {
	var total int64
	for _, artifact := range artifacts {
		total += artifact.SizeBytes
	}
	return total
}

// createRestoreRun records a restore as a run of its own.
//
// Same table as backups, keyed by operation, so history, progress and polling
// are one implementation.
func (s *Service) createRestoreRun(ctx context.Context, target Container, source BackupRun) (BackupRun, error) {
	now := time.Now().UTC()
	run := BackupRun{
		ID:                xid.New().String(),
		ContainerID:       source.ContainerID,
		ContainerName:     source.ContainerName,
		EnvironmentID:     target.EnvironmentID,
		Operation:         BackupOpRestore,
		SourceRunID:       source.ID,
		Status:            BackupStatusRunning,
		Kind:              source.Kind,
		ChainID:           source.ChainID,
		Consistency:       source.Consistency,
		TargetContainerID: target.ID,
		StartedAt:         now,
	}

	stamp := now.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_runs
		       (id, plan_id, container_id, container_name, environment_id,
		        operation, source_run_id, status, kind, chain_id, consistency,
		        target_container_id, progress_stage, progress_message,
		        started_at, created_at, updated_at)
		VALUES (?, NULL, ?, ?, ?, 'restore', ?, 'running', ?, ?, ?, ?, 'starting', '', ?, ?, ?)`,
		run.ID, run.ContainerID, run.ContainerName, run.EnvironmentID,
		run.SourceRunID, run.Kind, nullIfEmpty(run.ChainID), run.Consistency,
		run.TargetContainerID, stamp, stamp, stamp); err != nil {
		return BackupRun{}, fmt.Errorf("recording the restore: %w", err)
	}
	return run, nil
}
