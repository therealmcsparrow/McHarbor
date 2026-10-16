// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// backupUploadPath is where an agent streams a run's artifacts back.
//
// A path rather than an absolute URL: only the agent knows which address it
// actually reached McHarbor on, and it resolves this against that — the same
// arrangement the image pull already uses.
const backupUploadPath = "/api/system-containers/backups/internal/"

// backupDownloadPath is where an agent pulls a chain's artifacts from.
const backupDownloadPath = "/api/system-containers/backups/internal/download/"

// backupGrantTTL bounds how long an upload grant stays redeemable.
//
// A full copy of a large rootfs legitimately takes hours before the first byte
// arrives, so this is long; it expires at all because a grant is a bearer token
// for writing into McHarbor.
const backupGrantTTL = 12 * time.Hour

// backupUploadURL is the path an agent redeems its grant at.
func backupUploadURL(grantID string) string { return backupUploadPath + grantID }

// backupGrant authorises one agent to move one run's artifacts.
type backupGrant struct {
	token       string
	runID       string
	containerID string
	expires     time.Time
}

// backupGrants holds the live transfer grants.
type backupGrants struct {
	mu     sync.Mutex
	grants map[string]backupGrant
}

// newBackupGrants builds an empty grant store.
func newBackupGrants() *backupGrants {
	return &backupGrants{grants: make(map[string]backupGrant)}
}

// issue records a grant for one run and returns its id and token.
func (g *backupGrants) issue(runID, containerID string) (string, string, error) {
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
		token: token, runID: runID, containerID: containerID,
		expires: time.Now().Add(backupGrantTTL),
	}
	return id, token, nil
}

// redeem returns the run a grant authorises.
//
// Constant-time, because this is the only thing between an unauthenticated
// route and McHarbor's transfer path: a comparison that returned early would
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

// sweepLocked drops expired grants. Called under the lock by every accessor, so
// an abandoned run cannot leave a redeemable token behind indefinitely.
func (g *backupGrants) sweepLocked() {
	now := time.Now()
	for id, grant := range g.grants {
		if now.After(grant.expires) {
			delete(g.grants, id)
		}
	}
}

// backupUploadTarget is what an in-flight run's upload needs to know.
type backupUploadTarget struct {
	run          BackupRun
	container    Container
	destinations []backupDestination
}

// backupDestination is one resolved storage location.
type backupDestination struct {
	ID   string
	Name string
}

// backupDownload is one artifact an agent may pull.
type backupDownload struct {
	locationID string
	remotePath string
}

// StartBackupRun runs one backup of one container.
//
// Returns as soon as the run is recorded. A full copy of a large rootfs takes
// hours and an HTTP request is the wrong thing to hold open for it, so the run
// row is what the UI polls.
func (s *Service) StartBackupRun(ctx context.Context, containerID, planID string, req RunBackupInput) (BackupRun, error) {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return BackupRun{}, err
	}
	if ct.IsTemplate {
		// A template's rootfs is the backing store for every linked clone on
		// the pool. Backing it up is meaningful, restoring it over the top of
		// live clones is not, so the whole path is refused rather than offering
		// a backup that cannot be used.
		return BackupRun{}, invalid(codeTemplateBackup,
			"a template cannot be backed up: its rootfs is the backing store for every clone of it")
	}

	plan := BackupPlan{FullEvery: 7}
	if planID != "" {
		if plan, err = s.BackupPlanByID(ctx, planID); err != nil {
			return BackupRun{}, err
		}
		if plan.ContainerID != ct.ID {
			return BackupRun{}, ErrNotFound
		}
	}
	destinations, err := s.resolveDestinations(ctx, plan)
	if err != nil {
		return BackupRun{}, err
	}

	chain, err := s.chainFor(ctx, ct.ID, plan.ID)
	if err != nil {
		return BackupRun{}, err
	}
	kind, _ := s.decideRunKind(ctx, plan, chain, req.Kind)

	run, err := s.createBackupRun(ctx, ct, plan, kind, chain)
	if err != nil {
		return BackupRun{}, err
	}

	s.logBackupEvent(ctx, run.ID, "started", "starting", "info",
		"started: "+kind+" to "+strconv.Itoa(len(destinations))+" destination(s).")

	go s.executeBackupRun(context.WithoutCancel(ctx), ct, plan, run, chain, destinations)
	return run, nil
}

