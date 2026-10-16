// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// stopPollInterval is how often a graceful shutdown checks whether the
// container's init has actually gone.
const stopPollInterval = 500 * time.Millisecond

// Manager owns every system container on one host.
type Manager struct {
	// mu guards ops and cancels. Container handles are not cached: libcontainer
	// state lives on disk, so Load is cheap and always current, and a cached
	// handle would go stale the moment anything else touched the container.
	mu      sync.Mutex
	ops     map[string]*sync.Mutex
	cancels map[string]context.CancelFunc
	// consoles holds each running container's PID-1 console, also under mu.
	// The pty master exists only in this process, so an agent restart loses it.
	consoles map[string]*consoleHub
	// cpuSamples is the previous CPU reading per container, under mu, so a
	// stats call can turn cumulative CPU time into a percentage.
	cpuSamples map[string]cpuSample
	// inits holds the reaper of each init this process started, under mu. An
	// agent restart begins with none: an init started by an earlier agent process
	// was reparented to the nearest subreaper or PID 1, which reaps it instead, so
	// its exit status is never seen here and Status falls back to libcontainer's.
	inits map[string]*initReaper
	// rootfsUsage serves each container's rootfs size from a cached, bounded
	// walk, because measuring on every stats poll would read the whole tree.
	rootfsUsage *rootfsUsageCache

	onState func(State)

	// root is libcontainer's state root: StateRoot in production, a temporary
	// directory in tests, so the lifecycle can be exercised without /run.
	root string
	// stageBase is where unprivileged rootfs trees are bind-staged for runc:
	// DefaultStageBase in production, a searchable temporary directory in tests.
	stageBase string
	// configFor builds the libcontainer config for a spec. BuildConfig in
	// production; a test swaps it to use the cgroupfs driver where no systemd runs.
	configFor func(Spec, Paths) (*configs.Config, error)
}

// State is one container's confirmed state.
type State struct {
	ContainerID string
	State       string
	InitPID     int
	StartedAt   time.Time
	IPs         []string
	ExitReason  string
	LastError   string
}

// Process is one process inside a container.
type Process struct {
	PID         int
	HostPID     int
	PPID        int
	User        string
	Command     string
	MemoryBytes int64
}

// NewManager builds a container manager.
func NewManager(onState func(State)) *Manager {
	return &Manager{
		ops:         make(map[string]*sync.Mutex),
		cancels:     make(map[string]context.CancelFunc),
		consoles:    make(map[string]*consoleHub),
		cpuSamples:  make(map[string]cpuSample),
		inits:       make(map[string]*initReaper),
		rootfsUsage: newRootfsUsageCache(measureRootfs),
		onState:     onState,
		root:        StateRoot,
		stageBase:   DefaultStageBase,
		configFor:   BuildConfig,
	}
}

// stateDir is one container's libcontainer state directory under this manager's root.
func (m *Manager) stateDir(ctID string) string {
	return filepath.Join(m.root, ctID)
}

// load returns the handle of a container that has been started at least once.
// A never-started container is ErrNotRunning rather than ErrNotFound: it exists,
// libcontainer just has nothing to load until its first start writes state.json.
func (m *Manager) load(ctID string) (*libcontainer.Container, error) {
	p, err := probePresence(m.stateDir(ctID))
	if err != nil {
		return nil, err
	}
	switch p {
	case presenceAbsent:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, ctID)
	case presenceUnstarted:
		return nil, fmt.Errorf("%w: %s has never been started", ErrNotRunning, ctID)
	}
	container, err := libcontainer.Load(m.root, ctID)
	if err != nil {
		return nil, fmt.Errorf("lxc: load %s: %w", ctID, err)
	}
	return container, nil
}

// clearLeftover frees ctID for a fresh libcontainer.Create, which refuses any
// existing state directory with ErrExist, including one that was never started.
func (m *Manager) clearLeftover(ctID string) error {
	dir := m.stateDir(ctID)
	p, err := probePresence(dir)
	if err != nil {
		return err
	}
	var existing *libcontainer.Container
	state := ""
	if p == presenceLoaded {
		existing, err = libcontainer.Load(m.root, ctID)
		if err != nil {
			return fmt.Errorf("lxc: load the previous state of %s: %w", ctID, err)
		}
		state = containerState(existing)
	}

	action, err := planLeftover(p, state)
	if err != nil {
		return err
	}
	switch action {
	case leftoverRemoveDir:
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("lxc: remove the never-started state of %s: %w", ctID, err)
		}
	case leftoverDestroy:
		if err := existing.Destroy(); err != nil {
			return fmt.Errorf("lxc: destroy the previous state of %s: %w", ctID, err)
		}
	}
	return nil
}

