// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Moving a pool's data to a new target.
//
// Changing a pool's target only decides where the *next* volume is written.
// Everything already in it stays where it is, and every row naming those files
// keeps pointing at the old path — which is how an operator moves a pool to a
// new array and finds nothing has moved.
//
// The move runs on the host, because that is where the files are. What runs
// here is the orchestration nobody else can do: work out what changes, stop the
// guests, keep a job the UI can watch, and rewrite the pool row once the host
// reports it is done. Disk rows are rewritten by the plugin that owns them,
// from the guest events the host emits as it moves each file.
//
// Only a `dir` pool moves this way. A ZFS dataset or an LVM volume group is not
// a path that relocates by copying files.

// Job states.
const (
	migrationIdle    = "idle"
	migrationRunning = "running"
	migrationDone    = "done"
	migrationFailed  = "failed"
)

// migrationTimeout bounds the whole job. Long, because the copy underneath it
// is bounded by the disk rather than by anything McHarbor decides.
const migrationTimeout = 12 * time.Hour

// migrationGraceSeconds is the ACPI window a guest gets before the move takes
// its disk. The same figure the Stop button defaults to.
const migrationGraceSeconds = 60

// Guest lifecycle messages this plugin borrows. Their meaning belongs to the
// guest plugins; the two used here are the ones a move cannot avoid.
const (
	msgVMQuery    = "vm_query"
	msgVMShutdown = "vm_shutdown"
)

// PoolMovePlan is what moving a pool would involve, answered before anything
// moves so the operator confirms a real number.
type PoolMovePlan struct {
	PoolID  string `json:"poolId"`
	From    string `json:"from"`
	To      string `json:"to"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
	// RunningGuests must be stopped first. Listed rather than counted, because
	// "2 guests" is not something an operator can consent to.
	RunningGuests []string `json:"runningGuests"`
}

// PoolMoveStatus is a job as the UI reads it.
type PoolMoveStatus struct {
	PoolID     string     `json:"poolId"`
	State      string     `json:"state"`
	Percent    int        `json:"percent"`
	BytesMoved int64      `json:"bytesMoved"`
	BytesTotal int64      `json:"bytesTotal"`
	Moved      int        `json:"moved"`
	Failures   []string   `json:"failures,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	// StoppedGuests are the guests shut down to make the move safe. They are
	// not started again here: a start needs the guest's full definition, which
	// belongs to the plugin that owns the guest, so the operator starts them
	// from the machine list and the UI says so.
	StoppedGuests []string `json:"stoppedGuests,omitempty"`
}

// migrationJobs holds the in-flight and last-finished job per pool.
//
// In memory: a move cannot outlive the process blocked on the agent's reply
// anyway, and a restart mid-move is reported by the host's own state on the
// next plan rather than by a stale row here.
type migrationJobs struct {
	mu   sync.RWMutex
	jobs map[string]*PoolMoveStatus
}

func newMigrationJobs() *migrationJobs {
	return &migrationJobs{jobs: make(map[string]*PoolMoveStatus)}
}

func (m *migrationJobs) get(poolID string) (PoolMoveStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[poolID]
	if !ok {
		return PoolMoveStatus{PoolID: poolID, State: migrationIdle}, false
	}
	return *job, true
}

func (m *migrationJobs) start(poolID string, total int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[poolID]; ok && job.State == migrationRunning {
		return fmt.Errorf("a move is already running for this pool")
	}
	now := time.Now().UTC()
	m.jobs[poolID] = &PoolMoveStatus{
		PoolID: poolID, State: migrationRunning, BytesTotal: total, StartedAt: &now,
	}
	return nil
}