// executeBackupRun drives one run to completion in the background.
func (s *Service) executeBackupRun(
	ctx context.Context,
	ct Container,
	plan BackupPlan,
	run BackupRun,
	chain BackupChain,
	destinations []backupDestination,
) {
	ctx, cancel := context.WithTimeout(ctx, backupGrantTTL)
	defer cancel()

	grantID, token, err := s.backupGrants.issue(run.ID, ct.ID)
	if err != nil {
		s.failBackupRun(ctx, run.ID, err)
		return
	}
	defer s.backupGrants.revoke(grantID)

	s.registerBackupUpload(run.ID, backupUploadTarget{
		run: run, container: ct, destinations: destinations,
	})
	defer s.clearBackupUpload(run.ID)

	manifest, err := s.backupManifestFor(ctx, ct)
	if err != nil {
		s.failBackupRun(ctx, run.ID, err)
		return
	}
	// The archive holds host ids, so the range goes on record before a byte is
	// written: a backup nobody can say the range of restores only by inference.
	if err := s.recordRunIDMap(ctx, run.ID, idMapOf(ct)); err != nil {
		s.failBackupRun(ctx, run.ID, err)
		return
	}

	payload := &agentPayload{
		ContainerID: ct.ID,
		Backup: &agentBackupSpec{
			RunID:       run.ID,
			ChainID:     chain.ID,
			Kind:        run.Kind,
			RootfsPath:  ct.RootfsPath,
			PoolID:      ct.PoolID,
			PoolBackend: ct.PoolBackend,
			PoolTarget:  ct.PoolTarget,
			Manifest:    manifest,
			ListingName: chain.ListingName,
			Upload: &agentBackupUpload{
				URL:   backupUploadURL(grantID),
				Token: token,
			},
		},
	}

	s.updateBackupProgress(ctx, run.ID, "archiving", "")
	reply, err := s.call(ctx, ct.EnvironmentID, msgCTBackupRun, payload)
	if err != nil {
		s.failBackupRun(ctx, run.ID, s.withDestinationCause(ctx, run.ID, err))
		return
	}
	if !reply.Success {
		s.failBackupRun(ctx, run.ID, s.withDestinationCause(ctx, run.ID, agentError(reply)))
		return
	}

	// The agent's verdict is authoritative. If it fell back to a full copy the
	// chain restarts and the run records full — the database must never claim
	// an incremental that was not taken.
	result := reply.BackupResult
	if result == nil {
		s.failBackupRun(ctx, run.ID,
			fmt.Errorf("the host reported no outcome for this backup"))
		return
	}
	takenKind := result.Kind
	if takenKind == "" {
		takenKind = run.Kind
	}
	consistency := result.Consistency
	if consistency == "" {
		consistency = BackupConsistencyFilesystem
	}

	if _, err := s.recordChainRun(ctx, chain, run.ID, takenKind,
		result.FellBackToFull, result.FallbackReason); err != nil {
		s.logger.Warn("system containers: advancing a backup chain",
			"run", run.ID, "error", err)
	}

	s.finishBackupRun(ctx, run.ID, takenKind, result.FellBackToFull,
		result.FallbackReason, consistency, result.BytesWritten)

	if plan.ID != "" {
		if err := s.pruneBackupRuns(ctx, plan); err != nil {
			s.logger.Warn("system containers: pruning backups", "plan", plan.ID, "error", err)
		}
	}
}