// containerState maps a loaded container's libcontainer status onto this
// package's states; an unreadable status is unknown, never assumed stopped.
func containerState(container *libcontainer.Container) string {
	status, err := container.Status()
	if err != nil {
		return StateUnknown
	}
	switch status {
	case libcontainer.Running:
		return StateRunning
	case libcontainer.Created:
		return StateDefined
	case libcontainer.Paused:
		return StateFrozen
	case libcontainer.Stopped:
		return StateStopped
	}
	return StateUnknown
}

// prepare builds a spec's libcontainer config and refreshes config.json beside
// it. Create and Start both call it, so a start always runs the current spec.
// rootfs is the path runc is given: the pool path, or its staged bind mount.
func (m *Manager) prepare(spec Spec, rootfs string) (*configs.Config, error) {
	paths := PathsFor(spec.ID)
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("lxc: create runtime directory for %s: %w", spec.Name, err)
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return nil, fmt.Errorf("lxc: create state root: %w", err)
	}
	if _, err := os.Stat(spec.RootfsPath); err != nil {
		return nil, fmt.Errorf("lxc: rootfs for %s is missing at %s: %w", spec.Name, spec.RootfsPath, err)
	}

	config, err := m.configFor(spec, Paths{
		Dir:         paths.Dir,
		Rootfs:      rootfs,
		ConfigJSON:  paths.ConfigJSON,
		ConsoleSock: paths.ConsoleSock,
		StateDir:    m.stateDir(spec.ID),
	})
	if err != nil {
		return nil, err
	}

	if payload, err := json.MarshalIndent(config, "", "  "); err == nil {
		// Best-effort: the container is perfectly runnable without this file,
		// and failing a create because a debug artifact could not be written
		// would be the wrong trade.
		errlog.Log(os.WriteFile(paths.ConfigJSON, payload, 0o600))
	}
	return config, nil
}

func (m *Manager) lockFor(ctID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock, ok := m.ops[ctID]
	if !ok {
		lock = &sync.Mutex{}
		m.ops[ctID] = lock
	}
	return lock
}

// Create materializes a container without starting it.
//
// libcontainer.Create validates the config and claims the id now, so a bad spec
// fails at create rather than at first start; config.json is written beside it
// for support, the container equivalent of the VM's args.json. Create writes no
// state.json (runc v1.5.1), so the result is a never-started state directory.
func (m *Manager) Create(ctx context.Context, spec Spec) error {
	lock := m.lockFor(spec.ID)
	lock.Lock()
	defer lock.Unlock()

	config, err := m.prepare(spec, spec.RootfsPath)
	if err != nil {
		return err
	}

	// A previous incarnation of the same id, started or not, holds the name in
	// libcontainer's state root and makes Create fail with ErrExist.
	if err := m.clearLeftover(spec.ID); err != nil {
		return fmt.Errorf("lxc: create container %s: %w", spec.Name, err)
	}

	if _, err := libcontainer.Create(m.root, spec.ID, config); err != nil {
		return fmt.Errorf("lxc: create container %s: %w", spec.Name, err)
	}
	return nil
}

// Start boots a container's init.
//
// Every start creates the libcontainer container afresh from spec. Load cannot
// serve a start: it finds nothing after Create (no state.json yet) or after a
// reboot empties /run, and a loaded stopped container refuses a second init.
//
// The veth pairs are attached while init is parked on libcontainer's exec fifo,
// because the peer has to be moved into a network namespace that must exist
// first; Exec then releases init, so the guest boots with its interfaces there.
func (m *Manager) Start(ctx context.Context, spec Spec) (int, error) {
	lock := m.lockFor(spec.ID)
	lock.Lock()
	defer lock.Unlock()

	// Before staging: a running container refuses here, and staging first would
	// unmount the live container's staged rootfs on the way to that refusal.
	if err := m.clearLeftover(spec.ID); err != nil {
		return 0, fmt.Errorf("lxc: start %s: %w", spec.Name, err)
	}
	rootfs, err := m.stageRootfs(spec)
	if err != nil {
		return 0, err
	}
	pid, err := m.start(spec, rootfs)
	if err != nil {
		// Nothing runs from the staged tree now, and a leftover bind would keep
		// the pool's rootfs busy for a zfs destroy or an unmount.
		if releaseErr := m.releaseStaged(spec.ID); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
		return 0, err
	}
	return pid, nil
}

