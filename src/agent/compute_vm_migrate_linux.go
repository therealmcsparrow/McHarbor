// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Moving a guest to another host.
//
// One vm_migrate is always one host's half; McHarbor sequences the halves and
// owns the rollback. Two shapes:
//
//   - Cold: the guest is stopped, each disk is streamed from this host to
//     McHarbor and from McHarbor to the target (send / receive), McHarbor
//     defines and starts it there. The stream is never buffered: McHarbor
//     pipes one HTTP body into the other, so the slower host sets the pace.
//   - Live: the target starts QEMU waiting for an incoming migration with an
//     NBD server for the disks (live_prepare), the source mirrors every disk
//     into it and then migrates its RAM (live_send), and the target resumes
//     (live_finish). Every byte of that crosses one direct host-to-host TCP
//     path, over mutual TLS with a certificate McHarbor minted for this one
//     migration.

// migrateNVRAMDiskID names the UEFI variable store in a migration's disk list.
// Its path is never taken from the request: both ends derive it from the guest
// id, so the entry can only ever move that one file.
const migrateNVRAMDiskID = "nvram"

// migrateTLSID is the QOM id of the migration's TLS credential on both hosts.
const migrateTLSID = "mcharbor-migrate-tls"

// migrateStepTimeout bounds one QMP setup step, which never scales with the
// size of the guest.
const migrateStepTimeout = 30 * time.Second

// migrateLiveBudget bounds a live migration from the first mirror to the
// switchover. A guest dirtying memory faster than the link moves it never
// converges, and this is what ends that attempt instead of holding it forever.
const migrateLiveBudget = 6 * time.Hour

// migrateProgressEvery spaces migration progress events.
const migrateProgressEvery = 2 * time.Second

// handleVMMigrate answers vm_migrate, one phase at a time.
func (rt *computeRuntime) handleVMMigrate(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	spec := req.Migrate
	if spec == nil {
		return errors.New("compute: vm_migrate carried no migration")
	}
	if err := validateGuestID(req.VMID); err != nil {
		return err
	}
	out := &VMMigrateSpec{Phase: spec.Phase, MigrationID: spec.MigrationID}
	reply.Migrate = out

	var err error
	switch spec.Phase {
	case VMMigratePhaseSend:
		err = rt.migrateSend(ctx, req, out)
	case VMMigratePhaseReceive:
		err = rt.migrateReceive(ctx, req, out)
	case VMMigratePhaseLiveCheck:
		err = rt.migrateLiveCheck(ctx, req, out)
	case VMMigratePhaseLivePrepare:
		err = rt.migrateLivePrepare(ctx, req, reply, out)
	case VMMigratePhaseLiveSend:
		err = rt.migrateLiveSend(ctx, req)
	case VMMigratePhaseLiveFinish:
		err = rt.migrateLiveFinish(ctx, req)
	case VMMigratePhaseAbort:
		err = rt.migrateAbort(ctx, req)
	default:
		err = fmt.Errorf("compute: %q is not a migration phase", spec.Phase)
	}
	return err
}

// guestRunning reports whether a guest has a live QEMU on this host.
func (rt *computeRuntime) guestRunning(ctx context.Context, vmID string) bool {
	states, _, err := rt.vms.Query(ctx, []string{vmID})
	if err != nil {
		return false
	}
	for _, state := range states {
		if state.VMID == vmID && (state.State == vm.StateRunning || state.State == vm.StatePaused) {
			return true
		}
	}
	return false
}

// --- Cold ------------------------------------------------------------------

