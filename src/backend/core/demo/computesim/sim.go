// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package computesim simulates the compute side of the demo estate: KVM
// virtual machines, LXC system containers, storage pools and guest networks on
// every estate host that runs guests. It answers the agent's compute messages
// as a fakeagent Extension and seeds the compute plugins' tables to match, so
// the plugins' lists and the simulated hosts agree from the first page load.
package computesim

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
)

// ErrDemoRefused is the text every operation demo mode will not simulate
// carries back, so the operator reads why rather than a generic failure.
const ErrDemoRefused = "This operation is not available in demo mode."

// Guest states, spelled the way the kvm and lxc plugins store them.
const (
	stateDefined = "defined"
	stateRunning = "running"
	statePaused  = "paused"
	stateFrozen  = "frozen"
	stateStopped = "stopped"
	stateCrashed = "crashed"
)

type disk struct {
	ID         string
	PoolID     string
	Slot       int
	Bus        string
	Format     string
	SizeBytes  int64
	VolumePath string
	IsBoot     bool
	IsCDROM    bool
}

type nic struct {
	ID        string
	NetworkID string
	Bridge    string
	Slot      int
	MAC       string
	IP        string
}

type snapshot struct {
	ID          string
	ParentID    string
	Name        string
	Handle      string
	CreatedAt   time.Time
	SizeBytes   int64
	WithMemory  bool
	Consistency string
	savedState  string
}

type vm struct {
	ID          string
	Name        string
	Description string
	OS          osProfile
	VCPUs       int
	MemoryMB    int64
	MachineType string
	UUID        string
	State       string
	RunState    string
	LastError   string
	StartedAt   time.Time
	CreatedAt   time.Time
	IsTemplate  bool
	Autostart   bool
	Disks       []*disk
	NICs        []*nic
	Snapshots   []*snapshot
	Current     string
	load        float64
	// SpiceOptions is the display profile the last define or start carried. The
	// simulator runs no SPICE server; it keeps them so a demo round-trips them.
	SpiceOptions *coreagent.VMSpiceOptions
}

type ct struct {
	ID          string
	Name        string
	Description string
	OS          ctOS
	Role        ctRole
	PoolID      string
	RootfsPath  string
	State       string
	LastError   string
	ExitReason  string
	StartedAt   time.Time
	CreatedAt   time.Time
	InitPID     int
	CPUs        int
	MemoryMax   int64
	PidsMax     int64
	DiskQuota   int64
	Privileged  bool
	Nesting     bool
	IsTemplate  bool
	Autostart   bool
	NICs        []*nic
	Snapshots   []*snapshot
	load        float64
}

type pool struct {
	ID        string
	Name      string
	Backend   string
	Target    string
	Capacity  int64
	baseUsed  int64
	IsDefault bool
	CreatedAt time.Time
}

type network struct {
	ID         string
	Name       string
	Mode       string
	Bridge     string
	VLAN       int
	Subnet     string
	Gateway    string
	DHCP       bool
	RangeStart string
	RangeEnd   string
	NAT        bool
	Uplink     string
	IsDefault  bool
	CreatedAt  time.Time
	prefix     string
	nextHost   int
}

type cachedImage struct {
	ImageID   string
	Kind      string
	LocalPath string
	SizeBytes int64
	LastUsed  time.Time
}

// hostSim is one compute host's guests and plumbing.
type hostSim struct {
	host     *estate.Host
	index    int
	pools    []*pool
	networks []*network
	vms      map[string]*vm
	vmOrder  []string
	cts      map[string]*ct
	ctOrder  []string
	images   []cachedImage
	probedAt time.Time
}

// Sim is the compute simulator for the whole estate. It is safe for concurrent
// use; every read and mutation holds mu so the reconciler and a click never
// see a half-applied transition.
type Sim struct {
	mu       sync.Mutex
	seed     uint64
	builtAt  time.Time
	hosts    map[string]*hostSim
	order    []string
	sessions map[string]*consoleSession
	fetches  map[string]context.CancelFunc
	logger   *slog.Logger
	clock    func() time.Time
}

// Size is how many VMs and system containers the whole estate should hold. A
// negative value keeps the built-in amount, which is what New uses.
type Size struct {
	VMs              int
	SystemContainers int
}

// New builds the simulator for e from seed with the built-in sizes. now anchors
// creation times and uptimes so the estate looks lived-in whenever demo mode starts.
func New(e *estate.Estate, seed uint64, now time.Time) *Sim {
	return NewSized(e, seed, now, Size{VMs: -1, SystemContainers: -1})
}

