// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/ctsnap"
	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// The two halves of a container backup artifact.
//
// Two objects rather than one outer tar: the manifest is a few kilobytes and
// the rootfs is tens of gigabytes, and wrapping them together would mean
// staging a second copy of the large one purely to prepend the small one.
const (
	ctBackupPartManifest = "manifest"
	ctBackupPartRootfs   = "rootfs"
)

// defaultCTBackupScratch is where artifacts are staged when McHarbor names no
// directory. Under the container runtime root so it moves with the rest of the
// host's container state.
func defaultCTBackupScratch() string {
	return filepath.Join(lxc.RuntimeRoot(), ".backups")
}

// handleCTBackupRun answers ct_backup_run.
//
// The artifact is produced on the host and then streamed to McHarbor, which
// encrypts it in flight and hands it to the destination. The agent therefore
// holds no storage credential and no encryption key, which is the whole reason
// the bytes go via McHarbor rather than straight to the destination.
func (rt *computeRuntime) handleCTBackupRun(ctx context.Context, req *CTPayload, reply *CTPayload) error {
	spec := req.Backup
	if spec == nil {
		return fmt.Errorf("compute: %s carried no backup", MsgCTBackupRun)
	}
	if spec.Upload == nil || spec.Upload.URL == "" || spec.Upload.Token == "" {
		return fmt.Errorf("compute: %s carried nowhere to send the result", MsgCTBackupRun)
	}

	scratch := spec.ScratchDir
	if scratch == "" {
		scratch = defaultCTBackupScratch()
	}

	snapshot, isCopy := rt.ctRootfsSnapshotter(req.ContainerID, spec)
	request := lxc.BackupRequest{
		CTID:        req.ContainerID,
		RunID:       spec.RunID,
		Kind:        spec.Kind,
		Manifest:    spec.Manifest,
		RootfsPath:  spec.RootfsPath,
		ListingName: spec.ListingName,
		ScratchDir:  scratch,
		Snapshot:    snapshot,
		// A dir pool's snapshot is a tree copy taken while the container runs,
		// recorded as a live copy rather than a point-in-time snapshot.
		SnapshotIsCopy: isCopy,
		OnPhase: func(stage, detail string) {
			rt.publishCTBackupPhase(req.ContainerID, spec.RunID, stage, detail)
		},
		OnProgress: func(copied int64) {
			rt.publishCTBackupProgress(req.ContainerID, spec.RunID, "archiving", copied, 0)
		},
	}

	rt.publishCTBackupPhase(req.ContainerID, spec.RunID, "starting", "preparing the rootfs copy")
	result, err := rt.cts.Backup(ctx, request)
	// Staged files go whatever happened: a failed run that leaves a
	// rootfs-sized file behind fills the host, and the next run then fails for
	// a reason that names neither.
	defer cleanupCTStaged(result)
	if err != nil {
		return err
	}

	var uploaded int64
	for _, part := range []struct {
		name string
		path string
	}{
		{ctBackupPartManifest, result.ManifestFile},
		{ctBackupPartRootfs, result.RootfsArchive},
	} {
		if part.path == "" {
			continue
		}
		rt.publishCTBackupProgress(req.ContainerID, spec.RunID, "uploading", uploaded, result.BytesWritten)
		written, upErr := rt.uploadCTBackupPart(ctx, spec, part.name, result.Kind, part.path, uploaded,
			func(total int64) {
				rt.publishCTBackupProgress(req.ContainerID, spec.RunID, "uploading", total, result.BytesWritten)
			})
		if upErr != nil {
			return upErr
		}
		uploaded += written
	}

	reply.BackupResult = &CTBackupResult{
		Kind:           result.Kind,
		BytesWritten:   uploaded,
		ListingName:    result.ListingName,
		ListingPresent: result.ListingPresent,
		FellBackToFull: result.FellBackToFull,
		FallbackReason: result.FallbackReason,
		Consistency:    result.Consistency,
	}
	reply.BackupBytes = uploaded
	reply.Success = true
	return nil
}