// start is Start after the id is free and the rootfs is staged at rootfs.
func (m *Manager) start(spec Spec, rootfs string) (int, error) {
	config, err := m.prepare(spec, rootfs)
	if err != nil {
		return 0, err
	}
	container, err := libcontainer.Create(m.root, spec.ID, config)
	if err != nil {
		return 0, fmt.Errorf("lxc: start %s: %w", spec.Name, err)
	}

	paths := PathsFor(spec.ID)
	errlog.Log(os.Remove(paths.ConsoleSock))

	process := &libcontainer.Process{
		Args: InitArgvFor(spec),
		Env:  ContainerEnv(spec),
		Cwd:  "/",
		Init: true,
	}

	// A console socket gives PID 1 a real controlling terminal, which is what
	// makes the tty console show a login prompt rather than a bare stream. It
	// is created before Start so the first bytes of boot output are not lost —
	// the same ordering hazard the console gateway has, in a smaller place.
	console, err := newConsoleReceiver(paths.ConsoleSock)
	if err != nil {
		return 0, err
	}
	defer console.Close()
	process.ConsoleSocket = console.AgentEnd()
	process.ConsoleWidth, process.ConsoleHeight = initialWinsize(0, 0)

	// Start, not Run: Run would release init straight away, before the
	// interfaces below exist.
	if err := container.Start(process); err != nil {
		// A failed Start has already killed and waited on its init
		// (initProcess.start's terminate), so there is nothing to reap here.
		errlog.Log(container.Destroy())
		return 0, fmt.Errorf("lxc: start %s: %w", spec.Name, err)
	}
	// From here on only this reaper waits on init: libcontainer never does after
	// a successful Start, and every failure below goes through abandonStart.
	reaper := m.watchInit(spec.ID, process)

	pid, err := process.Pid()
	if err != nil {
		m.abandonStart(container, reaper)
		return 0, fmt.Errorf("lxc: %s started but reported no pid: %w", spec.Name, err)
	}

	// The pty master has to be taken off the socket before the deferred Close.
	// Closing the socket with the master still in flight closes the master, and
	// the hangup kills runc's init stage (a Go process with a SIGHUP handler)
	// if it lands before init has exec'd the guest.
	master, err := console.Receive()
	if err != nil {
		m.abandonStart(container, reaper)
		return 0, fmt.Errorf("lxc: receive the console of %s: %w", spec.Name, err)
	}
	m.serveConsole(spec.ID, master)

	// Networking. A failure here stops the container rather than leaving it
	// running with no interfaces, which would look like a working container
	// that mysteriously cannot be reached.
	if err := m.attachNICs(spec, pid); err != nil {
		m.abandonStart(container, reaper)
		return 0, err
	}

	// Without this init stays blocked on the exec fifo forever and the
	// container reports "defined" while nothing inside it ever runs.
	if err := container.Exec(); err != nil {
		m.abandonStart(container, reaper)
		return 0, fmt.Errorf("lxc: release init of %s: %w", spec.Name, err)
	}
	return pid, nil
}

// abandonStart tears down a start that failed after init was spawned, so the
// next start finds the id free instead of a parked init holding its cgroup, and
// the killed init is reaped rather than left a zombie.
func (m *Manager) abandonStart(container *libcontainer.Container, reaper *initReaper) {
	reaper.expect()
	errlog.Log(container.Signal(os.Kill))
	reaper.await(initReapTimeout)
	errlog.Log(container.Destroy())
}

// watchInit starts reaping a freshly started init. The reaper is registered
// before its goroutine starts, so an init that dies at once is still recognised
// as this start's; the goroutine ends when init exits.
func (m *Manager) watchInit(ctID string, process *libcontainer.Process) *initReaper {
	reaper := newInitReaper()
	m.mu.Lock()
	m.inits[ctID] = reaper
	m.mu.Unlock()
	go reaper.reap(process, func(r *initReaper) { m.initExited(ctID, r) })
	return reaper
}

// currentReaper is the reaper of ctID's latest init started by this process.
func (m *Manager) currentReaper(ctID string) *initReaper {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inits[ctID]
}

// expectExit marks ctID's running init as being stopped on purpose, so the exit
// the coming signal causes is not reported as a crash.
func (m *Manager) expectExit(ctID string) {
	if reaper := m.currentReaper(ctID); reaper != nil {
		reaper.expect()
	}
}