// migrateSend streams each named disk to McHarbor's relay.
//
// Only a stopped guest's disks are sent; the one exception is a list that holds
// nothing but the UEFI variable store, which a live migration copies ahead of
// the switchover. A disk with a backing chain is flattened into a scratch file
// first, so the target receives one standalone image.
func (rt *computeRuntime) migrateSend(ctx context.Context, req *VMPayload, out *VMMigrateSpec) error {
	disks := req.Migrate.Disks
	if len(disks) == 0 {
		return errors.New("compute: a migration send named no disks")
	}
	onlyNVRAM := true
	for _, disk := range disks {
		if disk.DiskID != migrateNVRAMDiskID {
			onlyNVRAM = false
		}
	}
	if !onlyNVRAM && rt.guestRunning(ctx, req.VMID) {
		return fmt.Errorf("compute: %s is running; a cold migration needs it stopped", req.VMID)
	}

	var total int64
	for i, disk := range disks {
		rt.publishMigrateProgress(req.VMID, percentOf(i, len(disks)), "sending", total)
		source := vm.PathsFor(req.VMID).VarsFile
		if disk.DiskID != migrateNVRAMDiskID {
			owned, err := sourceDiskFor(req.Spec, disk)
			if err != nil {
				return err
			}
			source = owned.VolumePath
		}
		sent, err := rt.sendMigrationDisk(ctx, req.VMID, disk, source, total)
		if err != nil {
			return fmt.Errorf("compute: sending disk %s: %w", disk.DiskID, err)
		}
		total += sent
	}
	rt.publishMigrateProgress(req.VMID, 100, "sent", total)
	out.BytesMoved = total
	return nil
}

// sendMigrationDisk uploads one disk, flattened when it has a backing chain.
func (rt *computeRuntime) sendMigrationDisk(ctx context.Context, vmID string, disk VMMigrateDisk, source string, base int64) (int64, error) {
	path := source
	format := defaultString(disk.Format, "raw")
	if disk.DiskID != migrateNVRAMDiskID {
		if backing, _ := imageBacking(ctx, source); backing != "" {
			flat, err := flattenForMigration(ctx, source)
			if err != nil {
				return 0, err
			}
			defer removeScratch(flat)
			path, format = flat, "qcow2"
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { errlog.Close(file) }()
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("sizing %s: %w", path, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("rewinding %s: %w", path, err)
	}

	url := rt.absoluteServerURL(disk.URL)
	if url == "" {
		return 0, errors.New("the migration relay URL could not be resolved against this host's McHarbor address")
	}
	body := &countingReader{inner: file, report: func(n int64) {
		rt.publishMigrateProgress(vmID, -1, "sending", base+n)
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return 0, fmt.Errorf("building the relay upload: %w", err)
	}
	request.ContentLength = size
	request.Header.Set("Authorization", "Bearer "+disk.Token)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-McHarbor-Disk", disk.DiskID)
	request.Header.Set("X-McHarbor-Format", format)

	response, err := rt.importClient().Do(request)
	if err != nil {
		return 0, fmt.Errorf("uploading to the relay: %w", err)
	}
	defer func() { errlog.Close(response.Body) }()
	errlog.Discard(response.Body, 64<<10)
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return 0, fmt.Errorf("McHarbor refused the relay upload with %s", response.Status)
	}
	return size, nil
}

// flattenForMigration writes a standalone copy of an image with a backing chain
// beside it, and returns its path. The caller removes it.
func flattenForMigration(ctx context.Context, source string) (string, error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("naming the flattened copy: %w", err)
	}
	flat := source + ".migrate-" + hex.EncodeToString(suffix)
	command := exec.CommandContext(ctx, "qemu-img", "convert", "-U", "-O", "qcow2", source, flat)
	if out, err := command.CombinedOutput(); err != nil {
		removeScratch(flat)
		return "", fmt.Errorf("flattening %s: %w (%s)", source, err, strings.TrimSpace(string(out)))
	}
	return flat, nil
}

// removeScratch removes a scratch file, logging rather than failing: the
// migration's outcome does not depend on it.
func removeScratch(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "compute: removing migration scratch %s: %v\n", path, err)
	}
}

