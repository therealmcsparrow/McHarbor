// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// backupUploadTimeout bounds streaming one artifact to McHarbor.
//
// A full copy of a large guest crosses the network here, so this is hours
// rather than minutes. It is bounded at all because a McHarbor that stopped
// reading would otherwise leave the request open indefinitely.
const backupUploadTimeout = 12 * time.Hour

// defaultBackupScratch is where artifacts are staged when McHarbor names no
// directory. Under the compute runtime root so it moves with the rest of the
// host's compute state.
func defaultBackupScratch() string {
	return filepath.Join(vm.RuntimeRoot(), ".backups")
}

// handleVMBackupRun answers vm_backup_run.
//
// The guest is quiesced here rather than inside the vm package because the
// freeze goes through the guest agent, which this layer owns — and because
// the thaw must cover the upload as well as the copy on the failure paths.
func (rt *computeRuntime) handleVMBackupRun(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	spec := req.Backup
	if spec == nil {
		return fmt.Errorf("compute: %s carried no backup", MsgVMBackupRun)
	}
	if spec.Upload == nil || spec.Upload.URL == "" || spec.Upload.Token == "" {
		return fmt.Errorf("compute: %s carried nowhere to send the result", MsgVMBackupRun)
	}

	scratch := spec.ScratchDir
	if scratch == "" {
		scratch = defaultBackupScratch()
	}

	request := vm.BackupRequest{
		VMID:       req.VMID,
		RunID:      spec.RunID,
		Kind:       spec.Kind,
		ScratchDir: scratch,
	}
	// Throttled to one event a second: query-block-jobs is polled four times
	// that fast, and a websocket message per poll per disk would cost more
	// than the copy it is reporting on.
	var lastProgress atomic.Int64
	request.OnProgress = func(diskID string, copied, total int64) {
		now := time.Now().UnixMilli()
		previous := lastProgress.Load()
		if now-previous < 1000 || !lastProgress.CompareAndSwap(previous, now) {
			return
		}
		rt.publishBackupCopyProgress(req.VMID, spec.RunID, diskID, copied, total)
	}
	for _, disk := range spec.Disks {
		request.Disks = append(request.Disks, vm.BackupDisk{
			DiskID:     disk.DiskID,
			VolumePath: disk.VolumePath,
			NodeName:   disk.NodeName,
			BitmapName: disk.BitmapName,
		})
	}

	// Quiesce around the copy only. Holding the guest's filesystems frozen
	// for the upload as well would freeze a busy guest for however long the
	// network takes, which is the one thing a backup must not do.
	consistency := "crash"
	if spec.Quiesce {
		rt.publishBackupPhase(req.VMID, spec.RunID, "quiescing",
			"asking the guest agent to freeze the filesystems")
		// Bounded on its own. A guest whose agent has stopped answering must
		// cost a minute and a crash-consistent copy, not the whole run's
		// budget: the request's deadline is hours, and a backup that never
		// starts is worse than one taken without quiescing.
		freezeCtx, cancelFreeze := context.WithTimeout(ctx, guestFreezeTimeout)
		var thaw func()
		consistency, thaw = rt.freezeGuestFilesystems(freezeCtx, req.VMID)
		cancelFreeze()
		defer thaw()
	}

	request.OnPhase = func(diskID, phase, detail string) {
		rt.publishBackupPhase(req.VMID, spec.RunID, phase, detail)
	}

	rt.publishBackupProgress(req.VMID, spec.RunID, "copying", 0)
	results, err := rt.vms.BackupDisks(ctx, request)
	// Staged files are removed whatever happened: a failed run that leaves a
	// guest-sized file behind fills the host and the next run fails for a
	// reason that names neither.
	defer cleanupStaged(results)
	if err != nil {
		return err
	}

	if spec.Quiesce {
		// Thaw before the upload rather than at return. The deferred thaw
		// above stays as the safety net for the error paths.
		rt.thawAfterCopy(ctx, req.VMID)
	}
	// A stopped guest's disk was copied at rest, so nothing was in flight to
	// be lost; reporting "crash" would warn about a quiesce that had nothing
	// to do.
	if len(results) > 0 && results[0].Offline {
		consistency = "filesystem"
	}

	var uploaded int64
	for _, result := range results {
		if result.StagedPath == "" {
			continue
		}
		rt.publishBackupProgress(req.VMID, spec.RunID, "uploading", uploaded)
		written, upErr := rt.uploadBackupArtifact(ctx, spec.Upload, result, uploaded,
			func(total int64) {
				rt.publishBackupProgress(req.VMID, spec.RunID, "uploading", total)
			})
		if upErr != nil {
			return upErr
		}
		uploaded += written
	}

	for _, result := range results {
		reply.BackupResult = append(reply.BackupResult, VMBackupDiskResult{
			DiskID:         result.DiskID,
			Kind:           result.Kind,
			BytesWritten:   result.BytesWritten,
			BitmapName:     result.BitmapName,
			BitmapPresent:  result.BitmapPresent,
			FellBackToFull: result.FellBackToFull,
			FallbackReason: result.FallbackReason,
			Consistency:    consistency,
		})
	}
	reply.BackupBytes = uploaded
	reply.Success = true
	return nil
}