// awaitReaped waits, bounded, until ctID's init has been reaped, so a stop that
// returns leaves no zombie behind it. An init this process did not start has no
// reaper and nothing to wait for.
func (m *Manager) awaitReaped(ctID string) {
	if reaper := m.currentReaper(ctID); reaper != nil {
		reaper.await(initReapTimeout)
	}
}

// lastExit is how ctID's most recent init ended, with ok false while it runs or
// when this process did not start it.
func (m *Manager) lastExit(ctID string) (initExit, bool) {
	reaper := m.currentReaper(ctID)
	if reaper == nil {
		return initExit{}, false
	}
	return reaper.result()
}

// initExited runs on the reaper goroutine once init is gone. A requested exit
// needs nothing: the stop that asked for it releases the staging itself. A crash
// is published, and its staged rootfs and console are released under the op
// lock, but only while no later start has replaced this init.
func (m *Manager) initExited(ctID string, reaper *initReaper) {
	exit, _ := reaper.result()
	if exit.Expected || m.currentReaper(ctID) != reaper {
		return
	}
	crashed, reason := classifyInitExit(exit)
	if m.onState != nil {
		state := State{ContainerID: ctID, State: StateStopped, ExitReason: reason}
		if crashed {
			state.State = StateCrashed
		}
		m.onState(state)
	}

	lock := m.lockFor(ctID)
	lock.Lock()
	var hub *consoleHub
	if m.currentReaper(ctID) == reaper {
		// Best-effort: the next start, a Stop or the agent-start sweep releases a
		// staging this one could not, and a crash has no caller to report to.
		errlog.Log(m.releaseStaged(ctID))
		m.mu.Lock()
		hub = m.consoles[ctID]
		m.mu.Unlock()
	}
	lock.Unlock()
	if hub == nil {
		return
	}

	// The pty normally hangs up by itself once PID 1's namespace is gone; ending
	// the hub only after a grace keeps the guest's last output in the replay.
	timer := time.NewTimer(consoleDrainGrace)
	defer timer.Stop()
	select {
	case <-hub.done:
	case <-timer.C:
		hub.end()
	}
}

// attachNICs gives the container each of its interfaces.
//
// A bridged NIC is a veth pair with one end enslaved and the other moved in. A
// direct NIC is a single child of a host adapter moved in whole — there is no
// host-side half, which is also why the host cannot reach it without a shim.
func (m *Manager) attachNICs(spec Spec, pid int) error {
	for _, nic := range spec.NICs {
		guestName := nic.GuestName
		if guestName == "" {
			guestName = "eth0"
		}

		if netconf.IsDirectMode(nic.AttachKind) {
			// The NIC carries the network's uplink, so the tag is resolved here
			// rather than at network create: stacking the container on the
			// untagged adapter would put it on the native VLAN.
			parent, err := netconf.EnsureVLANParent(nic.ParentLink, nic.VLANTag)
			if err != nil {
				return fmt.Errorf("lxc: resolve parent for %s: %w", guestName, err)
			}
			if _, _, err := netconf.CreateDirect(netconf.DirectSpec{
				Name:      netconf.DirectName(spec.ID, nic.Slot),
				Driver:    nic.AttachKind,
				Parent:    parent,
				LinkMode:  nic.LinkMode,
				MAC:       nic.MAC,
				NetnsPID:  pid,
				GuestName: guestName,
			}); err != nil {
				return fmt.Errorf("lxc: attach %s to %s: %w", guestName, nic.ParentLink, err)
			}
			if err := m.applyStaticAddress(nic, guestName, pid); err != nil {
				return err
			}
			continue
		}

		hostName := nic.VethHost
		peerName := nic.VethPeer
		if hostName == "" || peerName == "" {
			hostName, peerName = netconf.VethNames(spec.ID, nic.Slot)
		}

		if _, err := netconf.CreateVeth(netconf.VethSpec{
			HostName:  hostName,
			PeerName:  peerName,
			GuestName: guestName,
			Bridge:    nic.Bridge,
			VLANTag:   nic.VLANTag,
			MAC:       nic.MAC,
			NetnsPID:  pid,
		}); err != nil {
			return fmt.Errorf("lxc: attach %s to %s: %w", guestName, nic.Bridge, err)
		}

		if err := m.applyStaticAddress(nic, guestName, pid); err != nil {
			return err
		}
	}
	return nil
}

