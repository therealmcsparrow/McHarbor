// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
)

// Moving a virtual machine to another host.
//
// McHarbor sequences the two agents and owns the rollback; each agent only ever
// does its own half (see the agent's compute_vm_migrate_linux.go). The rule the
// whole file is written to: until the target runs the guest and McHarbor has
// recorded that, the source's copy is untouched, so every failure before that
// point is undone by removing what the target received and, for a cold move,
// starting the source again if it was running.

// migrationBudget bounds one migration end to end.
const migrationBudget = 7 * time.Hour

// migrationRollbackTimeout bounds each undo step, which runs on a context of its
// own because the migration's may be the reason it is running.
const migrationRollbackTimeout = 2 * time.Minute

// migrationShutdownGrace is how long a cold migration lets the guest shut down
// cleanly before the host escalates.
const migrationShutdownGrace = 120

// Migration states.
const (
	MigrationRunning   = "running"
	MigrationCompleted = "completed"
	MigrationFailed    = "failed"
	MigrationCancelled = "cancelled"
)

var (
	// ErrMigrateSameHost means the target is the guest's own host.
	ErrMigrateSameHost = errors.New("the target is the host the guest is already on")
	// ErrMigrateInFlight means the guest is already being moved.
	ErrMigrateInFlight = errors.New("this guest is already being migrated")
	// ErrMigrateUnsupported means one of the agents cannot do this kind of move.
	ErrMigrateUnsupported = errors.New("an agent involved cannot perform this migration")
	// ErrMigrateSnapshots, ErrMigrateDevices, ErrMigrateShares, ErrMigrateTPM and
	// ErrMigrateMedia name what ties a guest to its host. Each is refused rather
	// than silently dropped, because a guest that arrives without its snapshots
	// or its vTPM is not the guest that left.
	ErrMigrateSnapshots = errors.New("guests with snapshots cannot be migrated; delete or merge them first")
	ErrMigrateDevices   = errors.New("guests with passed-through host devices cannot be migrated")
	ErrMigrateShares    = errors.New("guests with host folder shares cannot be migrated")
	ErrMigrateTPM       = errors.New("guests with a virtual TPM cannot be migrated yet")
	ErrMigrateMedia     = errors.New("eject installer media before migrating")
	// ErrMigrateForbidden means the caller may not create guests on the target.
	ErrMigrateForbidden = errors.New("you may not migrate guests to that host")
	// ErrMigrateAddress means a live target address is not usable.
	ErrMigrateAddress = errors.New("the live migration target address is not usable")
	// ErrMigrationNone means there is no migration to act on.
	ErrMigrationNone = errors.New("this guest is not being migrated")
)

// MigrateRequest is the body of POST /api/vms/{id}/migrate.
type MigrateRequest struct {
	TargetEnvironmentID string `json:"targetEnvironmentId"`
	// The target pool, from addon-compute's pool list via the frontend, as for
	// a create: this plugin cannot read that table.
	TargetPoolID      string `json:"targetPoolId"`
	TargetPoolBackend string `json:"targetPoolBackend"`
	TargetPoolTarget  string `json:"targetPoolTarget"`
	// Live moves a running guest without stopping it. Cold is the default.
	Live bool `json:"live,omitempty"`
	// TargetAddress overrides the address the source dials for a live move;
	// empty picks the first one the target reports.
	TargetAddress  string `json:"targetAddress,omitempty"`
	MaxBandwidthMB int64  `json:"maxBandwidthMb,omitempty"`
	MaxDowntimeMS  int64  `json:"maxDowntimeMs,omitempty"`
	// NICs re-homes interfaces onto the target's networks; a NIC not named
	// keeps its bridge, which the target must then have.
	NICs []MigrateNIC `json:"nics,omitempty"`
}

// MigrateNIC re-homes one interface.
type MigrateNIC struct {
	NICID      string `json:"nicId"`
	NetworkID  string `json:"networkId"`
	Bridge     string `json:"bridge"`
	VLANTag    int    `json:"vlanTag,omitempty"`
	AttachKind string `json:"attachKind,omitempty"`
	ParentLink string `json:"parentLink,omitempty"`
	LinkMode   string `json:"linkMode,omitempty"`
}