// NewSized builds the simulator with size spread over the compute hosts in
// proportion to their built-in plans, landing exactly on the requested totals.
func NewSized(e *estate.Estate, seed uint64, now time.Time, size Size) *Sim {
	s := &Sim{
		seed:     seed,
		builtAt:  now,
		hosts:    map[string]*hostSim{},
		sessions: map[string]*consoleSession{},
		fetches:  map[string]context.CancelFunc{},
		logger:   slog.Default(),
		clock:    time.Now,
	}
	if e == nil {
		return s
	}
	var compute []*estate.Host
	for _, h := range e.Hosts {
		if h.Compute {
			compute = append(compute, h)
		}
	}
	plans := make([]hostPlan, len(compute))
	vmWeights := make([]int, len(compute))
	ctWeights := make([]int, len(compute))
	for i, h := range compute {
		plans[i] = planFor(h.Key)
		vmWeights[i] = plans[i].vms
		ctWeights[i] = plans[i].cts
	}
	if size.VMs >= 0 {
		for i, n := range distribute(size.VMs, vmWeights) {
			plans[i].vms = n
		}
	}
	if size.SystemContainers >= 0 {
		for i, n := range distribute(size.SystemContainers, ctWeights) {
			plans[i].cts = n
		}
	}
	macs := map[string]bool{}
	for i, h := range compute {
		s.hosts[h.Key] = buildHost(h, i+1, seed, now, macs, plans[i])
		s.order = append(s.order, h.Key)
	}
	return s
}

// distribute splits total over weights by largest remainder, so the parts sum
// to total exactly and keep the weights' proportions.
func distribute(total int, weights []int) []int {
	out := make([]int, len(weights))
	sum := 0
	for _, w := range weights {
		sum += w
	}
	if total <= 0 || sum == 0 {
		return out
	}
	type rem struct {
		i    int
		frac float64
	}
	rems := make([]rem, len(weights))
	given := 0
	for i, w := range weights {
		exact := float64(total) * float64(w) / float64(sum)
		out[i] = int(exact)
		given += out[i]
		rems[i] = rem{i, exact - float64(out[i])}
	}
	sort.Slice(rems, func(a, b int) bool {
		if rems[a].frac == rems[b].frac {
			return rems[a].i < rems[b].i
		}
		return rems[a].frac > rems[b].frac
	})
	for k := 0; given < total; k = (k + 1) % len(rems) {
		out[rems[k].i]++
		given++
	}
	return out
}

// Counts reports how many VMs, system containers, pools and networks the
// simulator holds, templates included, which is what SeedDB writes.
func (s *Sim) Counts() (vms, cts, pools, networks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, hs := range s.hosts {
		vms += len(hs.vms)
		cts += len(hs.cts)
		pools += len(hs.pools)
		networks += len(hs.networks)
	}
	return vms, cts, pools, networks
}

var _ fakeagent.Extension = (*Sim)(nil)

// Handle answers every compute, pool, network, image, VM, CT and console
// message; it returns false for anything else so other extensions can claim it.
// It never blocks the agent's read loop on a long operation.
func (s *Sim) Handle(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send fakeagent.Sender) bool {
	if msg == nil {
		return false
	}
	switch {
	case isVMType(msg.Type):
		s.handleVM(ctx, host, msg, send)
	case isCTType(msg.Type):
		s.handleCT(ctx, host, msg, send)
	case isConsoleType(msg.Type):
		s.handleConsole(ctx, host, msg, send)
	case isHostType(msg.Type):
		s.handleHostFamily(ctx, host, msg, send)
	default:
		return false
	}
	return true
}

func (s *Sim) deliver(send fakeagent.Sender, msg *coreagent.WSMessage) {
	if err := send(msg); err != nil {
		s.logger.Debug("demo compute reply not delivered", "type", msg.Type, "error", err)
	}
}

func (s *Sim) now() time.Time { return s.clock().UTC() }

// hostFor returns the simulated compute host for host, or nil when the host
// runs no guests; callers hold s.mu.
func (s *Sim) hostFor(host *estate.Host) *hostSim {
	if host == nil {
		return nil
	}
	return s.hosts[host.Key]
}

var vmTypes = map[string]bool{
	coreagent.MsgVMDefine: true, coreagent.MsgVMStart: true, coreagent.MsgVMShutdown: true,
	coreagent.MsgVMStop: true, coreagent.MsgVMReset: true, coreagent.MsgVMPause: true,
	coreagent.MsgVMResume: true, coreagent.MsgVMDelete: true, coreagent.MsgVMQuery: true,
	coreagent.MsgVMStats: true, coreagent.MsgHostDeviceList: true, coreagent.MsgVMLog: true,
	coreagent.MsgVMDiskList: true, coreagent.MsgVMDiskCreate: true, coreagent.MsgVMDiskResize: true,
	coreagent.MsgVMDiskDelete: true, coreagent.MsgVMSnapshotCreate: true,
	coreagent.MsgVMSnapshotRevert: true, coreagent.MsgVMSnapshotDelete: true,
	coreagent.MsgVMImportInspect: true, coreagent.MsgVMImportConvert: true,
	coreagent.MsgVMImportStage: true, coreagent.MsgVMImportDiscard: true,
	coreagent.MsgVMCloneRequest: true, coreagent.MsgVMGuestQuery: true,
	coreagent.MsgVMGuestAgentDisc: true, coreagent.MsgVMQEMUCapability: true,
	coreagent.MsgVMMemorySet: true, coreagent.MsgVMCPUSet: true, coreagent.MsgVMThrottleSet: true,
	coreagent.MsgVMImageInspect: true, coreagent.MsgVMBackupRun: true,
	coreagent.MsgVMBackupRestore: true, coreagent.MsgVMMigrate: true, coreagent.MsgVMCancel: true,
	coreagent.MsgVMSpiceTicket: true, coreagent.MsgVMDiskHotplug: true, coreagent.MsgVMMediaChange: true,
	coreagent.MsgVMNICHotplug: true,
}

