// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package vm

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Backup kinds, as McHarbor asks for them and as the agent reports back.
const (
	BackupKindFull        = "full"
	BackupKindIncremental = "incremental"
)

// backupJobTimeout bounds one disk's copy.
//
// Generous because a full copy of a large guest legitimately takes hours; it
// is bounded at all because a wedged backing store would otherwise hold the
// guest's operation lock forever.
const backupJobTimeout = 12 * time.Hour

// BackupDisk is one disk to copy.
type BackupDisk struct {
	DiskID     string
	VolumePath string
	NodeName   string
	BitmapName string
}

// BackupRequest is one backup run on one guest.
type BackupRequest struct {
	VMID       string
	RunID      string
	Kind       string
	ScratchDir string
	Disks      []BackupDisk
	// OnProgress is called every poll of the running copy, per disk. Optional,
	// and called from the copying goroutine, so the implementation must not
	// block.
	OnProgress func(diskID string, copied, total int64)
	// OnPhase names each step before the copy starts. Same contract.
	OnPhase func(diskID, phase, detail string)
}

// phase reports a step, tolerating a request that set no reporter.
func (r BackupRequest) phase(diskID, name, detail string) {
	if r.OnPhase != nil {
		r.OnPhase(diskID, name, detail)
	}
}

// BackupDiskResult is what actually happened to one disk.
//
// Kind is what was taken, which is not always what was asked for: only this
// side can see whether the bitmap still exists, so a downgrade is decided
// here and reported rather than assumed by the caller.
type BackupDiskResult struct {
	DiskID         string
	Kind           string
	StagedPath     string
	BytesWritten   int64
	BitmapName     string
	BitmapPresent  bool
	FellBackToFull bool
	FallbackReason string
	Consistency    string
	// Offline says the guest was stopped and the disk was copied at rest,
	// which is a cleaner copy than a quiesced one rather than a worse one.
	Offline bool
}

