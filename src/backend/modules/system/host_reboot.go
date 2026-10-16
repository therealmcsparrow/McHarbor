// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/swarm"
	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

const (
	// rebootTick is how often the reboot scheduler looks for due, running and
	// returning reboots. A reboot asked for "now" does not wait for it: the
	// scheduler is woken directly.
	rebootTick = 15 * time.Second
	// rebootRunTimeout bounds one whole execution, both stop phases included.
	rebootRunTimeout = time.Hour
	// rebootNotBackAfter is how long a host may stay unreachable after the
	// reboot was issued before the row is failed.
	rebootNotBackAfter = 45 * time.Minute
	// rebootNoRestartAfter is how long a host may stay reachable on the same boot
	// before McHarbor concludes the reboot never happened.
	rebootNoRestartAfter = 20 * time.Minute
	// rebootStaleAfter is how long a stopping row may go without an update
	// before it is treated as abandoned by a McHarbor that restarted mid-run.
	rebootStaleAfter = 30 * time.Minute
	// rebootMaxAhead bounds how far out a reboot may be scheduled.
	rebootMaxAhead = 90 * 24 * time.Hour
)

// HostRebootState is what the Updates tab renders: the target's reboot row, and
// why a new one cannot be scheduled at all when that is the case.
type HostRebootState struct {
	Reboot        *HostReboot `json:"reboot,omitempty"`
	BlockedReason string      `json:"blockedReason,omitempty"`
}

// HostRebootRequest schedules a reboot now or at a moment, never on a cron: a
// reboot is a one-off decision taken after one set of updates.
type HostRebootRequest struct {
	Confirm           bool       `json:"confirm"`
	Now               bool       `json:"now"`
	ScheduledAt       *time.Time `json:"scheduledAt,omitempty"`
	DrainedExternally bool       `json:"drainedExternally"`
}

// HostRebootInventory is what McHarbor could detect on the host, so the reboot
// dialog can say what will be stopped instead of only listing possibilities.
type HostRebootInventory struct {
	McHarborHost        bool `json:"mcharborHost"`
	ContainersKnown     bool `json:"containersKnown"`
	Containers          int  `json:"containers"`
	ContainersReturning int  `json:"containersReturning"`
	GuestsKnown         bool `json:"guestsKnown"`
	// Guests counts the running guests every reboot-drain plugin reported, whatever their kind.
	Guests          int      `json:"guests"`
	GuestBlockers   []string `json:"guestBlockers,omitempty"`
	Swarm           string   `json:"swarm,omitempty"`
	SwarmTasks      int      `json:"swarmTasks"`
	KubernetesKnown bool     `json:"kubernetesKnown"`
	Kubernetes      []string `json:"kubernetes,omitempty"`
}

// rebootHosts is every workload surface of one host the reboot acts on. Any of
// them may be nil when the host has none or it could not be reached.
type rebootHosts struct {
	containers   rebootContainerHost
	guests       rebootGuestHost
	swarm        rebootSwarm
	mcharborHost bool
	close        func()
}

// RebootService schedules host reboots, stops the host's workloads gracefully
// before issuing one, and brings back what a normal boot would have afterwards.
type RebootService struct {
	svc    *Service
	store  *rebootStore
	audit  *audit.Logger
	logger *slog.Logger
	budget rebootBudget

	// hostsFor and runCommand are the seams tests replace; in production they
	// reach Docker, the agent, and the privileged utility container.
	hostsFor   func(ctx context.Context, target HostTarget, actor string) (*rebootHosts, error)
	runCommand func(ctx context.Context, target HostTarget, script string, readWrite bool, timeout time.Duration) (*commandResult, error)

	// plugins and drains are how guests are reached: the live plugin router, and the
	// reboot-drain declarations of the loaded plugins. Either nil means no guests.
	plugins pluginRouteServer
	drains  func() []plugincaps.Capability

	wake   chan struct{}
	mu     sync.Mutex
	active map[string]bool
	// verified throttles the post-reboot check, which starts a utility
	// container on the host each time, to once a minute per target.
	verified map[string]time.Time
	wg       sync.WaitGroup
}

