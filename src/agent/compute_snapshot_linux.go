// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// VM snapshots.
//
// A guest's snapshot is not one object: it is one snapshot per disk, taken by
// whichever backend each disk's pool uses, plus — optionally — the machine's
// memory. The server stores a single handle per snapshot, so the agent keeps a
// manifest of the parts under the guest's runtime directory and hands back the
// snapshot's name as that handle.
//
// # Two disk mechanisms
//
// Offline, every backend goes through its storage driver. The `dir` driver's
// qcow2 mechanism renames the guest's image aside and creates a fresh overlay
// at the original path, so the path in the guest's definition never changes.
//
// Live, that rename is impossible — QEMU has the file open and is writing to
// it. The QMP equivalent, `blockdev-snapshot-sync`, works the other way round:
// the file the guest was writing to is frozen and becomes the snapshot, and the
// guest is handed a *new* overlay to write to. So a live qcow2 snapshot moves
// the guest's active file, and the new path is reported back to the server,
// which persists it. Copy-on-write backends have no such problem and use their
// driver either way.
//
// # Memory
//
// `savevm` is an HMP command that writes into a qcow2's internal snapshot area,
// which is no use to a guest whose disks are on ZFS or LVM. Migrating to a file
// is the supported equivalent and is backend-agnostic, so that is what a
// memory-state snapshot is: the guest is paused, its state is written beside
// the manifest, and a revert restarts QEMU with `-incoming` pointed at it.

// snapshotDirName is the subdirectory of a guest's runtime directory where its
// manifests and memory-state files live.
const snapshotDirName = "snapshots"

// liveOverlaySuffix marks the overlay QEMU is handed when a live qcow2 snapshot
// moves the guest's active file. It is distinct from the `.snap.` suffix the
// dir driver uses so the two are never confused when reading a directory.
const liveOverlaySuffix = ".live-"

// Snapshot mechanisms recorded in a manifest.
const (
	mechanismDriver   = "driver"
	mechanismLiveQcow = "qcow2-live"
)

// memorySaveTimeout bounds writing a guest's RAM to disk. A large guest on slow
// storage legitimately takes minutes; anything past this is wedged.
const memorySaveTimeout = 30 * time.Minute

// blockCommitTimeout bounds a live snapshot merge. The job copies data, so a
// large snapshot on slow storage legitimately takes a long while.
const blockCommitTimeout = 6 * time.Hour

// Errors this file raises, worded for an operator rather than a log reader.
var (
	errSnapshotNoSpec = errors.New(
		"compute: a snapshot needs the guest's definition, and none was sent")
	errSnapshotNoDisks = errors.New(
		"compute: this guest has no disks to snapshot")
	errSnapshotNotFound = errors.New(
		"compute: no snapshot by that name exists on this host")
	errSnapshotMemoryOffline = errors.New(
		"compute: a memory-state snapshot needs the guest running; it is not")
	errSnapshotRunning = errors.New(
		"compute: the guest must be stopped for that; stop it and try again")
)

