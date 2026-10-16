// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/capability"
	"github.com/therealmcsparrow/mcharbor-agent/compute/ctsnap"
	"github.com/therealmcsparrow/mcharbor-agent/compute/dhcp"
	"github.com/therealmcsparrow/mcharbor-agent/compute/hostdev"
	"github.com/therealmcsparrow/mcharbor-agent/compute/imagefetch"
	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/compute/supervisor"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
)

// computeRoot is where all compute state lives on the host. Everything under it
// is McHarbor's, named with the same prefix as the units and links, so an
// operator can always see what this feature put on their machine.
const computeRoot = "/var/lib/mcharbor/compute"

// imageCacheRoot is where downloaded guest images are kept.
const imageCacheRoot = computeRoot + "/images"

// seccompBuiltIn records whether this binary was compiled with the seccomp
// build tag. See compute_seccomp*.go — it is a build-time fact, and the
// capability probe reports it so the posture is never invisible.

// computeRuntime holds the agent's compute state.
//
// It is created lazily on the first compute message rather than at startup: an
// agent on a host that will never run guests should not create directories,
// probe hardware, or hold a D-Bus connection open for a feature nobody enabled.
type computeRuntime struct {
	logger *slog.Logger

	sup     supervisor.Supervisor
	pools   *storage.Registry
	images  *imagefetch.Fetcher
	vms     *vm.Manager
	cts     *lxc.Manager
	console *consoleRegistry

	// capabilityMu guards the cached probe. Probing runs external binaries and
	// is re-run on demand and every six hours, not on every message.
	capabilityMu sync.RWMutex
	capability   *capability.Report
	probedAt     time.Time
	// probing is closed when the in-flight probe finishes, so concurrent
	// callers wait for that one instead of each starting their own. A probe
	// spawns a dozen QEMU processes; one page-load firing several VM calls at
	// once was enough to bury a small host and time every one of them out.
	probing chan struct{}

	// send publishes an unsolicited message — a guest event, a state change, a
	// stats sample — back to the server.
	send func(WSMessage) error

	// resolveServerURL turns a server-relative path into an absolute URL
	// against the McHarbor address this agent dials. The server sends relative
	// paths because only the agent knows how it reaches McHarbor.
	resolveServerURL func(string) (string, error)

	// serverClient talks to McHarbor over plain HTTP(S). It honours
	// MCHARBOR_INSECURE and carries connect deadlines, which is what separates
	// "cannot reach McHarbor" from an import that appears to hang.
	serverClient *http.Client

	// dhcp serves addresses on managed bridges. Built unconditionally: it
	// needs no supervisor, and a host that cannot run guests can still carry a
	// network for guests running elsewhere.
	dhcp *dhcp.Manager

	// adoptedMu guards the external-switch bookkeeping below.
	adoptedMu sync.Mutex
	// adopted remembers how to put each adopted adapter back. The kernel
	// cannot be asked what an interface looked like before it was enslaved, so
	// losing this map means losing the ability to undo cleanly.
	adopted map[string]netconf.AdoptState
	// pendingRevert holds the rollback timer for an adoption the server has
	// not confirmed yet, keyed by network id.
	pendingRevert map[string]*time.Timer

	// hostLinks persists each direct network's host shim, so the agent can put
	// it back after a reboot; it is the agent's own store, shared with Docker.
	hostLinks *hostLinkStore

	// ctSnaps holds the backup runs currently holding a rootfs snapshot, so a
	// sweep for what a killed agent left never removes one still being read.
	ctSnaps *ctsnap.Active

	// backedVolumes records the volumes this agent created over a backing file,
	// so the images-in-use check covers a guest that has never been started.
	backedVolumes *backedVolumeLedger

	// migrations holds the live-migration targets this host has prepared and
	// not yet finished or aborted, keyed by migration id.
	migrations *migrationTargets

	// supCancel tears down the supervisor's system-bus connection. It is the
	// only thing that should ever end that connection's context — see
	// newComputeRuntime.
	supCancel context.CancelFunc
}

var (
	computeOnce    sync.Once
	computeState   *computeRuntime
	computeInitErr error
)

// computeCapabilityTTL is how long a probe result is reused before the next
// message triggers a refresh.
const computeCapabilityTTL = 6 * time.Hour