// backupManifestFor captures the container's definition for the artifact.
//
// Written into the backup rather than left in McHarbor's database because the
// case this feature exists for is the one where the database row is gone: the
// rootfs alone does not say what init it runs or how it was id-mapped, and
// guessing either produces a container that boots wrong.
func (s *Service) backupManifestFor(ctx context.Context, ct Container) (json.RawMessage, error) {
	manifest := backupManifest{
		Version:       1,
		ContainerID:   ct.ID,
		Name:          ct.Name,
		Description:   ct.Description,
		EnvironmentID: ct.EnvironmentID,
		OSFamily:      ct.OSFamily,
		OSVersion:     ct.OSVersion,
		InitSystem:    ct.InitSystem,
		Privileged:    ct.Privileged,
		Nesting:       ct.Nesting,
		PoolBackend:   ct.PoolBackend,
		RootfsPath:    ct.RootfsPath,
		Limits:        ct.Limits,
		TakenAt:       time.Now().UTC().Format(time.RFC3339),
	}
	idMap := idMapOf(ct)
	manifest.IDMap = &idMap

	// Best effort on both: a container whose interfaces cannot be read still
	// deserves a backup of its filesystem, and the manifest says less rather
	// than the run failing.
	if nics, err := s.NICs(ctx, ct.ID); err == nil {
		manifest.NICs = nics
	}
	if mounts, err := s.Mounts(ctx, ct.ID); err == nil {
		manifest.Mounts = mounts
	}

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding the backup manifest: %w", err)
	}
	return encoded, nil
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

// registerDownload records what a download grant authorises.
func (s *Service) registerDownload(grantID string, download backupDownload) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	if s.backupDownloads == nil {
		s.backupDownloads = map[string]backupDownload{}
	}
	s.backupDownloads[grantID] = download
}

// clearDownloads forgets every download grant a restore minted.
func (s *Service) clearDownloads(grantIDs []string) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	for _, id := range grantIDs {
		delete(s.backupDownloads, id)
	}
}

// downloadFor returns what a grant authorises.
func (s *Service) downloadFor(grantID string) (backupDownload, bool) {
	s.backupMu.RLock()
	defer s.backupMu.RUnlock()
	download, ok := s.backupDownloads[grantID]
	return download, ok
}

// fanOutBackup writes one artifact part to every destination at once.
//
// The body is a stream with a single read, so it is tee'd through one pipe per
// destination and copied once into an io.MultiWriter. Nothing is buffered: the
// slowest destination sets the pace, which is the correct backpressure — the
// agent's upload slows rather than McHarbor accumulating a rootfs-sized file it
// has nowhere to put.
func (s *Service) fanOutBackup(
	ctx context.Context,
	target backupUploadTarget,
	part, kind string,
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
		s.environmentName(ctx, target.container.EnvironmentID), target.container.EnvironmentID,
		target.container.Name, target.container.ID,
		target.run.ID, part, kind)
	writers := make([]io.Writer, 0, len(target.destinations))
	closers := make([]*io.PipeWriter, 0, len(target.destinations))
	errs := make(chan error, len(target.destinations))

	for _, dest := range target.destinations {
		reader, writer := io.Pipe()
		writers = append(writers, writer)
		closers = append(closers, writer)

		destID := s.beginDestination(ctx, target.run.ID, dest, part)
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

	// Every destination is waited on, so a run never finishes while an upload is
	// still in flight and the run row cannot claim success before the bytes have
	// actually landed.
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
	// A partial success stays a success: the artifact reached somewhere, and the
	// per-destination rows say which. Failing the whole run would hide a copy
	// that is genuinely there and restorable.
	return nil
}

// beginDestination records a destination row as uploading and returns its id.
func (s *Service) beginDestination(ctx context.Context, runID string, dest backupDestination, part string) string {
	destID := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_run_destinations
		       (id, run_id, storage_location_id, storage_location_name, part,
		        status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'uploading', ?, ?)`,
		destID, runID, dest.ID, dest.Name, part, now, now); err != nil {
		s.logger.Warn("system containers: recording a backup destination",
			"run", runID, "error", err)
	}
	return destID
}

// completeDestination records where an artifact landed.
func (s *Service) completeDestination(ctx context.Context, destID, remotePath, keyID string, written int64) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_backup_run_destinations
		   SET status = 'success', remote_path = ?, key_id = ?,
		       bytes_uploaded = ?, uploaded_at = ?, updated_at = ?
		 WHERE id = ?`, remotePath, keyID, written, now, now, destID); err != nil {
		s.logger.Warn("system containers: recording a finished upload",
			"destination", destID, "error", err)
	}
}