// NewRebootService wires the reboot service to the system service, which owns
// the Docker pool and the host command runner.
func NewRebootService(svc *Service, database *sql.DB, auditLog *audit.Logger, logger *slog.Logger) *RebootService {
	s := &RebootService{
		svc:    svc,
		store:  &rebootStore{database: database},
		audit:  auditLog,
		logger: logger,
		budget: defaultRebootBudget,
		wake:   make(chan struct{}, 1),
		active: map[string]bool{},

		verified: map[string]time.Time{},
	}
	s.hostsFor = s.realHosts
	s.runCommand = svc.runRebootCommand
	return s
}

// resolveTarget turns the request's environment into the stored id, so the
// "default" alias and the real id share one reboot row.
func (s *RebootService) resolveTarget(ctx context.Context, target HostTarget) (HostTarget, error) {
	if target.Self {
		return target, nil
	}
	envID, err := s.svc.pool.ResolveEnvironmentIDContext(ctx, target.EnvID)
	if err != nil {
		return target, errRebootTargetMissing
	}
	var found int
	if err := s.store.database.QueryRowContext(ctx, `SELECT 1 FROM environments WHERE id = ?`, envID).Scan(&found); err != nil {
		return target, errRebootTargetMissing
	}
	return EnvTarget(envID), nil
}

// State returns the target's reboot row and whether a new one may be scheduled.
func (s *RebootService) State(ctx context.Context, target HostTarget) (*HostRebootState, error) {
	target, err := s.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	rec, err := s.store.get(ctx, target.rowID())
	if err != nil {
		return nil, err
	}
	state := &HostRebootState{Reboot: rec}
	if demo.Active() {
		state.BlockedReason = RebootReasonDemoMode
	}
	return state, nil
}

// Schedule stores a reboot for the target, now or at req.ScheduledAt.
func (s *RebootService) Schedule(ctx context.Context, target HostTarget, req HostRebootRequest, userID, username string) (*HostReboot, error) {
	if demo.Active() {
		return nil, refuse(RebootReasonDemoMode, "demo mode never reboots a host")
	}
	if !req.Confirm {
		return nil, refuse(RebootReasonNotConfirmed, "the reboot was not confirmed")
	}
	at, err := rebootTime(req, time.Now())
	if err != nil {
		return nil, err
	}
	target, err = s.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	rec, err := s.store.schedule(ctx, target.rowID(), at, req.DrainedExternally, userID, username)
	if err != nil {
		return nil, err
	}
	if !at.After(time.Now()) {
		s.poke()
	}
	return rec, nil
}

// rebootTime validates when the reboot should run. A moment a minute in the past
// is accepted as "now", because the browser's clock and the server's differ.
func rebootTime(req HostRebootRequest, now time.Time) (time.Time, error) {
	if req.Now {
		return now.UTC(), nil
	}
	if req.ScheduledAt == nil || req.ScheduledAt.IsZero() {
		return time.Time{}, refuse(RebootReasonInvalidTime, "a scheduled reboot needs a date and time")
	}
	at := req.ScheduledAt.UTC()
	if at.Before(now.Add(-time.Minute)) {
		return time.Time{}, refuse(RebootReasonInvalidTime, "the reboot time is in the past")
	}
	if at.After(now.Add(rebootMaxAhead)) {
		return time.Time{}, refuse(RebootReasonInvalidTime, "a reboot can be scheduled at most 90 days ahead")
	}
	if at.Before(now) {
		return now.UTC(), nil
	}
	return at, nil
}

// Cancel removes the target's reboot while it is still only scheduled.
func (s *RebootService) Cancel(ctx context.Context, target HostTarget) error {
	target, err := s.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	return s.store.cancel(ctx, target.rowID())
}