// MigrationStatus is what GET /api/vms/{id}/migration answers. It carries no
// token, certificate or key: those exist only in the agents' payloads.
type MigrationStatus struct {
	ID                  string     `json:"id"`
	VMID                string     `json:"vmId"`
	SourceEnvironmentID string     `json:"sourceEnvironmentId"`
	TargetEnvironmentID string     `json:"targetEnvironmentId"`
	Mode                string     `json:"mode"` // cold | live
	State               string     `json:"state"`
	Step                string     `json:"step,omitempty"`
	Percent             int        `json:"percent"`
	Error               string     `json:"error,omitempty"`
	Warning             string     `json:"warning,omitempty"`
	StartedAt           time.Time  `json:"startedAt"`
	FinishedAt          *time.Time `json:"finishedAt,omitempty"`
}

// migrationJob is one migration in flight or just finished.
type migrationJob struct {
	mu     sync.Mutex
	status MigrationStatus
	req    MigrateRequest
	vm     VM
	token  string
	cancel context.CancelFunc
	grants []string
}

func (j *migrationJob) snapshot() MigrationStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

func (j *migrationJob) update(step string, percent int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if step != "" {
		j.status.Step = step
	}
	if percent >= 0 && percent <= 100 {
		j.status.Percent = percent
	}
}

func (j *migrationJob) finish(state, detail, warning string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now().UTC()
	j.status.State, j.status.Error, j.status.Warning, j.status.FinishedAt = state, detail, warning, &now
	if state == MigrationCompleted {
		j.status.Percent = 100
	}
}

// migrationJobs holds one job per guest.
type migrationJobs struct {
	mu   sync.Mutex
	byVM map[string]*migrationJob
}

func newMigrationJobs() *migrationJobs {
	return &migrationJobs{byVM: make(map[string]*migrationJob)}
}

// claim registers a job, refusing a second while one runs.
func (m *migrationJobs) claim(job *migrationJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.byVM[job.status.VMID]; ok && current.snapshot().State == MigrationRunning {
		return ErrMigrateInFlight
	}
	m.byVM[job.status.VMID] = job
	return nil
}

func (m *migrationJobs) get(vmID string) (*migrationJob, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.byVM[vmID]
	return job, ok
}

// migrationState returns the lazily built job and relay registries.
func (s *Service) migrationState() (*migrationJobs, *migrationRelay) {
	s.migrationOnce.Do(func() {
		s.migrationJobs = newMigrationJobs()
		s.relay = newMigrationRelay()
	})
	return s.migrationJobs, s.relay
}

// Migrate validates a move and starts it in the background.
func (s *Service) Migrate(ctx context.Context, vmID string, req MigrateRequest) (MigrationStatus, error) {
	job, err := s.prepareMigration(ctx, vmID, req)
	if err != nil {
		return MigrationStatus{}, err
	}
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationBudget)
	job.mu.Lock()
	job.cancel = cancel
	job.mu.Unlock()
	// goroutine lifecycle: bounded by migrationBudget and ended early by
	// CancelMigration; every agent call inside inherits runCtx.
	go func() {
		defer cancel()
		s.runMigration(runCtx, job)
	}()
	return job.snapshot(), nil
}