// snapshotDisk is one disk's part of a snapshot.
type snapshotDisk struct {
	DiskID      string `json:"diskId"`
	Slot        int    `json:"slot"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	// FrozenPath is the file that holds the snapshot's contents.
	FrozenPath string `json:"frozenPath"`
	// ActivePath is what the guest writes to afterwards. It differs from the
	// path at snapshot time only on the live qcow2 path.
	ActivePath string `json:"activePath"`
	// Handle is the backend's own name for the snapshot: a zfs snapshot name,
	// an LV, a btrfs subvolume, or the frozen qcow2's path.
	Handle    string `json:"handle"`
	Mechanism string `json:"mechanism"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

// snapshotManifest is everything needed to revert or delete one snapshot.
//
// Written to disk because the server stores a single handle per snapshot and a
// guest with three disks has three of them. Keeping the parts here means the
// server's model stays simple and the agent stays the only thing that has to
// know how each backend names its own snapshots.
type snapshotManifest struct {
	Name        string         `json:"name"`
	VMID        string         `json:"vmId"`
	CreatedAt   time.Time      `json:"createdAt"`
	WithMemory  bool           `json:"withMemory"`
	MemoryFile  string         `json:"memoryFile,omitempty"`
	Consistency string         `json:"consistency"`
	Disks       []snapshotDisk `json:"disks"`
}

// snapshotRoot is where one guest's manifests live.
func snapshotRoot(vmID string) string {
	return filepath.Join(vm.PathsFor(vmID).Dir, snapshotDirName)
}

// manifestPath is one snapshot's manifest file.
func manifestPath(vmID, name string) string {
	// filepath.Base defends the directory against a name carrying a path. The
	// name is validated before it reaches here on the create path, and arrives
	// from a database column on revert and delete — which is exactly the case
	// worth defending against.
	return filepath.Join(snapshotRoot(vmID), filepath.Base(name)+".json")
}

// writeManifest records a snapshot.
//
// Written last, after every disk is frozen: a manifest naming a snapshot that
// does not exist would offer a revert that destroys the guest's disk for
// nothing.
func writeManifest(m snapshotManifest) error {
	root := snapshotRoot(m.VMID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("compute: create %s: %w", root, err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("compute: encoding the snapshot manifest: %w", err)
	}
	return os.WriteFile(manifestPath(m.VMID, m.Name), data, 0o600)
}

// readManifest loads one snapshot's manifest.
func readManifest(vmID, name string) (snapshotManifest, error) {
	var m snapshotManifest
	data, err := os.ReadFile(manifestPath(vmID, name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m, fmt.Errorf("%w: %s", errSnapshotNotFound, name)
		}
		return m, fmt.Errorf("compute: reading the snapshot manifest: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("compute: the snapshot manifest for %s is unreadable: %w", name, err)
	}
	return m, nil
}

// handleVMSnapshot answers the three vm_snapshot_* messages.
func (rt *computeRuntime) handleVMSnapshot(ctx context.Context, messageType string, req *VMPayload, reply *VMPayload) error {
	name := strings.TrimSpace(req.SnapshotName)
	if name == "" {
		return fmt.Errorf("compute: %s carried no snapshot name", messageType)
	}

	switch messageType {
	case MsgVMSnapshotCreate:
		return rt.createVMSnapshot(ctx, name, req, reply)
	case MsgVMSnapshotRevert:
		return rt.revertVMSnapshot(ctx, name, req, reply)
	case MsgVMSnapshotDelete:
		return rt.deleteVMSnapshot(ctx, name, req, reply)
	}
	return fmt.Errorf("compute: unknown snapshot operation %s", messageType)
}

// vmIsRunning reports whether QEMU is up for this guest.
func (rt *computeRuntime) vmIsRunning(ctx context.Context, vmID string) (bool, error) {
	states, _, err := rt.vms.Query(ctx, []string{vmID})
	if err != nil {
		return false, fmt.Errorf("compute: checking the guest's state: %w", err)
	}
	for _, state := range states {
		if state.VMID != vmID {
			continue
		}
		return state.State == vm.StateRunning || state.State == vm.StatePaused, nil
	}
	return false, nil
}

// snapshotDisksFrom turns the definition's disks into the parts to snapshot.
//
// CD-ROMs and read-only disks are skipped: an installer ISO is not the guest's
// state, and snapshotting it would double the storage for nothing.
func snapshotDisksFrom(spec *VMSpec) []VMDiskSpec {
	var out []VMDiskSpec
	for _, disk := range spec.Disks {
		if disk.IsCDROM || disk.ReadOnly || disk.VolumePath == "" {
			continue
		}
		out = append(out, disk)
	}
	return out
}

// freezeGuestFilesystems quiesces the guest through its agent, when it has one.
//
// Returns the consistency actually achieved and a thaw function. Without a
// guest agent the snapshot is crash-consistent — which is what a guest sees
// after a power cut, and recoverable for any journalling filesystem — and the
// caller reports that rather than claiming more.
func (rt *computeRuntime) freezeGuestFilesystems(ctx context.Context, vmID string) (string, func()) {
	socket := vm.PathsFor(vmID).GuestAgent
	if _, err := os.Stat(socket); err != nil {
		return "crash", func() {}
	}
	guest, err := qmp.DialGuest(ctx, socket)
	if err != nil {
		rt.logger.Debug("compute: no guest agent for a consistent snapshot",
			"vm", vmID, "error", err)
		return "crash", func() {}
	}
	if _, err := guest.FsFreeze(ctx); err != nil {
		rt.logger.Warn("compute: freezing the guest's filesystems failed; "+
			"the snapshot will be crash-consistent", "vm", vmID, "error", err)
		errlog.Close(guest)
		return "crash", func() {}
	}
	return "filesystem", func() {
		// Thawing must happen whatever else failed: a guest left frozen stops
		// answering every write, which looks exactly like a hung machine.
		thawCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), computeQuickTimeout)
		defer cancel()
		if _, err := guest.FsThaw(thawCtx); err != nil {
			rt.logger.Error("compute: thawing the guest's filesystems failed; "+
				"the guest may be unresponsive until it is restarted",
				"vm", vmID, "error", err)
		}
		errlog.Close(guest)
	}
}

// publishSnapshotProgress reports how far a snapshot has got, through the guest
// event channel the server already subscribes to.
//
// Milestones rather than a byte count: the work is a handful of named steps —
// quiesce, pause, write the memory, freeze each disk — and a guest with 16 GB of
// RAM sits in one of them for minutes. What the operator needs is which one.
func (rt *computeRuntime) publishSnapshotProgress(vmID string, percent int, step string) {
	rt.publishVMEvent(vm.Event{
		VMID: vmID,
		Kind: "snapshot_progress",
		At:   time.Now().UTC(),
		Detail: map[string]string{
			"percent": strconv.Itoa(percent),
			"step":    step,
		},
	})
}

// createVMSnapshot takes one snapshot of every writable disk, and of memory
// when asked.
func (rt *computeRuntime) createVMSnapshot(ctx context.Context, name string, req *VMPayload, reply *VMPayload) error {
	if err := storage.ValidateSnapshotName(name); err != nil {
		return err
	}
	if req.Spec == nil {
		return errSnapshotNoSpec
	}
	disks := snapshotDisksFrom(req.Spec)
	if len(disks) == 0 {
		return errSnapshotNoDisks
	}
	if _, err := readManifest(req.VMID, name); err == nil {
		return fmt.Errorf("compute: this guest already has a snapshot called %q", name)
	}

	running, err := rt.vmIsRunning(ctx, req.VMID)
	if err != nil {
		return err
	}
	if req.WithMemory && !running {
		return errSnapshotMemoryOffline
	}

	manifest := snapshotManifest{
		Name: name, VMID: req.VMID, CreatedAt: time.Now().UTC(),
		WithMemory: req.WithMemory, Consistency: "crash",
	}

	if !running {
		// Nothing is writing, so every backend can use its own driver and the
		// guest's disk paths stay exactly as they are.
		rt.publishSnapshotProgress(req.VMID, 10, "disks")
		if err := rt.snapshotDisksOffline(ctx, name, disks, &manifest); err != nil {
			return err
		}
		rt.publishSnapshotProgress(req.VMID, 100, "done")
		if err := writeManifest(manifest); err != nil {
			return err
		}
		reply.SnapshotName = name
		reply.Consistency = manifest.Consistency
		return nil
	}

	return rt.snapshotRunningGuest(ctx, name, req, reply, disks, &manifest)
}

// snapshotDisksOffline snapshots a stopped guest's disks through their drivers.
func (rt *computeRuntime) snapshotDisksOffline(ctx context.Context, name string, disks []VMDiskSpec, manifest *snapshotManifest) error {
	taken := make([]snapshotDisk, 0, len(disks))
	// A partial snapshot is not a snapshot: if the third disk fails, the first
	// two are rolled back so the guest is left exactly as it was.
	defer func() {
		if len(manifest.Disks) == 0 {
			rt.discardPartialSnapshot(ctx, taken)
		}
	}()

	for _, disk := range disks {
		driver, err := rt.pools.Get(disk.PoolBackend)
		if err != nil {
			return err
		}
		pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}
		snap, err := driver.Snapshot(ctx, pool, disk.VolumePath, name)
		if err != nil {
			return fmt.Errorf("compute: snapshotting the disk in slot %d: %w", disk.Slot, err)
		}
		taken = append(taken, snapshotDisk{
			DiskID: disk.DiskID, Slot: disk.Slot,
			PoolID: disk.PoolID, PoolBackend: disk.PoolBackend, PoolTarget: disk.PoolTarget,
			FrozenPath: snap.Handle, ActivePath: disk.VolumePath,
			Handle: snap.Handle, Mechanism: mechanismDriver, SizeBytes: snap.SizeBytes,
		})
	}
	manifest.Disks = taken
	return nil
}