// applyStaticAddress sets a container interface's address from the host side.
//
// Done from outside so it works regardless of which network config format the
// guest distribution uses; the in-rootfs config is written separately, for
// persistence across the guest's own reboots.
//
// The IPv6 pair is a second address on the same interface, applied on its own
// so a guest whose IPv4 comes from DHCP can still carry a fixed IPv6 address.
func (m *Manager) applyStaticAddress(nic NIC, guestName string, pid int) error {
	if nic.AddressMode == "static" && nic.StaticIP != "" {
		if err := netconf.ConfigureAddress(pid, guestName, netconf.AddressConfig{
			CIDR:    nic.StaticIP,
			Gateway: nic.StaticGW,
		}); err != nil {
			return err
		}
	}
	if nic.StaticIPv6 == "" {
		return nil
	}
	return netconf.ConfigureAddress(pid, guestName, netconf.AddressConfig{
		CIDR:    nic.StaticIPv6,
		Gateway: nic.StaticGW6,
	})
}

// Shutdown asks a container's init to stop, escalating after the grace period.
//
// The signal is the one that init actually listens for — SIGRTMIN+3 for
// systemd, SIGTERM otherwise. Sending SIGTERM to systemd's PID 1 does nothing,
// waits out the timeout and then kills the container, which is the entire
// reason "stopping a container takes 90 seconds" is a common complaint
// elsewhere.
func (m *Manager) Shutdown(ctx context.Context, ctID string, initSystem string, grace time.Duration) (escalated bool, err error) {
	lock := m.lockFor(ctID)
	lock.Lock()
	defer lock.Unlock()

	escalated, err = m.shutdown(ctx, ctID, initSystem, grace)
	if err != nil {
		return escalated, err
	}
	m.awaitReaped(ctID)
	return escalated, m.releaseStaged(ctID)
}

// shutdown is Shutdown under the container's op lock, before the staged rootfs
// is released.
func (m *Manager) shutdown(ctx context.Context, ctID string, initSystem string, grace time.Duration) (bool, error) {
	container, err := m.load(ctID)
	if err != nil {
		// Absent or never started is already the desired state.
		return false, ignoreNotLive(err)
	}

	m.expectExit(ctID)
	if err := container.Signal(unixSignal(StopSignalFor(initSystem))); err != nil {
		if errors.Is(err, libcontainer.ErrNotRunning) {
			return false, nil
		}
		return false, fmt.Errorf("lxc: signal %s: %w", ctID, err)
	}

	if grace <= 0 {
		grace = 30 * time.Second
	}
	deadline := time.Now().Add(grace)
	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
		}
		status, err := container.Status()
		if err != nil || status == libcontainer.Stopped {
			return false, nil
		}
	}

	return true, m.Stop(ctx, ctID)
}

// Stop kills a container's whole cgroup and releases its staged rootfs, which
// also clears one left by a container that had already exited on its own.
func (m *Manager) Stop(ctx context.Context, ctID string) error {
	if err := m.kill(ctx, ctID); err != nil {
		return err
	}
	m.awaitReaped(ctID)
	return m.releaseStaged(ctID)
}

// kill is Stop without the staging release.
func (m *Manager) kill(ctx context.Context, ctID string) error {
	container, err := m.load(ctID)
	if err != nil {
		return ignoreNotLive(err)
	}
	// Signalling the container signals every process in its cgroup, not only
	// its init — a container whose init is already gone still has children.
	m.expectExit(ctID)
	if err := container.Signal(os.Kill); err != nil {
		// A container that ran and exited still loads, and signalling it
		// reports ErrNotRunning; stopped is exactly what Stop wanted.
		if errors.Is(err, libcontainer.ErrNotRunning) {
			return nil
		}
		return fmt.Errorf("lxc: kill %s: %w", ctID, err)
	}

	// Wait briefly for the cgroup to empty so a delete straight afterwards
	// does not race the kernel reaping.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := container.Status()
		if err != nil || status == libcontainer.Stopped {
			return nil //nolint:nilerr // an unreadable container status counts as stopped so the wait ends
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stopPollInterval):
		}
	}
	return nil
}

// ignoreNotLive swallows the two load failures that mean "nothing is running":
// no state at all, and a container that was created but never started.
func ignoreNotLive(err error) error {
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotRunning) {
		return nil
	}
	return err
}

// Freeze suspends every process in a container through the cgroup freezer.
func (m *Manager) Freeze(_ context.Context, ctID string) error {
	container, err := m.load(ctID)
	if err != nil {
		return err
	}
	if err := container.Pause(); err != nil {
		return fmt.Errorf("lxc: freeze %s: %w", ctID, err)
	}
	return nil
}

