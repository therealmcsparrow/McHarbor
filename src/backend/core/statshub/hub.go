// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package statshub holds the latest container stats per environment, so every
// reader — the containers page in each open tab, the metrics endpoints and the
// background collector — shares one sample instead of each fanning out its own
// per-container stats calls. An environment is fed either by pushes from its
// agent or by a single-flight pull.
package statshub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Mode names how an environment's current snapshot was obtained.
type Mode string

// The two stats paths an environment can be on.
const (
	ModePull Mode = "pull"
	ModePush Mode = "push"
)

// MaxBatchesPerSecond is the push ingest budget across every agent. When the
// fleet would exceed it at the configured interval, acks stretch the interval.
const MaxBatchesPerSecond = 10

// MinPushInterval and MaxPushInterval bound every interval the hub hands an
// agent, whatever the settings or the backpressure computation ask for.
const (
	MinPushInterval = 5 * time.Second
	MaxPushInterval = 300 * time.Second
)

// Sample is one container's stats at one instant, in the units the metrics
// tables store.
type Sample struct {
	ID         string
	Name       string
	Key        string
	CPUPercent float64
	MemUsage   int64
	MemLimit   int64
	MemPercent float64
	NetRx      int64
	NetTx      int64
	BlockRead  int64
	BlockWrite int64
	PIDs       uint64
}

// Snapshot is every running container of one environment at one instant.
type Snapshot struct {
	Samples   []Sample
	SampledAt time.Time
	Mode      Mode
}

// Batch is one push from an agent. Session is fixed for one agent connection
// and Seq increases within it, which is what makes ordering checkable.
type Batch struct {
	Session   string
	Seq       uint64
	SampledAt time.Time
	Samples   []Sample
}

// PushResult is the hub's answer to a batch: whether it is now the current
// snapshot, whether it had been seen before, and the interval the agent should
// push at next.
type PushResult struct {
	Applied   bool
	Duplicate bool
	Interval  time.Duration
}

// PushStatus describes an environment's push stream for the stats-path display.
type PushStatus struct {
	Active      bool
	LastBatchAt time.Time
	Interval    time.Duration
}

// Puller fetches a fresh snapshot for one environment the expensive way.
type Puller func(ctx context.Context, envID string) ([]Sample, error)

// Options configures a Hub. Pull is required; the rest have defaults.
type Options struct {
	Pull Puller
	// PullTimeout bounds one background pull for envID. An agent environment
	// needs far longer than a local socket.
	PullTimeout func(envID string) time.Duration
	// RefreshAfter is how old a pulled snapshot may get before a read triggers
	// a background refresh.
	RefreshAfter time.Duration
	// MaxStale is how old a pulled snapshot may be served while its refreshes
	// keep failing; past it the read reports the failure instead of old numbers.
	MaxStale time.Duration
	// FirstReadWait caps how long the first read of an environment waits for
	// its first sweep. A pull through an agent can take a minute at fleet
	// scale; past this the read returns an empty snapshot and the sweep
	// finishes in the background for the next poll.
	FirstReadWait time.Duration
	// PushGrace is how long a subscribed agent has to deliver its first batch
	// before reads stop waiting for it and pull instead.
	PushGrace time.Duration
	// BaseInterval is the operator's configured push interval.
	BaseInterval func() time.Duration
	Now          func() time.Time
}

type envState struct {
	pulled   *Snapshot
	pulling  chan struct{}
	pullErr  error
	pushed   *Snapshot
	session  string
	seq      uint64
	lastPush time.Time
	interval time.Duration
	// subscribedAt is when the agent was last asked to push; zero when it
	// never was, or when it disconnected since.
	subscribedAt time.Time
}

// Hub is the per-environment stats cache. The zero value is not usable; build
// one with New.
type Hub struct {
	opts Options
	mu   sync.Mutex
	envs map[string]*envState
	// wg tracks background pulls so Close can wait them out.
	wg sync.WaitGroup
	// ctx ends every background pull when the hub is closed.
	ctx    context.Context //nolint:containedctx // hub-lifetime root for detached refreshes that must outlive the request that triggered them
	cancel context.CancelFunc
	// remote, when set, is asked before every pull; see SetRemote.
	remote atomic.Pointer[Remote]
}