// thawAfterCopy releases the guest once the copy is done.
//
// Best effort and idempotent: the deferred thaw in the caller runs too, and
// thawing an already-thawed guest is a no-op rather than an error.
func (rt *computeRuntime) thawAfterCopy(ctx context.Context, vmID string) {
	thawCtx, cancel := context.WithTimeout(ctx, guestQueryTimeout)
	defer cancel()
	errlog.Log(rt.handleGuestQuery(thawCtx, &VMPayload{VMID: vmID, GuestQuery: "fsthaw"}, &VMPayload{}))
}

// cleanupStaged removes every artifact a run staged.
func cleanupStaged(results []vm.BackupDiskResult) {
	for _, result := range results {
		if result.StagedPath != "" {
			errlog.Log(os.Remove(result.StagedPath))
		}
	}
}

// uploadBackupArtifact streams one staged file to McHarbor.
//
// The body is the file itself; McHarbor encrypts it in flight and fans it out
// to the configured destinations. The agent therefore holds no storage
// credential and no encryption key — the reason the artifact goes here rather
// than straight to the destination.
//
// It reuses the import path's client rather than building one: McHarbor is
// commonly reached on a LAN address its certificate does not cover, and that
// client already carries the one accepted exception plus a deadline.
func (rt *computeRuntime) uploadBackupArtifact(
	ctx context.Context,
	upload *VMBackupUploadSpec,
	result vm.BackupDiskResult,
	alreadyUploaded int64,
	progress func(int64),
) (int64, error) {
	file, err := os.Open(result.StagedPath)
	if err != nil {
		return 0, fmt.Errorf("compute: opening the staged backup: %w", err)
	}
	defer func() { errlog.Close(file) }()

	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("compute: sizing the staged backup: %w", err)
	}

	uploadCtx, cancel := context.WithTimeout(ctx, backupUploadTimeout)
	defer cancel()

	// McHarbor sends a path, not an absolute URL: only the agent knows which
	// address it actually reached its McHarbor on, and that is the one the
	// upload has to go back to.
	url := rt.absoluteServerURL(upload.URL)
	if url == "" {
		return 0, fmt.Errorf("compute: the backup upload URL could not be resolved against this host's McHarbor address")
	}

	counted := &progressReader{inner: file, base: alreadyUploaded, report: progress}
	request, err := http.NewRequestWithContext(uploadCtx, http.MethodPut, url, counted)
	if err != nil {
		return 0, fmt.Errorf("compute: building the backup upload: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+upload.Token)
	request.Header.Set("Content-Type", "application/octet-stream")
	// Named on the request rather than in the path so one grant covers every
	// disk in a run: the grant is what authorises, and a per-disk grant would
	// multiply the number of one-shot secrets in flight for no gain.
	request.Header.Set("X-McHarbor-Disk", result.DiskID)
	request.Header.Set("X-McHarbor-Backup-Kind", result.Kind)
	request.ContentLength = info.Size()

	response, err := rt.importClient().Do(request)
	if err != nil {
		return 0, fmt.Errorf("compute: uploading the backup: %w", err)
	}
	defer func() { errlog.Close(response.Body) }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return 0, fmt.Errorf("compute: McHarbor refused the backup upload with %s", response.Status)
	}
	return info.Size(), nil
}

// progressReader reports how far an upload has got.
type progressReader struct {
	inner      *os.File
	base       int64
	sent       int64
	report     func(int64)
	lastReport time.Time
}