// Inventory reports what the reboot would stop on the target, as far as McHarbor
// can see. Each part fails soft: an unreadable surface is reported as unknown.
func (s *RebootService) Inventory(ctx context.Context, target HostTarget) (*HostRebootInventory, error) {
	target, err := s.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	inv := &HostRebootInventory{McHarborHost: target.Self}
	if demo.Active() {
		return inv, nil
	}
	hosts, err := s.hostsFor(ctx, target, "")
	if err != nil {
		s.logger.Warn("system: reading reboot inventory failed", "target", target.rowID(), "error", err)
		return inv, nil
	}
	defer hosts.close()
	inv.McHarborHost = inv.McHarborHost || hosts.mcharborHost

	if hosts.containers != nil {
		if running, err := hosts.containers.RunningContainers(ctx); err == nil {
			inv.ContainersKnown = true
			inv.Containers = len(running)
			for _, ctr := range running {
				if ctr.comesBackAfterBoot() {
					inv.ContainersReturning++
				}
				if ctr.SwarmTask {
					inv.SwarmTasks++
				}
			}
		}
	}
	if hosts.guests != nil {
		if running, err := hosts.guests.RunningGuests(ctx); err == nil {
			inv.GuestsKnown = true
			inv.Guests = len(running)
			for _, guest := range running {
				if !guest.Graceful {
					inv.GuestBlockers = append(inv.GuestBlockers, guest.label())
				}
			}
		}
	}
	if hosts.swarm != nil {
		if role, _, err := hosts.swarm.Role(ctx); err == nil {
			inv.Swarm = role
		}
	}
	if found, err := s.kubernetesNode(ctx, target); err == nil {
		inv.KubernetesKnown = true
		inv.Kubernetes = found
	}
	return inv, nil
}

// kubernetesNode reports the kubelet processes running on the host, if any.
func (s *RebootService) kubernetesNode(ctx context.Context, target HostTarget) ([]string, error) {
	result, err := s.runCommand(ctx, target, kubernetesProbeScript, false, 90*time.Second)
	if err != nil {
		return nil, err
	}
	return parseKubernetesProbe(result.Output), nil
}

// realHosts reaches the target's Docker daemon, and its compute agent when it
// has one. The self host never runs guests: those live behind an agent.
func (s *RebootService) realHosts(ctx context.Context, target HostTarget, actor string) (*rebootHosts, error) {
	if target.Self {
		return s.selfHosts(ctx)
	}
	cli, err := s.svc.pool.GetContext(ctx, target.EnvID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}
	docker, mcharbor := newDockerRebootHost(ctx, cli)
	hosts := &rebootHosts{containers: docker, swarm: docker, mcharborHost: mcharbor, close: func() {}}
	if s.svc.pool.IsAgentEnvContext(ctx, target.EnvID) && s.drains != nil {
		if conn, ok := s.svc.pool.AgentConnectionContext(ctx, target.EnvID); ok && conn != nil && conn.Transport != nil && conn.Transport.ComputeSupported() {
			hosts.guests = newGuestRebootHost(s.drains(), s.plugins, target.EnvID, rebootPrincipal(ctx, actor))
		}
	}
	return hosts, nil
}

// rebootPrincipal is who a drain call acts as: the requester a reboot records as its actor,
// or, for an inventory read, the user whose request it is. The plugin's own gate judges them.
func rebootPrincipal(ctx context.Context, actor string) *auth.User {
	if id, ok := strings.CutPrefix(actor, "user:"); ok && id != "" {
		return &auth.User{ID: id}
	}
	return auth.UserFromContext(ctx)
}