// migrateReceive pulls each disk from McHarbor's relay into a new volume in
// the target pool. What it made is recorded under the migration id, so an abort
// can remove exactly that; a failure here removes it before answering.
func (rt *computeRuntime) migrateReceive(ctx context.Context, req *VMPayload, out *VMMigrateSpec) error {
	spec := req.Migrate
	if len(spec.Disks) == 0 {
		return errors.New("compute: a migration receive named no disks")
	}
	if rt.guestRunning(ctx, req.VMID) {
		return fmt.Errorf("compute: %s is already running on this host", req.VMID)
	}
	target := &migrationTarget{VMID: req.VMID, Token: spec.Token}
	if err := rt.migrations.add(spec.MigrationID, target); err != nil {
		return err
	}

	var total int64
	for i, disk := range spec.Disks {
		rt.publishMigrateProgress(req.VMID, percentOf(i, len(spec.Disks)), "receiving", total)
		landed, received, err := rt.receiveMigrationDisk(ctx, req, disk, target, total)
		if err != nil {
			if undoErr := rt.undoTarget(context.WithoutCancel(ctx), target); undoErr != nil {
				rt.logger.Warn("compute: undoing a failed migration target", "vm", target.VMID, "error", undoErr)
			}
			if _, takeErr := rt.migrations.take(spec.MigrationID, spec.Token); takeErr != nil {
				rt.logger.Debug("compute: migration already cleared", "migration", spec.MigrationID)
			}
			return fmt.Errorf("compute: receiving disk %s: %w", disk.DiskID, err)
		}
		total += received
		out.Disks = append(out.Disks, landed)
	}
	rt.publishMigrateProgress(req.VMID, 100, "received", total)
	out.BytesMoved = total
	return nil
}

// receiveMigrationDisk lands one disk: straight into a file volume, or through
// a scratch file and qemu-img into a block volume, which cannot hold qcow2.
func (rt *computeRuntime) receiveMigrationDisk(
	ctx context.Context, req *VMPayload, disk VMMigrateDisk, target *migrationTarget, base int64,
) (VMMigrateDisk, int64, error) {
	landed := VMMigrateDisk{DiskID: disk.DiskID, Slot: disk.Slot}
	body, format, err := rt.openRelayDownload(ctx, disk)
	if err != nil {
		return landed, 0, err
	}
	defer func() { errlog.Close(body) }()
	counted := &countingReader{inner: body, report: func(n int64) {
		rt.publishMigrateProgress(req.VMID, -1, "receiving", base+n)
	}}

	if disk.DiskID == migrateNVRAMDiskID {
		n, err := rt.receiveNVRAM(req, counted)
		return landed, n, err
	}

	driver, err := rt.pools.Get(disk.PoolBackend)
	if err != nil {
		return landed, 0, err
	}
	if disk.PoolBackend == storage.BackendDir || disk.PoolBackend == storage.BackendBtrfs {
		if err := storage.ValidatePoolTarget(disk.PoolTarget); err != nil {
			return landed, 0, err
		}
	}
	pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}
	volume, err := driver.CreateVolume(ctx, storage.CreateVolumeRequest{
		Pool: pool, Name: volumeNameFor(req.VMID, disk.Slot), Kind: storage.VolumeDisk,
		Size: disk.SizeBytes, Format: format, Sparse: true,
	})
	if err != nil {
		return landed, 0, err
	}
	target.Volumes = append(target.Volumes, migratedVolume{
		Backend: disk.PoolBackend, PoolID: disk.PoolID, Target: disk.PoolTarget, Path: volume.Path,
	})

	var written int64
	if !volume.IsBlock {
		written, err = writeInto(volume.Path, counted, os.O_WRONLY|os.O_TRUNC)
		landed.TargetFormat = format
	} else if format == "raw" {
		written, err = writeInto(volume.Path, counted, os.O_WRONLY)
		landed.TargetFormat = "raw"
	} else {
		written, err = rt.convertIntoBlock(ctx, counted, volume.Path)
		landed.TargetFormat = "raw"
	}
	if err != nil {
		return landed, written, err
	}
	landed.VolumePath = volume.Path
	return landed, written, nil
}

