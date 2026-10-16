// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
	"github.com/therealmcsparrow/mcharbor-agent/compute/supervisor"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Guest states. These are the values the backend stores, and the agent only
// ever reports one it has actually confirmed.
const (
	StateDefined   = "defined"
	StateRunning   = "running"
	StatePaused    = "paused"
	StateStopping  = "stopping"
	StateStopped   = "stopped"
	StateCrashed   = "crashed"
	StateMigrating = "migrating"
	// StateUnknown is a first-class answer, not a failure. It is what the
	// agent reports when it cannot reach a guest's monitor — guessing that a
	// VM is still running because it was a moment ago is how a management UI
	// lies to its operator.
	StateUnknown = "unknown"
)

// defaultGrace is the ACPI powerdown window before escalating to a hard quit.
const defaultGrace = 90 * time.Second

// stopWait bounds awaitStopped. Units are created with TimeoutStopSec=30, so
// this is that plus room for systemd's SIGKILL to land and be reaped.
const stopWait = 40 * time.Second

// monitorDialRetries bounds how long start waits for QEMU to open its monitor
// socket. QEMU creates it early, but not instantly, and treating the first
// failed dial as fatal would make every start flaky.
const (
	monitorDialRetries  = 40
	monitorDialInterval = 250 * time.Millisecond
)

var (
	// ErrNotFound means this host has no such VM.
	ErrNotFound = errors.New("vm: not found")
	// ErrAlreadyOwned means another McHarbor instance holds this VM's lock.
	ErrAlreadyOwned = errors.New("vm: this VM is managed by another McHarbor instance")
	// ErrNotRunning means the guest exists but is not up, so there is nothing
	// to sample.
	ErrNotRunning = errors.New("vm: not running")
)

// Manager owns every VM on one host.
type Manager struct {
	sup supervisor.Supervisor

	// mu guards monitors and ops. It is never held while a QMP command is in
	// flight — that would serialize every guest behind the slowest one.
	mu       sync.Mutex
	monitors map[string]*qmp.Monitor
	// ops serializes operations per VM. Two snapshots of one guest at once, or
	// a delete racing a start, are correctness problems rather than merely
	// surprising, so each VM gets its own lock and the UI shows an
	// "operation in progress" state.
	ops map[string]*sync.Mutex

	// cancels lets a long operation — clone, disk allocate, migration — be
	// stopped from the transport, per the goroutine lifecycle rule.
	cancels map[string]context.CancelFunc

	// onEvent receives translated QMP events. It must not block.
	onEvent func(Event)
	// onState receives unsolicited state transitions.
	onState func(State)

	// statsMu guards cpuSamples, which holds the previous CPU reading per
	// guest so Stats can report a rate rather than a lifetime total.
	statsMu    sync.Mutex
	cpuSamples map[string]cpuSample

	// deleted holds the unplugs waiting for QEMU's DEVICE_DELETED, keyed by
	// guest and device id. Guarded by mu.
	deleted map[string][]chan struct{}
}

// Event is a translated QMP asynchronous event.
type Event struct {
	VMID   string
	Kind   string
	Device string
	Detail map[string]string
	At     time.Time
}

// State is one guest's confirmed state.
type State struct {
	VMID         string
	State        string
	UnitName     string
	UnitActive   string
	UnitSub      string
	PID          int
	QMPReachable bool
	StartedAt    time.Time
	ExitReason   string
	LastError    string
	// RunState is QEMU's own run state, unmapped. State collapses a dozen of
	// them into the handful McHarbor models, and the two that matter most —
	// io-error and guest-panicked — are invisible once collapsed: an operator
	// looking at "Paused" cannot tell a deliberate pause from a guest frozen
	// because its disk backend failed.
	RunState string
	// BlockJobs are the mirrors, commits and backups QEMU is running for this
	// guest. Without them a snapshot that is still flattening looks like a
	// guest that has hung.
	BlockJobs []qmp.BlockJob
}

// NewManager builds a manager over a supervisor.
func NewManager(sup supervisor.Supervisor, onEvent func(Event), onState func(State)) *Manager {
	return &Manager{
		sup:        sup,
		monitors:   make(map[string]*qmp.Monitor),
		ops:        make(map[string]*sync.Mutex),
		cancels:    make(map[string]context.CancelFunc),
		onEvent:    onEvent,
		onState:    onState,
		cpuSamples: make(map[string]cpuSample),
		deleted:    make(map[string][]chan struct{}),
	}
}