// Unfreeze thaws a frozen container.
func (m *Manager) Unfreeze(_ context.Context, ctID string) error {
	container, err := m.load(ctID)
	if err != nil {
		return err
	}
	if err := container.Resume(); err != nil {
		return fmt.Errorf("lxc: thaw %s: %w", ctID, err)
	}
	return nil
}

// Delete removes a container's runtime state and its veth devices. The rootfs
// is the caller's to remove, because only it knows the pool.
func (m *Manager) Delete(ctx context.Context, spec Spec) error {
	lock := m.lockFor(spec.ID)
	lock.Lock()
	defer lock.Unlock()

	if err := m.Stop(ctx, spec.ID); err != nil {
		return err
	}
	// Both shapes of leftover: a stopped container is destroyed with its cgroup,
	// and a never-started state directory, which Load cannot see, is removed.
	if err := m.clearLeftover(spec.ID); err != nil {
		return fmt.Errorf("lxc: destroy %s: %w", spec.Name, err)
	}

	CleanupNICs(spec)

	paths := PathsFor(spec.ID)
	if err := os.RemoveAll(paths.Dir); err != nil {
		return fmt.Errorf("lxc: remove runtime directory for %s: %w", spec.Name, err)
	}

	m.mu.Lock()
	delete(m.ops, spec.ID)
	delete(m.cancels, spec.ID)
	delete(m.cpuSamples, spec.ID)
	delete(m.inits, spec.ID)
	m.rootfsUsage.forget(spec.ID)
	m.mu.Unlock()
	return nil
}

// CleanupNICs removes the veth devices a container owned.
func CleanupNICs(spec Spec) {
	for _, nic := range spec.NICs {
		hostName := nic.VethHost
		if hostName == "" {
			hostName, _ = netconf.VethNames(spec.ID, nic.Slot)
		}
		// Deleting the host end takes the peer with it: a veth pair cannot
		// outlive half of itself.
		errlog.Log(netconf.DeleteLink(hostName))
	}
}

// UpdateLimits applies new cgroup values to a running container.
//
// This is a genuine advantage over VMs and the UI should make it visible: the
// resource sliders on a running container apply immediately, with no restart,
// because cgroup v2 values are writable in place.
func (m *Manager) UpdateLimits(_ context.Context, spec Spec) (Limits, error) {
	container, err := m.load(spec.ID)
	if err != nil {
		if ignoreNotLive(err) == nil {
			// Nothing to write in place: Start builds the config from the spec,
			// so the new limits take effect on the next start.
			return spec.Limits, nil
		}
		return Limits{}, err
	}
	if containerState(container) == StateStopped {
		return spec.Limits, nil
	}

	config := container.Config()
	config.Cgroups = buildCgroups(spec)

	// Set writes through the same manager libcontainer used to create the
	// container, so the delegation and ordering rules are honoured rather than
	// re-derived by writing cgroup files directly.
	if err := container.Set(config); err != nil {
		return Limits{}, fmt.Errorf("lxc: apply limits to %s: %w", spec.Name, err)
	}
	return spec.Limits, nil
}

// Status reads one container's confirmed state.
func (m *Manager) Status(_ context.Context, ctID string) (State, error) {
	state := State{ContainerID: ctID}

	p, err := probePresence(m.stateDir(ctID))
	if err != nil {
		state.State = StateUnknown
		state.LastError = "the container's state could not be read"
		return state, nil //nolint:nilerr // an unreadable state is reported as StateUnknown inside the State value, not as an error
	}
	if unloaded, ok := unloadedState(p); ok {
		// No state.json: never created here, or created and not yet started.
		// Either way nothing runs, which is a confirmed observation.
		state.State = unloaded
		return state, nil
	}

	container, err := libcontainer.Load(m.root, ctID)
	if err != nil {
		state.State = StateUnknown
		state.LastError = "the container's state could not be read"
		return state, nil //nolint:nilerr // an unreadable state is reported as StateUnknown inside the State value, not as an error
	}

	state.State = containerState(container)
	if state.State == StateUnknown {
		state.LastError = "the container's state could not be read"
	}
	if state.State == StateStopped {
		// libcontainer only knows the init is gone; its parent knows how it went.
		if exit, ok := m.lastExit(ctID); ok {
			crashed, reason := classifyInitExit(exit)
			if crashed {
				state.State = StateCrashed
			}
			state.ExitReason = reason
		}
	}

	if runtimeState, err := container.State(); err == nil {
		state.StartedAt = runtimeState.Created
		state.InitPID = reportedInitPID(state.State, runtimeState.InitProcessPid)
	}

	if state.State == StateRunning && state.InitPID > 0 {
		// Read from the container's own netns rather than asked of the guest.
		// Unlike a guest-agent answer this is the host's own observation, so
		// it carries no "guest-reported" caveat.
		if ips, err := netconf.ContainerAddresses(state.InitPID); err == nil {
			state.IPs = ips
		}
	}
	return state, nil
}