// snapshotRunningGuest snapshots a live guest.
//
// The ordering is what makes the result consistent: quiesce, pause if memory is
// wanted, freeze every disk, then release in reverse. A disk frozen before the
// guest is quiesced is a disk frozen mid-write.
func (rt *computeRuntime) snapshotRunningGuest(ctx context.Context, name string, req *VMPayload, reply *VMPayload, disks []VMDiskSpec, manifest *snapshotManifest) error {
	monitor, err := rt.vms.Monitor(ctx, req.VMID)
	if err != nil {
		return fmt.Errorf("compute: reaching the running guest: %w", err)
	}

	rt.publishSnapshotProgress(req.VMID, 5, "quiescing")
	consistency, thaw := rt.freezeGuestFilesystems(ctx, req.VMID)
	manifest.Consistency = consistency
	defer thaw()

	if req.WithMemory || req.PauseGuest {
		// Paused before the state is written: migrating a running guest
		// converges by re-copying dirty pages, and a snapshot has nothing to
		// converge towards. A pause without memory is the weaker guarantee the
		// scheduler asks for when the guest has no agent to quiesce it.
		rt.publishSnapshotProgress(req.VMID, 15, "pausing")
		if err := monitor.Pause(ctx); err != nil {
			return fmt.Errorf("compute: pausing the guest for its snapshot: %w", err)
		}
		resume := func() {
			resumeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), computeQuickTimeout)
			defer cancel()
			if err := monitor.Resume(resumeCtx); err != nil {
				rt.logger.Error("compute: the guest could not be resumed after its snapshot",
					"vm", req.VMID, "error", err)
			}
		}
		defer resume()
		if manifest.Consistency == "crash" {
			// Honest, not flattering: the vCPUs are stopped so nothing is
			// mid-write at the block layer, but without a guest agent the
			// guest's own page cache was never flushed.
			manifest.Consistency = "paused"
		}
	}

	if req.WithMemory {
		root := snapshotRoot(req.VMID)
		if err := os.MkdirAll(root, 0o700); err != nil {
			return fmt.Errorf("compute: create %s: %w", root, err)
		}
		memoryFile := filepath.Join(root, filepath.Base(name)+".memory")
		// The long pole on a guest with real memory, and the one step worth
		// naming: everything else here is seconds.
		rt.publishSnapshotProgress(req.VMID, 25, "memory")
		saveCtx, cancel := context.WithTimeout(ctx, memorySaveTimeout)
		err := monitor.SaveMemoryState(saveCtx, memoryFile)
		cancel()
		if err != nil {
			errlog.Log(os.Remove(memoryFile))
			return fmt.Errorf("compute: saving the guest's memory: %w", err)
		}
		manifest.MemoryFile = memoryFile
		// A snapshot that carries the machine's state is consistent by
		// construction, whatever the guest agent could or could not do.
		manifest.Consistency = "application"
	}

	rt.publishSnapshotProgress(req.VMID, 70, "disks")
	if err := rt.snapshotDisksLive(ctx, monitor, name, disks, manifest); err != nil {
		if manifest.MemoryFile != "" {
			errlog.Log(os.Remove(manifest.MemoryFile))
		}
		return err
	}
	if err := writeManifest(*manifest); err != nil {
		return err
	}

	rt.publishSnapshotProgress(req.VMID, 100, "done")
	reply.SnapshotName = name
	reply.Consistency = manifest.Consistency
	// Only the disks whose active file moved. The server persists these; a
	// disk that kept its path is left alone.
	for _, disk := range manifest.Disks {
		if disk.Mechanism != mechanismLiveQcow {
			continue
		}
		reply.Disks = append(reply.Disks, VMDiskSpec{
			DiskID: disk.DiskID, Slot: disk.Slot,
			VolumePath: disk.ActivePath, Format: "qcow2",
		})
	}
	return nil
}