// capabilityWait is how long a request waits for a probe that is still
// running before answering with what is already known.
const capabilityWait = 8 * time.Second

// capabilityProbeBudget bounds one probe. Generous because it is off the
// request path; bounded because a probe that never returns would leave the
// cache empty and every later request waiting the full capabilityWait.
const capabilityProbeBudget = 2 * time.Minute

// compute returns the runtime, creating it on first use.
func (a *Agent) compute(ctx context.Context) (*computeRuntime, error) {
	computeOnce.Do(func() {
		// The runtime is a process-wide singleton whose background work outlives
		// the request that happened to build it, so it keeps the values only.
		computeState, computeInitErr = newComputeRuntime(context.WithoutCancel(ctx), a)
	})
	return computeState, computeInitErr
}

// errNoSupervisor is what a guest operation answers on a host with no systemd
// reachable. It names the missing piece, because "no usable process supervisor"
// arriving from an image download told the operator nothing about which of the
// two halves of compute was actually unavailable.
var errNoSupervisor = errors.New("compute: this host has no reachable process supervisor, so guests cannot be run here; images and storage are unaffected")

// supervisorMode reports the supervisor the probe should advertise. "none" is a
// real answer: the host is usable for images and storage and not for guests,
// and the capability panel renders that honestly.
func (rt *computeRuntime) supervisorMode() string {
	if rt.sup == nil {
		return "none"
	}
	return rt.sup.Mode()
}