// selfHosts reaches the Docker daemon on the machine McHarbor is installed on.
func (s *RebootService) selfHosts(ctx context.Context) (*rebootHosts, error) {
	mode, cli, err := resolveSelfHost(ctx)
	if err != nil {
		return nil, err
	}
	if mode != selfHostViaDocker {
		native, nerr := dockerclient.New(dockerclient.FromEnv)
		if nerr != nil {
			return &rebootHosts{mcharborHost: true, close: func() {}}, nil
		}
		pingCtx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
		defer cancel()
		if _, perr := native.Ping(pingCtx, dockerclient.PingOptions{}); perr != nil {
			if cerr := native.Close(); cerr != nil {
				s.logger.Warn("system: closing self host docker client failed", "error", cerr)
			}
			return &rebootHosts{mcharborHost: true, close: func() {}}, nil
		}
		cli = native
	}
	docker, _ := newDockerRebootHost(ctx, cli)
	return &rebootHosts{
		containers: docker, swarm: docker, mcharborHost: true,
		close: func() {
			if cerr := cli.Close(); cerr != nil {
				s.logger.Warn("system: closing self host docker client failed", "error", cerr)
			}
		},
	}, nil
}

// runRebootCommand runs a host script on the target through the privileged
// utility container. Unlike runHostCommand it also serves agent environments,
// whose Docker API the agent proxies.
func (s *Service) runRebootCommand(ctx context.Context, target HostTarget, script string, readWrite bool, timeout time.Duration) (*commandResult, error) {
	if target.Self {
		return s.runSelfHostCommand(ctx, script, readWrite, timeout)
	}
	cli, err := s.pool.GetContext(ctx, target.EnvID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}
	return s.runHostCommandWith(ctx, cli, script, readWrite, timeout)
}

// StartScheduler runs due reboots, confirms finished ones and restores what they
// stopped, until ctx ends. The table is created here on first use.
func (s *RebootService) StartScheduler(ctx context.Context) {
	schemaCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	err := ensureRebootSchema(schemaCtx, s.store.database)
	cancel()
	if err != nil {
		s.logger.Error("system: creating the host reboot table failed; scheduled reboots are disabled", "error", err)
		return
	}
	go func() {
		ticker := time.NewTicker(rebootTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.wg.Wait()
				return
			case <-ticker.C:
			case <-s.wake:
			}
			if lead := hostUpdateLeader.Load(); lead != nil && !(*lead)(ctx) {
				continue
			}
			s.tick(ctx)
		}
	}()
}

// poke wakes the scheduler without waiting for the next tick.
func (s *RebootService) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// tick starts due reboots, fails abandoned ones and checks returning hosts.
func (s *RebootService) tick(ctx context.Context) {
	rows, err := s.store.list(ctx, RebootStatusScheduled, RebootStatusRunning, RebootStatusStoppingWorkloads, RebootStatusRebooting)
	if err != nil {
		s.logger.Error("system: reading host reboots failed", "error", err)
		return
	}
	now := time.Now()
	for _, rec := range rows {
		switch rec.Status {
		case RebootStatusScheduled:
			if !rec.ScheduledAt.After(now) {
				s.launch(ctx, rec)
			}
		case RebootStatusRunning, RebootStatusStoppingWorkloads:
			if !s.isActive(rec.targetID) && now.Sub(rec.updatedAt) > rebootStaleAfter {
				s.fail(ctx, rec, refuse(RebootReasonInterrupted,
					"McHarbor stopped while this reboot was stopping workloads, so the host was not rebooted. Anything already stopped stays stopped."))
			}
		case RebootStatusRebooting:
			if !s.isActive(rec.targetID) && s.verifyDue(rec.targetID, now) {
				s.verify(ctx, rec)
			}
		}
	}
}

// verifyDue reports whether the target's post-reboot check may run again, and
// records that it is about to.
func (s *RebootService) verifyDue(targetID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.verified[targetID]; ok && now.Sub(last) < time.Minute {
		return false
	}
	s.verified[targetID] = now
	return true
}

// isActive reports whether this process is working on the target right now.
func (s *RebootService) isActive(targetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[targetID]
}

// launch claims a due reboot and runs it in the background, tracked so the
// scheduler's shutdown waits for it.
func (s *RebootService) launch(ctx context.Context, rec HostReboot) {
	claimed, err := s.store.claim(ctx, rec.ID)
	if err != nil {
		s.logger.Error("system: claiming host reboot failed", "target", rec.targetID, "error", err)
		return
	}
	if !claimed {
		return
	}
	s.mu.Lock()
	s.active[rec.targetID] = true
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.active, rec.targetID)
			s.mu.Unlock()
		}()
		runCtx, cancel := context.WithTimeout(ctx, rebootRunTimeout)
		defer cancel()
		s.execute(runCtx, rec)
	}()
}