// snapshotDisksLive freezes a running guest's disks.
//
// Copy-on-write backends snapshot through their own driver, which is safe on a
// live volume. A `dir` pool cannot: its driver renames the guest's image, and
// QEMU has that file open. QMP's own external snapshot is used instead, which
// freezes the current file and hands the guest a new overlay.
func (rt *computeRuntime) snapshotDisksLive(ctx context.Context, monitor *qmp.Monitor, name string, disks []VMDiskSpec, manifest *snapshotManifest) error {
	taken := make([]snapshotDisk, 0, len(disks))
	defer func() {
		if len(manifest.Disks) == 0 {
			rt.discardPartialSnapshot(ctx, taken)
		}
	}()

	for _, disk := range disks {
		driver, err := rt.pools.Get(disk.PoolBackend)
		if err != nil {
			return err
		}
		pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}

		if disk.PoolBackend != storage.BackendDir {
			snap, err := driver.Snapshot(ctx, pool, disk.VolumePath, name)
			if err != nil {
				return fmt.Errorf("compute: snapshotting the disk in slot %d: %w", disk.Slot, err)
			}
			taken = append(taken, snapshotDisk{
				DiskID: disk.DiskID, Slot: disk.Slot,
				PoolID: disk.PoolID, PoolBackend: disk.PoolBackend, PoolTarget: disk.PoolTarget,
				FrozenPath: snap.Handle, ActivePath: disk.VolumePath,
				Handle: snap.Handle, Mechanism: mechanismDriver, SizeBytes: snap.SizeBytes,
			})
			continue
		}

		// The node is resolved by filename rather than assumed to be the
		// `blk<slot>` from the command line: after the first live snapshot the
		// guest's top layer is a node QEMU named, and addressing the original
		// one would snapshot a layer nothing is writing to.
		//
		// From that file the chain is walked to its top, because the path the
		// server sent can be one layer behind — a snapshot that failed after
		// QEMU made the overlay leaves the guest writing somewhere McHarbor
		// never recorded, and snapshotting a node that has become a backing
		// file is refused outright.
		current, err := monitor.TopNodeForFile(ctx, disk.VolumePath)
		if err != nil {
			return fmt.Errorf("compute: locating the live disk in slot %d: %w", disk.Slot, err)
		}
		live := current.Image.Filename
		if live != disk.VolumePath {
			rt.logger.Warn("compute: the recorded disk was not the guest's top layer; "+
				"snapshotting what it is actually writing to",
				"slot", disk.Slot, "recorded", disk.VolumePath, "live", live)
		}
		overlay := liveOverlayPath(live, name)
		if _, err := os.Stat(overlay); err == nil {
			return fmt.Errorf("compute: a snapshot overlay for %q already exists on slot %d", name, disk.Slot)
		}
		overlayNode := overlayNodeName(disk.Slot, name)
		if err := monitor.SnapshotSync(ctx, current.NodeName, overlayNode, overlay); err != nil {
			return fmt.Errorf("compute: snapshotting the live disk in slot %d: %w", disk.Slot, err)
		}
		size := int64(0)
		if st, statErr := os.Stat(live); statErr == nil {
			size = st.Size()
		}
		taken = append(taken, snapshotDisk{
			DiskID: disk.DiskID, Slot: disk.Slot,
			PoolID: disk.PoolID, PoolBackend: disk.PoolBackend, PoolTarget: disk.PoolTarget,
			FrozenPath: live, ActivePath: overlay,
			Handle: live, Mechanism: mechanismLiveQcow, SizeBytes: size,
		})
	}
	manifest.Disks = taken
	return nil
}