// Query reconciles every container on this host.
//
// Directories under the state root that the backend has no row for are returned
// as orphans, never destroyed — the same rule the VM side follows, for the same
// reason.
func (m *Manager) Query(ctx context.Context, known []string) ([]State, []string, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("lxc: read state root: %w", err)
	}

	knownSet := make(map[string]bool, len(known))
	for _, id := range known {
		knownSet[id] = true
	}

	var states []State
	var orphans []string
	seen := map[string]bool{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ctID := entry.Name()
		seen[ctID] = true
		if !knownSet[ctID] {
			orphans = append(orphans, ctID)
			continue
		}
		state, err := m.Status(ctx, ctID)
		if err != nil {
			continue
		}
		states = append(states, state)
	}

	for _, ctID := range known {
		if !seen[ctID] {
			states = append(states, State{ContainerID: ctID, State: StateStopped})
		}
	}
	return states, orphans, nil
}

// Sample reads a running or frozen container's cgroup counters through the
// cgroup manager libcontainer built for it, plus its network counters.
func (m *Manager) Sample(ctx context.Context, ctID string) (Stats, error) {
	container, err := m.load(ctID)
	if err != nil {
		return Stats{}, err
	}
	switch containerState(container) {
	case StateRunning, StateFrozen:
	default:
		return Stats{}, fmt.Errorf("%w: %s", ErrNotRunning, ctID)
	}

	stats, err := m.readStats(container)
	if err != nil {
		return Stats{}, err
	}
	current := cpuSample{usageNSec: stats.CPUUsageNSec, at: stats.SampledAt}

	m.mu.Lock()
	previous := m.cpuSamples[ctID]
	m.mu.Unlock()
	if !usableCPUSample(previous, current) {
		// No recent baseline: take one now rather than report a load of zero.
		previous = current
		select {
		case <-ctx.Done():
			return Stats{}, ctx.Err()
		case <-time.After(cpuResampleWindow):
		}
		if stats, err = m.readStats(container); err != nil {
			return Stats{}, err
		}
		current = cpuSample{usageNSec: stats.CPUUsageNSec, at: stats.SampledAt}
	}
	m.mu.Lock()
	m.cpuSamples[ctID] = current
	m.mu.Unlock()

	config := container.Config()
	quota, period := int64(0), uint64(0)
	if config.Cgroups != nil && config.Cgroups.Resources != nil {
		quota, period = config.Cgroups.CpuQuota, config.Cgroups.CpuPeriod
	}
	stats.CPUPercent = cpuPercent(previous, current, cpuAllowance(quota, period, runtime.NumCPU()))
	stats.RootfsUsedBytes = m.rootfsUsage.get(ctx, ctID, config.Rootfs, rootfsUsageFirstWait)
	return stats, nil
}

// readStats takes one reading of a container's counters.
func (m *Manager) readStats(container *libcontainer.Container) (Stats, error) {
	raw, err := container.Stats()
	if err != nil {
		return Stats{}, fmt.Errorf("lxc: sample %s: %w", container.ID(), err)
	}

	stats := Stats{ContainerID: container.ID(), SampledAt: time.Now()}
	if cg := raw.CgroupStats; cg != nil {
		stats.CPUUsageNSec = cg.CpuStats.CpuUsage.TotalUsage
		stats.MemoryBytes = cg.MemoryStats.Usage.Usage
		stats.MemoryMaxBytes = unlimitedAsZero(cg.MemoryStats.Usage.Limit)
		stats.SwapBytes = cg.MemoryStats.SwapOnlyUsage.Usage
		stats.PidsCurrent = cg.PidsStats.Current
		stats.PidsMax = unlimitedAsZero(cg.PidsStats.Limit)
		for _, entry := range cg.BlkioStats.IoServiceBytesRecursive {
			switch strings.ToLower(entry.Op) {
			case "read":
				stats.IOReadBytes += entry.Value
			case "write":
				stats.IOWriteBytes += entry.Value
			}
		}
	}

	state, err := container.State()
	if err != nil {
		return stats, nil //nolint:nilerr // an unreadable libcontainer state leaves the stats without the cgroup extras rather than failing them
	}
	// libcontainer's stats carry no memory.high, so it is read from the cgroup
	// itself: what is in force, not what the spec asked for.
	if dir := state.CgroupPaths[""]; dir != "" {
		if body, err := os.ReadFile(filepath.Join(dir, "memory.high")); err == nil {
			configured := uint64(0)
			if res := container.Config().Cgroups; res != nil && res.Resources != nil {
				configured = parseCgroupLimit(res.Unified["memory.high"])
			}
			stats.MemoryHighBytes = reportedMemoryHigh(configured, parseCgroupLimit(string(body)), os.Getpagesize())
		}
	}
	// The veths are attached by this package, not declared in the libcontainer
	// config, so raw.Interfaces is empty; the container's own netns table is not.
	if state.InitProcessPid > 0 {
		if body, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(state.InitProcessPid), "net", "dev")); err == nil {
			stats.NetRxBytes, stats.NetTxBytes = parseNetDev(string(body))
		}
	}
	return stats, nil
}

