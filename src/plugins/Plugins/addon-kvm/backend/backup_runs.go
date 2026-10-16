// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const msgVMBackupRun = "vm_backup_run"

// backupUploadPath is where an agent streams a run's artifacts back.
//
// A path rather than an absolute URL: only the agent knows which address it
// actually reached McHarbor on, and it resolves this against that — the same
// arrangement the import pull already uses.
const backupUploadPath = "/api/vms/backups/internal/"

// backupUploadURL is the path an agent redeems its grant at.
func (s *Service) backupUploadURL(grantID string) string {
	return backupUploadPath + grantID
}

// backupGrantTTL bounds how long an upload grant stays redeemable.
//
// A full copy of a large guest legitimately takes hours before the first byte
// arrives, so this is long; it expires at all because a grant is a bearer
// token for writing into McHarbor.
const backupGrantTTL = 12 * time.Hour

// backupGrant authorises one agent to stream one run's artifacts back.
type backupGrant struct {
	token   string
	runID   string
	vmID    string
	expires time.Time
}

// backupGrants holds the live upload grants.
type backupGrants struct {
	mu     sync.Mutex
	grants map[string]backupGrant
}

// newBackupGrants builds an empty grant store.
func newBackupGrants() *backupGrants {
	return &backupGrants{grants: make(map[string]backupGrant)}
}

// issue records a grant for one run and returns its id and token.
func (g *backupGrants) issue(runID, vmID string) (string, string, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", fmt.Errorf("minting a backup grant: %w", err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("minting a backup grant: %w", err)
	}
	id := hex.EncodeToString(idBytes)
	token := hex.EncodeToString(tokenBytes)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()
	g.grants[id] = backupGrant{
		token: token, runID: runID, vmID: vmID,
		expires: time.Now().Add(backupGrantTTL),
	}
	return id, token, nil
}

// redeem returns the run a grant authorises.
//
// Constant-time, because this is the only thing between an unauthenticated
// route and McHarbor's upload path: a comparison that returned early would
// leak the token through how long a wrong guess takes to reject.
func (g *backupGrants) redeem(id, token string) (backupGrant, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()

	grant, ok := g.grants[id]
	if !ok {
		return backupGrant{}, false
	}
	if subtle.ConstantTimeCompare([]byte(grant.token), []byte(token)) != 1 {
		return backupGrant{}, false
	}
	return grant, true
}

// revoke drops a grant once its run is over.
func (g *backupGrants) revoke(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.grants, id)
}

// sweepLocked drops expired grants. Called under the lock by every accessor,
// so an abandoned run cannot leave a redeemable token behind indefinitely.
func (g *backupGrants) sweepLocked() {
	now := time.Now()
	for id, grant := range g.grants {
		if now.After(grant.expires) {
			delete(g.grants, id)
		}
	}
}

// StartBackupRun runs one backup of one machine.
//
// Synchronous from the caller's point of view but bounded by the agent's own
// long-command budget: the handler starts it in the background and the UI
// polls the run row, exactly as the container module does.
func (s *Service) StartBackupRun(ctx context.Context, vmID, planID string, req RunBackupInput) (BackupRun, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return BackupRun{}, err
	}

	plan := BackupPlan{FullEvery: 7, Quiesce: true}
	if planID != "" {
		if plan, err = s.BackupPlanByID(ctx, planID); err != nil {
			return BackupRun{}, err
		}
	}
	destinations, err := s.resolveDestinations(ctx, plan)
	if err != nil {
		return BackupRun{}, err
	}

	disks := backupableDisks(vm)
	if len(disks) == 0 {
		return BackupRun{}, fmt.Errorf("%w: this machine has no disks that can be backed up",
			ErrBackupNoDisks)
	}

	// One chain per disk, and the kind settled per disk before anything
	// starts. A run whose disks disagree about their baseline is a run whose
	// restore path differs per disk.
	kind := BackupKindIncremental
	if req.Kind == BackupKindFull {
		kind = BackupKindFull
	}
	chains := make(map[string]BackupChain, len(disks))
	for _, disk := range disks {
		chain, err := s.chainFor(ctx, vm.ID, disk.ID, plan.ID)
		if err != nil {
			return BackupRun{}, err
		}
		decided, _ := s.decideRunKind(ctx, plan, chain, req.Kind)
		if decided == BackupKindFull {
			kind = BackupKindFull
		}
		chains[disk.ID] = chain
	}

	run, err := s.createBackupRun(ctx, vm, plan, kind, chains, disks)
	if err != nil {
		return BackupRun{}, err
	}

	s.logBackupEvent(ctx, run.ID, "started", "starting", "info",
		"started: "+kind+", "+strconv.Itoa(len(disks))+" disk(s) to "+
			strconv.Itoa(len(destinations))+" destination(s).")

	go s.executeBackupRun(context.WithoutCancel(ctx), vm, plan, run, chains, disks, destinations)
	return run, nil
}