// prepareMigration checks everything that can be checked before anything moves,
// and registers the job.
func (s *Service) prepareMigration(ctx context.Context, vmID string, req MigrateRequest) (*migrationJob, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return nil, err
	}
	if req.TargetEnvironmentID != "" && req.TargetPoolID != "" {
		pool, err := s.resolvePool(ctx, req.TargetEnvironmentID,
			PoolRef{ID: req.TargetPoolID, Backend: req.TargetPoolBackend, Target: req.TargetPoolTarget})
		if err != nil {
			return nil, err
		}
		req.TargetPoolBackend, req.TargetPoolTarget = pool.Backend, pool.Target
	}
	if err := s.migrationBlockers(ctx, vm, req); err != nil {
		return nil, err
	}

	token, err := newSecret(32)
	if err != nil {
		return nil, err
	}
	mode := "cold"
	if req.Live {
		mode = "live"
	}
	job := &migrationJob{
		req: req, vm: vm, token: token,
		status: MigrationStatus{
			ID: xid.New().String(), VMID: vm.ID,
			SourceEnvironmentID: vm.EnvironmentID, TargetEnvironmentID: req.TargetEnvironmentID,
			Mode: mode, State: MigrationRunning, Step: "preparing", StartedAt: time.Now().UTC(),
		},
	}
	jobs, _ := s.migrationState()
	if err := jobs.claim(job); err != nil {
		return nil, err
	}
	return job, nil
}

// migrationBlockers is every reason a guest cannot move, checked up front.
func (s *Service) migrationBlockers(ctx context.Context, vm VM, req MigrateRequest) error {
	if req.TargetEnvironmentID == "" {
		return invalid(codeHostRequired, "a target host is required")
	}
	if req.TargetEnvironmentID == vm.EnvironmentID {
		return ErrMigrateSameHost
	}
	if req.TargetPoolID == "" || req.TargetPoolBackend == "" || req.TargetPoolTarget == "" {
		return invalid(codePoolRequired, "a target storage pool is required")
	}
	if vm.IsTemplate {
		if clones, err := s.linkedClones(ctx, vm.ID); err != nil {
			return err
		} else if len(clones) > 0 {
			return fmt.Errorf("%w: %s", ErrTemplateInUse, strings.Join(clones, ", "))
		}
	}
	if len(vm.Snapshots) > 0 {
		return ErrMigrateSnapshots
	}
	if len(vm.Devices) > 0 {
		return ErrMigrateDevices
	}
	if vm.TPMEnabled {
		return ErrMigrateTPM
	}
	for _, disk := range vm.Disks {
		if disk.IsCDROM {
			return ErrMigrateMedia
		}
	}
	if shares, err := s.listShares(ctx, vm.ID); err != nil {
		return err
	} else if len(shares) > 0 {
		return ErrMigrateShares
	}
	if req.Live && !vm.Running() {
		return ErrNotRunning
	}
	if req.TargetAddress != "" && !validMigrationAddress(req.TargetAddress) {
		return ErrMigrateAddress
	}
	for _, env := range []string{vm.EnvironmentID, req.TargetEnvironmentID} {
		if s.rpc == nil {
			return ErrHostUnavailable
		}
		if available, reason := s.rpc.Available(ctx, env); !available {
			return fmt.Errorf("%w: %s", ErrHostUnavailable, reason)
		}
		feature := FeatureMigrateCold
		if req.Live {
			feature = FeatureMigrateLive
		}
		if !s.SupportsFeature(ctx, env, feature) {
			return ErrMigrateUnsupported
		}
	}
	return nil
}

// validMigrationAddress accepts an IP or a plain host name, the only things the
// agent will dial.
func validMigrationAddress(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}
	if len(value) > 253 || strings.HasPrefix(value, "-") || strings.HasPrefix(value, ".") {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
	}
	return value != ""
}

// MigrationStatus returns a guest's current or last migration.
func (s *Service) MigrationStatus(vmID string) (MigrationStatus, bool) {
	jobs, _ := s.migrationState()
	job, ok := jobs.get(vmID)
	if !ok {
		return MigrationStatus{}, false
	}
	return job.snapshot(), true
}

// CancelMigration stops a running migration. The rollback runs as for any
// other failure, so the guest ends up where it started.
func (s *Service) CancelMigration(ctx context.Context, vmID string) error {
	jobs, relay := s.migrationState()
	job, ok := jobs.get(vmID)
	if !ok || job.snapshot().State != MigrationRunning {
		return ErrMigrationNone
	}
	job.mu.Lock()
	grants := append([]string(nil), job.grants...)
	stop := job.cancel
	job.mu.Unlock()
	if stop == nil {
		return ErrMigrationNone
	}
	for _, grant := range grants {
		relay.revoke(grant)
	}
	if job.req.Live {
		// The agent registered the live send under this key; cancelling it
		// aborts the mirror and the RAM transfer and leaves the source running.
		if _, err := s.call(ctx, job.vm.EnvironmentID, msgVMCancel, agentPayload{VMID: "migrate-" + vmID}); err != nil {
			s.logger.Warn("kvm: asking the source to stop a live migration", "vm", vmID, "error", err)
		}
	}
	stop()
	return nil
}