// execute runs one reboot: detect what the host runs, stop it gracefully, and
// issue the reboot only when everything stopped. It forces nothing, ever.
func (s *RebootService) execute(ctx context.Context, rec HostReboot) {
	target := rec.target()
	if demo.Active() {
		s.fail(ctx, rec, refuse(RebootReasonDemoMode, "demo mode never reboots a host"))
		return
	}
	hosts, err := s.hostsFor(ctx, target, "user:"+rec.requesterID)
	if err != nil {
		s.fail(ctx, rec, refuse(RebootReasonHostUnreachable, "the host could not be reached: %v", err))
		return
	}
	defer hosts.close()

	s.phase(ctx, rec, RebootStatusRunning, "detect")
	if found, err := s.kubernetesNode(ctx, target); err == nil && len(found) > 0 && !rec.DrainedExternally {
		s.fail(ctx, rec, refuse(RebootReasonKubernetesNode,
			"this host is a Kubernetes node (%s). Cordon and drain it first, then confirm that in the reboot dialog.", strings.Join(found, ", ")))
		return
	}

	s.phase(ctx, rec, RebootStatusStoppingWorkloads, "swarm")
	plan, err := s.prepareSwarm(ctx, hosts, rec.DrainedExternally)
	if err != nil {
		s.fail(ctx, rec, err)
		return
	}

	result, err := stopWorkloads(ctx, hosts.guests, hosts.containers, s.budget, func(phase string) {
		s.phase(ctx, rec, RebootStatusStoppingWorkloads, phase)
	})
	if err != nil {
		s.fail(ctx, rec, s.rollback(ctx, hosts, result, plan, err))
		return
	}
	if result != nil {
		for _, ctr := range result.Containers {
			if ctr.comesBackAfterBoot() {
				plan.Containers = append(plan.Containers, restoreContainer{ID: ctr.ID, Name: ctr.Name})
			}
		}
	}

	if err := s.store.markRebooting(ctx, rec.ID, plan, time.Now()); err != nil {
		s.fail(ctx, rec, s.rollback(ctx, hosts, result, plan, refuse(RebootReasonRebootCommand, "the reboot could not be recorded: %v", err)))
		return
	}
	out, err := s.runCommand(ctx, target, rebootCommandScript(), true, 2*time.Minute)
	if err != nil || out.ExitCode != 0 {
		detail := ""
		if err != nil {
			detail = err.Error()
		} else {
			detail = strings.TrimSpace(out.Output)
		}
		s.fail(ctx, rec, s.rollback(ctx, hosts, result, plan, refuse(RebootReasonRebootCommand, "the host refused the reboot command: %s", detail)))
		return
	}
	s.record(ctx, rec, "host_reboot_executed", fmt.Sprintf("host reboot issued after stopping %d guests and %d containers gracefully",
		len(resultGuests(result)), len(resultContainers(result))))
}

// resultGuests is result.Guests, tolerating a nil result.
func resultGuests(result *stopResult) []rebootGuest {
	if result == nil {
		return nil
	}
	return result.Guests
}

// resultContainers is result.Containers, tolerating a nil result.
func resultContainers(result *stopResult) []rebootContainer {
	if result == nil {
		return nil
	}
	return result.Containers
}