// ctRootfsSnapshotter builds the pool-snapshot half of the quiesce decision, or
// nil when this pool cannot take one; isCopy marks a snapshot that copies.
//
// Every backend answers with a directory the archiver can read: btrfs a
// read-only subvolume, zfs the snapshot's .zfs/snapshot path, lvm a thin
// snapshot mounted read-only under ctsnap.MountRoot, and dir a tree copy taken
// with the container frozen. Anything that fails falls through to the freezer.
func (rt *computeRuntime) ctRootfsSnapshotter(
	ctID string,
	spec *CTBackupSpec,
) (snapshot func(context.Context) (string, func(), error), isCopy bool) {
	if _, err := ctsnap.SnapshotName(spec.RunID); err != nil {
		rt.logger.Warn("compute: this backup run cannot name a snapshot; the container will be frozen instead",
			"container", ctID, "run", spec.RunID, "error", err)
		return nil, false
	}
	switch spec.PoolBackend {
	case storage.BackendBtrfs:
		return rt.ctDriverSnapshotter(ctID, spec, "copy-on-write btrfs snapshot"), false
	case storage.BackendDir:
		return rt.ctDriverSnapshotter(ctID, spec, "copy of the rootfs, taken frozen"), true
	case storage.BackendZFS:
		return rt.ctZFSSnapshotter(ctID, spec), false
	case storage.BackendLVM:
		return rt.ctLVMSnapshotter(ctID, spec), false
	default:
		return nil, false
	}
}

// ctDriverSnapshotter snapshots through the pool driver, for the two backends
// whose driver already answers with a readable path beside the rootfs.
func (rt *computeRuntime) ctDriverSnapshotter(
	ctID string,
	spec *CTBackupSpec,
	mechanism string,
) func(context.Context) (string, func(), error) {
	driver, err := rt.pools.Get(spec.PoolBackend)
	if err != nil {
		return nil
	}
	pool := storage.Pool{ID: spec.PoolID, Backend: spec.PoolBackend, Target: spec.PoolTarget}
	name, err := ctsnap.SnapshotName(spec.RunID)
	if err != nil {
		return nil
	}
	mech := spec.PoolBackend
	if spec.PoolBackend == storage.BackendDir {
		// The dir driver's DeleteSnapshot removes a tree only for "copy"; any
		// other mechanism is read as a qcow2 overlay to fold back.
		mech = "copy"
	}
	remove := func(ctx context.Context, path string) {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		defer cancel()
		snap := storage.Snapshot{Handle: path, Mechanism: mech}
		if delErr := driver.DeleteSnapshot(releaseCtx, pool, spec.RootfsPath, snap); delErr != nil {
			rt.logger.Warn("compute: removing a container backup snapshot failed",
				"container", ctID, "snapshot", path, "error", delErr)
		}
	}

	return func(ctx context.Context) (string, func(), error) {
		done := rt.ctSnaps.Add(spec.RunID)
		rt.sweepCTSiblings(ctx, ctID, spec, remove)

		// Named here as well as by the driver, so a copy that failed halfway is
		// still removed: the driver returns no handle for a partial tree.
		expected := spec.RootfsPath + ".snap." + name
		snap, snapErr := driver.Snapshot(ctx, pool, spec.RootfsPath, name)
		if snapErr != nil {
			if !errors.Is(snapErr, storage.ErrExists) {
				remove(ctx, expected)
			}
			done()
			return "", func() {}, snapErr
		}
		info, statErr := os.Stat(snap.Handle)
		if statErr != nil || !info.IsDir() {
			// Not a tree the archiver can read. Removed again rather than left
			// behind, because a snapshot nothing will ever use still occupies
			// the pool.
			remove(ctx, snap.Handle)
			done()
			return "", func() {}, fmt.Errorf("compute: this pool's snapshot of %s is not a readable directory",
				spec.RootfsPath)
		}
		rt.publishCTBackupPhase(ctID, spec.RunID, "quiescing", "reading the rootfs from a "+mechanism)
		return snap.Handle, func() {
			remove(ctx, snap.Handle)
			done()
		}, nil
	}
}

// sweepCTSiblings removes this container's dir or btrfs copies left by a run
// that no longer exists — an agent killed mid-backup — before taking a new one.
func (rt *computeRuntime) sweepCTSiblings(
	ctx context.Context,
	ctID string,
	spec *CTBackupSpec,
	remove func(context.Context, string),
) {
	entries, err := os.ReadDir(filepath.Dir(spec.RootfsPath))
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	for _, stale := range ctsnap.StaleSiblings(spec.RootfsPath, names, rt.ctSnaps) {
		rt.logger.Info("compute: removing a container backup copy a previous run left behind",
			"container", ctID, "path", stale)
		remove(ctx, stale)
	}
}