// lockFor returns the per-VM operation lock.
func (m *Manager) lockFor(vmID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock, ok := m.ops[vmID]
	if !ok {
		lock = &sync.Mutex{}
		m.ops[vmID] = lock
	}
	return lock
}

// Define materializes a VM's runtime directory without starting it.
//
// Everything that can fail cheaply happens here — the firmware copy, the
// directory, the ownership lock — so a start is as close to "exec and it works"
// as it can be.
func (m *Manager) Define(ctx context.Context, spec Spec) error {
	lock := m.lockFor(spec.ID)
	lock.Lock()
	defer lock.Unlock()

	paths := PathsFor(spec.ID)
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		return fmt.Errorf("vm: create runtime directory for %s: %w", spec.Name, err)
	}

	if err := m.claim(paths); err != nil {
		return err
	}

	if spec.Firmware == "uefi" && spec.FirmwareVars == "" {
		if err := m.prepareFirmwareVars(spec, paths); err != nil {
			return err
		}
	}
	return nil
}

// claim writes the per-VM ownership lock.
//
// Two McHarbor instances pointed at one host would each believe they own its
// guests, and the second one's "start" would race the first one's "stop" on the
// same disk. The lock makes the second claimant fail with an error that says
// what happened rather than corrupting a disk.
func (m *Manager) claim(paths Paths) error {
	marker := []byte(strconv.Itoa(os.Getpid()) + "\n")

	existing, err := os.ReadFile(paths.LockFile)
	if err == nil {
		pid, convErr := strconv.Atoi(strings.TrimSpace(string(existing)))
		if convErr == nil && pid != os.Getpid() && processAlive(pid) {
			return fmt.Errorf("%w (held by pid %d)", ErrAlreadyOwned, pid)
		}
		// A stale lock from an agent that is no longer running is ours to
		// take: the guest survived the agent, which is the whole point of
		// systemd supervision.
	}
	if err := os.WriteFile(paths.LockFile, marker, 0o600); err != nil {
		return fmt.Errorf("vm: claim %s: %w", paths.LockFile, err)
	}
	return nil
}

// processAlive reports whether a pid is still running. Signal 0 performs the
// permission and existence checks without delivering anything.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(nil) == nil
}

// prepareFirmwareVars gives the VM its own writable UEFI variable store.
//
// The distribution's OVMF_VARS.fd is a read-only template shared by every
// guest. A VM writing its boot entries into the shared copy would either fail
// or corrupt every other guest's, so each gets a private copy at define time.
func (m *Manager) prepareFirmwareVars(spec Spec, paths Paths) error {
	template := spec.FirmwareVarsTemplate
	if template == "" {
		template = findVarsTemplate(spec.FirmwareCode)
	}
	if template == "" {
		return fmt.Errorf("vm: %s is set to UEFI but no OVMF variable template was found "+
			"beside %s", spec.Name, spec.FirmwareCode)
	}

	sourceFile := paths.VarsFile + ".source"
	if _, err := os.Stat(paths.VarsFile); err == nil {
		previous := errlog.Value(os.ReadFile(sourceFile))
		if strings.TrimSpace(string(previous)) == template {
			// Same template as last time: keep the store, because it holds this
			// guest's boot order.
			return nil
		}
		// A different template means the firmware itself changed — switching
		// Secure Boot on moves the guest from a key-less store to an enrolled
		// one. Keeping the old store would leave Secure Boot in setup mode and
		// make the toggle look like it did nothing, so it is re-seeded below.
	}

	data, err := os.ReadFile(template)
	if err != nil {
		return fmt.Errorf("vm: read firmware template %s: %w", template, err)
	}
	if err := os.WriteFile(paths.VarsFile, data, 0o600); err != nil {
		return fmt.Errorf("vm: write firmware variables for %s: %w", spec.Name, err)
	}
	if err := os.WriteFile(sourceFile, []byte(template), 0o600); err != nil {
		return fmt.Errorf("vm: record firmware template for %s: %w", spec.Name, err)
	}
	return nil
}