// Processes lists the processes in a container's cgroup.
//
// This is a genuinely useful thing a system container has that a VM does not:
// the host can see inside without any in-guest agent, because the processes are
// host processes in a namespace.
func (m *Manager) Processes(_ context.Context, ctID string) ([]Process, error) {
	container, err := m.load(ctID)
	if err != nil {
		return nil, err
	}
	pids, err := container.Processes()
	if err != nil {
		return nil, fmt.Errorf("lxc: list processes in %s: %w", ctID, err)
	}
	sort.Ints(pids)

	out := make([]Process, 0, len(pids))
	for _, pid := range pids {
		proc := Process{HostPID: pid}
		// Everything below is best-effort: a process that exits between the
		// listing and the read is normal, and skipping its details is better
		// than failing the whole call.
		if cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline")); err == nil {
			proc.Command = strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " "))
		}
		if status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status")); err == nil {
			applyProcStatus(&proc, string(status))
		}
		if proc.Command == "" {
			continue
		}
		out = append(out, proc)
	}
	return out, nil
}

// applyProcStatus reads the fields of /proc/<pid>/status this listing shows.
//
// NSpid is the interesting one: its last field is the pid as seen *inside* the
// container, which is what an operator comparing against `ps` in the console
// expects to see.
func applyProcStatus(proc *Process, status string) {
	for _, line := range strings.Split(status, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "PPid":
			if n, err := strconv.Atoi(value); err == nil {
				proc.PPID = n
			}
		case "NSpid":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				if n, err := strconv.Atoi(fields[len(fields)-1]); err == nil {
					proc.PID = n
				}
			}
		case "VmRSS":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				if n, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
					proc.MemoryBytes = n * 1024
				}
			}
		}
	}
	if proc.PID == 0 {
		proc.PID = proc.HostPID
	}
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

// DetectShell picks a shell that exists in a container's rootfs.
//
// Following the Docker terminal path's own convention: try bash, fall back to
// sh, and say clearly when neither is there rather than opening a session that
// immediately dies. Lstat, because a distribution's /bin/sh is usually an
// absolute symlink that would resolve against the host's root, not the guest's.
func DetectShell(rootfs string) ([]string, error) {
	for _, shell := range []string{"/bin/bash", "/usr/bin/bash", "/bin/sh", "/usr/bin/sh"} {
		if _, err := os.Lstat(filepath.Join(rootfs, strings.TrimPrefix(shell, "/"))); err == nil {
			return []string{shell}, nil
		}
	}
	return nil, errors.New("lxc: this container's filesystem has neither bash nor sh")
}

// initReapTimeout bounds how long a stop waits for a killed init to be reaped.
// SIGKILL on a pid namespace's init is not ignorable, so this is only ever hit by
// a process stuck in uninterruptible sleep, and the stop must not hang on it.
const initReapTimeout = 10 * time.Second

// consoleDrainGrace is how long a crashed container's console may keep draining
// after its init was reaped before the agent closes it; normally the pty hangs up
// on its own first, and the grace keeps the guest's last words in the replay.
const consoleDrainGrace = 2 * time.Second

// rootfsUsageFirstWait is how long a sample waits for a container's first
// measurement before answering without it.
const rootfsUsageFirstWait = 2 * time.Second

// cpuResampleWindow is how long a stats call waits for a second reading when it
// has no recent previous one: cumulative CPU time alone says nothing about load.
const cpuResampleWindow = 250 * time.Millisecond