// newComputeRuntime builds the compute runtime.
//
// A failure to reach systemd is not fatal here: the CLI fallback covers it, and
// the capability probe reports which path is in use so the operator can see the
// downgrade rather than infer it from behaviour.
//
// **No supervisor at all is not fatal either.** It used to be, and that was the
// wrong boundary: the whole runtime refused to build, so a host without systemd
// could not list, download or evict an image — none of which needs a supervisor.
// Fetching is a download to disk and a checksum; `imagefetch` never touches one.
// Only guests do, through vm.NewManager and the console.
//
// So the supervisor is now optional. Its absence disables the guest half and is
// reported by the capability probe as supervisor mode "none"; images, pools and
// networks carry on working.
func newComputeRuntime(ctx context.Context, a *Agent) (*computeRuntime, error) {
	logger := slog.Default().With("subsystem", "compute")

	// The locations McHarbor set are restored before anything is built: the
	// image fetcher takes its cache root at construction, and a guest's QEMU
	// command line already points at the VM root it was started under.
	storedRoots := a.loadComputeRoots()
	applyStoredRoots(storedRoots)
	imageRoot := storedRoots.ImageRoot
	if imageRoot == "" {
		imageRoot = imageCacheRoot
	}

	if err := os.MkdirAll(computeRoot, 0o700); err != nil {
		return nil, fmt.Errorf("compute: create %s: %w", computeRoot, err)
	}

	// The bus connection lives as long as the agent, so it must NOT be built
	// from a context that this function cancels on the way out. go-dbus closes
	// the connection when its context is done:
	//
	//	go func() { <-conn.ctx.Done(); conn.Close() }()
	//
	// Built from a 30s timeout context with a deferred cancel, the system bus
	// was therefore closed the moment the runtime finished initialising. The
	// capability probe still passed — systemd was reachable when it ran — the
	// guest still defined, and only the start failed, with "dbus: connection
	// closed by user". Every guest start on the dbus path failed this way.
	supCtx, supCancel := context.WithCancel(ctx)

	sup, err := supervisor.New(supCtx)
	if err != nil {
		logger.Warn("systemd system bus unreachable; falling back to systemd-run",
			"error", err)
		supCancel()
		sup, err = supervisor.NewSystemdRun()
		if err != nil {
			// Guests need one; images, pools and networks do not. Logged at
			// warn and surfaced through the capability probe, so the operator
			// sees the limit on the host's own page rather than as a failure
			// on an unrelated button.
			logger.Warn("no process supervisor available; guests are unavailable on this host, images and storage still work",
				"error", err)
			sup = nil
		}
	}

	fetcher, err := imagefetch.New(imageRoot, distroKeyring())
	if err != nil {
		supCancel()
		return nil, err
	}
	// Pulling an uploaded image from McHarbor uses the agent's own client, so
	// it honours MCHARBOR_INSECURE exactly as the control channel and the
	// archive transfers do. Mirror downloads keep the fetcher's strict client.
	fetcher.UseServerClient(a.httpClient)

	rt := &computeRuntime{
		logger: logger,
		sup:    sup,
		pools: storage.NewRegistry(
			storage.NewDirDriver(),
			storage.NewZFSDriver(),
			storage.NewBtrfsDriver(),
			storage.NewLVMDriver(),
		),
		images:           fetcher,
		console:          newConsoleRegistry(),
		send:             a.sendCompute,
		resolveServerURL: a.resolveServerURL,
		serverClient:     a.httpClient,
		adopted:          make(map[string]netconf.AdoptState),
		pendingRevert:    make(map[string]*time.Timer),
		hostLinks:        a.hostLinks,
		supCancel:        supCancel,
		ctSnaps:          ctsnap.NewActive(),
		backedVolumes:    &backedVolumeLedger{path: filepath.Join(computeRoot, "backed-volumes.json")},
		migrations:       newMigrationTargets(),
	}
	rt.dhcp = dhcp.NewManager(filepath.Join(computeRoot, "dhcp"), rt.publishLease, logger)
	// The bridge and the taps survive an agent restart because they are kernel
	// state; the DHCP server does not, and a guest that cannot renew its lease
	// simply loses its address with nothing in the UI to explain it.
	if restored, err := rt.dhcp.RestoreAll(); err != nil {
		logger.Warn("compute: restoring managed network DHCP failed", "error", err)
	} else if restored > 0 {
		logger.Info("compute: managed network DHCP restored", "networks", restored)
	}

	// Left nil without a supervisor. Every guest entry point checks it and
	// answers with errNoSupervisor, which names the missing piece instead of
	// panicking on a nil manager.
	if sup != nil {
		rt.vms = vm.NewManager(sup, rt.publishVMEvent, rt.publishVMState)
	}
	rt.cts = lxc.NewManager(rt.publishCTState)

	// Adopt whatever is already running. The agent may have restarted, or
	// self-updated, while guests kept running under systemd — which is the
	// whole point of transient units — so reconciliation on connect is normal
	// operation rather than recovery.
	//
	// Skipped with no supervisor: there are no transient units to adopt, and
	// the manager that would query them was never built.
	if rt.vms != nil {
		go rt.reconcile(ctx)
	}

	// A container backup killed mid-run leaves a snapshot mounted under /run or
	// a snapshot volume in the pool. Bounded by its own timeout, like reconcile.
	go rt.sweepCTSnapshots(ctx)

	// A killed agent leaves unprivileged rootfs trees bind-staged under /run; the
	// sweep releases those of containers no longer running, and is bounded by its
	// own timeout the same way.
	go rt.sweepCTStaging(ctx)

	// Built up front so inserting one is instant. Independent of the
	// supervisor: the discs are files in the image cache, and a host with no
	// guests of its own can still hand one to a guest being built on it.
	go rt.prepareGuestDiscs(ctx)

	return rt, nil
}

// reconcile reports what is actually running after the agent starts.
//
// Units the server does not know about are reported as orphans and never
// killed: the operator may have a very good reason for them, and an agent that
// silently stops processes it does not recognise is not one anybody should run
// as root.
func (rt *computeRuntime) reconcile(parent context.Context) {
	// Belt and braces beside the caller's check. This runs in its own
	// goroutine, so a nil manager here is a panic on a background stack that
	// takes the whole agent down — which is exactly what it did.
	if rt.vms == nil {
		return
	}

	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	states, orphans, err := rt.vms.Query(ctx, nil)
	if err != nil {
		rt.logger.Warn("VM reconciliation failed", "error", err)
		return
	}
	if len(orphans) > 0 {
		rt.logger.Info("found VM units the server does not know about",
			"count", len(orphans), "units", orphans)
	}
	for _, state := range states {
		rt.publishVMState(state)
	}
}