// prepareSwarm pauses a Swarm manager node so the tasks it stops are placed on
// other nodes, and refuses a worker that still runs tasks it cannot move.
func (s *RebootService) prepareSwarm(ctx context.Context, hosts *rebootHosts, drained bool) (restorePlan, error) {
	plan := restorePlan{}
	if hosts.swarm == nil {
		return plan, nil
	}
	role, nodeID, err := hosts.swarm.Role(ctx)
	if err != nil || role == "" {
		return plan, nil //nolint:nilerr // Swarm state is best-effort: a host with no readable Swarm role has nothing to pause
	}
	if role == swarmRoleWorker {
		if drained || hosts.containers == nil {
			return plan, nil
		}
		running, err := hosts.containers.RunningContainers(ctx)
		if err != nil {
			return plan, nil //nolint:nilerr // an unlistable container set cannot show a blocking task, so the worker check is skipped
		}
		for _, ctr := range running {
			if ctr.SwarmTask {
				return plan, refuse(RebootReasonSwarmWorker,
					"this host is a Swarm worker running service tasks, and a worker cannot pause itself. Drain it from a manager first, then confirm that in the reboot dialog.")
			}
		}
		return plan, nil
	}
	current, err := hosts.swarm.Availability(ctx, nodeID)
	if err != nil {
		return plan, refuse(RebootReasonSwarmPause, "the Swarm node could not be read: %v", err)
	}
	if current != string(swarm.NodeAvailabilityActive) {
		return plan, nil
	}
	if err := hosts.swarm.SetAvailability(ctx, nodeID, string(swarm.NodeAvailabilityPause)); err != nil {
		return plan, refuse(RebootReasonSwarmPause, "the Swarm node could not be paused: %v", err)
	}
	plan.SwarmNodeID = nodeID
	plan.SwarmAvailability = current
	return plan, nil
}

// rollback undoes what an aborted reboot did to Docker: every container it
// stopped is started again and the Swarm node is un-paused. Guests it shut down
// stay off, because starting them belongs to the compute plugins; the detail says so.
func (s *RebootService) rollback(ctx context.Context, hosts *rebootHosts, result *stopResult, plan restorePlan, cause error) error {
	refusal, ok := AsRebootRefusal(cause)
	if !ok {
		slog.Error("system: reboot aborted with an unclassified error", "error", cause)
		refusal = &RebootRefusal{Reason: RebootReasonRebootCommand, Detail: "the reboot command could not be completed; the server log has the cause."}
	}
	detail := refusal.Detail
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	if hosts.containers != nil && len(resultContainers(result)) > 0 {
		if failed := restartContainers(restoreCtx, hosts.containers, result.Containers); len(failed) > 0 {
			detail += " McHarbor could not start these containers again: " + strings.Join(failed, ", ") + "."
		} else {
			detail += " The containers McHarbor had stopped were started again."
		}
	}
	if plan.SwarmNodeID != "" && hosts.swarm != nil {
		if err := hosts.swarm.SetAvailability(restoreCtx, plan.SwarmNodeID, plan.SwarmAvailability); err != nil {
			detail += " The Swarm node is still paused: " + err.Error() + "."
		}
	}
	if guests := resultGuests(result); len(guests) > 0 {
		names := make([]string, 0, len(guests))
		for _, guest := range guests {
			names = append(names, guest.label())
		}
		detail += " These guests were asked to shut down, and McHarbor does not start them again: " + strings.Join(names, ", ") + "."
	}
	return &RebootRefusal{Reason: refusal.Reason, Detail: detail}
}

// verify checks a rebooting host: once it is back on a new boot, it restores
// what the reboot stopped; if it never comes back, the row is failed.
func (s *RebootService) verify(ctx context.Context, rec HostReboot) {
	if rec.RebootIssuedAt == nil {
		s.fail(ctx, rec, refuse(RebootReasonInterrupted, "the reboot has no record of being issued"))
		return
	}
	issued := *rec.RebootIssuedAt
	since := time.Since(issued)
	out, err := s.runCommand(ctx, rec.target(), uptimeScript, false, 90*time.Second)
	if err != nil || out.ExitCode != 0 {
		if since > rebootNotBackAfter {
			s.fail(ctx, rec, refuse(RebootReasonHostNotBack, "the host has not come back %s after the reboot was issued", rebootNotBackAfter))
		}
		return
	}
	uptime, err := parseUptime(out.Output)
	if err != nil {
		return
	}
	bootedAt := time.Now().Add(-uptime)
	if !bootedAt.After(issued) {
		if since > rebootNoRestartAfter {
			s.fail(ctx, rec, refuse(RebootReasonHostDidNotRestart,
				"the host is reachable but has not restarted %s after the reboot was issued; systemd may have refused or be waiting on a unit", rebootNoRestartAfter))
		}
		return
	}
	s.restore(ctx, rec)
}