// executeBackupRun drives one run to completion in the background.
func (s *Service) executeBackupRun(
	ctx context.Context,
	vm VM,
	plan BackupPlan,
	run BackupRun,
	chains map[string]BackupChain,
	disks []Disk,
	destinations []backupDestination,
) {
	ctx, cancel := context.WithTimeout(ctx, backupGrantTTL)
	defer cancel()

	grantID, token, err := s.backupGrants.issue(run.ID, vm.ID)
	if err != nil {
		s.failBackupRun(ctx, run.ID, err)
		return
	}
	defer s.backupGrants.revoke(grantID)

	s.registerBackupUpload(run.ID, backupUploadTarget{
		run: run, vm: vm, destinations: destinations,
	})
	defer s.clearBackupUpload(run.ID)

	payload := agentPayload{
		VMID: vm.ID,
		Backup: &agentBackupSpec{
			RunID:   run.ID,
			ChainID: run.ChainID,
			Kind:    run.Kind,
			Quiesce: plan.Quiesce,
			Upload: &agentBackupUpload{
				URL:   s.backupUploadURL(grantID),
				Token: token,
			},
		},
	}
	for _, disk := range disks {
		payload.Backup.Disks = append(payload.Backup.Disks, agentBackupDisk{
			DiskID:     disk.ID,
			VolumePath: disk.VolumePath,
			NodeName:   fmt.Sprintf("blk%d", disk.Slot),
			BitmapName: chains[disk.ID].BitmapName,
		})
	}

	s.updateBackupProgress(ctx, run.ID, "copying", "")
	reply, err := s.call(ctx, vm.EnvironmentID, msgVMBackupRun, payload)
	if err != nil {
		s.failBackupRun(ctx, run.ID, err)
		return
	}

	// The agent's verdict is authoritative. If it fell back to a full copy on
	// any disk, that disk's chain restarts and the run records full — the
	// database must never claim an incremental that was not taken.
	takenKind := run.Kind
	var fellBack bool
	var reason string
	var total int64
	for _, result := range reply.BackupResult {
		total += result.BytesWritten
		if result.FellBackToFull {
			fellBack = true
			takenKind = BackupKindFull
			if reason == "" {
				reason = result.FallbackReason
			}
		}
		if chain, ok := chains[result.DiskID]; ok {
			if _, err := s.recordChainRun(ctx, chain, run.ID,
				result.Kind, result.FellBackToFull, result.FallbackReason); err != nil {
				s.logger.Warn("kvm: advancing a backup chain", "run", run.ID, "error", err)
			}
		}
	}

	consistency := "crash"
	if len(reply.BackupResult) > 0 && reply.BackupResult[0].Consistency != "" {
		consistency = reply.BackupResult[0].Consistency
	}
	s.finishBackupRun(ctx, run.ID, takenKind, fellBack, reason, consistency, total)

	if plan.ID != "" {
		if err := s.pruneBackupRuns(ctx, plan); err != nil {
			s.logger.Warn("kvm: pruning backups", "plan", plan.ID, "error", err)
		}
	}
}

// backupUploadTarget is what an in-flight run's upload needs to know.
type backupUploadTarget struct {
	run          BackupRun
	vm           VM
	destinations []backupDestination
}

// backupDestination is one resolved storage location.
type backupDestination struct {
	ID   string
	Name string
}

// registerBackupUpload records where a run's incoming artifacts should go.
func (s *Service) registerBackupUpload(runID string, target backupUploadTarget) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	if s.backupUploads == nil {
		s.backupUploads = map[string]backupUploadTarget{}
	}
	s.backupUploads[runID] = target
}

// clearBackupUpload forgets a finished run's upload target.
func (s *Service) clearBackupUpload(runID string) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	delete(s.backupUploads, runID)
}