// liveOverlayPath names the file the guest writes to after a live snapshot.
//
// Built from the disk's own name rather than by appending to whatever it is
// called now: chaining the suffix produced `disk-0.live-a.qcow2.live-b.qcow2`
// and grew with every snapshot. Every overlay is a sibling of the disk, which
// is what lets the guest's directory move as one thing.
func liveOverlayPath(current, name string) string {
	dir := filepath.Dir(current)
	base := filepath.Base(current)
	if cut := strings.Index(base, liveOverlaySuffix); cut > 0 {
		base = base[:cut]
	}
	return filepath.Join(dir, base+liveOverlaySuffix+filepath.Base(name)+".qcow2")
}

// discardPartialSnapshot undoes the disks a failed snapshot already froze.
//
// Best effort, and logged rather than returned: the caller is already failing
// with the reason the operator needs, and a cleanup error on top of it would
// replace that reason with a less useful one.
func (rt *computeRuntime) discardPartialSnapshot(ctx context.Context, taken []snapshotDisk) {
	for _, disk := range taken {
		driver, err := rt.pools.Get(disk.PoolBackend)
		if err != nil {
			continue
		}
		pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}
		switch disk.Mechanism {
		case mechanismLiveQcow:
			// The guest is writing into the overlay; it cannot simply be
			// removed. Left in place and reported, because deleting a file
			// QEMU has open would take the guest's disk with it.
			rt.logger.Warn("compute: a failed snapshot left a live overlay behind",
				"slot", disk.Slot, "overlay", disk.ActivePath)
		default:
			if err := driver.DeleteSnapshot(ctx, pool,
				disk.ActivePath, storage.Snapshot{Handle: disk.Handle}); err != nil {
				rt.logger.Warn("compute: rolling back a partial snapshot failed",
					"slot", disk.Slot, "handle", disk.Handle, "error", err)
			}
		}
	}
}

// revertVMSnapshot rolls a guest back to a snapshot.
//
// The guest must be stopped. Reverting under a running QEMU would swap the file
// beneath its open descriptors, which corrupts rather than reverts.
//
// Every qcow2 chain is reopened as a *new* branch over the snapshot's frozen
// file, and the only file discarded is the one the guest is writing to now —
// never the path recorded at snapshot time, which a later snapshot on the
// branch being left has since frozen as its own data.
func (rt *computeRuntime) revertVMSnapshot(ctx context.Context, name string, req *VMPayload, reply *VMPayload) error {
	manifest, err := readManifest(req.VMID, name)
	if err != nil {
		return err
	}
	running, err := rt.vmIsRunning(ctx, req.VMID)
	if err != nil {
		return err
	}
	if running {
		return errSnapshotRunning
	}

	live := liveDiskPaths(req.Spec)
	protected := snapshotDataPaths(req.VMID)

	for _, disk := range manifest.Disks {
		current := disk.ActivePath
		if path, ok := live[disk.Slot]; ok && path != "" {
			current = path
		}

		if isQcowChain(disk) && !isDirPath(disk.FrozenPath) {
			// The guest is never pointed at the frozen file itself: other
			// branches are backed by it, and writing into it would rewrite
			// their history too. The branch is created before anything is
			// discarded, so a failure here leaves the guest with a disk.
			branch, err := newOverlayOver(ctx, disk.FrozenPath)
			if err != nil {
				return fmt.Errorf("compute: reopening slot %d on the snapshot: %w", disk.Slot, err)
			}
			rt.discardStaleOverlay(current, protected)
			reply.Disks = append(reply.Disks, VMDiskSpec{
				DiskID: disk.DiskID, Slot: disk.Slot,
				VolumePath: branch, Format: "qcow2",
			})
			continue
		}

		driver, err := rt.pools.Get(disk.PoolBackend)
		if err != nil {
			return err
		}
		pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}
		mechanism := "qcow2"
		if isDirPath(disk.Handle) {
			// A container rootfs on a dir pool was copied aside rather than
			// layered, and restoring it is a copy back.
			mechanism = "copy"
		}
		if err := driver.RevertSnapshot(ctx, pool, current,
			storage.Snapshot{Handle: disk.Handle, Mechanism: mechanism}); err != nil {
			return fmt.Errorf("compute: reverting the disk in slot %d: %w", disk.Slot, err)
		}
	}

	reply.SnapshotName = name
	if !manifest.WithMemory || manifest.MemoryFile == "" {
		return nil
	}
	if _, err := os.Stat(manifest.MemoryFile); err != nil {
		// The disks are already back. Saying the memory is gone is more useful
		// than failing the whole revert, which would leave the operator unsure
		// which half happened.
		rt.logger.Warn("compute: this snapshot's memory state is missing; "+
			"the guest will boot rather than resume",
			"vm", req.VMID, "snapshot", name, "error", err)
		reply.ErrorCode = "memory_state_missing"
	}
	return nil
}