func (m *migrationJobs) update(poolID string, apply func(*PoolMoveStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[poolID]; ok {
		apply(job)
	}
}

// PlanPoolMove reports what moving this pool to target would involve.
func (s *Service) PlanPoolMove(ctx context.Context, poolID, target string) (PoolMovePlan, error) {
	pool, to, err := s.movablePool(ctx, poolID, target)
	if err != nil {
		return PoolMovePlan{}, err
	}
	plan := PoolMovePlan{
		PoolID: pool.ID, From: pool.Target, To: to, RunningGuests: []string{},
	}
	if pool.Target == to {
		return plan, nil
	}

	result, err := s.rpc.MigrateRoots(ctx, pool.EnvironmentID, MigrationRequest{
		Plan: true, DiskFrom: pool.Target, DiskTo: to,
	})
	if err != nil {
		return plan, err
	}
	plan.Entries = result.Entries
	plan.Bytes = result.Bytes
	plan.RunningGuests = s.runningGuests(ctx, pool.EnvironmentID)
	return plan, nil
}

// StartPoolMove stops what has to stop and moves the pool's data.
//
// Returns as soon as the job is registered: the move itself is hours of copy on
// the far side, and the operator watches it through the status endpoint rather
// than through an HTTP request nothing would keep open.
func (s *Service) StartPoolMove(ctx context.Context, poolID, target string) (PoolMoveStatus, error) {
	plan, err := s.PlanPoolMove(ctx, poolID, target)
	if err != nil {
		return PoolMoveStatus{}, err
	}
	if plan.From == plan.To {
		return PoolMoveStatus{PoolID: poolID, State: migrationIdle}, nil
	}
	if err := s.migrations.start(poolID, plan.Bytes); err != nil {
		return PoolMoveStatus{}, err
	}

	go s.runPoolMove(poolID, plan)
	status, _ := s.migrations.get(poolID)
	return status, nil
}

// runPoolMove is the job body. Every exit updates the status, because a job
// that stops reporting is indistinguishable from one that is still copying.
func (s *Service) runPoolMove(poolID string, plan PoolMovePlan) {
	ctx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
	defer cancel()

	fail := func(err error) {
		now := time.Now().UTC()
		s.migrations.update(poolID, func(job *PoolMoveStatus) {
			job.State = migrationFailed
			job.Error = err.Error()
			job.FinishedAt = &now
		})
		s.logger.Error("compute: moving a pool failed", "pool", poolID, "error", err)
		// The request that started the move returned as soon as the job was
		// queued, so its audit row says only that a relocation began. A move
		// that stopped half-way can leave disks split across two locations,
		// which is the state worth being able to find later.
		s.recordAudit("plugin.compute.pool.move_failed", map[string]any{
			"poolId": poolID, "from": plan.From, "to": plan.To, "error": err.Error(),
		})
	}

	pool, err := s.GetPool(ctx, poolID)
	if err != nil {
		fail(err)
		return
	}

	stopped, err := s.stopGuestsForMove(ctx, pool.EnvironmentID)
	if err != nil {
		fail(err)
		return
	}

	// Progress arrives on the transport while the call below is blocked, so
	// the watcher is what the UI reads until the reply lands.
	watchCtx, stopWatch := context.WithCancel(ctx)
	go s.watchMoveProgress(watchCtx, pool.EnvironmentID, poolID)

	result, err := s.rpc.MigrateRoots(ctx, pool.EnvironmentID, MigrationRequest{
		DiskFrom: plan.From, DiskTo: plan.To,
	})
	stopWatch()
	if err != nil {
		fail(err)
		return
	}

	// The pool row last: a pool still pointing at the old target would
	// allocate the next volume back where the data just left.
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_pools SET target = ?, updated_at = ?
		 WHERE id = ?`, plan.To, time.Now().UTC().Format(time.RFC3339), poolID); err != nil {
		fail(fmt.Errorf("recording the pool's new target: %w", err))
		return
	}

	now := time.Now().UTC()
	failures := make([]string, 0, len(result.Failures))
	for _, failure := range result.Failures {
		failures = append(failures, failure.Path+": "+failure.Reason)
	}
	s.migrations.update(poolID, func(job *PoolMoveStatus) {
		job.State = migrationDone
		job.Percent = 100
		job.BytesMoved = job.BytesTotal
		job.Moved = len(result.Moved)
		job.Failures = failures
		job.StoppedGuests = stopped
		job.FinishedAt = &now
	})
	s.recordAudit("plugin.compute.pool.moved", map[string]any{
		"poolId": poolID, "from": plan.From, "to": plan.To,
		"environmentId": pool.EnvironmentID, "moved": len(result.Moved),
		"failures": len(failures), "stoppedGuests": len(stopped),
	})
}

// RepairPool fixes what a move leaves behind inside the files themselves.
//
// A qcow2 names its backing file by path and a snapshot manifest names every
// frozen disk the same way, so a pool whose files moved can hold guests that no
// longer open. Nothing is copied here and no guest is stopped, which makes it
// the recovery path — including for a pool someone moved by hand.
func (s *Service) RepairPool(ctx context.Context, poolID string) (MigrationResult, error) {
	pool, _, err := s.movablePool(ctx, poolID, "")
	if err != nil {
		return MigrationResult{}, err
	}
	return s.rpc.MigrateRoots(ctx, pool.EnvironmentID, MigrationRequest{
		Repair: true, DiskTo: pool.Target,
	})
}

// movablePool loads a pool and checks that moving it can be asked for at all.
func (s *Service) movablePool(ctx context.Context, poolID, target string) (Pool, string, error) {
	if s.rpc == nil {
		return Pool{}, "", ErrNotAgentEnv
	}
	pool, err := s.GetPool(ctx, poolID)
	if err != nil {
		return Pool{}, "", err
	}
	if pool.Backend != BackendDir {
		return Pool{}, "", fmt.Errorf("%w: only a directory pool moves by copying files", ErrInvalidTarget)
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return pool, pool.Target, nil
	}
	if !strings.HasPrefix(target, "/") {
		return Pool{}, "", fmt.Errorf("%w: %s is not an absolute path", ErrInvalidTarget, target)
	}
	return pool, target, nil
}

// watchMoveProgress copies what the host reports into the job.
func (s *Service) watchMoveProgress(ctx context.Context, envID, poolID string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			progress, ok := s.rpc.MigrationProgress(envID)
			if !ok {
				continue
			}
			s.migrations.update(poolID, func(job *PoolMoveStatus) {
				job.Percent = progress.Percent
				job.BytesMoved = progress.Bytes
			})
		}
	}
}

// runningGuests asks the host which guests are up.
//
// A raw vm_query rather than anything richer: guest lifecycle belongs to the
// plugins that own guests, and this one needs exactly two facts about them —
// which are running, and that they stop when asked.
func (s *Service) runningGuests(ctx context.Context, envID string) []string {
	raw, err := s.rpc.send(ctx, envID, msgVMQuery, []byte(`{}`))
	if err != nil {
		return nil
	}
	var reply agentVMStates
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil
	}
	out := make([]string, 0, 4)
	for _, state := range reply.States {
		if strings.EqualFold(state.State, "running") || strings.EqualFold(state.State, "paused") {
			out = append(out, state.VMID)
		}
	}
	return out
}

// stopGuestsForMove shuts down everything that is up, and reports what it
// stopped so the operator knows what to start again.
//
// The host refuses to move a running guest's disk, which is the backstop; this
// is the part that means the operator does not have to do it by hand first.
func (s *Service) stopGuestsForMove(ctx context.Context, envID string) ([]string, error) {
	running := s.runningGuests(ctx, envID)
	stopped := make([]string, 0, len(running))
	for _, guestID := range running {
		payload, err := json.Marshal(map[string]any{
			"vmId": guestID, "graceSeconds": migrationGraceSeconds,
		})
		if err != nil {
			return stopped, err
		}
		if _, err := s.rpc.send(ctx, envID, msgVMShutdown, payload); err != nil {
			return stopped, fmt.Errorf("stopping %s before the move: %w", guestID, err)
		}
		stopped = append(stopped, guestID)
	}
	return stopped, nil
}