// ctZFSSnapshotter reads a zfs snapshot through <mountpoint>/.zfs/snapshot, so
// nothing is mounted by McHarbor and there is no mount to leak.
func (rt *computeRuntime) ctZFSSnapshotter(ctID string, spec *CTBackupSpec) func(context.Context) (string, func(), error) {
	return func(ctx context.Context) (string, func(), error) {
		mounts, err := ctsnap.Mounts()
		if err != nil {
			return "", func() {}, err
		}
		plan, err := ctsnap.PlanZFS(ctx, ctsnap.ExecRunner, mounts, spec.RootfsPath, spec.RunID)
		if err != nil {
			rt.logger.Info("compute: this container's zfs snapshot is not readable; freezing instead",
				"container", ctID, "reason", err)
			return "", func() {}, err
		}
		done := rt.ctSnaps.Add(spec.RunID)
		if sweepErr := ctsnap.Sweep(ctx, ctsnap.ExecRunner, rt.ctSnaps,
			ctsnap.SweepOptions{ZFSDataset: plan.Dataset}); sweepErr != nil {
			rt.logger.Warn("compute: sweeping stale container backup snapshots failed", "error", sweepErr)
		}
		release, err := ctsnap.TakeZFS(ctx, ctsnap.ExecRunner, plan)
		if err != nil {
			done()
			return "", func() {}, err
		}
		rt.publishCTBackupPhase(ctID, spec.RunID, "quiescing", "reading the rootfs from zfs snapshot "+plan.Snapshot)
		return plan.Tree, rt.ctSnapshotRelease(ctID, plan.Snapshot, release, done), nil
	}
}

// ctLVMSnapshotter mounts a thin snapshot read-only under ctsnap.MountRoot.
// The mount and the volume are both named for the run, which is what lets a
// restarted agent find and remove them if this one is killed mid-backup.
func (rt *computeRuntime) ctLVMSnapshotter(ctID string, spec *CTBackupSpec) func(context.Context) (string, func(), error) {
	return func(ctx context.Context) (string, func(), error) {
		done := rt.ctSnaps.Add(spec.RunID)
		if sweepErr := ctsnap.Sweep(ctx, ctsnap.ExecRunner, rt.ctSnaps,
			ctsnap.SweepOptions{LVM: true}); sweepErr != nil {
			rt.logger.Warn("compute: sweeping stale container backup snapshots failed", "error", sweepErr)
		}
		mounts, err := ctsnap.Mounts()
		if err != nil {
			done()
			return "", func() {}, err
		}
		plan, err := ctsnap.PlanLVM(ctx, ctsnap.ExecRunner, mounts, spec.RootfsPath, spec.RunID)
		if err != nil {
			rt.logger.Info("compute: this container's lvm snapshot is not readable; freezing instead",
				"container", ctID, "reason", err)
			done()
			return "", func() {}, err
		}
		release, err := ctsnap.TakeLVM(ctx, ctsnap.ExecRunner, plan)
		if err != nil {
			done()
			return "", func() {}, err
		}
		rt.publishCTBackupPhase(ctID, spec.RunID, "quiescing",
			"reading the rootfs from lvm snapshot "+plan.VG+"/"+plan.Snapshot+", mounted read-only")
		return plan.Tree, rt.ctSnapshotRelease(ctID, plan.VG+"/"+plan.Snapshot, release, done), nil
	}
}

// ctSnapshotRelease adapts a ctsnap release to the manager's func(), logging a
// failure rather than losing it. The run leaves the active set either way: a
// snapshot that would not come out is the next sweep's to remove.
func (rt *computeRuntime) ctSnapshotRelease(ctID, snapshot string, release func() error, done func()) func() {
	return func() {
		defer done()
		if err := release(); err != nil {
			rt.logger.Warn("compute: releasing a container backup snapshot failed; the next sweep will retry",
				"container", ctID, "snapshot", snapshot, "error", err)
		}
	}
}

// sweepCTSnapshots removes what a killed agent left behind: mounts under
// ctsnap.MountRoot, snapshot volumes and zfs snapshots named for a backup run.
// Run once when the runtime is built, when no run of this process can hold one.
func (rt *computeRuntime) sweepCTSnapshots(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	if err := ctsnap.Sweep(ctx, ctsnap.ExecRunner, rt.ctSnaps,
		ctsnap.SweepOptions{LVM: true, AllZFS: true}); err != nil {
		rt.logger.Warn("compute: sweeping stale container backup snapshots failed", "error", err)
	}
}