// openRelayDownload opens the relay's stream for one disk.
func (rt *computeRuntime) openRelayDownload(ctx context.Context, disk VMMigrateDisk) (io.ReadCloser, string, error) {
	url := rt.absoluteServerURL(disk.URL)
	if url == "" {
		return nil, "", errors.New("the migration relay URL could not be resolved against this host's McHarbor address")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building the relay download: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+disk.Token)
	response, err := rt.importClient().Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("downloading from the relay: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		errlog.Close(response.Body)
		return nil, "", fmt.Errorf("McHarbor refused the relay download with %s", response.Status)
	}
	format := strings.TrimSpace(response.Header.Get("X-McHarbor-Format"))
	switch format {
	case "qcow2", "raw":
	case "":
		format = defaultString(disk.Format, "raw")
	default:
		errlog.Close(response.Body)
		return nil, "", fmt.Errorf("the relay announced an unknown disk format %q", format)
	}
	return response.Body, format, nil
}

// receiveNVRAM writes the UEFI variable store at the path the guest id names,
// and records the local template beside it so the next start keeps it.
func (rt *computeRuntime) receiveNVRAM(req *VMPayload, body io.Reader) (int64, error) {
	if req.Spec == nil {
		return 0, errors.New("the variable store needs the guest's spec")
	}
	paths := vm.PathsFor(req.VMID)
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		return 0, fmt.Errorf("preparing %s: %w", paths.Dir, err)
	}
	written, err := writeInto(paths.VarsFile, io.LimitReader(body, 64<<20), os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return written, err
	}
	_, template := rt.firmwarePaths(req.Spec.Firmware == "uefi" && req.Spec.SecureBoot)
	if template != "" {
		if err := os.WriteFile(paths.VarsFile+".source", []byte(template), 0o600); err != nil {
			return written, fmt.Errorf("recording the variable store's template: %w", err)
		}
	}
	return written, nil
}

// writeInto copies a stream into a file or block device.
func writeInto(path string, body io.Reader, flags int) (int64, error) {
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", path, err)
	}
	written, copyErr := io.Copy(file, body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return written, fmt.Errorf("writing %s: %w", path, err)
	}
	return written, nil
}

// convertIntoBlock spools a qcow2 stream to scratch and converts it onto a
// block volume, which cannot hold qcow2 itself.
func (rt *computeRuntime) convertIntoBlock(ctx context.Context, body io.Reader, device string) (int64, error) {
	scratchDir := filepath.Join(computeRoot, "migrate")
	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		return 0, fmt.Errorf("preparing %s: %w", scratchDir, err)
	}
	scratch, err := os.CreateTemp(scratchDir, "incoming-*.qcow2")
	if err != nil {
		return 0, fmt.Errorf("creating the scratch image: %w", err)
	}
	scratchPath := scratch.Name()
	defer removeScratch(scratchPath)
	written, copyErr := io.Copy(scratch, body)
	if err := errors.Join(copyErr, scratch.Close()); err != nil {
		return written, fmt.Errorf("spooling the incoming image: %w", err)
	}
	command := exec.CommandContext(ctx, "qemu-img", "convert", "-n", "-O", "raw", scratchPath, device)
	if out, err := command.CombinedOutput(); err != nil {
		return written, fmt.Errorf("writing onto %s: %w (%s)", device, err, strings.TrimSpace(string(out)))
	}
	return written, nil
}

// --- Live ------------------------------------------------------------------

// migrateLiveCheck answers the pre-flight both hosts get: whether the guest
// runs here with SPICE, and where this host can be reached.
func (rt *computeRuntime) migrateLiveCheck(ctx context.Context, req *VMPayload, out *VMMigrateSpec) error {
	out.ListenAddrs = hostAddresses()
	if rt.guestRunning(ctx, req.VMID) {
		if _, err := os.Stat(vm.PathsFor(req.VMID).SPICESocket); err == nil {
			out.SPICE = true
		}
		out.HotplugLayout = vm.HasHotplugLayout(vm.PathsFor(req.VMID).ArgsFile)
	}
	return nil
}