// findVarsTemplate locates the writable-variables template for a firmware
// image. Distributions name these inconsistently, so several forms are tried.
func findVarsTemplate(codePath string) string {
	if codePath == "" {
		return ""
	}
	dir := filepath.Dir(codePath)
	base := filepath.Base(codePath)

	candidates := []string{
		strings.Replace(base, "CODE", "VARS", 1),
		strings.Replace(base, "code", "vars", 1),
		"OVMF_VARS.fd",
		"OVMF_VARS_4M.fd",
	}
	for _, candidate := range candidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// Start boots a defined VM.
//
// The sequence matters: the unit is started with QEMU paused (-S), the monitor
// is attached, the event pump begins, and only then are the vCPUs released. A
// guest whose console attached after it had already booted would miss the
// firmware output, which is exactly what an operator needs to see when a boot
// goes wrong.
func (m *Manager) Start(ctx context.Context, spec Spec, binary string) error {
	lock := m.lockFor(spec.ID)
	lock.Lock()
	defer lock.Unlock()

	paths := PathsFor(spec.ID)
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		return fmt.Errorf("vm: create runtime directory for %s: %w", spec.Name, err)
	}
	if err := m.claim(paths); err != nil {
		return err
	}

	// Re-checked on every start, not only at define: a guest whose Secure Boot
	// was switched on after it was created still holds the key-less variable
	// store it was seeded with, and booting that would leave Secure Boot in
	// setup mode. prepareFirmwareVars returns immediately when the template has
	// not changed.
	if spec.Firmware == "uefi" && spec.FirmwareVars == "" {
		if err := m.prepareFirmwareVars(spec, paths); err != nil {
			return err
		}
	}

	// Stale sockets from a previous run stop QEMU binding its own, and the
	// error it gives does not name the reason.
	for _, sock := range []string{paths.QMPSocket, paths.VNCSocket, paths.SerialSock, paths.GuestAgent, paths.SPICESocket} {
		errlog.Log(os.Remove(sock))
	}

	// The vTPM is a process of its own that QEMU connects to on startup, so it
	// has to be listening before QEMU is asked to dial it.
	if spec.TPMEnabled {
		if err := m.startTPM(ctx, spec, paths); err != nil {
			return err
		}
	}

	// virtiofs daemons are the same shape of problem: QEMU dials each
	// vhost-user socket at startup and fails outright if nothing is listening,
	// so every daemon has to be up before the guest is launched.
	if len(spec.Shares) > 0 {
		if err := m.startShares(ctx, spec); err != nil {
			m.abandonTPM(ctx, spec)
			return err
		}
	}

	// Before the descriptors and the command line, so the seed takes the next
	// SATA port like any other CD-ROM.
	seeded := AttachSeed(&spec, paths)

	// Descriptors are assigned before the command line is built, because a NIC
	// on a direct network is named in it by descriptor number rather than by
	// interface name.
	direct := AssignNetdevFDs(&spec)

	argv, err := BuildArgv(binary, spec, paths)
	if err != nil {
		m.abandonTPM(ctx, spec)
		return err
	}
	// The args file records QEMU's own command line, never the wrapper, so it
	// stays the thing an operator can read and reproduce by hand.
	if err := WriteArgsFile(paths.ArgsFile, argv); err != nil {
		m.abandonTPM(ctx, spec)
		return err
	}
	if direct {
		// A macvtap reaches QEMU as an open file and this start is detached,
		// so the unit runs the agent's own shim, which places the descriptors
		// and execs QEMU.
		argv, err = NetdevExecArgv(spec, argv)
		if err != nil {
			m.abandonTPM(ctx, spec)
			return err
		}
	}

	unitName := supervisor.VMUnitName(spec.ID)
	if err := m.sup.Start(ctx, supervisor.UnitSpec{
		Name:             unitName,
		Description:      "McHarbor VM " + spec.Name,
		Argv:             argv,
		WorkingDirectory: paths.Dir,
		KillMode:         "mixed",
		// QEMU answers a SIGTERM by exiting immediately, which is a hard power
		// cut for the guest. The graceful path is an ACPI powerdown over QMP,
		// so systemd's own stop is only ever the escalation — and this bounds
		// how long it waits before SIGKILL.
		TimeoutStopSec: 30,
	}); err != nil {
		m.abandonTPM(ctx, spec)
		return fmt.Errorf("vm: start %s: %w", spec.Name, err)
	}

	monitor, err := m.attach(ctx, spec.ID, paths.QMPSocket)
	if err != nil {
		// The unit started but the monitor never appeared, so the guest is
		// unmanageable. Stopping it is better than leaving a QEMU nobody can
		// talk to holding the VM's disks open.
		errlog.Log(m.sup.Stop(ctx, unitName))
		m.abandonTPM(ctx, spec)
		return err
	}

	// A link the operator cut stays cut across a restart. Done while the
	// guest is still frozen, so it never sees the cable plugged in.
	for _, nic := range spec.NICs {
		if !nic.LinkDown {
			continue
		}
		if _, err := monitor.Run(ctx, "set_link", map[string]interface{}{
			"name": fmt.Sprintf("nic%d", nic.Slot), "up": false,
		}); err != nil {
			return fmt.Errorf("vm: cutting the link of NIC %d on %s: %w", nic.Slot, spec.Name, err)
		}
	}

	// Release the vCPUs. Everything above this line ran while the guest was
	// frozen at its first instruction.
	if err := monitor.Resume(ctx); err != nil {
		return fmt.Errorf("vm: release vcpus for %s: %w", spec.Name, err)
	}
	if seeded {
		// Logged, not returned: the guest is running, and a missing marker only
		// means the seed is attached once more on the next start.
		errlog.Log(MarkSeedDelivered(paths))
	}
	return nil
}