// sweepCTStaging releases the staged rootfs mounts a killed agent left behind.
func (rt *computeRuntime) sweepCTStaging(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	if err := rt.cts.SweepStaging(ctx); err != nil {
		rt.logger.Warn("compute: releasing stale staged container rootfs mounts failed", "error", err)
	}
}

// cleanupCTStaged removes what a run staged on the host.
func cleanupCTStaged(result lxc.BackupResult) {
	for _, path := range []string{result.RootfsArchive, result.ManifestFile} {
		if path != "" {
			errlog.Log(os.Remove(path))
		}
	}
}

// uploadCTBackupPart streams one staged file to McHarbor.
//
// It reuses the import path's client rather than building one: McHarbor is
// commonly reached on a LAN address its certificate does not cover, and that
// client already carries the one accepted exception plus a deadline.
func (rt *computeRuntime) uploadCTBackupPart(
	ctx context.Context,
	spec *CTBackupSpec,
	part, kind, path string,
	alreadyUploaded int64,
	progress func(int64),
) (int64, error) {
	file, err := os.Open(path)
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
	url := rt.absoluteServerURL(spec.Upload.URL)
	if url == "" {
		return 0, fmt.Errorf("compute: the backup upload URL could not be resolved against this host's McHarbor address")
	}

	counted := &progressReader{inner: file, base: alreadyUploaded, report: progress}
	request, err := http.NewRequestWithContext(uploadCtx, http.MethodPut, url, counted)
	if err != nil {
		return 0, fmt.Errorf("compute: building the backup upload: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+spec.Upload.Token)
	request.Header.Set("Content-Type", "application/octet-stream")
	// Named on the request rather than in the path so one grant covers both
	// parts of a run: the grant is what authorises, and a per-part grant would
	// multiply the one-shot secrets in flight for no gain.
	request.Header.Set("X-McHarbor-Part", part)
	request.Header.Set("X-McHarbor-Backup-Kind", kind)
	request.ContentLength = info.Size()

	response, err := rt.importClient().Do(request)
	if err != nil {
		return 0, fmt.Errorf("compute: uploading the backup: %w", err)
	}
	defer func() { errlog.Close(response.Body) }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return 0, uploadRefusal(response.Status, response.Body)
	}
	return info.Size(), nil
}

// handleCTBackupRestore answers ct_backup_restore.
//
// The chain is downloaded in full before anything is applied, and the tree is
// rebuilt beside the target rather than over it, so every step before the final
// move can fail leaving the container exactly as it was.
func (rt *computeRuntime) handleCTBackupRestore(ctx context.Context, req *CTPayload, reply *CTPayload) error {
	spec := req.Restore
	if spec == nil {
		return fmt.Errorf("compute: %s carried no restore", MsgCTBackupRestore)
	}
	if len(spec.Artifacts) == 0 {
		return fmt.Errorf("compute: %s carried nothing to restore", MsgCTBackupRestore)
	}

	scratch := spec.ScratchDir
	if scratch == "" {
		scratch = filepath.Join(defaultCTBackupScratch(), "restore-"+spec.RunID)
	}
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		return fmt.Errorf("compute: preparing the restore scratch directory: %w", err)
	}
	defer func() { errlog.Log(os.RemoveAll(scratch)) }()

	rt.publishCTBackupProgress(req.ContainerID, spec.RunID, "downloading", 0, 0)
	links, err := rt.downloadCTChain(ctx, req.ContainerID, scratch, spec)
	if err != nil {
		return err
	}

	rt.publishCTBackupProgress(req.ContainerID, spec.RunID, "rebuilding", 0, 0)
	result, err := rt.cts.RestoreRootfs(ctx, lxc.RestoreRequest{
		CTID:        req.ContainerID,
		RunID:       spec.RunID,
		TargetPath:  spec.TargetPath,
		Links:       links,
		ScratchDir:  scratch,
		SourceIDMap: ctIDMap(spec.SourceIDMap),
		TargetIDMap: ctIDMap(spec.TargetIDMap),
		OnPhase: func(stage, detail string) {
			rt.publishCTBackupPhase(req.ContainerID, spec.RunID, stage, detail)
		},
	})
	if err != nil {
		return err
	}
	if result.IDShift == lxc.IDShiftApplied {
		rt.logger.Info("compute: restored rootfs re-owned into the container's id range",
			"container", req.ContainerID, "run", spec.RunID, "entries", result.EntriesMoved)
	}

	acknowledgeRestore(reply, result)
	return nil
}