// BackupDisks copies a guest's disks into ScratchDir and reports what it did.
//
// The caller streams the staged files away and removes them; this returns
// paths rather than readers because the copy and the upload are separately
// long and separately failable, and interleaving them would mean a slow
// destination holding the guest's filesystems frozen.
func (m *Manager) BackupDisks(ctx context.Context, req BackupRequest) ([]BackupDiskResult, error) {
	if len(req.Disks) == 0 {
		return nil, fmt.Errorf("vm: backup %s named no disks", req.RunID)
	}

	lock := m.lockFor(req.RunID)
	lock.Lock()
	defer lock.Unlock()

	if err := os.MkdirAll(req.ScratchDir, 0o700); err != nil {
		return nil, fmt.Errorf("vm: preparing the backup scratch directory: %w", err)
	}

	req.phase("", "connecting", "opening the guest's QMP monitor")
	monitor, err := m.Monitor(ctx, req.VMID)
	if errors.Is(err, ErrNotFound) {
		return backupDisksOffline(ctx, req)
	}
	if err != nil {
		return nil, err
	}

	req.phase("", "planning", "reading each disk's change log")

	// Decided for every disk before anything is copied. A run where one disk
	// is incremental and another silently fell back is a run whose restore
	// path differs per disk, so the whole picture is settled first.
	plans := make([]diskPlan, 0, len(req.Disks))
	for _, disk := range req.Disks {
		plans = append(plans, planDiskBackup(ctx, monitor, disk, req.Kind))
	}

	results := make([]BackupDiskResult, 0, len(plans))
	for _, plan := range plans {
		result, err := m.backupOneDisk(ctx, monitor, req, plan)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// diskPlan is one disk's settled decision.
type diskPlan struct {
	disk           BackupDisk
	kind           string
	bitmapPresent  bool
	fellBackToFull bool
	fallbackReason string
}

// planDiskBackup decides full or incremental for one disk, conservatively.
//
// Every path that cannot prove the bitmap is usable downgrades to a full
// copy. That asymmetry is deliberate: a needless full copy costs time, and an
// incremental against a bitmap that is missing, inconsistent or was rebuilt
// at a different granularity produces an archive that restores to a corrupt
// disk without ever reporting an error.
func planDiskBackup(ctx context.Context, monitor *qmp.Monitor, disk BackupDisk, requested string) diskPlan {
	plan := diskPlan{disk: disk, kind: BackupKindFull}

	if requested != BackupKindIncremental || disk.BitmapName == "" {
		return plan
	}

	bitmap, found, err := monitor.Bitmap(ctx, disk.NodeName, disk.BitmapName)
	switch {
	case err != nil:
		plan.fellBackToFull = true
		plan.fallbackReason = "the host could not read this disk's change log"
		return plan
	case !found:
		plan.fellBackToFull = true
		plan.fallbackReason = "this disk's change log is gone, so what changed since the last backup is unknown"
		return plan
	case bitmap.Inconsistent:
		plan.fellBackToFull = true
		plan.fallbackReason = "this disk's change log did not survive the last shutdown and cannot be trusted"
		return plan
	case !bitmap.Recording:
		plan.fellBackToFull = true
		plan.fallbackReason = "this disk's change log stopped recording, so writes since then were not tracked"
		return plan
	}

	plan.kind = BackupKindIncremental
	plan.bitmapPresent = true
	return plan
}

// backupOneDisk produces one disk's artifact.
func (m *Manager) backupOneDisk(
	ctx context.Context,
	monitor *qmp.Monitor,
	req BackupRequest,
	plan diskPlan,
) (BackupDiskResult, error) {
	result := BackupDiskResult{
		DiskID:         plan.disk.DiskID,
		Kind:           plan.kind,
		BitmapName:     plan.disk.BitmapName,
		BitmapPresent:  plan.bitmapPresent,
		FellBackToFull: plan.fellBackToFull,
		FallbackReason: plan.fallbackReason,
	}

	// A full run is where a chain starts, so the bitmap is created here and
	// begins recording from this point — which is exactly what makes the next
	// run's delta correct.
	if plan.kind == BackupKindFull && plan.disk.BitmapName != "" {
		_, found, bitmapErr := monitor.Bitmap(ctx, plan.disk.NodeName, plan.disk.BitmapName)
		errlog.Log(bitmapErr)
		if found {
			// A stale bitmap from an invalidated chain would otherwise
			// accumulate dirt from before this full copy.
			errlog.Log(monitor.RemoveBitmap(ctx, plan.disk.NodeName, plan.disk.BitmapName))
		}
		if err := monitor.AddBitmap(ctx, plan.disk.NodeName, plan.disk.BitmapName); err != nil {
			return result, err
		}
		result.BitmapPresent = true
	}

	target := filepath.Join(req.ScratchDir,
		fmt.Sprintf("%s-%s.qcow2", req.RunID, plan.disk.DiskID))
	errlog.Log(os.Remove(target))

	targetNode := backupNodeName(plan.disk.DiskID)
	req.phase(plan.disk.DiskID, "preparing", "creating the staging image")
	if err := createBackupTarget(ctx, target, plan.disk.VolumePath); err != nil {
		return result, err
	}
	req.phase(plan.disk.DiskID, "attaching", "attaching the staging image to the guest")
	// The node name is fixed per disk, so a node an earlier run failed to
	// detach would refuse this attach as a duplicate until the guest restarted.
	detachBackupNode(ctx, monitor, targetNode)
	if err := monitor.AddBlockdevFile(ctx, targetNode, target, "qcow2"); err != nil {
		errlog.Log(os.Remove(target))
		return result, err
	}
	defer detachBackupNode(ctx, monitor, targetNode)

	jobCtx, cancel := context.WithTimeout(ctx, backupJobTimeout)
	defer cancel()

	jobID := "backup-" + req.RunID + "-" + plan.disk.DiskID
	sync := plan.kind
	req.phase(plan.disk.DiskID, "starting", "asking QEMU to start the copy")
	if err := monitor.BlockdevBackup(jobCtx, jobID,
		plan.disk.NodeName, targetNode, sync, plan.disk.BitmapName); err != nil {
		errlog.Log(os.Remove(target))
		return result, err
	}
	if err := monitor.AwaitBlockJob(jobCtx, jobID, false, func(offset, total int64) {
		if req.OnProgress != nil {
			req.OnProgress(plan.disk.DiskID, offset, total)
		}
	}); err != nil {
		errlog.Log(os.Remove(target))
		return result, fmt.Errorf("vm: backing up %s: %w", plan.disk.DiskID, err)
	}

	info, err := os.Stat(target)
	if err != nil {
		return result, fmt.Errorf("vm: the backup of %s was not written: %w", plan.disk.DiskID, err)
	}
	result.StagedPath = target
	result.BytesWritten = info.Size()
	return result, nil
}

// createBackupTarget allocates the destination image.
//
// Sized from the source so a full copy has somewhere to land; an incremental
// writes only its dirty clusters into the same virtual size, which is what
// lets the chain be applied over the full without any resizing at restore.
func createBackupTarget(ctx context.Context, target, source string) error {
	size, err := virtualSizeOf(ctx, source)
	if err != nil {
		return err
	}
	if _, err := runImgTool(ctx, "create", "-f", "qcow2", target,
		strconv.FormatInt(size, 10)); err != nil {
		return fmt.Errorf("vm: creating the backup target: %w", err)
	}
	return nil
}

// virtualSizeOf reads a disk's virtual size.
//
// The backup target must match it exactly: an incremental writes only its
// dirty clusters into that same virtual size, which is what lets the chain be
// applied over the full copy at restore without resizing anything.
func virtualSizeOf(ctx context.Context, path string) (int64, error) {
	// -U is mandatory here. The guest being backed up is running, and it holds
	// a write lock on its own disk; without --force-share qemu-img refuses to
	// open the image at all and the backup fails before it starts.
	out, err := runImgTool(ctx, "info", "-U", "--output=json", path)
	if err != nil {
		return 0, fmt.Errorf("vm: reading the size of %s: %w", path, err)
	}
	var info struct {
		VirtualSize int64 `json:"virtual-size"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return 0, fmt.Errorf("vm: reading the size of %s: %w", path, err)
	}
	if info.VirtualSize <= 0 {
		return 0, fmt.Errorf("vm: %s reports no size", path)
	}
	return info.VirtualSize, nil
}

// backupNodeName is the QMP node name for one disk's staging image.
//
// Hashed rather than built from the disk id: an xid is 20 characters, and
// "backup-<xid>-file" -- the node QEMU derives for the protocol layer -- comes
// to 32, one over the limit. It is deterministic so a retry and the cleanup
// that follows it name the same node.
func backupNodeName(diskID string) string {
	sum := sha256.Sum256([]byte(diskID))
	return "mcbk" + hex.EncodeToString(sum[:6])
}

// offlineIncrementalReason is why a stopped guest's requested incremental
// became a full copy.
const offlineIncrementalReason = "the machine was stopped, so its disks were copied in full; the next run while it is running can be incremental again"

// backupDisksOffline copies a stopped guest's disks with qemu-img.
//
// Always full: the change log lives in QEMU's block layer, and nothing here
// can copy only its dirty clusters without starting the guest. qemu-img holds
// the image lock for the copy, so a start attempted meanwhile fails on the
// lock instead of writing into a disk that is half copied.
func backupDisksOffline(ctx context.Context, req BackupRequest) ([]BackupDiskResult, error) {
	results := make([]BackupDiskResult, 0, len(req.Disks))
	for _, disk := range req.Disks {
		result, err := backupOneDiskOffline(ctx, req, disk)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// backupOneDiskOffline produces one stopped disk's artifact.
func backupOneDiskOffline(ctx context.Context, req BackupRequest, disk BackupDisk) (BackupDiskResult, error) {
	result := BackupDiskResult{
		DiskID:     disk.DiskID,
		Kind:       BackupKindFull,
		BitmapName: disk.BitmapName,
		Offline:    true,
	}
	if req.Kind == BackupKindIncremental {
		result.FellBackToFull = true
		result.FallbackReason = offlineIncrementalReason
	}

	target := filepath.Join(req.ScratchDir,
		fmt.Sprintf("%s-%s.qcow2", req.RunID, disk.DiskID))
	errlog.Log(os.Remove(target))

	req.phase(disk.DiskID, "copying", "copying the stopped machine's disk")
	if err := copyDiskOffline(ctx, req, disk, target); err != nil {
		errlog.Log(os.Remove(target))
		return result, err
	}

	// Reset only after a good copy: failing earlier keeps the old chain's
	// bitmap, so the previous backups stay the ones the next run builds on.
	if disk.BitmapName != "" {
		result.BitmapPresent = resetBitmapOffline(ctx, disk.VolumePath, disk.BitmapName)
	}

	info, err := os.Stat(target)
	if err != nil {
		return result, fmt.Errorf("vm: the backup of %s was not written: %w", disk.DiskID, err)
	}
	result.StagedPath = target
	result.BytesWritten = info.Size()
	return result, nil
}

// copyDiskOffline runs qemu-img convert, reporting its progress.
//
// Not through runImgTool: that is bounded at a minute for metadata calls, and
// this is the copy itself.
func copyDiskOffline(ctx context.Context, req BackupRequest, disk BackupDisk, target string) error {
	binary, err := exec.LookPath("qemu-img")
	if err != nil {
		return fmt.Errorf("vm: qemu-img is not installed on this host: %w", err)
	}
	total, err := virtualSizeOf(ctx, disk.VolumePath)
	if err != nil {
		return err
	}

	copyCtx, cancel := context.WithTimeout(ctx, backupJobTimeout)
	defer cancel()

	cmd := exec.CommandContext(copyCtx, binary, "convert", "-p", "-O", "qcow2", disk.VolumePath, target)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("vm: backing up %s: %w", disk.DiskID, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("vm: backing up %s: %w", disk.DiskID, err)
	}
	reportConvertProgress(stdout, func(percent float64) {
		if req.OnProgress != nil {
			req.OnProgress(disk.DiskID, int64(percent/100*float64(total)), total)
		}
	})
	if err := cmd.Wait(); err != nil {
		// A guest started mid-copy surfaces here as qemu-img's lock error,
		// which names the cause better than anything this could add.
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return fmt.Errorf("vm: backing up %s: %w: %s", disk.DiskID, err, detail)
		}
		return fmt.Errorf("vm: backing up %s: %w", disk.DiskID, err)
	}
	return nil
}

// reportConvertProgress parses qemu-img's "(NN.NN/100%)" progress lines.
//
// They are separated by carriage returns, not newlines, so a line scanner
// would see one line at the very end.
func reportConvertProgress(r io.Reader, report func(float64)) {
	scanner := bufio.NewScanner(r)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		for i, b := range data {
			if b == '\r' || b == '\n' {
				return i + 1, data[:i], nil
			}
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		text = strings.TrimPrefix(text, "(")
		text, _, found := strings.Cut(text, "/100%")
		if !found {
			continue
		}
		if percent, err := strconv.ParseFloat(text, 64); err == nil {
			report(percent)
		}
	}
}

// resetBitmapOffline restarts a stopped disk's change log at this copy.
//
// Best effort, and reported rather than returned: a disk left without its
// bitmap costs the next run a full copy, which planDiskBackup already detects,
// whereas failing here would throw away a backup that is complete.
func resetBitmapOffline(ctx context.Context, volumePath, bitmapName string) bool {
	if _, err := runImgTool(ctx, "bitmap", "--remove", volumePath, bitmapName); err != nil {
		// A disk with no bitmap yet is the normal first run; one that really
		// survived makes the add below fail and the function report false.
		_ = err
	}
	_, err := runImgTool(ctx, "bitmap", "--add", volumePath, bitmapName)
	return err == nil
}

// backupNodeDetachTimeout bounds closing a staging image's block nodes.
const backupNodeDetachTimeout = 30 * time.Second

// detachBackupNode closes a staging image's block nodes, best effort.
//
// Detached from the run's cancellation: a run that failed on its deadline is
// exactly the one whose nodes would otherwise stay attached to the guest.
func detachBackupNode(ctx context.Context, monitor *qmp.Monitor, nodeName string) {
	detachCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupNodeDetachTimeout)
	defer cancel()
	errlog.Log(monitor.RemoveBlockdevFile(detachCtx, nodeName))
}

// imgToolTimeout bounds one qemu-img invocation.
//
// These are metadata operations — create and info — not copies, so a minute
// is generous; the copy itself runs inside QEMU through blockdev-backup.
const imgToolTimeout = 60 * time.Second

// runImgTool runs one qemu-img subcommand.
func runImgTool(ctx context.Context, args ...string) (string, error) {
	binary, err := exec.LookPath("qemu-img")
	if err != nil {
		return "", fmt.Errorf("vm: qemu-img is not installed on this host: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, imgToolTimeout)
	defer cancel()

	out, err := exec.CommandContext(runCtx, binary, args...).CombinedOutput()
	if err != nil {
		// qemu-img says why on stderr — a lock held by the running guest, a
		// missing backing file, a corrupt header. Returning only "exit status
		// 1" sends the operator to a log this process does not keep.
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			return string(out), fmt.Errorf("qemu-img %s: %w", args[0], err)
		}
		return string(out), fmt.Errorf("qemu-img %s: %w: %s", args[0], err, detail)
	}
	return string(out), nil
}