// migrateLivePrepare starts the target half: volumes, the TLS credential, and a
// QEMU waiting for the incoming state with an NBD server for the disks.
func (rt *computeRuntime) migrateLivePrepare(ctx context.Context, req *VMPayload, reply *VMPayload, out *VMMigrateSpec) error {
	spec := req.Migrate
	if req.Spec == nil || len(spec.Disks) == 0 {
		return errors.New("compute: a live prepare needs the guest's spec and its disks")
	}
	if err := checkMigrationTLS(spec.TLS); err != nil {
		return err
	}
	if spec.SPICE && !rt.spiceSupported(ctx) {
		return errors.New("compute: the guest runs with SPICE and this host's QEMU has no SPICE, so its state cannot be loaded here")
	}
	if rt.guestRunning(ctx, req.VMID) {
		return fmt.Errorf("compute: %s is already running on this host", req.VMID)
	}

	target := &migrationTarget{VMID: req.VMID, Token: spec.Token}
	if err := rt.migrations.add(spec.MigrationID, target); err != nil {
		return err
	}
	fail := func(err error) error {
		if undoErr := rt.undoTarget(context.WithoutCancel(ctx), target); undoErr != nil {
			rt.logger.Warn("compute: undoing a failed migration target", "vm", target.VMID, "error", undoErr)
		}
		if _, takeErr := rt.migrations.take(spec.MigrationID, spec.Token); takeErr != nil {
			rt.logger.Debug("compute: migration already cleared", "migration", spec.MigrationID)
		}
		return err
	}

	volumes := make(map[int]VMMigrateDisk, len(spec.Disks))
	for _, disk := range spec.Disks {
		driver, err := rt.pools.Get(disk.PoolBackend)
		if err != nil {
			return fail(err)
		}
		pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}
		volume, err := driver.CreateVolume(ctx, storage.CreateVolumeRequest{
			Pool: pool, Name: volumeNameFor(req.VMID, disk.Slot), Kind: storage.VolumeDisk,
			Size: disk.SizeBytes, Format: defaultString(disk.Format, "qcow2"), Sparse: true,
		})
		if err != nil {
			return fail(err)
		}
		target.Volumes = append(target.Volumes, migratedVolume{
			Backend: disk.PoolBackend, PoolID: disk.PoolID, Target: disk.PoolTarget, Path: volume.Path,
		})
		volumes[disk.Slot] = VMMigrateDisk{
			DiskID: disk.DiskID, Slot: disk.Slot, VolumePath: volume.Path, TargetFormat: volume.Format,
		}
	}

	paths := vm.PathsFor(req.VMID)
	target.TLSDir = filepath.Join(paths.Dir, "migrate-tls")
	if err := writeMigrationTLS(target.TLSDir, spec.TLS, "server"); err != nil {
		return fail(err)
	}

	nbdPort, err := freeTCPPort(ctx)
	if err != nil {
		return fail(err)
	}
	migrationPort, err := freeTCPPort(ctx)
	if err != nil {
		return fail(err)
	}

	guest := *req.Spec
	guest.Disks = nil
	for _, disk := range req.Spec.Disks {
		landed, ok := volumes[disk.Slot]
		if !ok || disk.IsCDROM {
			continue
		}
		disk.VolumePath = landed.VolumePath
		disk.Format = landed.TargetFormat
		disk.BackingPath = ""
		guest.Disks = append(guest.Disks, disk)
	}
	startReq := &VMPayload{VMID: req.VMID, Spec: &guest}
	err = rt.startVMWith(ctx, startReq, reply, func(runtime *vm.Spec) {
		runtime.SPICE = spec.SPICE
		runtime.LegacyLayout = !spec.HotplugLayout
		runtime.IncomingURI = "defer"
	})
	if err != nil {
		return fail(err)
	}
	target.Started = true

	setupCtx, cancel := context.WithTimeout(ctx, migrateStepTimeout)
	defer cancel()
	monitor, err := rt.vms.Monitor(setupCtx, req.VMID)
	if err != nil {
		return fail(err)
	}
	steps := []func() error{
		func() error { return monitor.AddTLSCreds(setupCtx, migrateTLSID, target.TLSDir, "server") },
		func() error { return monitor.StartNBDServer(setupCtx, "0.0.0.0", nbdPort, migrateTLSID) },
	}
	for _, disk := range guest.Disks {
		node, export := fmt.Sprintf("blk%d", disk.Slot), fmt.Sprintf("disk%d", disk.Slot)
		steps = append(steps, func() error { return monitor.ExportNBD(setupCtx, node, export) })
	}
	steps = append(steps,
		func() error { return monitor.SetMigrateTLS(setupCtx, migrateTLSID) },
		func() error {
			return monitor.MigrateIncoming(setupCtx, migrationURI("0.0.0.0", migrationPort))
		},
	)
	for _, step := range steps {
		if err := step(); err != nil {
			return fail(fmt.Errorf("compute: preparing the incoming migration: %w", err))
		}
	}

	out.TargetPort = migrationPort
	out.NBDPort = nbdPort
	out.ListenAddrs = hostAddresses()
	for _, disk := range spec.Disks {
		out.Disks = append(out.Disks, volumes[disk.Slot])
	}
	return nil
}