// Capability returns the cached probe, refreshing it when stale or forced.
func (rt *computeRuntime) Capability(ctx context.Context, force bool) capability.Report {
	rt.capabilityMu.Lock()
	if cached := rt.capability; cached != nil && !force &&
		time.Since(rt.probedAt) < computeCapabilityTTL {
		rt.capabilityMu.Unlock()
		return *cached
	}
	waiting := rt.probing
	if waiting == nil {
		waiting = make(chan struct{})
		rt.probing = waiting
		go rt.probeCapability(context.WithoutCancel(ctx), waiting)
	}
	rt.capabilityMu.Unlock()

	// Bounded independently of the caller's deadline. The probe runs a dozen
	// external binaries, any one of which can be slow on a loaded host, and a
	// request that blocks on all of them times out on the server rather than
	// answering. Waiting briefly and then answering with what is known keeps
	// the agent responsive whatever the host is doing.
	select {
	case <-waiting:
	case <-time.After(capabilityWait):
	case <-ctx.Done():
	}

	rt.capabilityMu.RLock()
	defer rt.capabilityMu.RUnlock()
	if rt.capability != nil {
		return *rt.capability
	}
	return capability.Report{AgentVersion: agentVersion}
}

// probeCapability runs one probe and publishes it, on its own deadline.
//
// Detached from any request: a caller giving up must not cancel the probe, or
// a UI that retries every few seconds restarts it forever and it never
// completes once.
func (rt *computeRuntime) probeCapability(parent context.Context, done chan struct{}) {
	ctx, cancel := context.WithTimeout(parent, capabilityProbeBudget)
	defer cancel()

	report := capability.Probe(ctx, seccompBuiltIn, rt.supervisorMode())
	report.AgentVersion = agentVersion

	rt.capabilityMu.Lock()
	rt.capability = &report
	rt.probedAt = time.Now()
	rt.probing = nil
	rt.capabilityMu.Unlock()
	close(done)
}

// qemuBinaryFor picks the emulator for an architecture, refusing one without
// KVM acceleration.
//
// TCG is refused rather than offered: a software-emulated guest is slow enough
// to look broken, and shipping it would generate support load for a mode nobody
// actually wants.
func (rt *computeRuntime) qemuBinaryFor(ctx context.Context, arch string) (string, error) {
	report := rt.Capability(ctx, false)
	for _, binary := range report.QEMUBinaries {
		if arch != "" && binary.Arch != arch {
			continue
		}
		if binary.KVMAccel {
			return binary.Path, nil
		}
	}
	return "", fmt.Errorf("no KVM-accelerated qemu-system binary is available on this host")
}

// firmwarePaths resolves the UEFI firmware for a guest from the probe, so the
// argv builder never guesses at a path that differs per distribution.
func (rt *computeRuntime) firmwarePaths(secureBoot bool) (code, varsTemplate string) {
	return capability.OVMFPaths(secureBoot)
}

// publishVMEvent forwards a translated QMP event to the server.
func (rt *computeRuntime) publishVMEvent(event vm.Event) {
	if rt.send == nil {
		return
	}
	payload := &VMEvent{
		VMID:      event.VMID,
		Kind:      event.Kind,
		Device:    event.Device,
		Detail:    event.Detail,
		Timestamp: event.At.UTC().Format(time.RFC3339),
	}
	if err := rt.send(WSMessage{
		Type: MsgVMEvent,
		VM:   &VMPayload{Operation: "event", VMID: event.VMID, Event: payload},
	}); err != nil {
		rt.logger.Debug("publishing VM event failed", "vm", event.VMID, "error", err)
	}
}