// uploadProgressInterval spaces upload progress events. Every Read reporting
// overflowed McHarbor's guest event queue within a second of the upload start.
const uploadProgressInterval = time.Second

// Read passes bytes through and reports the running total across the run.
func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		r.sent += int64(n)
		if r.report != nil && (err != nil || time.Since(r.lastReport) >= uploadProgressInterval) {
			r.lastReport = time.Now()
			r.report(r.base + r.sent)
		}
	}
	return n, err
}

// publishBackupProgress reports a run's stage through the guest event channel
// the server already subscribes to.
// publishBackupCopyProgress reports how far the running copy has got.
//
// Separate from publishBackupProgress because the copy is the phase that takes
// the time, and the operator needs a moving number rather than a stage name
// that does not change for twenty minutes.
func (rt *computeRuntime) publishBackupCopyProgress(vmID, runID, diskID string, copied, total int64) {
	rt.publishVMEvent(vm.Event{
		VMID: vmID,
		Kind: "backup_progress",
		At:   time.Now().UTC(),
		Detail: map[string]string{
			"runId":  runID,
			"stage":  "copying",
			"diskId": diskID,
			"bytes":  fmt.Sprintf("%d", copied),
			"total":  fmt.Sprintf("%d", total),
		},
	})
}

// guestFreezeTimeout bounds the quiesce handshake.
const guestFreezeTimeout = 60 * time.Second

// publishBackupPhase names the step a run is on before any bytes move.
//
// Everything between accepting the request and the first block-job poll --
// the quiesce, the QMP connect, the bitmap check, creating the target -- is
// silent otherwise, and a run stuck in one of them looks exactly like a run
// stuck in another.
func (rt *computeRuntime) publishBackupPhase(vmID, runID, phase, detail string) {
	rt.publishVMEvent(vm.Event{
		VMID: vmID,
		Kind: "backup_progress",
		At:   time.Now().UTC(),
		Detail: map[string]string{
			"runId":  runID,
			"stage":  phase,
			"detail": detail,
		},
	})
}

func (rt *computeRuntime) publishBackupProgress(vmID, runID, stage string, bytes int64) {
	rt.publishVMEvent(vm.Event{
		VMID: vmID,
		Kind: "backup_progress",
		At:   time.Now().UTC(),
		Detail: map[string]string{
			"runId": runID,
			"stage": stage,
			"bytes": fmt.Sprintf("%d", bytes),
		},
	})
}

// qcow2Overhead is the slack added to a rebuilt image's expected size.
//
// The rebuilt disk holds the chain's data plus qcow2 metadata -- L2 and
// refcount tables, a few MiB for a large disk -- and `qemu-img convert` can
// round clusters up. A tenth with a 256 MiB floor covers both without
// reintroducing the virtual-size estimate this replaced.
func qcow2Overhead(dataBytes int64) int64 {
	slack := dataBytes / 10
	if slack < 256<<20 {
		slack = 256 << 20
	}
	return slack
}