// migrateLiveSend mirrors every disk into the target and then moves the RAM.
//
// A failure anywhere cancels what it started and leaves the source running,
// which is the whole promise of a live migration: it either moves the guest or
// changes nothing.
func (rt *computeRuntime) migrateLiveSend(ctx context.Context, req *VMPayload) error {
	spec := req.Migrate
	if err := validateMigrationPeer(spec.TargetHost, spec.TargetPort, spec.NBDPort); err != nil {
		return err
	}
	if err := checkMigrationTLS(spec.TLS); err != nil {
		return err
	}
	if !rt.guestRunning(ctx, req.VMID) {
		return fmt.Errorf("compute: %s is not running; migrate it cold instead", req.VMID)
	}

	liveCtx, cancel := context.WithTimeout(ctx, migrateLiveBudget)
	defer cancel()
	rt.vms.RegisterCancel("migrate-"+req.VMID, cancel)
	defer rt.vms.Cancel("migrate-" + req.VMID)

	monitor, err := rt.vms.Monitor(liveCtx, req.VMID)
	if err != nil {
		return err
	}
	tlsDir := filepath.Join(vm.PathsFor(req.VMID).Dir, "migrate-tls")
	if err := writeMigrationTLS(tlsDir, spec.TLS, "client"); err != nil {
		return err
	}
	defer removeTLSDir(tlsDir)

	var jobs, nodes []string
	cleanup := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), migrateStepTimeout)
		defer cleanupCancel()
		errlog.Log(monitor.MigrateCancel(cleanupCtx))
		for _, job := range jobs {
			errlog.Log(monitor.BlockJobCancel(cleanupCtx, job))
		}
		// A cancelled mirror takes a moment to let go of its target node.
		time.Sleep(time.Second)
		for _, node := range nodes {
			errlog.Log(monitor.RemoveBlockdev(cleanupCtx, node))
		}
		errlog.Log(monitor.DelObject(cleanupCtx, migrateTLSID))
	}

	if err := monitor.AddTLSCreds(liveCtx, migrateTLSID, tlsDir, "client"); err != nil {
		return fmt.Errorf("compute: loading the migration certificate: %w", err)
	}
	for _, disk := range spec.Disks {
		owned, err := sourceDiskFor(req.Spec, disk)
		if err != nil {
			cleanup()
			return err
		}
		top, err := monitor.TopNodeForFile(liveCtx, owned.VolumePath)
		if err != nil {
			cleanup()
			return fmt.Errorf("compute: finding disk %s in the running guest: %w", disk.DiskID, err)
		}
		node, job := fmt.Sprintf("mig-nbd%d", disk.Slot), fmt.Sprintf("mig-mirror%d", disk.Slot)
		if err := monitor.AddNBDBlockdev(liveCtx, node, spec.TargetHost, spec.NBDPort,
			fmt.Sprintf("disk%d", disk.Slot), migrateTLSID); err != nil {
			cleanup()
			return fmt.Errorf("compute: reaching the target's disk %s: %w", disk.DiskID, err)
		}
		nodes = append(nodes, node)
		if err := monitor.Mirror(liveCtx, job, top.NodeName, node); err != nil {
			cleanup()
			return fmt.Errorf("compute: mirroring disk %s: %w", disk.DiskID, err)
		}
		jobs = append(jobs, job)
	}

	last := time.Time{}
	if err := monitor.AwaitJobsReady(liveCtx, jobs, func(done, total int64) {
		if time.Since(last) >= migrateProgressEvery {
			last = time.Now()
			rt.publishMigrateProgress(req.VMID, ratio(done, total, 70), "mirroring", done)
		}
	}); err != nil {
		cleanup()
		return err
	}

	if err := monitor.SetMigrateParameters(liveCtx, qmp.MigrateParameters{
		MaxBandwidth:  spec.MaxBandwidthMB << 20,
		DowntimeLimit: spec.MaxDowntimeMS,
	}); err != nil {
		cleanup()
		return fmt.Errorf("compute: setting the migration limits: %w", err)
	}
	if err := monitor.SetMigrateTLS(liveCtx, migrateTLSID); err != nil {
		cleanup()
		return fmt.Errorf("compute: securing the migration channel: %w", err)
	}
	if err := monitor.Migrate(liveCtx, migrationURI(spec.TargetHost, spec.TargetPort), false); err != nil {
		cleanup()
		return fmt.Errorf("compute: starting the migration: %w", err)
	}
	if err := monitor.AwaitMigration(liveCtx, func(status qmp.MigrateStatus) {
		if time.Since(last) >= migrateProgressEvery {
			last = time.Now()
			if percent := status.Percent(); percent >= 0 {
				rt.publishMigrateProgress(req.VMID, 70+percent*30/100, "migrating", 0)
			}
		}
	}); err != nil {
		cleanup()
		return err
	}

	// The guest now runs on the target. The source is paused post-migration;
	// its mirrors are released and its QEMU is ended.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), migrateStepTimeout)
	defer finishCancel()
	for _, job := range jobs {
		errlog.Log(monitor.BlockJobCancel(finishCtx, job))
	}
	if err := rt.vms.Stop(finishCtx, req.VMID); err != nil {
		rt.logger.Warn("compute: the migrated guest's old QEMU did not stop", "vm", req.VMID, "error", err)
	}
	rt.releaseGuestTaps(req.VMID)
	rt.publishMigrateProgress(req.VMID, 100, "migrated", 0)
	return nil
}