// liveDiskPaths maps each slot to the file the guest writes to now.
//
// A revert may only discard this file. The path a manifest recorded as active
// is a snapshot older than whatever happened since, and on a branch with a
// later snapshot it is that snapshot's frozen data.
func liveDiskPaths(spec *VMSpec) map[int]string {
	out := make(map[int]string)
	if spec == nil {
		return out
	}
	for _, disk := range spec.Disks {
		if disk.IsCDROM || disk.VolumePath == "" {
			continue
		}
		out[disk.Slot] = disk.VolumePath
	}
	return out
}

// snapshotDataPaths collects every file this guest's snapshots hold their data
// in, across all of its branches.
//
// Read from the manifests rather than tracked, because the answer has to
// include branches this revert knows nothing about.
func snapshotDataPaths(vmID string) map[string]bool {
	out := make(map[string]bool)
	entries, err := os.ReadDir(snapshotRoot(vmID))
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		manifest, err := readManifest(vmID, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		for _, disk := range manifest.Disks {
			if disk.FrozenPath != "" {
				out[disk.FrozenPath] = true
			}
			if disk.Handle != "" {
				out[disk.Handle] = true
			}
		}
	}
	return out
}

// discardStaleOverlay drops the overlay the guest was writing to before a
// revert, unless a snapshot holds its data there.
//
// Best effort: the branch the guest is moving to already exists at this point,
// so failing the revert over a leftover file would report a revert that did in
// fact happen as having failed.
func (rt *computeRuntime) discardStaleOverlay(path string, protected map[string]bool) {
	if path == "" || protected[path] {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		rt.logger.Warn("compute: the overlay discarded by a revert could not be removed",
			"path", path, "error", err)
	}
}

// isDirPath reports whether a snapshot's data is a copied directory rather than
// a qcow2 file, which is how a container rootfs on a dir pool is snapshotted.
func isDirPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// newOverlayOver opens a fresh branch on top of a frozen snapshot file.
//
// The branch is named from the frozen file plus a timestamp, so reverting to
// the same snapshot twice produces two branches rather than colliding on one
// path that the first branch's descendants are still backed by.
func newOverlayOver(ctx context.Context, frozenPath string) (string, error) {
	branch := fmt.Sprintf("%s%d.qcow2", frozenPath+liveOverlaySuffix, time.Now().Unix())
	if err := storage.CreateOverlay(ctx, frozenPath, branch); err != nil {
		return "", err
	}
	return branch, nil
}