// abandonTPM tears down a vTPM started for a guest whose QEMU never came up,
// so a failed start does not leave a swtpm holding the socket.
func (m *Manager) abandonTPM(ctx context.Context, spec Spec) {
	// The virtiofs daemons go with it. Both are helper processes started for a
	// guest that never came up, and leaving either behind holds a socket the
	// next start cannot bind.
	m.stopShares(ctx, spec.ID)
	if !spec.TPMEnabled {
		return
	}
	errlog.Log(m.stopTPM(ctx, spec.ID))
}

// attach dials the QMP monitor, retrying while QEMU creates its socket, and
// starts the event pump.
func (m *Manager) attach(ctx context.Context, vmID, socketPath string) (*qmp.Monitor, error) {
	var lastErr error
	for attempt := 0; attempt < monitorDialRetries; attempt++ {
		monitor, err := qmp.Dial(ctx, socketPath)
		if err == nil {
			m.mu.Lock()
			// A monitor left over from a previous start would keep its own
			// read loop alive against a dead socket.
			if previous, ok := m.monitors[vmID]; ok {
				errlog.Close(previous)
			}
			m.monitors[vmID] = monitor
			m.mu.Unlock()

			go m.pumpEvents(vmID, monitor)
			return monitor, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("vm: attach monitor for %s: %w", vmID, ctx.Err())
		case <-time.After(monitorDialInterval):
		}
	}
	return nil, fmt.Errorf("vm: QEMU for %s never opened its monitor socket: %w", vmID, lastErr)
}

// pumpEvents translates QMP events and forwards them.
//
// Its stop path is the monitor closing, which happens on Close, on a socket
// failure, and when QEMU exits — so this goroutine cannot outlive the guest it
// belongs to.
func (m *Manager) pumpEvents(vmID string, monitor *qmp.Monitor) {
	defer func() {
		m.mu.Lock()
		if m.monitors[vmID] == monitor {
			delete(m.monitors, vmID)
		}
		m.mu.Unlock()
	}()

	for {
		select {
		case <-monitor.Done():
			return
		case ev, ok := <-monitor.Events():
			if !ok {
				return
			}
			m.dispatchEvent(vmID, ev)
		}
	}
}