// migrateLiveFinish commits a target half. For a live move it closes the NBD
// server and resumes the guest; for a cold one it only forgets the transfer, so
// no later abort can remove the volumes the guest now runs on.
func (rt *computeRuntime) migrateLiveFinish(ctx context.Context, req *VMPayload) error {
	target, err := rt.migrations.take(req.Migrate.MigrationID, req.Migrate.Token)
	if err != nil {
		return err
	}
	if !target.Started {
		return nil
	}
	defer removeTLSDir(target.TLSDir)
	stepCtx, cancel := context.WithTimeout(ctx, migrateStepTimeout)
	defer cancel()
	monitor, err := rt.vms.Monitor(stepCtx, target.VMID)
	if err != nil {
		return err
	}
	errlog.Log(monitor.StopNBDServer(stepCtx))
	errlog.Log(monitor.DelObject(stepCtx, migrateTLSID))
	status, err := monitor.Status(stepCtx)
	if err != nil {
		return err
	}
	if !status.Running {
		if err := monitor.Resume(stepCtx); err != nil {
			return fmt.Errorf("compute: resuming the migrated guest: %w", err)
		}
	}
	return nil
}

// migrateAbort undoes a prepared or received target: its QEMU if one was
// started, and every volume the migration created.
func (rt *computeRuntime) migrateAbort(ctx context.Context, req *VMPayload) error {
	target, err := rt.migrations.take(req.Migrate.MigrationID, req.Migrate.Token)
	if err != nil {
		return err
	}
	return rt.undoTarget(ctx, target)
}