// deleteVMSnapshot drops a snapshot without touching the guest's current state.
func (rt *computeRuntime) deleteVMSnapshot(ctx context.Context, name string, req *VMPayload, reply *VMPayload) error {
	manifest, err := readManifest(req.VMID, name)
	if err != nil {
		return err
	}
	running, err := rt.vmIsRunning(ctx, req.VMID)
	if err != nil {
		return err
	}

	// The branch directly below this snapshot, when it has one. Its files are
	// what absorb the deleted layer; the guest's own disk is only the right
	// target when the snapshot is the tip of the branch the guest is on.
	child := strings.TrimSpace(req.ChildSnapshot)
	var childDisks map[int]snapshotDisk
	if child != "" {
		if running {
			return fmt.Errorf(
				"compute: deleting snapshot %q needs the guest stopped, because "+
					"another snapshot is still backed by its disks", name)
		}
		childManifest, err := readManifest(req.VMID, child)
		if err != nil {
			return err
		}
		childDisks = make(map[int]snapshotDisk, len(childManifest.Disks))
		for _, disk := range childManifest.Disks {
			childDisks[disk.Slot] = disk
		}
	}

	var monitor *qmp.Monitor
	if running {
		if monitor, err = rt.vms.Monitor(ctx, req.VMID); err != nil {
			return fmt.Errorf("compute: reaching the running guest: %w", err)
		}
	}

	for _, disk := range manifest.Disks {
		if below, ok := childDisks[disk.Slot]; ok {
			// Fold into the child branch's own file rather than the live one.
			disk.ActivePath = below.FrozenPath
		}
		driver, err := rt.pools.Get(disk.PoolBackend)
		if err != nil {
			return err
		}
		pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}

		// Every qcow2 chain — the live mechanism and a `dir` pool's offline one
		// alike — has the guest's own files in it, and QEMU holds the write
		// lock on all of them while it runs. qemu-img cannot touch those; the
		// merge has to be asked of QEMU itself.
		// A snapshot on a branch the guest has left is not in its chain at all:
		// reverting discards the overlay that used to sit above it, and the
		// files that remain are ones nothing has open. Those are deleted the
		// same way a stopped guest's are, which is why this asks the monitor
		// rather than trusting the recorded path — that path is exactly what
		// goes stale when a guest branches.
		offChain := running && isQcowChain(disk) && !monitor.InChain(ctx, disk.FrozenPath)
		if offChain {
			rt.logger.Info("compute: the snapshot is not in the running guest's chain; "+
				"removing it without the guest",
				"slot", disk.Slot, "file", disk.FrozenPath)
		}

		if running && isQcowChain(disk) && !offChain {
			moved, err := rt.commitLiveSnapshot(ctx, monitor, disk)
			if err != nil {
				return err
			}
			if moved != "" {
				reply.Disks = append(reply.Disks, VMDiskSpec{
					DiskID: disk.DiskID, Slot: disk.Slot,
					VolumePath: moved, Format: "qcow2",
				})
				rt.repointManifests(req.VMID, name, disk.Slot, disk.ActivePath, moved)
			}
			continue
		}

		if offChain {
			if err := rt.removeDetachedSnapshot(req.VMID, name, disk); err != nil {
				return err
			}
			continue
		}

		if disk.Mechanism == mechanismLiveQcow {
			if err := foldLiveOverlay(ctx, driver, pool, disk); err != nil {
				return err
			}
			continue
		}

		if err := driver.DeleteSnapshot(ctx, pool, disk.ActivePath,
			storage.Snapshot{Handle: disk.Handle, Mechanism: "qcow2"}); err != nil {
			return fmt.Errorf("compute: deleting the snapshot of slot %d: %w", disk.Slot, err)
		}
	}

	if manifest.MemoryFile != "" {
		if err := os.Remove(manifest.MemoryFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			rt.logger.Warn("compute: removing a snapshot's memory state failed",
				"file", manifest.MemoryFile, "error", err)
		}
	}
	if err := os.Remove(manifestPath(req.VMID, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("compute: removing the snapshot manifest: %w", err)
	}
	return nil
}

// removeDetachedSnapshot deletes a snapshot nothing is reading through.
//
// The case is a guest that branched: reverting discarded the overlay that sat
// above this snapshot, so its file backs nothing and no merge is possible or
// needed. Refused rather than guessed at when another branch still names it —
// removing a file two snapshots read through would take the other one with it.
func (rt *computeRuntime) removeDetachedSnapshot(vmID, name string, disk snapshotDisk) error {
	for _, other := range otherManifestNames(vmID, name) {
		manifest, err := readManifest(vmID, other)
		if err != nil {
			continue
		}
		for _, entry := range manifest.Disks {
			if entry.FrozenPath == disk.FrozenPath || entry.Handle == disk.FrozenPath {
				return fmt.Errorf(
					"compute: snapshot %q on slot %d is still the data of %q; delete that first",
					name, disk.Slot, other)
			}
		}
	}

	if isDirPath(disk.FrozenPath) {
		if err := os.RemoveAll(disk.FrozenPath); err != nil {
			return fmt.Errorf("compute: removing the snapshot of slot %d: %w", disk.Slot, err)
		}
		return nil
	}
	if err := os.Remove(disk.FrozenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("compute: removing the snapshot of slot %d: %w", disk.Slot, err)
	}
	return nil
}

// otherManifestNames lists a guest's snapshots apart from one.
func otherManifestNames(vmID, skip string) []string {
	entries, err := os.ReadDir(snapshotRoot(vmID))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		other := strings.TrimSuffix(entry.Name(), ".json")
		if other == skip {
			continue
		}
		out = append(out, other)
	}
	return out
}

// isQcowChain reports whether this disk's snapshot lives in a qcow2 backing
// chain, which is the case the running guest holds the write lock on.
func isQcowChain(disk snapshotDisk) bool {
	return disk.Mechanism == mechanismLiveQcow || disk.PoolBackend == storage.BackendDir
}