// dispatchEvent maps a QMP event onto McHarbor's own vocabulary.
func (m *Manager) dispatchEvent(vmID string, ev qmp.Event) {
	if ev.Event == "DEVICE_DELETED" {
		if device, ok := ev.Data["device"].(string); ok && device != "" {
			m.signalDeleted(vmID, device)
		}
	}
	if m.onEvent == nil {
		return
	}

	kind := strings.ToLower(ev.Event)
	out := Event{VMID: vmID, Kind: kind, At: ev.Time(), Detail: map[string]string{}}

	switch ev.Event {
	case "SHUTDOWN":
		out.Kind = "shutdown"
		// The guest asked to stop versus something stopped it: only the first
		// is a clean shutdown, and conflating them makes a crash look normal.
		if reason, ok := ev.Data["reason"].(string); ok {
			out.Detail["reason"] = reason
		}
	case "RESET":
		out.Kind = "reset"
	case "STOP":
		out.Kind = "stop"
	case "RESUME":
		out.Kind = "resume"
	case "POWERDOWN":
		out.Kind = "powerdown"
	case "GUEST_PANICKED":
		out.Kind = "guest_panic"
	case "BLOCK_IO_ERROR":
		// The single most useful event here. QEMU pauses the guest on ENOSPC,
		// and without naming the device the operator sees only "the VM froze".
		out.Kind = "block_io_error"
		if device, ok := ev.Data["device"].(string); ok {
			out.Device = device
		}
		if nodeName, ok := ev.Data["node-name"].(string); ok {
			out.Detail["nodeName"] = nodeName
		}
		if reason, ok := ev.Data["reason"].(string); ok {
			out.Detail["reason"] = reason
		}
	case "DEVICE_DELETED":
		// The confirmation half of an unplug. device_del only asks; the guest
		// decides, and on a vCPU or a NIC it regularly declines. Without this
		// event an unplug that never happened is indistinguishable from one
		// that did.
		out.Kind = "device_deleted"
		if device, ok := ev.Data["device"].(string); ok {
			out.Device = device
		}
		if path, ok := ev.Data["path"].(string); ok {
			out.Detail["path"] = path
		}
	case "BLOCK_JOB_COMPLETED", "BLOCK_JOB_ERROR", "BLOCK_JOB_READY":
		out.Kind = strings.ToLower(ev.Event)
		if device, ok := ev.Data["device"].(string); ok {
			out.Device = device
		}
		if reason, ok := ev.Data["error"].(string); ok {
			out.Detail["error"] = reason
		}
	case "RTC_CHANGE":
		out.Kind = "rtc_change"
	default:
		// Unmapped events are forwarded with their raw name rather than
		// dropped, so a QEMU version that adds one is visible in the activity
		// feed instead of silently invisible.
	}
	m.onEvent(out)
}

// Monitor returns a VM's live monitor, reconnecting if the socket came back.
func (m *Manager) Monitor(ctx context.Context, vmID string) (*qmp.Monitor, error) {
	m.mu.Lock()
	monitor, ok := m.monitors[vmID]
	m.mu.Unlock()

	if ok && monitor.Ready() {
		return monitor, nil
	}

	paths := PathsFor(vmID)
	if _, err := os.Stat(paths.QMPSocket); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, vmID)
	}
	return m.attach(ctx, vmID, paths.QMPSocket)
}

// Shutdown asks the guest to power down, escalating after the grace period.
//
// The escalation is deliberate and visible: the caller records that it happened
// so an operator can see that this guest ignored ACPI, rather than wondering
// why a "graceful" stop took a minute and a half.
func (m *Manager) Shutdown(ctx context.Context, vmID string, grace time.Duration) (escalated bool, err error) {
	lock := m.lockFor(vmID)
	lock.Lock()
	defer lock.Unlock()

	if grace <= 0 {
		grace = defaultGrace
	}

	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		// No monitor means no guest to ask politely. Stopping the unit is the
		// only remaining option and reaches the desired state.
		return true, m.sup.Stop(ctx, supervisor.VMUnitName(vmID))
	}

	if err := monitor.Powerdown(ctx); err != nil && !qmp.IsClosed(err) {
		return false, fmt.Errorf("vm: powerdown %s: %w", vmID, err)
	}

	deadline := time.Now().Add(grace)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
		}

		status, err := m.sup.Status(ctx, supervisor.VMUnitName(vmID))
		if errors.Is(err, supervisor.ErrNoUnit) || (err == nil && !status.Running()) {
			return false, nil
		}
	}

	// ACPI was ignored. Before killing the guest, ask its agent to shut itself
	// down: a guest that drops ACPI — a Windows guest sitting on a "you have
	// unsaved work" dialog is the common one — very often still answers here,
	// and the difference is a clean shutdown instead of a power cut.
	if m.guestShutdown(ctx, vmID) {
		softDeadline := time.Now().Add(grace)
		for time.Now().Before(softDeadline) {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-ticker.C:
			}
			status, err := m.sup.Status(ctx, supervisor.VMUnitName(vmID))
			if errors.Is(err, supervisor.ErrNoUnit) || (err == nil && !status.Running()) {
				// It went down, but only after ACPI was ignored, so the
				// escalation is still reported — the operator should know this
				// guest needs the agent to stop cleanly.
				return true, nil
			}
		}
	}

	// The guest did not go. Escalate.
	return true, m.Stop(ctx, vmID)
}