// backupUploadFor returns a run's upload target.
func (s *Service) backupUploadFor(runID string) (backupUploadTarget, bool) {
	s.backupMu.RLock()
	defer s.backupMu.RUnlock()
	target, ok := s.backupUploads[runID]
	return target, ok
}

// HandleBackupUpload serves PUT /api/vms/backups/internal/{grantId}.
//
// Public because the caller is an agent and holds no session; it redeems a
// one-shot grant McHarbor minted moments earlier. The request body is piped
// straight to the destinations — nothing is written to McHarbor's disk, which
// is what makes backing up a 200 GB guest possible at all.
func (h *Handler) HandleBackupUpload(w http.ResponseWriter, r *http.Request) {
	grantID := chi.URLParam(r, "grantId")
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if grantID == "" || token == "" {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	grant, ok := h.svc.backupGrants.redeem(grantID, token)
	if !ok {
		h.logger.Warn("kvm: backup upload refused", "grant", grantID, "remote", r.RemoteAddr)
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	target, ok := h.svc.backupUploadFor(grant.runID)
	if !ok {
		fail(w, r, http.StatusConflict, codeNotFound)
		return
	}

	diskID := strings.TrimSpace(r.Header.Get("X-McHarbor-Disk"))
	kind := strings.TrimSpace(r.Header.Get("X-McHarbor-Backup-Kind"))
	if diskID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()
	h.extendTransferDeadlines(w)

	if err := h.svc.fanOutBackup(r.Context(), target, diskID, kind, r.Body, r.ContentLength); err != nil {
		h.logger.Error("kvm: storing a backup", "run", grant.runID, "error", err)
		h.respondErr(w, r, "store backup", err)
		return
	}
	okMsg(w, msgBackupStored)
}

// extendTransferDeadlines lifts the server's 30s read and 120s write limits for
// one artifact transfer, which lasts as long as the disk takes to cross.
//
// Bounded by the grant's lifetime, the same budget the agent works to. Without
// it every upload longer than thirty seconds failed mid-stream as a 502.
func (h *Handler) extendTransferDeadlines(w http.ResponseWriter) {
	deadline := time.Now().Add(backupGrantTTL)
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(deadline); err != nil {
		h.logger.Warn("kvm: cannot extend the backup transfer read deadline", "error", err)
	}
	if err := controller.SetWriteDeadline(deadline); err != nil {
		h.logger.Warn("kvm: cannot extend the backup transfer write deadline", "error", err)
	}
}

// fanOutBackup writes one artifact to every destination at once.
//
// The body is a stream with a single read, so it is tee'd through one pipe per
// destination and copied once into an io.MultiWriter. Nothing is buffered:
// the slowest destination sets the pace for all of them, which is the correct
// backpressure — the agent's upload slows rather than McHarbor accumulating a
// guest-sized file it has nowhere to put.
func (s *Service) fanOutBackup(
	ctx context.Context,
	target backupUploadTarget,
	diskID, kind string,
	body io.Reader,
	size int64,
) error {
	streamer, err := s.streamHandle()
	if err != nil {
		return err
	}
	if len(target.destinations) == 0 {
		return invalid(codeNoDestination, "this backup has no destination")
	}

	key := backupObjectKey(
		s.environmentName(ctx, target.vm.EnvironmentID), target.vm.EnvironmentID,
		target.vm.Name, target.vm.ID,
		target.run.ID, diskID, kind)
	writers := make([]io.Writer, 0, len(target.destinations))
	closers := make([]*io.PipeWriter, 0, len(target.destinations))
	errs := make(chan error, len(target.destinations))

	for _, dest := range target.destinations {
		reader, writer := io.Pipe()
		writers = append(writers, writer)
		closers = append(closers, writer)

		destID := s.beginDestination(ctx, target.run.ID, dest, diskID)
		go func(dest backupDestination, destID string, reader *io.PipeReader) {
			result, upErr := streamer.UploadStream(ctx, plugin.StorageStreamRequest{
				LocationID: dest.ID,
				Key:        key,
				Body:       reader,
				SizeHint:   size,
				Encrypt:    true,
			})
			if upErr != nil {
				// Draining matters: an upload that returned early would
				// otherwise block the MultiWriter and stall every other
				// destination behind a failure that has already happened.
				_ = reader.CloseWithError(upErr)
				s.failDestination(ctx, destID, upErr)
				errs <- upErr
				return
			}
			_ = reader.Close()
			s.completeDestination(ctx, destID, result.RemotePath, result.KeyID, result.BytesWritten)
			errs <- nil
		}(dest, destID, reader)
	}

	_, copyErr := io.Copy(io.MultiWriter(writers...), body)
	for _, writer := range closers {
		if copyErr != nil {
			_ = writer.CloseWithError(copyErr)
			continue
		}
		_ = writer.Close()
	}

	// Every destination is waited on, so a run never finishes while an upload
	// is still in flight and the run row cannot claim success before the bytes
	// have actually landed.
	var failures []string
	for range target.destinations {
		if err := <-errs; err != nil {
			failures = append(failures, hostErrorDetail(err))
		}
	}
	if copyErr != nil {
		return fmt.Errorf("receiving the backup: %w", copyErr)
	}
	if len(failures) == len(target.destinations) {
		return fmt.Errorf("every destination refused this backup: %s", strings.Join(failures, "; "))
	}
	// A partial success stays a success: the artifact reached somewhere, and
	// the per-destination rows say which. Failing the whole run would hide a
	// copy that is genuinely there and restorable.
	return nil
}

// beginDestination records a destination row as uploading and returns its id.
func (s *Service) beginDestination(ctx context.Context, runID string, dest backupDestination, diskID string) string {
	destID := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_backup_run_destinations
		       (id, run_id, storage_location_id, storage_location_name, disk_id,
		        status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'uploading', ?, ?)`,
		destID, runID, dest.ID, dest.Name, diskID, now, now); err != nil {
		s.logger.Warn("kvm: recording a backup destination", "run", runID, "error", err)
	}
	return destID
}

// completeDestination records where an artifact landed.
func (s *Service) completeDestination(ctx context.Context, destID, remotePath, keyID string, written int64) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_run_destinations
		   SET status = 'success', remote_path = ?, key_id = ?,
		       bytes_uploaded = ?, uploaded_at = ?, updated_at = ?
		 WHERE id = ?`, remotePath, keyID, written, now, now, destID); err != nil {
		s.logger.Warn("kvm: recording a finished upload", "destination", destID, "error", err)
	}
}

// failDestination records why one destination did not receive the artifact.
func (s *Service) failDestination(ctx context.Context, destID string, cause error) {
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_run_destinations
		   SET status = 'failure', error = ?, updated_at = ?
		 WHERE id = ?`,
		hostErrorDetail(cause), time.Now().UTC().Format(time.RFC3339), destID); err != nil {
		s.logger.Warn("kvm: recording a failed upload", "destination", destID, "error", err)
	}
}

// streamHandle returns the host's streaming upload handle.
//
// Asserted rather than required: a host too old to stream is refused by name
// here, which is a better answer than a plugin that fails at the first
// upload with a nil dereference.
func (s *Service) streamHandle() (plugin.StorageStreamHandle, error) {
	if s.storage == nil {
		return nil, invalid(codeNoDestination,
			"this McHarbor has no storage destinations configured")
	}
	streamer, ok := s.storage.(plugin.StorageStreamHandle)
	if !ok {
		return nil, invalid(codeNoDestination,
			"this version of McHarbor cannot stream a backup to a destination")
	}
	return streamer, nil
}

// resolveDestinations turns a plan's destination ids into names.
func (s *Service) resolveDestinations(ctx context.Context, plan BackupPlan) ([]backupDestination, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("%w: this McHarbor has no storage destinations configured",
			ErrBackupStorageUnavailable)
	}
	known := map[string]string{}
	for _, location := range s.storage.Locations() {
		known[location.ID] = location.Name
	}

	out := make([]backupDestination, 0, len(plan.DestinationIDs))
	for _, id := range plan.DestinationIDs {
		name, ok := known[id]
		if !ok {
			// A destination that has been deleted since the plan was written
			// is named rather than skipped: the operator's schedule is now
			// writing somewhere that does not exist.
			return nil, fmt.Errorf("%w: one of this plan's destinations no longer exists",
				ErrBackupDestinationGone)
		}
		out = append(out, backupDestination{ID: id, Name: name})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: this backup has no destination", ErrBackupNoDestination)
	}
	return out, nil
}

// backupableDisks is the set a backup covers.
//
// Discs and read-only disks are skipped: an installer ISO is not the
// machine's state, and copying it would double the transfer for nothing.
func backupableDisks(vm VM) []Disk {
	var out []Disk
	for _, disk := range vm.Disks {
		if disk.IsCDROM || disk.ReadOnly || disk.VolumePath == "" {
			continue
		}
		out = append(out, disk)
	}
	return out
}

// createBackupRun records a run before any work starts.
func (s *Service) createBackupRun(
	ctx context.Context,
	vm VM,
	plan BackupPlan,
	kind string,
	chains map[string]BackupChain,
	disks []Disk,
) (BackupRun, error) {
	now := time.Now().UTC()
	run := BackupRun{
		ID:            xid.New().String(),
		PlanID:        plan.ID,
		VMID:          vm.ID,
		EnvironmentID: vm.EnvironmentID,
		Operation:     BackupOpBackup,
		Status:        BackupStatusRunning,
		Kind:          kind,
		Consistency:   "crash",
		StartedAt:     now,
	}
	for _, disk := range disks {
		run.IncludedDisks = append(run.IncludedDisks, disk.ID)
		if run.ChainID == "" {
			run.ChainID = chains[disk.ID].ID
		}
	}

	stamp := now.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_backup_runs
		       (id, plan_id, vm_id, environment_id, operation, status, kind,
		        chain_id, consistency, included_disks, progress_stage,
		        progress_message, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'backup', 'running', ?, ?, 'crash', ?, 'starting', '', ?, ?, ?)`,
		run.ID, nullIfEmpty(run.PlanID), run.VMID, run.EnvironmentID, run.Kind,
		nullIfEmpty(run.ChainID), encodeDiskList(run.IncludedDisks),
		stamp, stamp, stamp); err != nil {
		return BackupRun{}, fmt.Errorf("recording the backup run: %w", err)
	}
	return run, nil
}