// recordMigrationProgress applies a progress event from either agent.
func (s *Service) recordMigrationProgress(vmID string, detail map[string]string) {
	jobs, _ := s.migrationState()
	job, ok := jobs.get(vmID)
	if !ok {
		return
	}
	percent := -1
	if value, err := strconv.Atoi(detail["percent"]); err == nil {
		percent = value
	}
	job.update(detail["step"], percent)
}

// runMigration drives one job to an end state.
func (s *Service) runMigration(ctx context.Context, job *migrationJob) {
	var err error
	var warning string
	if job.req.Live {
		warning, err = s.runLiveMigration(ctx, job)
	} else {
		warning, err = s.runColdMigration(ctx, job)
	}
	switch {
	case err == nil:
		job.finish(MigrationCompleted, "", warning)
		s.recordAudit("plugin.kvm.vm.migration_completed", map[string]any{
			"vmId": job.vm.ID, "migrationId": job.status.ID,
			"from": job.vm.EnvironmentID, "to": job.req.TargetEnvironmentID, "mode": job.status.Mode,
		})
	case errors.Is(err, context.Canceled):
		job.finish(MigrationCancelled, "", warning)
	default:
		s.logger.Error("kvm: migration failed", "vm", job.vm.ID, "migration", job.status.ID, "error", err)
		job.finish(MigrationFailed, hostErrorDetail(err), warning)
		s.recordAudit("plugin.kvm.vm.migration_failed", map[string]any{
			"vmId": job.vm.ID, "migrationId": job.status.ID, "mode": job.status.Mode,
		})
	}
}

// rollbackCtx is a fresh bounded context for an undo step.
func rollbackCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), migrationRollbackTimeout)
}

// relayDisk issues one relay grant and records it on the job for cancellation.
func (s *Service) relayDisk(job *migrationJob) (relayGrant, error) {
	_, relay := s.migrationState()
	grant, err := relay.issue()
	if err != nil {
		return relayGrant{}, err
	}
	job.mu.Lock()
	job.grants = append(job.grants, grant.id)
	job.mu.Unlock()
	return grant, nil
}

// releaseGrants closes every relay slot the job opened.
func (s *Service) releaseGrants(job *migrationJob) {
	_, relay := s.migrationState()
	job.mu.Lock()
	grants := job.grants
	job.grants = nil
	job.mu.Unlock()
	for _, grant := range grants {
		relay.revoke(grant)
	}
}

// transferPlan is the relay traffic for a cold move or a live NVRAM pre-copy.
type transferPlan struct {
	send    []agentMigrateDisk
	receive []agentMigrateDisk
}