// commitLiveSnapshot deletes one disk's snapshot underneath a running guest.
//
// Returns the guest's new active path when the merge moved it, which happens
// only when the snapshot was the bottom of the chain and there was nothing
// below to commit into. The server persists that path; left unwritten the guest
// would start from a file that is no longer the live one.
func (rt *computeRuntime) commitLiveSnapshot(ctx context.Context, monitor *qmp.Monitor, disk snapshotDisk) (string, error) {
	frozen, err := monitor.NodeForFile(ctx, disk.FrozenPath)
	if err != nil {
		return "", fmt.Errorf("compute: locating the snapshot of slot %d in the live chain: %w", disk.Slot, err)
	}
	// The layer above the snapshot, asked for by what it is backed by. The
	// recorded active path is stale the moment a guest branches, and it was
	// stale here often enough to be worth never reading again.
	active, err := monitor.NodeAbove(ctx, disk.FrozenPath)
	if err != nil {
		return "", fmt.Errorf("compute: locating the live disk in slot %d: %w", disk.Slot, err)
	}
	if frozen.NodeName == active.NodeName {
		return "", fmt.Errorf("compute: the snapshot of slot %d is the guest's live disk", disk.Slot)
	}

	jobID := fmt.Sprintf("commit-%d-%d", disk.Slot, time.Now().UnixNano())
	commitCtx, cancel := context.WithTimeout(ctx, blockCommitTimeout)
	defer cancel()

	if frozen.Image.BackingFile != "" {
		// An intermediate layer: its data merges into the layer below and the
		// chain simply gets shorter. Nothing the guest writes to moves, and
		// QEMU finishes this on its own.
		if err := monitor.BlockCommit(commitCtx, jobID, active.NodeName, frozen.NodeName, ""); err != nil {
			return "", fmt.Errorf("compute: merging the snapshot of slot %d: %w", disk.Slot, err)
		}
		if err := monitor.AwaitBlockJob(commitCtx, jobID, false, nil); err != nil {
			return "", fmt.Errorf("compute: merging the snapshot of slot %d: %w", disk.Slot, err)
		}
		if err := os.Remove(disk.FrozenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			rt.logger.Warn("compute: the merged snapshot file could not be removed",
				"file", disk.FrozenPath, "error", err)
		}
		return "", nil
	}

	// Nothing below the snapshot to merge into, so everything above it merges
	// down instead and QEMU pivots onto the snapshot's own file. The guest ends
	// up exactly where it was, in a different file.
	if err := monitor.BlockCommit(commitCtx, jobID, active.NodeName, "", frozen.NodeName); err != nil {
		return "", fmt.Errorf("compute: merging the snapshot of slot %d: %w", disk.Slot, err)
	}
	if err := monitor.AwaitBlockJob(commitCtx, jobID, true, nil); err != nil {
		return "", fmt.Errorf("compute: merging the snapshot of slot %d: %w", disk.Slot, err)
	}
	if err := os.Remove(disk.ActivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		rt.logger.Warn("compute: the merged overlay could not be removed",
			"file", disk.ActivePath, "error", err)
	}
	return disk.FrozenPath, nil
}

// repointManifests rewrites the other snapshots' record of where the guest
// writes, after a live merge moved it.
//
// Best effort and logged: the merge already happened, and a manifest that still
// names the old path would offer a revert onto a file that no longer exists.
func (rt *computeRuntime) repointManifests(vmID, skip string, slot int, oldPath, newPath string) {
	entries, err := os.ReadDir(snapshotRoot(vmID))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		other := strings.TrimSuffix(entry.Name(), ".json")
		if other == skip {
			continue
		}
		manifest, err := readManifest(vmID, other)
		if err != nil {
			continue
		}
		changed := false
		for i := range manifest.Disks {
			if manifest.Disks[i].Slot == slot && manifest.Disks[i].ActivePath == oldPath {
				manifest.Disks[i].ActivePath = newPath
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := writeManifest(manifest); err != nil {
			rt.logger.Warn("compute: repointing a snapshot manifest after a merge failed",
				"vm", vmID, "snapshot", other, "error", err)
		}
	}
}

// nodeNameMaxLen is the longest node name QEMU will accept: `bdrv_assign_node_name`
// rejects 32 characters or more with the opaque error "Node name too long".
const nodeNameMaxLen = qmp.MaxNodeNameLen

// overlayNodeName names the overlay node for a live snapshot of one disk slot,
// truncating with a hash suffix because a timestamped snapshot name alone
// already exceeds what QEMU accepts.
func overlayNodeName(slot int, name string) string {
	prefix := fmt.Sprintf("blk%d-", slot)
	node := prefix + sanitizeNodeName(name)
	if len(node) <= nodeNameMaxLen {
		return node
	}
	sum := fnv.New32a()
	errlog.Value(sum.Write([]byte(name)))
	suffix := fmt.Sprintf("-%08x", sum.Sum32())
	keep := nodeNameMaxLen - len(prefix) - len(suffix)
	if keep < 0 {
		keep = 0
	}
	return prefix + sanitizeNodeName(name)[:keep] + suffix
}

// sanitizeNodeName reduces a snapshot name to what QEMU accepts as a node name,
// which is alphanumerics, dashes, dots and underscores.
func sanitizeNodeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// foldLiveOverlay removes a live qcow2 snapshot by folding it back into the
// chain, which is the same operation the dir driver performs offline.
func foldLiveOverlay(ctx context.Context, driver storage.Driver, pool storage.Pool, disk snapshotDisk) error {
	if err := driver.DeleteSnapshot(ctx, pool, disk.ActivePath,
		storage.Snapshot{Handle: disk.FrozenPath, Mechanism: "qcow2"}); err != nil {
		return fmt.Errorf("compute: deleting the snapshot of slot %d: %w", disk.Slot, err)
	}
	return nil
}