// New returns a Hub reading through opts.Pull.
func New(opts Options) *Hub {
	if opts.PullTimeout == nil {
		opts.PullTimeout = func(string) time.Duration { return 20 * time.Second }
	}
	if opts.RefreshAfter <= 0 {
		opts.RefreshAfter = 4 * time.Second
	}
	if opts.FirstReadWait <= 0 {
		opts.FirstReadWait = 3 * time.Second
	}
	if opts.PushGrace <= 0 {
		opts.PushGrace = 2 * time.Minute
	}
	if opts.MaxStale <= 0 {
		opts.MaxStale = 2 * time.Minute
	}
	if opts.BaseInterval == nil {
		opts.BaseInterval = func() time.Duration { return DefaultPushInterval }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{opts: opts, envs: make(map[string]*envState), ctx: ctx, cancel: cancel}
}

// Close cancels in-flight background pulls and waits for them to return.
func (h *Hub) Close() {
	h.cancel()
	h.wg.Wait()
}

func (h *Hub) state(envID string) *envState {
	st, ok := h.envs[envID]
	if !ok {
		st = &envState{}
		h.envs[envID] = st
	}
	return st
}

// pushFresh reports whether st's pushed snapshot is recent enough to serve:
// three missed intervals means the stream has stopped, whatever its session says.
func (h *Hub) pushFresh(st *envState, now time.Time) bool {
	return st.pushed != nil && now.Sub(st.lastPush) <= 3*st.interval
}

// awaitingPush reports whether st's agent was subscribed recently enough that
// its first batch is still due, in which case pulling would only duplicate it.
func (h *Hub) awaitingPush(st *envState, now time.Time) bool {
	return !st.subscribedAt.IsZero() && now.Sub(st.subscribedAt) < h.opts.PushGrace
}

// MarkSubscribed records that envID's agent was just asked to push, so reads
// wait for its batches instead of starting a sweep the push makes redundant.
func (h *Hub) MarkSubscribed(envID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state(envID).subscribedAt = h.opts.Now()
}

// Snapshot returns envID's current stats. A fresh push is served as is; a
// cached pull is served at once and refreshed in the background when stale, so
// a slow agent never holds a page request for the length of a full sweep. Only
// the first read of an environment waits, and concurrent first reads share one pull.
func (h *Hub) Snapshot(ctx context.Context, envID string) (Snapshot, error) {
	h.mu.Lock()
	st := h.state(envID)
	now := h.opts.Now()
	if h.pushFresh(st, now) {
		snap := *st.pushed
		h.mu.Unlock()
		return snap, nil
	}
	if h.awaitingPush(st, now) {
		h.mu.Unlock()
		return Snapshot{Mode: ModePush}, nil
	}
	if st.pulled != nil {
		snap := *st.pulled
		age := now.Sub(snap.SampledAt)
		if age > h.opts.RefreshAfter {
			h.startPullLocked(ctx, envID, st)
		}
		err := st.pullErr
		h.mu.Unlock()
		if err != nil && age > h.opts.MaxStale {
			return Snapshot{}, err
		}
		return snap, nil
	}
	wait := h.startPullLocked(ctx, envID, st)
	h.mu.Unlock()

	timer := time.NewTimer(h.opts.FirstReadWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-timer.C:
		return Snapshot{Mode: ModePull}, nil
	case <-wait:
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if st.pulled == nil {
		return Snapshot{}, st.pullErr
	}
	return *st.pulled, nil
}

// SnapshotFresh is Snapshot for a reader that must not accept old numbers — the
// collector writing history. It waits for a pull when the cache is older than maxAge.
func (h *Hub) SnapshotFresh(ctx context.Context, envID string, maxAge time.Duration) (Snapshot, error) {
	h.mu.Lock()
	st := h.state(envID)
	now := h.opts.Now()
	if h.pushFresh(st, now) {
		snap := *st.pushed
		h.mu.Unlock()
		return snap, nil
	}
	if st.pulled != nil && now.Sub(st.pulled.SampledAt) <= maxAge {
		snap := *st.pulled
		h.mu.Unlock()
		return snap, nil
	}
	wait := h.startPullLocked(ctx, envID, st)
	h.mu.Unlock()

	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-wait:
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if st.pullErr != nil || st.pulled == nil {
		return Snapshot{}, st.pullErr
	}
	return *st.pulled, nil
}

// startPullLocked begins a background pull for envID unless one is running,
// and returns the channel that closes when it finishes. The pull keeps ctx's values but not its
// cancellation, since every waiter shares it; only hub shutdown cancels it. h.mu must be held.
func (h *Hub) startPullLocked(ctx context.Context, envID string, st *envState) chan struct{} {
	if st.pulling != nil {
		return st.pulling
	}
	done := make(chan struct{})
	st.pulling = done
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		pullCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.opts.PullTimeout(envID))
		stop := h.cancelOnShutdown(cancel)
		samples, mode, err := h.pull(pullCtx, envID)
		stop()
		cancel()
		h.mu.Lock()
		if err == nil {
			st.pulled = &Snapshot{Samples: samples, SampledAt: h.opts.Now(), Mode: mode}
		}
		st.pullErr = err
		st.pulling = nil
		h.mu.Unlock()
		close(done)
	}()
	return done
}