// guestShutdown asks the guest agent to power the guest off from inside.
//
// Reports whether the request was accepted, not whether the guest went: the
// caller waits for that. A guest with no agent simply returns false, which is
// the ordinary case and not an error worth logging on every hard stop.
func (m *Manager) guestShutdown(ctx context.Context, vmID string) bool {
	socket := PathsFor(vmID).GuestAgent
	if _, err := os.Stat(socket); err != nil {
		return false
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	guest, err := qmp.DialGuest(dialCtx, socket)
	if err != nil {
		return false
	}
	defer func() { errlog.Close(guest) }()

	return guest.Shutdown(dialCtx) == nil
}

// Stop terminates a guest immediately.
func (m *Manager) Stop(ctx context.Context, vmID string) error {
	monitor, err := m.Monitor(ctx, vmID)
	if err == nil {
		// `quit` lets QEMU flush its block layer on the way out, which a
		// SIGKILL would not. It is still a hard stop for the guest.
		if err := monitor.Quit(ctx); err == nil {
			errlog.Log(m.closeMonitor(vmID))
			// The unit still has to be stopped so systemd collects it.
			errlog.Log(m.sup.Stop(ctx, supervisor.VMUnitName(vmID)))
			if err := m.awaitStopped(ctx, vmID); err != nil {
				return err
			}
			m.stopShares(ctx, vmID)
			return m.stopTPM(ctx, vmID)
		}
	}
	errlog.Log(m.closeMonitor(vmID))
	if err := m.sup.Stop(ctx, supervisor.VMUnitName(vmID)); err != nil {
		return fmt.Errorf("vm: stop %s: %w", vmID, err)
	}
	if err := m.awaitStopped(ctx, vmID); err != nil {
		return err
	}
	m.stopShares(ctx, vmID)
	if err := m.stopTPM(ctx, vmID); err != nil {
		return err
	}
	return nil
}

// awaitStopped waits for a guest's unit to leave the active state.
//
// systemd accepts a stop job and returns; QEMU can take seconds to flush and
// exit, and until it does the unit is still active. Returning before that made
// the backend write "stopped" while the agent would still refuse a start with
// "is already running" — the guest looked stopped and could not be started for
// as long as it took to really go. The bound is systemd's own TimeoutStopSec
// for these units plus a margin for the SIGKILL that follows it.
func (m *Manager) awaitStopped(ctx context.Context, vmID string) error {
	unitName := supervisor.VMUnitName(vmID)
	deadline := time.Now().Add(stopWait)
	for {
		status, err := m.sup.Status(ctx, unitName)
		if errors.Is(err, supervisor.ErrNoUnit) || (err == nil && !status.Running()) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("vm: %s was still running %s after it was told to stop",
				vmID, stopWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// Reset hard-resets a running guest.
func (m *Manager) Reset(ctx context.Context, vmID string) error {
	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		return err
	}
	return monitor.Reset(ctx)
}

// Pause stops a guest's vCPUs.
func (m *Manager) Pause(ctx context.Context, vmID string) error {
	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		return err
	}
	return monitor.Pause(ctx)
}

// Resume restarts paused vCPUs.
func (m *Manager) Resume(ctx context.Context, vmID string) error {
	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		return err
	}
	return monitor.Resume(ctx)
}

// closeMonitor drops a VM's monitor.
func (m *Manager) closeMonitor(vmID string) error {
	m.mu.Lock()
	monitor, ok := m.monitors[vmID]
	delete(m.monitors, vmID)
	m.mu.Unlock()
	// Every teardown path goes through here, so it is also where the CPU
	// counter is dropped: a guest that starts again under the same id must
	// not have its first percentage measured against the previous boot.
	m.forgetCPUSample(vmID)
	if !ok {
		return nil
	}
	return monitor.Close()
}

// Delete removes a VM's unit and runtime directory. Volumes are the caller's to
// remove, because only it knows which pool they came from.
func (m *Manager) Delete(ctx context.Context, vmID string) error {
	lock := m.lockFor(vmID)
	lock.Lock()
	defer lock.Unlock()

	errlog.Log(m.closeMonitor(vmID))
	unitName := supervisor.VMUnitName(vmID)
	if err := m.sup.Stop(ctx, unitName); err != nil && !errors.Is(err, supervisor.ErrNoUnit) {
		return fmt.Errorf("vm: stop %s before delete: %w", vmID, err)
	}
	errlog.Log(m.sup.Reset(ctx, unitName))
	// Key material must not outlive the guest, and it no longer lives inside
	// the runtime directory removed below.
	m.stopShares(ctx, vmID)
	errlog.Log(m.stopTPM(ctx, vmID))
	errlog.Log(removeTPMState(vmID))

	paths := PathsFor(vmID)
	// PathsFor builds this from RuntimeRoot and an id, so it cannot address
	// anything outside the compute runtime tree.
	if err := os.RemoveAll(paths.Dir); err != nil {
		return fmt.Errorf("vm: remove runtime directory for %s: %w", vmID, err)
	}

	m.mu.Lock()
	delete(m.ops, vmID)
	delete(m.cancels, vmID)
	m.mu.Unlock()
	return nil
}

// Query reconciles every VM on this host against what the backend believes.
//
// Units matching the VM pattern that the backend has no row for are returned as
// orphans. They are never killed: the operator may have a very good reason for
// them, and an agent that silently stops processes it does not recognise is not
// one anybody should run as root.
func (m *Manager) Query(ctx context.Context, known []string) ([]State, []string, error) {
	units, err := m.sup.List(ctx, supervisor.VMUnitPattern)
	if err != nil {
		return nil, nil, err
	}

	knownSet := make(map[string]bool, len(known))
	for _, id := range known {
		knownSet[id] = true
	}

	var states []State
	var orphans []string
	seen := map[string]bool{}

	for _, unit := range units {
		kind, vmID, ok := supervisor.GuestIDFromUnit(unit.Name)
		if !ok || kind != "vm" {
			continue
		}
		seen[vmID] = true
		if !knownSet[vmID] {
			orphans = append(orphans, unit.Name)
			continue
		}
		states = append(states, m.stateFromUnit(ctx, vmID, unit))
	}

	// A VM the backend knows about but this host has no unit for. Reported as
	// stopped rather than unknown, because the absence of a unit is itself a
	// confirmed observation — the agent is up and looked.
	for _, vmID := range known {
		if seen[vmID] {
			continue
		}
		states = append(states, State{VMID: vmID, State: StateStopped})
	}
	return states, orphans, nil
}

// stateFromUnit derives a guest state from its unit plus, where reachable, its
// monitor.
//
// The unit alone is not enough: a QEMU that is running but paused shows an
// active unit, and reporting that as "running" would tell the operator their
// guest is fine when it is frozen.
func (m *Manager) stateFromUnit(ctx context.Context, vmID string, unit supervisor.UnitStatus) State {
	state := State{
		VMID:       vmID,
		UnitName:   unit.Name,
		UnitActive: unit.ActiveState,
		UnitSub:    unit.SubState,
		PID:        unit.MainPID,
		StartedAt:  unit.StartedAt(),
	}

	switch {
	case unit.Failed():
		state.State = StateCrashed
		state.ExitReason = unit.Result
		return state
	case !unit.Running():
		state.State = StateStopped
		return state
	}

	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		// The process is up but unreachable. That is genuinely unknown, and
		// saying so is the honest answer.
		state.State = StateUnknown
		state.LastError = "the guest's QMP monitor is not reachable"
		return state
	}
	state.QMPReachable = true

	status, err := monitor.Status(ctx)
	if err != nil {
		state.State = StateUnknown
		state.LastError = "the guest did not answer a status query"
		return state
	}

	state.RunState = status.Status
	switch status.Status {
	case "running":
		state.State = StateRunning
	case "paused", "prelaunch":
		state.State = StatePaused
	case "io-error":
		// QEMU pauses the guest when a disk backend fails — usually a full
		// pool. Collapsing it into "running", which the default branch below
		// used to do, is the single most misleading state this function can
		// produce: the guest is stopped dead and the UI said it was fine.
		state.State = StatePaused
	case "shutdown", "guest-panicked":
		state.State = StateStopping
	case "postmigrate", "finish-migrate":
		state.State = StateMigrating
	default:
		state.State = StateRunning
	}

	// Best effort: a guest whose monitor answered a status query but not this
	// is still worth reporting a state for, so the error is dropped rather
	// than downgrading the whole state to unknown.
	if jobs, err := monitor.BlockJobs(ctx); err == nil {
		state.BlockJobs = jobs
	}
	return state
}