// publishLease forwards a DHCP lease to the server.
//
// Leases take the offline path like stats do: a lease replayed an hour late is
// stale information about a guest that has probably moved on, and dropping it
// is better than recording it as current.
func (rt *computeRuntime) publishLease(lease dhcp.Lease) {
	if rt.send == nil {
		return
	}
	payload := &ComputeLease{
		NetworkID: lease.NetworkID,
		MAC:       lease.MAC,
		IP:        lease.IP,
		Hostname:  lease.Hostname,
		Action:    lease.Action,
	}
	if !lease.ExpiresAt.IsZero() {
		payload.ExpiresAt = lease.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if err := rt.send(WSMessage{
		Type: MsgNetLeaseEvent,
		Net:  &ComputeNetPayload{Operation: "lease", NetworkID: lease.NetworkID, Lease: payload},
	}); err != nil {
		rt.logger.Debug("publishing lease failed",
			"network", lease.NetworkID, "mac", lease.MAC, "error", err)
	}
}

// publishVMState forwards an unsolicited state transition.
func (rt *computeRuntime) publishVMState(state vm.State) {
	if rt.send == nil {
		return
	}
	if err := rt.send(WSMessage{
		Type: MsgVMState,
		VM:   &VMPayload{Operation: "state", VMID: state.VMID, State: toProtocolVMState(state)},
	}); err != nil {
		rt.logger.Debug("publishing VM state failed", "vm", state.VMID, "error", err)
	}
}

// publishCTState forwards an unsolicited container state transition.
func (rt *computeRuntime) publishCTState(state lxc.State) {
	if rt.send == nil {
		return
	}
	if err := rt.send(WSMessage{
		Type: MsgCTState,
		CT: &CTPayload{
			Operation:   "state",
			ContainerID: state.ContainerID,
			State:       toProtocolCTState(state),
		},
	}); err != nil {
		rt.logger.Debug("publishing container state failed", "container", state.ContainerID, "error", err)
	}
}

// toProtocolVMState converts a manager state into the wire shape.
func toProtocolVMState(in vm.State) *VMState {
	out := &VMState{
		VMID:         in.VMID,
		State:        in.State,
		UnitName:     in.UnitName,
		UnitActive:   in.UnitActive,
		UnitSub:      in.UnitSub,
		PID:          in.PID,
		QMPReachable: in.QMPReachable,
		ExitReason:   in.ExitReason,
		LastError:    in.LastError,
		RunState:     in.RunState,
	}
	for _, job := range in.BlockJobs {
		out.BlockJobs = append(out.BlockJobs, VMBlockJob{
			Device: job.Device,
			Type:   job.Type,
			Len:    job.Len,
			Offset: job.Offset,
			Status: job.Status,
			Error:  job.Error,
		})
	}
	if !in.StartedAt.IsZero() {
		out.StartedAt = in.StartedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// toProtocolVMStats converts a guest sample into the wire shape.
func toProtocolVMStats(in vm.Stats) *VMStats {
	out := &VMStats{
		VMID:           in.VMID,
		CPUPercent:     in.CPUPercent,
		CPUUsageNSec:   in.CPUUsageNSec,
		MemoryBytes:    in.MemoryBytes,
		BalloonBytes:   in.BalloonBytes,
		DiskReadBytes:  in.DiskReadBytes,
		DiskWriteBytes: in.DiskWriteBytes,
		NetRxBytes:     in.NetRxBytes,
		NetTxBytes:     in.NetTxBytes,
	}
	if !in.SampledAt.IsZero() {
		out.SampledAt = in.SampledAt.UTC().Format(time.RFC3339)
	}
	return out
}

// toProtocolHostDevice converts an enumerated device into the wire shape.
func toProtocolHostDevice(in hostdev.Device) HostDevice {
	return HostDevice{
		Kind:        in.Kind,
		Address:     in.Address,
		Description: in.Description,
		Vendor:      in.Vendor,
		Product:     in.Product,
		Driver:      in.Driver,
		Class:       in.Class,
		IOMMUGroup:  in.IOMMUGroup,
		Assignable:  in.Assignable,
		Reason:      in.Reason,
	}
}

// toProtocolCTState converts a container state into the wire shape.
func toProtocolCTState(in lxc.State) *CTState {
	out := &CTState{
		ContainerID: in.ContainerID,
		State:       in.State,
		InitPID:     in.InitPID,
		IPs:         in.IPs,
		ExitReason:  in.ExitReason,
		LastError:   in.LastError,
	}
	if !in.StartedAt.IsZero() {
		out.StartedAt = in.StartedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// distroKeyring builds the OpenPGP keyring used to verify distribution
// signatures from the keys compiled into imagefetch.
//
// Keys ship with the agent binary rather than being fetched, because a key
// downloaded alongside the artifact it authenticates proves only that both came
// from the same place — which is exactly what an attacker who controls the
// mirror also controls. A key that fails to load leaves no keyring at all, so a
// signed catalog entry is refused rather than accepted unchecked.
func distroKeyring() imagefetch.Keyring {
	keys, err := imagefetch.BundledKeys()
	if err != nil {
		slog.Error("compute: bundled signing keys unreadable; signed images will be refused", "error", err)
		return nil
	}
	ring, err := imagefetch.NewKeyring(keys...)
	if err != nil {
		slog.Error("compute: bundled signing keys unusable; signed images will be refused", "error", err)
		return nil
	}
	return ring
}