// failDestination records why one destination did not receive the artifact.
func (s *Service) failDestination(ctx context.Context, destID string, cause error) {
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_backup_run_destinations
		   SET status = 'failure', error = ?, updated_at = ?
		 WHERE id = ?`,
		hostErrorDetail(cause), time.Now().UTC().Format(time.RFC3339), destID); err != nil {
		s.logger.Warn("system containers: recording a failed upload",
			"destination", destID, "error", err)
	}
}

// streamHandle returns the host's streaming upload handle.
//
// Asserted rather than assumed: a host too old to stream is refused by name
// here, which is a better answer than a plugin that fails at the first upload
// with a nil dereference.
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
	_ = ctx
	if s.storage == nil {
		return nil, invalid(codeNoDestination,
			"this McHarbor has no storage destinations configured")
	}
	known := map[string]string{}
	for _, location := range s.storage.Locations() {
		known[location.ID] = location.Name
	}

	out := make([]backupDestination, 0, len(plan.DestinationIDs))
	for _, id := range plan.DestinationIDs {
		name, ok := known[id]
		if !ok {
			// A destination deleted since the plan was written is named rather
			// than skipped: the operator's schedule is now writing somewhere
			// that does not exist.
			return nil, invalid(codeNoDestination,
				"one of this plan's destinations no longer exists")
		}
		out = append(out, backupDestination{ID: id, Name: name})
	}
	if len(out) == 0 {
		return nil, invalid(codeNoDestination, "this backup has no destination")
	}
	return out, nil
}

// createBackupRun records a run before any work starts.
func (s *Service) createBackupRun(
	ctx context.Context,
	ct Container,
	plan BackupPlan,
	kind string,
	chain BackupChain,
) (BackupRun, error) {
	now := time.Now().UTC()
	run := BackupRun{
		ID:            xid.New().String(),
		PlanID:        plan.ID,
		ContainerID:   ct.ID,
		ContainerName: ct.Name,
		EnvironmentID: ct.EnvironmentID,
		Operation:     BackupOpBackup,
		Status:        BackupStatusRunning,
		Kind:          kind,
		ChainID:       chain.ID,
		Consistency:   BackupConsistencyFilesystem,
		StartedAt:     now,
	}

	stamp := now.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_runs
		       (id, plan_id, container_id, container_name, environment_id,
		        operation, status, kind, chain_id, consistency,
		        progress_stage, progress_message, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'backup', 'running', ?, ?, ?, 'starting', '', ?, ?, ?)`,
		run.ID, nullIfEmpty(run.PlanID), run.ContainerID, run.ContainerName,
		run.EnvironmentID, run.Kind, nullIfEmpty(run.ChainID), run.Consistency,
		stamp, stamp, stamp); err != nil {
		return BackupRun{}, fmt.Errorf("recording the backup run: %w", err)
	}
	return run, nil
}

// updateBackupProgress records how far a run has got.
func (s *Service) updateBackupProgress(ctx context.Context, runID, stage, message string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_backup_runs
		   SET progress_stage = ?, progress_message = ?, progress_updated_at = ?, updated_at = ?
		 WHERE id = ?`, stage, message, now, now, runID); err != nil {
		s.logger.Debug("system containers: recording backup progress",
			"run", runID, "error", err)
	}
}

// finishBackupRun records a successful outcome. It clears the progress message,
// or the last copy frame ("512 KiB of 3.4 MiB (14%)") outlives the finished run.
//
// The duration is computed here rather than in SQL. julianday is SQLite's own,
// and this plugin's queries run unchanged against Postgres, MySQL and SQL Server
// — a date function in a runtime query is a portability bug waiting for the
// first operator who moves their database.
func (s *Service) finishBackupRun(
	ctx context.Context,
	runID, kind string,
	fellBack bool,
	reason, consistency string,
	total int64,
) {
	now := time.Now().UTC()
	duration := s.runDuration(ctx, runID, now)
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_backup_runs
		   SET status = 'success', kind = ?, fell_back_to_full = ?, fallback_reason = ?,
		       consistency = ?, bytes_total = ?, progress_stage = 'done', progress_message = '',
		       completed_at = ?, duration_ms = ?, updated_at = ?
		 WHERE id = ?`,
		kind, boolToInt(fellBack), nullIfEmpty(reason), consistency, total,
		now.Format(time.RFC3339), duration, now.Format(time.RFC3339), runID); err != nil {
		s.logger.Error("system containers: recording a finished backup",
			"run", runID, "error", err)
	}

	message := "finished."
	severity := "success"
	if fellBack && reason != "" {
		message = "finished as a full copy: " + reason
		severity = "warning"
	}
	s.logBackupEvent(ctx, runID, "completed", "done", severity, message)
}