// RegisterCancel records a cancel function for a long operation.
func (m *Manager) RegisterCancel(key string, cancel context.CancelFunc) {
	m.mu.Lock()
	m.cancels[key] = cancel
	m.mu.Unlock()
}

// Cancel stops a long operation by key.
func (m *Manager) Cancel(key string) bool {
	m.mu.Lock()
	cancel, ok := m.cancels[key]
	delete(m.cancels, key)
	m.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// Close releases every monitor. The event pumps stop with them.
func (m *Manager) Close() error {
	m.mu.Lock()
	monitors := make([]*qmp.Monitor, 0, len(m.monitors))
	for _, monitor := range m.monitors {
		monitors = append(monitors, monitor)
	}
	m.monitors = make(map[string]*qmp.Monitor)
	m.mu.Unlock()

	for _, monitor := range monitors {
		errlog.Close(monitor)
	}
	return nil
}

// CleanupNICs removes the tap and direct devices a VM owned.
//
// Called on delete and after a failed start, so a guest that never came up does
// not leave interfaces behind for the operator to find later and wonder about.
func CleanupNICs(spec Spec) {
	for _, nic := range spec.NICs {
		// DeleteLink refuses anything outside the managed namespace, so a
		// crafted name cannot take out the host's uplink.
		if nic.TapName != "" {
			errlog.Log(netconf.DeleteLink(nic.TapName))
		}
		if nic.DevicePath != "" {
			errlog.Log(netconf.DeleteLink(netconf.DirectName(spec.ID, nic.Slot)))
		}
	}
}

// deletedKey names one guest's device in the DEVICE_DELETED waiter table.
func deletedKey(vmID, device string) string { return vmID + "/" + device }

// watchDeleted registers for one device's DEVICE_DELETED. It is called before
// device_del is sent, so an event that arrives at once is not missed.
func (m *Manager) watchDeleted(vmID, device string) (<-chan struct{}, func()) {
	ch := make(chan struct{})
	key := deletedKey(vmID, device)
	m.mu.Lock()
	m.deleted[key] = append(m.deleted[key], ch)
	m.mu.Unlock()
	release := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		waiters := m.deleted[key]
		for i, w := range waiters {
			if w == ch {
				m.deleted[key] = append(waiters[:i], waiters[i+1:]...)
				break
			}
		}
		if len(m.deleted[key]) == 0 {
			delete(m.deleted, key)
		}
	}
	return ch, release
}

// signalDeleted wakes every unplug waiting on device and forgets them.
func (m *Manager) signalDeleted(vmID, device string) {
	key := deletedKey(vmID, device)
	m.mu.Lock()
	waiters := m.deleted[key]
	delete(m.deleted, key)
	m.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
}

// WithHotplug runs fn against a running guest's devices, holding the guest's
// operation lock so a hot-plug never races a snapshot, a stop or another
// hot-plug of the same guest.
func (m *Manager) WithHotplug(ctx context.Context, spec Spec, fn func(*Hotplug) error) error {
	lock := m.lockFor(spec.ID)
	lock.Lock()
	defer lock.Unlock()

	monitor, err := m.Monitor(ctx, spec.ID)
	if err != nil {
		return err
	}
	vmID := spec.ID
	return fn(&Hotplug{
		Mon:         monitor,
		MachineType: spec.MachineType,
		IOThread:    spec.IOThread,
		Deleted: func(device string) (<-chan struct{}, func()) {
			return m.watchDeleted(vmID, device)
		},
	})
}