// Remote reads envID's snapshot from the node that holds its agent. ok is false when
// this node should pull for itself.
type Remote func(ctx context.Context, envID string) (snap Snapshot, ok bool, err error)

// SetRemote makes every pull ask the node holding the agent first. In active/active the
// agent pushes to that node only, so its snapshot is fresher and far cheaper than a
// per-container sweep forwarded through it.
func (h *Hub) SetRemote(remote Remote) {
	if remote == nil {
		h.remote.Store(nil)
		return
	}
	h.remote.Store(&remote)
}

// pull fetches a snapshot, from the owning node when there is one and it answers, and
// otherwise with the local sweep.
func (h *Hub) pull(ctx context.Context, envID string) ([]Sample, Mode, error) {
	var remoteErr error
	if remote := h.remote.Load(); remote != nil {
		snap, ok, err := (*remote)(ctx, envID)
		if ok {
			mode := snap.Mode
			if mode == "" {
				mode = ModePull
			}
			return snap.Samples, mode, nil
		}
		remoteErr = err
	}
	samples, err := h.opts.Pull(ctx, envID)
	if err != nil && remoteErr != nil {
		return nil, ModePull, errors.Join(err, remoteErr)
	}
	return samples, ModePull, err
}

// Push applies one agent batch. Batches are ordered by (session, seq): a seq
// at or below the last one seen in the same session is a replay and is
// acknowledged without being applied, so a resent batch never rolls the
// snapshot backwards and never counts twice.
func (h *Hub) Push(envID string, b Batch) PushResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.state(envID)
	now := h.opts.Now()
	interval := h.intervalLocked(now, envID)
	st.interval = interval

	if b.Session == st.session && b.Seq <= st.seq {
		return PushResult{Duplicate: true, Interval: interval}
	}
	st.session = b.Session
	st.seq = b.Seq
	st.lastPush = now
	sampledAt := b.SampledAt
	if sampledAt.IsZero() {
		sampledAt = now
	}
	st.pushed = &Snapshot{Samples: b.Samples, SampledAt: sampledAt, Mode: ModePush}
	return PushResult{Applied: true, Interval: interval}
}

// intervalLocked is the push interval every agent is told to use: the
// operator's setting, stretched so the whole fleet stays inside
// MaxBatchesPerSecond. Fifty agents at 5s is ten batches a second; a hundred
// are told to slow to 10s.
func (h *Hub) intervalLocked(now time.Time, pushing string) time.Duration {
	base := clampInterval(h.opts.BaseInterval())
	active := 0
	for id, st := range h.envs {
		if id == pushing || h.pushFresh(st, now) {
			active++
		}
	}
	needed := time.Duration(active) * time.Second / MaxBatchesPerSecond
	return clampInterval(max(base, needed))
}

// Interval returns the push interval an agent should use right now, for the
// initial subscription.
func (h *Hub) Interval() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.intervalLocked(h.opts.Now(), "")
}

// Status reports envID's push stream.
func (h *Hub) Status(envID string) PushStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.envs[envID]
	if !ok {
		return PushStatus{}
	}
	return PushStatus{Active: h.pushFresh(st, h.opts.Now()), LastBatchAt: st.lastPush, Interval: st.interval}
}

// PushActive reports whether envID is currently fed by its agent's pushes.
func (h *Hub) PushActive(envID string) bool {
	return h.Status(envID).Active
}

// Forget drops envID's push state, so a disconnected agent's last batch is not
// served as if it were live and the environment falls back to pulling.
func (h *Hub) Forget(envID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if st, ok := h.envs[envID]; ok {
		st.pushed = nil
		st.session = ""
		st.seq = 0
		st.lastPush = time.Time{}
		st.subscribedAt = time.Time{}
	}
}

func clampInterval(d time.Duration) time.Duration {
	return min(max(d, MinPushInterval), MaxPushInterval)
}

// cancelOnShutdown arranges for cancel to run when the hub shuts down; the returned func detaches it once the pull is done.
func (h *Hub) cancelOnShutdown(cancel context.CancelFunc) func() bool {
	return context.AfterFunc(h.ctx, cancel)
}