// undoTarget removes what a migration target made.
func (rt *computeRuntime) undoTarget(ctx context.Context, target *migrationTarget) error {
	var failures []error
	if target.Started {
		if err := rt.vms.Delete(ctx, target.VMID); err != nil {
			failures = append(failures, err)
		}
		rt.releaseGuestTaps(target.VMID)
	}
	removeTLSDir(target.TLSDir)
	for _, volume := range target.Volumes {
		driver, err := rt.pools.Get(volume.Backend)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		pool := storage.Pool{ID: volume.PoolID, Backend: volume.Backend, Target: volume.Target}
		if err := driver.DeleteVolume(ctx, pool, volume.Path); err != nil {
			failures = append(failures, fmt.Errorf("removing %s: %w", volume.Path, err))
		}
	}
	return errors.Join(failures...)
}

// checkMigrationTLS refuses a live migration without complete, parseable TLS
// material, before anything is written to disk.
func checkMigrationTLS(material *VMMigrateTLS) error {
	if material == nil || material.CACert == "" || material.Cert == "" || material.Key == "" {
		return errors.New("compute: a live migration needs its TLS certificate, key and CA")
	}
	if _, err := tls.X509KeyPair([]byte(material.Cert), []byte(material.Key)); err != nil {
		return fmt.Errorf("compute: the migration certificate does not match its key: %w", err)
	}
	return nil
}

// writeMigrationTLS lays the material out the way QEMU's tls-creds-x509 reads
// it: ca-cert.pem plus <endpoint>-cert.pem and <endpoint>-key.pem, 0600 in a
// 0700 directory under the guest's runtime directory.
func writeMigrationTLS(dir string, material *VMMigrateTLS, endpoint string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("compute: preparing %s: %w", dir, err)
	}
	files := map[string]string{
		"ca-cert.pem":          material.CACert,
		endpoint + "-cert.pem": material.Cert,
		endpoint + "-key.pem":  material.Key,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			removeTLSDir(dir)
			return fmt.Errorf("compute: writing %s: %w", name, err)
		}
	}
	return nil
}

// removeTLSDir deletes a migration's key material. It is short-lived by design
// and must not outlive the migration on either host.
func removeTLSDir(dir string) {
	if dir == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(os.Stderr, "compute: removing migration TLS material %s: %v\n", dir, err)
	}
}

// freeTCPPort asks the kernel for a port nothing holds. QEMU binds it a moment
// later; the window in between is the price of not asking the operator for one.
func freeTCPPort(ctx context.Context) (int, error) {
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("compute: finding a free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, fmt.Errorf("compute: releasing a probe port: %w", err)
	}
	return port, nil
}

// ratio scales done/total into 0..span.
func ratio(done, total int64, span int) int {
	if total <= 0 {
		return 0
	}
	return int(done * int64(span) / total)
}

// publishMigrateProgress reports a migration's progress through the guest event
// channel. A negative percent means "bytes moved, percent unchanged".
func (rt *computeRuntime) publishMigrateProgress(vmID string, percent int, step string, bytes int64) {
	detail := map[string]string{"step": step, "bytes": strconv.FormatInt(bytes, 10)}
	if percent >= 0 {
		detail["percent"] = strconv.Itoa(percent)
	}
	rt.publishVMEvent(vm.Event{VMID: vmID, Kind: "migrate_progress", At: time.Now().UTC(), Detail: detail})
}

// countingReader reports how many bytes have passed, at most every couple of
// seconds, so a large transfer does not flood the event channel.
type countingReader struct {
	inner  io.Reader
	read   int64
	last   time.Time
	report func(int64)
}

// Read passes bytes through and reports the running total.
func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	r.read += int64(n)
	if r.report != nil && (err != nil || time.Since(r.last) >= migrateProgressEvery) {
		r.last = time.Now()
		r.report(r.read)
	}
	return n, err
}