// restore starts the containers a normal boot would have brought back and puts
// the Swarm node back to its old availability.
func (s *RebootService) restore(ctx context.Context, rec HostReboot) {
	hosts, err := s.hostsFor(ctx, rec.target(), "user:"+rec.requesterID)
	if err != nil {
		return
	}
	defer hosts.close()

	var problems []string
	if len(rec.plan.Containers) > 0 && hosts.containers != nil {
		containers := make([]rebootContainer, 0, len(rec.plan.Containers))
		for _, ctr := range rec.plan.Containers {
			containers = append(containers, rebootContainer{ID: ctr.ID, Name: ctr.Name})
		}
		if failed := restartContainers(ctx, hosts.containers, containers); len(failed) > 0 {
			problems = append(problems, "these containers did not start again: "+strings.Join(failed, ", "))
		}
	}
	if rec.plan.SwarmNodeID != "" && hosts.swarm != nil {
		if err := hosts.swarm.SetAvailability(ctx, rec.plan.SwarmNodeID, rec.plan.SwarmAvailability); err != nil {
			problems = append(problems, "the Swarm node is still paused: "+err.Error())
		}
	}
	reason, detail := "", ""
	if len(problems) > 0 {
		reason, detail = RebootReasonRestorePartial, strings.Join(problems, "; ")
	}
	if err := s.store.finish(ctx, rec.ID, RebootStatusDone, reason, detail); err != nil {
		s.logger.Error("system: finishing host reboot failed", "target", rec.targetID, "error", err)
		return
	}
	s.record(ctx, rec, "host_reboot_completed", fmt.Sprintf("host came back after the reboot; %d containers restarted", len(rec.plan.Containers)))
}

// phase records progress; a failed write is logged, never fatal to the run.
func (s *RebootService) phase(ctx context.Context, rec HostReboot, status, phase string) {
	if err := s.store.setPhase(ctx, rec.ID, status, phase); err != nil {
		s.logger.Warn("system: recording host reboot phase failed", "target", rec.targetID, "error", err)
	}
}

// fail records a refused or failed reboot with its reason and detail.
func (s *RebootService) fail(ctx context.Context, rec HostReboot, cause error) {
	reason, detail := RebootReasonRebootCommand, cause.Error()
	if refusal, ok := AsRebootRefusal(cause); ok {
		reason, detail = refusal.Reason, refusal.Detail
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hostCallTimeout)
	defer cancel()
	if err := s.store.finish(writeCtx, rec.ID, RebootStatusFailed, reason, detail); err != nil {
		s.logger.Error("system: recording failed host reboot failed", "target", rec.targetID, "error", err)
	}
	s.logger.Warn("system: host reboot refused", "target", rec.targetID, "reason", reason, "detail", detail)
	s.record(writeCtx, rec, "host_reboot_failed", reason+": "+detail)
}

// record writes an audit entry on behalf of the operator who scheduled the reboot.
func (s *RebootService) record(ctx context.Context, rec HostReboot, action, details string) {
	if s.audit == nil {
		return
	}
	entityType, entityID := auditEntity(rec.target())
	entry := audit.Entry{Action: action, EntityType: entityType, EntityID: entityID, Details: details}
	if !rec.target().Self {
		entry.EnvironmentID = rec.targetID
	}
	if err := s.audit.RecordContext(ctx, rec.requesterID, rec.RequestedBy, entry); err != nil {
		s.logger.Warn("system: auditing host reboot failed", "action", action, "error", err)
	}
}

// errRebootTargetMissing is returned when the environment named does not exist.
var errRebootTargetMissing = errors.New("environment not found")