// backupKeyMissingMarker is the host's sentence for a destination refused because
// no backup encryption key is installed. Matched as text: the host's sentinel
// lives in a module a plugin must not link against.
const backupKeyMissingMarker = "no backup encryption key is installed"

// errorHintBackupKeyMissing tells the interface to name the Settings page that
// installs the key, which is the whole of the fix.
const errorHintBackupKeyMissing = "backup_key_missing"

// backupErrorHint classifies a run's error into a fix the interface can name, or
// "" when the message is all there is to say.
func backupErrorHint(message string) string {
	if strings.Contains(message, backupKeyMissingMarker) {
		return errorHintBackupKeyMissing
	}
	return ""
}

// withDestinationCause replaces a run's transport-level failure with the reasons
// its destinations recorded. The agent only sees McHarbor answer 502; the
// destination row holds why, and that is what the operator can act on.
func (s *Service) withDestinationCause(ctx context.Context, runID string, cause error) error {
	destinations, err := s.runDestinations(ctx, runID)
	if err != nil {
		return cause
	}
	seen := map[string]bool{}
	var reasons []string
	for _, dest := range destinations {
		reason := strings.TrimSpace(dest.Error)
		if dest.Status != "failure" || reason == "" {
			continue
		}
		name := dest.LocationName
		if name == "" {
			name = dest.LocationID
		}
		line := name + ": " + reason
		if seen[line] {
			continue
		}
		seen[line] = true
		reasons = append(reasons, line)
	}
	if len(reasons) == 0 {
		return cause
	}
	return errors.New(strings.Join(reasons, "; "))
}