// updateBackupProgress records how far a run has got.
func (s *Service) updateBackupProgress(ctx context.Context, runID, stage, message string) {
	_, _ = s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_runs
		   SET progress_stage = ?, progress_message = ?, progress_updated_at = ?, updated_at = ?
		 WHERE id = ?`,
		stage, message, time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339), runID)
}

// finishBackupRun records a successful outcome.
func (s *Service) finishBackupRun(
	ctx context.Context,
	runID, kind string,
	fellBack bool,
	reason, consistency string,
	total int64,
) {
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_runs
		   SET status = 'success', kind = ?, fell_back_to_full = ?, fallback_reason = ?,
		       consistency = ?, bytes_total = ?, progress_stage = 'done',
		       completed_at = ?, duration_ms = CAST((julianday(?) - julianday(started_at)) * 86400000 AS INTEGER),
		       updated_at = ?
		 WHERE id = ?`,
		kind, boolToInt(fellBack), nullIfEmpty(reason), consistency, total,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
		now.Format(time.RFC3339), runID); err != nil {
		s.logger.Error("kvm: recording a finished backup", "run", runID, "error", err)
	}
	message := "finished."
	severity := "success"
	if fellBack && reason != "" {
		message = "finished as a full copy: " + reason
		severity = "warning"
	} else if consistency == ConsistencyCrash {
		message = "finished, crash-consistent: the guest was not quiesced."
		severity = "warning"
	}
	s.logBackupEvent(ctx, runID, "completed", "done", severity, message)
}

// failBackupRun records a failure with the host's own words.
func (s *Service) failBackupRun(ctx context.Context, runID string, cause error) {
	now := time.Now().UTC().Format(time.RFC3339)
	detail := hostErrorDetail(cause)
	if detail == "" && cause != nil {
		detail = cause.Error()
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_backup_runs
		   SET status = 'failure', error = ?, progress_stage = 'failed',
		       completed_at = ?, updated_at = ?
		 WHERE id = ?`, detail, now, now, runID); err != nil {
		s.logger.Error("kvm: recording a failed backup", "run", runID, "error", err)
	}
	if detail == "" {
		detail = "failed."
	} else {
		detail = "failed: " + detail
	}
	s.logBackupEvent(ctx, runID, "failed", "failed", "error", detail)
}

// ErrChainIncomplete means a backup's chain is missing a link it needs.
var ErrChainIncomplete = errors.New("this backup's chain is incomplete")