// acknowledgeRestore answers a finished restore with what it did about
// ownership. McHarbor waits for this: an agent that predates re-owning sends
// none, and a restore into another id range is then reported failed rather
// than finished with every file owned by the wrong range.
func acknowledgeRestore(reply *CTPayload, result lxc.RestoreResult) {
	reply.BackupResult = &CTBackupResult{IDShift: result.IDShift}
	reply.Success = true
}

// ctIDMap converts the wire range to the manager's, keeping nil as "not sent".
func ctIDMap(in *CTIDMap) *lxc.IDMap {
	if in == nil {
		return nil
	}
	return &lxc.IDMap{
		Privileged: in.Privileged,
		UIDStart:   in.UIDStart, UIDCount: in.UIDCount,
		GIDStart: in.GIDStart, GIDCount: in.GIDCount,
	}
}

// downloadCTChain pulls every rootfs link of a chain, in the order it has to be
// applied in.
//
// The manifest parts are skipped: they describe the container rather than its
// filesystem, and McHarbor already holds the definition it is restoring into.
// Sorting here rather than trusting the slice is what stops an incremental
// being applied before the full it builds on.
func (rt *computeRuntime) downloadCTChain(
	ctx context.Context,
	ctID, scratch string,
	spec *CTRestoreSpec,
) ([]string, error) {
	wanted := make([]CTRestoreArtifactRef, 0, len(spec.Artifacts))
	for _, artifact := range spec.Artifacts {
		if artifact.Part == ctBackupPartManifest {
			continue
		}
		wanted = append(wanted, artifact)
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("compute: the restore of %s carried no rootfs archive", ctID)
	}
	sort.SliceStable(wanted, func(i, j int) bool { return wanted[i].Sequence < wanted[j].Sequence })

	out := make([]string, 0, len(wanted))
	for index, artifact := range wanted {
		target := filepath.Join(scratch,
			fmt.Sprintf("%03d-%s.tar.gz", artifact.Sequence, artifact.Kind))
		if err := rt.downloadCTArtifact(ctx, artifact, target); err != nil {
			return nil, err
		}
		out = append(out, target)
		rt.publishCTBackupProgress(ctID, spec.RunID, "downloading", int64(index+1), int64(len(wanted)))
	}
	return out, nil
}

// downloadCTArtifact pulls one link straight to disk.
//
// Never through memory: these are whole filesystems, and there is no size an
// agent can assume is safe to buffer.
func (rt *computeRuntime) downloadCTArtifact(ctx context.Context, artifact CTRestoreArtifactRef, target string) error {
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

// publishCTBackupProgress reports a run's stage and byte count.
//
// Its own message rather than the VM event channel: the guest kind is part of
// an event's identity, and a container reported as a machine reaches the wrong
// plugin.
func (rt *computeRuntime) publishCTBackupProgress(ctID, runID, stage string, bytes, total int64) {
	detail := map[string]string{
		"runId": runID,
		"stage": stage,
		"bytes": fmt.Sprintf("%d", bytes),
	}
	if total > 0 {
		detail["total"] = fmt.Sprintf("%d", total)
	}
	rt.sendCTBackupProgress(ctID, detail)
}

// publishCTBackupPhase names the step a run is on before any bytes move.
//
// Everything between accepting the request and the first archive write — the
// quiesce, the listing check, the manifest — is silent otherwise, and a run
// stuck in one of them looks exactly like a run stuck in another.
func (rt *computeRuntime) publishCTBackupPhase(ctID, runID, stage, detail string) {
	rt.sendCTBackupProgress(ctID, map[string]string{
		"runId":  runID,
		"stage":  stage,
		"detail": detail,
	})
}

// sendCTBackupProgress writes one progress frame, dropping it rather than
// failing the run it is reporting on.
func (rt *computeRuntime) sendCTBackupProgress(ctID string, detail map[string]string) {
	if rt.send == nil {
		return
	}
	if err := rt.send(WSMessage{
		Type: MsgCTBackupProgress,
		CT: &CTPayload{
			Operation:   "backup_progress",
			ContainerID: ctID,
			Progress:    detail,
		},
	}); err != nil {
		rt.logger.Debug("publishing container backup progress failed",
			"container", ctID, "stage", detail["stage"], "error", err)
	}
}