var ctTypes = map[string]bool{
	coreagent.MsgCTCreate: true, coreagent.MsgCTStart: true, coreagent.MsgCTShutdown: true,
	coreagent.MsgCTStop: true, coreagent.MsgCTRestart: true, coreagent.MsgCTFreeze: true,
	coreagent.MsgCTUnfreeze: true, coreagent.MsgCTDelete: true, coreagent.MsgCTQuery: true,
	coreagent.MsgCTProcesses: true, coreagent.MsgCTUpdateLimits: true,
	coreagent.MsgCTSnapshotCreate: true, coreagent.MsgCTSnapshotRevert: true,
	coreagent.MsgCTSnapshotDelete: true, coreagent.MsgCTCloneRequest: true,
	coreagent.MsgCTCancel: true, coreagent.MsgCTBackupRun: true, coreagent.MsgCTBackupRestore: true,
}

var hostTypes = map[string]bool{
	coreagent.MsgComputeProbeRequest: true, coreagent.MsgComputeConfigRequest: true,
	coreagent.MsgComputeMigrateRequest: true,
	coreagent.MsgPoolDetectRequest:     true, coreagent.MsgPoolCreateRequest: true,
	coreagent.MsgPoolStatsRequest: true, coreagent.MsgPoolRemoveRequest: true,
	coreagent.MsgPoolBrowseRequest: true, coreagent.MsgPoolFileRequest: true,
	coreagent.MsgNetDetectRequest: true, coreagent.MsgNetEnsureRequest: true,
	coreagent.MsgNetRemoveRequest: true, coreagent.MsgNetAdoptConfirm: true,
	coreagent.MsgImageFetchRequest: true, coreagent.MsgImageListRequest: true,
	coreagent.MsgImageRemoveRequest: true, coreagent.MsgImageCancelRequest: true,
}

func isVMType(t string) bool   { return vmTypes[t] }
func isCTType(t string) bool   { return ctTypes[t] }
func isHostType(t string) bool { return hostTypes[t] }
func isConsoleType(t string) bool {
	switch t {
	case coreagent.MsgConsoleStart, coreagent.MsgConsoleInput, coreagent.MsgConsoleResize, coreagent.MsgConsoleEnd:
		return true
	}
	return false
}

// rng is splitmix64, the same generator the estate uses, so a seed fully
// determines the compute estate without anything being stored.
type rng struct{ state uint64 }

func newRNG(seed uint64) *rng { return &rng{state: seed} }

func (r *rng) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

func (r *rng) between(lo, hi int) int { return lo + r.intn(hi-lo+1) }

func (r *rng) float() float64 { return float64(r.next()>>11) / float64(1<<53) }

func (r *rng) chance(p float64) bool { return r.float() < p }

func pick[T any](r *rng, items []T) T { return items[r.intn(len(items))] }

// stableID derives an xid-shaped identifier from the seed and a name, with the
// creation time in its leading bytes the way a real xid carries it, so the ids
// survive a restart and still sort by age.
func stableID(seed uint64, created time.Time, parts ...string) string {
	key := fmt.Sprintf("%x|%s", seed, strings.Join(parts, "|"))
	var raw [12]byte
	binary.BigEndian.PutUint32(raw[0:4], uint32(created.Unix()))
	binary.BigEndian.PutUint64(raw[4:12], estate.StableHash(key))
	id, err := xid.FromBytes(raw[:])
	if err != nil {
		return fmt.Sprintf("%020x", estate.StableHash(key))
	}
	return id.String()
}

// stableUUID derives the SMBIOS UUID a guest reports from its id.
func stableUUID(id string) string {
	a := estate.StableHash("uuid-a|" + id)
	b := estate.StableHash("uuid-b|" + id)
	return fmt.Sprintf("%08x-%04x-4%03x-%04x-%012x",
		uint32(a>>32), uint16(a>>16), uint16(a)&0x0fff, (uint16(b>>48)&0x3fff)|0x8000, b&0xffffffffffff)
}

func stateOr(state, fallback string) string {
	if state == "" {
		return fallback
	}
	return state
}