// failBackupRun records a failure with the host's own words.
func (s *Service) failBackupRun(ctx context.Context, runID string, cause error) {
	now := time.Now().UTC()
	detail := hostErrorDetail(cause)
	if detail == "" && cause != nil {
		detail = cause.Error()
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_backup_runs
		   SET status = 'failure', error = ?, progress_stage = 'failed',
		       completed_at = ?, duration_ms = ?, updated_at = ?
		 WHERE id = ?`, detail, now.Format(time.RFC3339),
		s.runDuration(ctx, runID, now), now.Format(time.RFC3339), runID); err != nil {
		s.logger.Error("system containers: recording a failed backup",
			"run", runID, "error", err)
	}

	if detail == "" {
		detail = "failed."
	} else {
		detail = "failed: " + detail
	}
	s.logBackupEvent(ctx, runID, "failed", "failed", "error", detail)
}

// runDuration is how long a run took, in milliseconds.
//
// Zero when the row cannot be read: a duration McHarbor is unsure of is better
// shown as unknown than as a number derived from a guess.
func (s *Service) runDuration(ctx context.Context, runID string, finished time.Time) int64 {
	rows, err := s.db.Query(ctx,
		`SELECT started_at FROM plugin_addon_lxc_backup_runs WHERE id = ?`, runID)
	if err != nil {
		return 0
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0
	}
	var started string
	if err := rows.Scan(&started); err != nil {
		return 0
	}
	at := parseTime(started)
	if at.IsZero() {
		return 0
	}
	elapsed := finished.Sub(at).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

// runDestinations lists where one run's artifacts went.
func (s *Service) runDestinations(ctx context.Context, runID string) ([]BackupRunDestination, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupDestinationColumns+`
		  FROM plugin_addon_lxc_backup_run_destinations
		 WHERE run_id = ? ORDER BY created_at LIMIT ?`, runID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading backup destinations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []BackupRunDestination{}
	for rows.Next() {
		dest, err := scanBackupRunDestination(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dest)
	}
	return out, rows.Err()
}

// ListBackupRuns returns a container's runs, newest first, with destinations.
func (s *Service) ListBackupRuns(ctx context.Context, containerID string, limit int) ([]BackupRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_lxc_backup_runs
		 WHERE container_id = ? ORDER BY started_at DESC LIMIT ?`, containerID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []BackupRun{}
	for rows.Next() {
		run, err := scanBackupRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing backup runs: %w", err)
	}
	for i := range out {
		if out[i].Destinations, err = s.runDestinations(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// BackupRunByID reads one run with its destinations.
func (s *Service) BackupRunByID(ctx context.Context, id string) (BackupRun, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_lxc_backup_runs WHERE id = ?`, id)
	if err != nil {
		return BackupRun{}, fmt.Errorf("reading the backup run: %w", err)
	}
	found := rows.Next()
	var run BackupRun
	if found {
		run, err = scanBackupRun(rows)
	}
	_ = rows.Close()
	if err != nil {
		return BackupRun{}, err
	}
	if !found {
		return BackupRun{}, ErrNotFound
	}
	if run.Destinations, err = s.runDestinations(ctx, run.ID); err != nil {
		return BackupRun{}, err
	}
	return run, nil
}

// DeleteBackupRun removes one run on request.
func (s *Service) DeleteBackupRun(ctx context.Context, id string) error {
	run, err := s.BackupRunByID(ctx, id)
	if err != nil {
		return err
	}
	if run.Status == BackupStatusRunning {
		return invalid(codeBadRequest, "this backup is still running")
	}
	// Refused rather than cascading: deleting a full copy that later
	// incrementals build on makes every one of them unrestorable, and the
	// operator asking for this one deletion cannot see that from here.
	dependents, err := s.dependentRuns(ctx, run)
	if err != nil {
		return err
	}
	if dependents > 0 {
		return fmt.Errorf("%w: %d later backup(s) build on this one",
			ErrChainIncomplete, dependents)
	}
	return s.deleteBackupRun(ctx, run)
}

// dependentRuns counts later incrementals that rest on this run.
func (s *Service) dependentRuns(ctx context.Context, run BackupRun) (int, error) {
	if run.Kind != BackupKindFull || run.ChainID == "" {
		return 0, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT COUNT(*) FROM plugin_addon_lxc_backup_runs
		 WHERE chain_id = ? AND operation = 'backup' AND status = 'success'
		   AND kind = 'incremental' AND started_at > ?`,
		run.ChainID, run.StartedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("counting dependent backups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return 0, rows.Err()
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, fmt.Errorf("counting dependent backups: %w", err)
	}
	return count, rows.Err()
}

// lastGoodBackupAt is when a container's newest successful backup started, or
// the zero time when it has none.
//
// Read without a period bound, because a report window that starts after the
// last good copy must still age it — which is the whole point of the line the
// protection report now prints.
func (s *Service) lastGoodBackupAt(ctx context.Context, containerID string) (time.Time, error) {
	rows, err := s.db.Query(ctx, `
		SELECT started_at FROM plugin_addon_lxc_backup_runs
		 WHERE container_id = ? AND operation = 'backup' AND status = 'success'
		 ORDER BY started_at DESC LIMIT 1`, containerID)
	if err != nil {
		return time.Time{}, fmt.Errorf("reading the last good backup: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return time.Time{}, rows.Err()
	}
	var started string
	if err := rows.Scan(&started); err != nil {
		return time.Time{}, fmt.Errorf("reading the last good backup: %w", err)
	}
	return parseTime(started), rows.Err()
}