// handleVMBackupRestore answers vm_backup_restore.
//
// The order matters: space is checked, then the chain is downloaded, then the
// disks are rebuilt beside their targets, and only then is anything moved into
// place. Every step before the move can fail leaving the machine exactly as it
// was.
func (rt *computeRuntime) handleVMBackupRestore(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	spec := req.Restore
	if spec == nil {
		return fmt.Errorf("compute: %s carried no restore", MsgVMBackupRestore)
	}
	if len(spec.Disks) == 0 || len(spec.Artifacts) == 0 {
		return fmt.Errorf("compute: %s carried nothing to restore", MsgVMBackupRestore)
	}

	scratch := spec.ScratchDir
	if scratch == "" {
		scratch = filepath.Join(defaultBackupScratch(), "restore-"+spec.RunID)
	}
	defer func() { errlog.Log(os.RemoveAll(scratch)) }()

	// Checked before a byte crosses the network. A restore that runs out of
	// room an hour in has already cost the operator the machine's downtime.
	//
	// Sized from the artifacts, not from the disk's virtual size. A qcow2
	// rebuilt from a chain holds exactly the chain's data, so a 1 GiB backup
	// of a 40 GiB disk rebuilds to about 1 GiB -- demanding 40 refuses a
	// restore that would comfortably have fit.
	var chainBytes int64
	for _, artifact := range spec.Artifacts {
		chainBytes += artifact.SizeBytes
	}

	var required, downloads int64
	if chainBytes > 0 {
		downloads = chainBytes
		required = chainBytes + qcow2Overhead(chainBytes)
	} else {
		// A host too old to send sizes. Fall back to the virtual size on both
		// counts: over-cautious, and the only safe direction to be wrong in.
		for _, disk := range spec.Disks {
			if size, err := currentDiskSize(ctx, disk.VolumePath); err == nil {
				required += size
			}
		}
		downloads = required
	}
	if _, err := vm.CheckRestoreSpace(scratch, required, downloads); err != nil {
		return err
	}

	rt.publishBackupProgress(req.VMID, spec.RunID, "downloading", 0)
	links, err := rt.downloadChain(ctx, scratch, spec)
	if err != nil {
		return err
	}

	rt.publishBackupProgress(req.VMID, spec.RunID, "rebuilding", 0)
	request := vm.RestoreRequest{
		VMID: req.VMID, RunID: spec.RunID, ScratchDir: scratch, Links: links,
	}
	for _, disk := range spec.Disks {
		request.Disks = append(request.Disks, vm.RestoreDisk{
			DiskID: disk.DiskID, VolumePath: disk.VolumePath,
		})
	}
	if err := rt.vms.RestoreDisks(ctx, request); err != nil {
		return err
	}

	reply.Success = true
	return nil
}

// downloadChain pulls every link of a chain to the scratch directory.
func (rt *computeRuntime) downloadChain(
	ctx context.Context,
	scratch string,
	spec *VMRestoreSpec,
) ([]vm.RestoreLink, error) {
	out := make([]vm.RestoreLink, 0, len(spec.Artifacts))
	for index, artifact := range spec.Artifacts {
		target := filepath.Join(scratch,
			fmt.Sprintf("%s-%03d-%s.qcow2", artifact.DiskID, artifact.Sequence, artifact.Kind))
		if err := rt.downloadArtifact(ctx, artifact, target); err != nil {
			return nil, err
		}
		out = append(out, vm.RestoreLink{
			DiskID:   artifact.DiskID,
			Kind:     artifact.Kind,
			Path:     target,
			Sequence: artifact.Sequence,
		})
		rt.publishBackupProgress("", spec.RunID, "downloading", int64(index+1))
	}
	return out, nil
}

// downloadArtifact pulls one link straight to disk.
//
// Never through memory: these are disk images, and there is no size an agent
// can assume is safe to buffer.
func (rt *computeRuntime) downloadArtifact(ctx context.Context, artifact VMRestoreArtifactRef, target string) error {
	url := rt.absoluteServerURL(artifact.URL)
	if url == "" {
		return fmt.Errorf("compute: the backup download URL could not be resolved against this host's McHarbor address")
	}

	downloadCtx, cancel := context.WithTimeout(ctx, backupUploadTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("compute: building the backup download: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+artifact.Token)

	response, err := rt.importClient().Do(request)
	if err != nil {
		return fmt.Errorf("compute: downloading the backup: %w", err)
	}
	defer func() { errlog.Close(response.Body) }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("compute: McHarbor refused the backup download with %s", response.Status)
	}

	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("compute: creating the downloaded backup: %w", err)
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		errlog.Close(file)
		return fmt.Errorf("compute: writing the downloaded backup: %w", err)
	}
	return file.Close()
}

// currentDiskSize reads a disk's virtual size, for the space pre-flight.
func currentDiskSize(ctx context.Context, path string) (int64, error) {
	binary, err := exec.LookPath("qemu-img")
	if err != nil {
		return 0, err
	}
	runCtx, cancel := context.WithTimeout(ctx, computeQuickTimeout)
	defer cancel()

	// -U for the same reason as virtualSizeOf: the guest holds a write lock on
	// its own disk while it runs.
	out, err := exec.CommandContext(runCtx, binary, "info", "-U", "--output=json", path).CombinedOutput()
	if err != nil {
		if detail := strings.TrimSpace(string(out)); detail != "" {
			return 0, fmt.Errorf("qemu-img info: %w: %s", err, detail)
		}
		return 0, fmt.Errorf("qemu-img info: %w", err)
	}
	var info struct {
		VirtualSize int64 `json:"virtual-size"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return 0, err
	}
	return info.VirtualSize, nil
}