// planTransfer issues a grant per disk, plus the UEFI variable store.
func (s *Service) planTransfer(job *migrationJob, includeDisks bool) (transferPlan, error) {
	var plan transferPlan
	add := func(disk agentMigrateDisk) error {
		grant, err := s.relayDisk(job)
		if err != nil {
			return err
		}
		url := migrationRelayPath + grant.id
		send, receive := disk, disk
		send.URL, send.Token = url, grant.sendToken
		receive.URL, receive.Token = url, grant.receiveToken
		receive.SourcePath = ""
		receive.PoolID, receive.PoolBackend, receive.PoolTarget =
			job.req.TargetPoolID, job.req.TargetPoolBackend, job.req.TargetPoolTarget
		plan.send = append(plan.send, send)
		plan.receive = append(plan.receive, receive)
		return nil
	}
	if includeDisks {
		for _, disk := range job.vm.Disks {
			if err := add(agentMigrateDisk{
				DiskID: disk.ID, Slot: disk.Slot, SourcePath: disk.VolumePath,
				Format: disk.Format, SizeBytes: disk.SizeBytes,
			}); err != nil {
				return plan, err
			}
		}
	}
	if job.vm.Firmware == "uefi" {
		if err := add(agentMigrateDisk{DiskID: migrateNVRAMDiskID, Slot: -1, Format: "raw"}); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

// transfer runs the send and receive halves together and waits for both. If
// either fails the other is cancelled, and the receive side's leftovers are the
// caller's to abort.
func (s *Service) transfer(ctx context.Context, job *migrationJob, migrationID string, plan transferPlan, targetSpec *agentSpec) ([]agentMigrateDisk, error) {
	transferCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var sendErr, receiveErr error
	var landed []agentMigrateDisk
	wg.Add(2)
	// goroutine lifecycle: both halves end with their agent call, which
	// transferCtx bounds; the first failure cancels the other.
	go func() {
		defer wg.Done()
		_, sendErr = s.call(transferCtx, job.vm.EnvironmentID, msgVMMigrate, agentPayload{
			VMID: job.vm.ID, Spec: s.buildSpec(ctx, job.vm),
			Migrate: &agentMigrate{Phase: migratePhaseSend, MigrationID: migrationID, Disks: plan.send},
		})
		if sendErr != nil {
			cancel()
			s.releaseGrants(job)
		}
	}()
	go func() {
		defer wg.Done()
		reply, err := s.call(transferCtx, job.req.TargetEnvironmentID, msgVMMigrate, agentPayload{
			VMID: job.vm.ID, Spec: targetSpec,
			Migrate: &agentMigrate{Phase: migratePhaseReceive, MigrationID: migrationID,
				Token: job.token, Disks: plan.receive},
		})
		receiveErr = err
		if err != nil {
			cancel()
			s.releaseGrants(job)
			return
		}
		if reply.Migrate != nil {
			landed = reply.Migrate.Disks
		}
	}()
	wg.Wait()
	if err := errors.Join(sendErr, receiveErr); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return landed, nil
}

// targetSpec is the guest as the target will run it: the target's pool, the
// paths the target reported, and the re-homed interfaces.
func (s *Service) targetSpec(ctx context.Context, job *migrationJob, landed []agentMigrateDisk) *agentSpec {
	spec := s.buildSpec(ctx, job.vm)
	byDisk := make(map[string]agentMigrateDisk, len(landed))
	for _, disk := range landed {
		byDisk[disk.DiskID] = disk
	}
	for i := range spec.Disks {
		disk := &spec.Disks[i]
		disk.PoolID, disk.PoolBackend, disk.PoolTarget =
			job.req.TargetPoolID, job.req.TargetPoolBackend, job.req.TargetPoolTarget
		disk.BackingPath = ""
		if arrived, ok := byDisk[disk.DiskID]; ok {
			disk.VolumePath = arrived.VolumePath
			if arrived.TargetFormat != "" {
				disk.Format = arrived.TargetFormat
			}
		}
	}
	nics := make(map[string]MigrateNIC, len(job.req.NICs))
	for _, nic := range job.req.NICs {
		nics[nic.NICID] = nic
	}
	for i := range spec.NICs {
		nic := &spec.NICs[i]
		if to, ok := nics[nic.NICID]; ok {
			nic.NetworkID, nic.Bridge, nic.VLANTag = to.NetworkID, to.Bridge, to.VLANTag
			nic.AttachKind, nic.ParentLink, nic.LinkMode = to.AttachKind, to.ParentLink, to.LinkMode
		}
		// The slot the source's QEMU chose means nothing to the target's.
		nic.TapName = ""
	}
	return spec
}

// abortTarget undoes a target half: its definition, then what it received.
func (s *Service) abortTarget(job *migrationJob, migrationID string, defined bool) {
	ctx, cancel := rollbackCtx()
	defer cancel()
	if defined {
		if _, err := s.call(ctx, job.req.TargetEnvironmentID, msgVMDelete, agentPayload{VMID: job.vm.ID}); err != nil {
			s.logger.Warn("kvm: migration rollback could not remove the target's definition",
				"vm", job.vm.ID, "error", err)
		}
	}
	if _, err := s.call(ctx, job.req.TargetEnvironmentID, msgVMMigrate, agentPayload{
		VMID:    job.vm.ID,
		Migrate: &agentMigrate{Phase: migratePhaseAbort, MigrationID: migrationID, Token: job.token},
	}); err != nil {
		s.logger.Warn("kvm: migration rollback could not remove what the target received",
			"vm", job.vm.ID, "migration", migrationID, "error", err)
	}
}

// restartSource starts the guest where it was, after a cold move failed.
func (s *Service) restartSource(job *migrationJob) {
	ctx, cancel := rollbackCtx()
	defer cancel()
	if err := s.Start(ctx, job.vm.ID); err != nil {
		s.logger.Error("kvm: migration rollback could not restart the guest on its own host",
			"vm", job.vm.ID, "error", err)
	}
}

// runColdMigration stops the guest, moves its disks through the relay, and
// brings it up on the target.
func (s *Service) runColdMigration(ctx context.Context, job *migrationJob) (string, error) {
	wasRunning := job.vm.Running()
	migrationID := job.status.ID
	defer s.releaseGrants(job)

	if wasRunning {
		job.update("stopping", 0)
		if _, err := s.Shutdown(ctx, job.vm.ID, migrationShutdownGrace); err != nil {
			return "", fmt.Errorf("stopping the guest before moving it: %w", err)
		}
	}
	if err := s.setState(ctx, job.vm.ID, StateMigrating, ""); err != nil {
		s.logger.Warn("kvm: marking a guest migrating", "vm", job.vm.ID, "error", err)
	}
	fail := func(err error, defined bool) (string, error) {
		s.abortTarget(job, migrationID, defined)
		stateCtx, cancel := rollbackCtx()
		defer cancel()
		if stateErr := s.setState(stateCtx, job.vm.ID, StateStopped, ""); stateErr != nil {
			s.logger.Warn("kvm: restoring a guest's state after a failed migration", "vm", job.vm.ID, "error", stateErr)
		}
		if wasRunning {
			s.restartSource(job)
		}
		return "", err
	}

	plan, err := s.planTransfer(job, true)
	if err != nil {
		return fail(err, false)
	}
	job.update("transferring", 5)
	landed, err := s.transfer(ctx, job, migrationID, plan, s.targetSpec(ctx, job, nil))
	if err != nil {
		return fail(err, false)
	}

	spec := s.targetSpec(ctx, job, landed)
	job.update("defining", 90)
	if _, err := s.call(ctx, job.req.TargetEnvironmentID, msgVMDefine, agentPayload{VMID: job.vm.ID, Spec: spec}); err != nil {
		return fail(err, true)
	}
	if wasRunning {
		job.update("starting", 95)
		if _, err := s.call(ctx, job.req.TargetEnvironmentID, msgVMStart, agentPayload{VMID: job.vm.ID, Spec: spec}); err != nil {
			return fail(err, true)
		}
	}
	state := StateStopped
	if wasRunning {
		state = StateRunning
	}
	if err := s.commitMigration(ctx, job, spec, state); err != nil {
		return fail(err, true)
	}
	// The target owns the guest now; clear its record of the transfer so a
	// later abort can never remove the volumes the guest runs on.
	s.forgetTarget(job, migrationID)
	return s.cleanupSource(job), nil
}

// forgetTarget tells the target the migration is committed.
func (s *Service) forgetTarget(job *migrationJob, migrationID string) {
	ctx, cancel := rollbackCtx()
	defer cancel()
	if _, err := s.call(ctx, job.req.TargetEnvironmentID, msgVMMigrate, agentPayload{
		VMID:    job.vm.ID,
		Migrate: &agentMigrate{Phase: migratePhaseLiveFinish, MigrationID: migrationID, Token: job.token},
	}); err != nil {
		s.logger.Warn("kvm: the target did not confirm the committed migration", "vm", job.vm.ID, "error", err)
	}
}

// cleanupSource removes the guest and its disks from the host it left. A
// failure here is a warning, not a failed migration: the guest runs on the
// target and its record points there.
func (s *Service) cleanupSource(job *migrationJob) string {
	ctx, cancel := rollbackCtx()
	defer cancel()
	if _, err := s.call(ctx, job.vm.EnvironmentID, msgVMDelete, agentPayload{
		VMID: job.vm.ID, Spec: s.buildSpec(ctx, job.vm), DeleteVolumes: true,
	}); err != nil {
		s.logger.Warn("kvm: the migrated guest's old copy was not removed", "vm", job.vm.ID, "error", err)
		return "The guest now runs on the new host, but its old disks could not be removed from the previous one: " +
			hostErrorDetail(err)
	}
	return ""
}

// commitMigration points the guest's rows at the target.
func (s *Service) commitMigration(ctx context.Context, job *migrationJob, spec *agentSpec, state string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_vms SET environment_id = ?, state = ?, updated_at = ? WHERE id = ?`,
		job.req.TargetEnvironmentID, state, now, job.vm.ID); err != nil {
		return fmt.Errorf("recording the guest's new host: %w", err)
	}
	for _, disk := range spec.Disks {
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_kvm_disks
			   SET pool_id = ?, pool_backend = ?, pool_target = ?, volume_path = ?, format = ?,
			       backing_path = NULL, updated_at = ?
			 WHERE id = ? AND vm_id = ?`,
			disk.PoolID, disk.PoolBackend, disk.PoolTarget, disk.VolumePath, disk.Format, now,
			disk.DiskID, job.vm.ID); err != nil {
			return fmt.Errorf("recording a disk's new location: %w", err)
		}
	}
	for _, nic := range job.req.NICs {
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_kvm_nics
			   SET network_id = ?, bridge = ?, vlan_tag = ?, attach_kind = ?, parent_link = ?,
			       link_mode = ?, updated_at = ?
			 WHERE id = ? AND vm_id = ?`,
			nic.NetworkID, nic.Bridge, nullIfZero(nic.VLANTag), nullIfEmpty(nic.AttachKind),
			nullIfEmpty(nic.ParentLink), nullIfEmpty(nic.LinkMode), now, nic.NICID, job.vm.ID); err != nil {
			return fmt.Errorf("recording an interface's new network: %w", err)
		}
	}
	return nil
}

// runLiveMigration mirrors the disks and moves the RAM while the guest runs.
func (s *Service) runLiveMigration(ctx context.Context, job *migrationJob) (string, error) {
	migrationID := job.status.ID
	defer s.releaseGrants(job)

	job.update("checking", 0)
	source, err := s.call(ctx, job.vm.EnvironmentID, msgVMMigrate, agentPayload{
		VMID: job.vm.ID, Migrate: &agentMigrate{Phase: migratePhaseLiveCheck},
	})
	if err != nil {
		return "", err
	}
	target, err := s.call(ctx, job.req.TargetEnvironmentID, msgVMMigrate, agentPayload{
		VMID: job.vm.ID, Migrate: &agentMigrate{Phase: migratePhaseLiveCheck},
	})
	if err != nil {
		return "", err
	}
	address := job.req.TargetAddress
	if address == "" && target.Migrate != nil && len(target.Migrate.ListenAddrs) > 0 {
		address = target.Migrate.ListenAddrs[0]
	}
	if !validMigrationAddress(address) {
		return "", ErrMigrateAddress
	}
	spice := source.Migrate != nil && source.Migrate.SPICE
	// The target builds the source's device model, hot-plug ports or not; a
	// source booted before the ports existed reports none.
	hotplugLayout := source.Migrate != nil && source.Migrate.HotplugLayout
	if spice && !s.SupportsFeature(ctx, job.req.TargetEnvironmentID, FeatureSPICE) {
		return "", fmt.Errorf("%w: the guest runs with SPICE and the target's QEMU has none", ErrMigrateUnsupported)
	}

	material, err := mintMigrationTLS(address, time.Now())
	if err != nil {
		return "", err
	}

	// The UEFI variable store is a file beside the guest, not a disk QEMU
	// mirrors, so it crosses ahead of the switchover through the relay.
	if job.vm.Firmware == "uefi" {
		job.update("copying firmware variables", 2)
		plan, err := s.planTransfer(job, false)
		if err != nil {
			return "", err
		}
		nvramID := migrationID + "nv"
		if _, err := s.transfer(ctx, job, nvramID, plan, s.buildSpec(ctx, job.vm)); err != nil {
			s.abortTarget(job, nvramID, false)
			return "", err
		}
		s.forgetTarget(job, nvramID)
	}

	var disks []agentMigrateDisk
	var sends []agentMigrateDisk
	for _, disk := range job.vm.Disks {
		disks = append(disks, agentMigrateDisk{
			DiskID: disk.ID, Slot: disk.Slot, Format: "qcow2", SizeBytes: disk.SizeBytes,
			PoolID: job.req.TargetPoolID, PoolBackend: job.req.TargetPoolBackend, PoolTarget: job.req.TargetPoolTarget,
		})
		sends = append(sends, agentMigrateDisk{DiskID: disk.ID, Slot: disk.Slot, SourcePath: disk.VolumePath})
	}

	job.update("preparing the target", 5)
	prepared, err := s.call(ctx, job.req.TargetEnvironmentID, msgVMMigrate, agentPayload{
		VMID: job.vm.ID, Spec: s.targetSpec(ctx, job, nil),
		Migrate: &agentMigrate{
			Phase: migratePhaseLivePrepare, MigrationID: migrationID, Token: job.token,
			Disks: disks, TLS: &material.server, SPICE: spice, HotplugLayout: hotplugLayout,
		},
	})
	if err != nil || prepared.Migrate == nil {
		s.abortTarget(job, migrationID, true)
		if err == nil {
			err = errors.New("the target did not describe what it prepared")
		}
		return "", err
	}

	job.update("mirroring", 10)
	if _, err := s.call(ctx, job.vm.EnvironmentID, msgVMMigrate, agentPayload{
		VMID: job.vm.ID, Spec: s.buildSpec(ctx, job.vm),
		Migrate: &agentMigrate{
			Phase: migratePhaseLiveSend, MigrationID: migrationID, Live: true,
			TargetHost: address, TargetPort: prepared.Migrate.TargetPort, NBDPort: prepared.Migrate.NBDPort,
			TLS: &material.client, Disks: sends,
			MaxBandwidthMB: job.req.MaxBandwidthMB, MaxDowntimeMS: job.req.MaxDowntimeMS,
		},
	}); err != nil {
		// The source agent cancelled its mirror and migration and kept the
		// guest running; the target's waiting QEMU and volumes go.
		s.abortTarget(job, migrationID, true)
		return "", err
	}

	// From here the guest runs on the target and nothing is rolled back: the
	// record is moved first, so a later failure cannot leave it pointing at a
	// host the guest has left.
	spec := s.targetSpec(ctx, job, prepared.Migrate.Disks)
	commitCtx, cancel := rollbackCtx()
	defer cancel()
	if err := s.commitMigration(commitCtx, job, spec, StateRunning); err != nil {
		return "", fmt.Errorf("the guest moved but its record could not be updated: %w", err)
	}
	job.update("resuming", 97)
	var warnings []string
	if _, err := s.call(commitCtx, job.req.TargetEnvironmentID, msgVMMigrate, agentPayload{
		VMID:    job.vm.ID,
		Migrate: &agentMigrate{Phase: migratePhaseLiveFinish, MigrationID: migrationID, Token: job.token},
	}); err != nil {
		warnings = append(warnings, "The target did not confirm the guest resumed: "+hostErrorDetail(err))
	}
	if warning := s.cleanupSource(job); warning != "" {
		warnings = append(warnings, warning)
	}
	return strings.Join(warnings, " "), nil
}
