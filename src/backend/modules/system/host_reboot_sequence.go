// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Reboot refusal reasons. They are machine codes the UI translates, persisted
// beside the operator-facing detail so a failed row explains itself in any locale.
const (
	RebootReasonDemoMode          = "demo_mode"
	RebootReasonInProgress        = "reboot_in_progress"
	RebootReasonInvalidTime       = "invalid_time"
	RebootReasonNotConfirmed      = "not_confirmed"
	RebootReasonNotScheduled      = "not_scheduled"
	RebootReasonHostUnreachable   = "host_unreachable"
	RebootReasonKubernetesNode    = "kubernetes_node"
	RebootReasonSwarmWorker       = "swarm_worker"
	RebootReasonSwarmPause        = "swarm_pause_failed"
	RebootReasonGuestNotGraceful  = "guest_not_graceful"
	RebootReasonGuestFailed       = "guest_shutdown_failed"
	RebootReasonGuestTimeout      = "guest_timeout"
	RebootReasonContainerFailed   = "container_stop_failed"
	RebootReasonContainerTimeout  = "container_timeout"
	RebootReasonRebootCommand     = "reboot_command_failed"
	RebootReasonHostNotBack       = "host_not_back"
	RebootReasonHostDidNotRestart = "host_did_not_restart"
	RebootReasonInterrupted       = "interrupted"
	RebootReasonRestorePartial    = "restore_partial"
)

// RebootRefusal is a reboot that was stopped on purpose, before anything was
// forced: Reason is the machine code, Detail the sentence an operator reads.
type RebootRefusal struct {
	Reason string
	Detail string
}

// Error renders the refusal for logs.
func (e *RebootRefusal) Error() string { return e.Reason + ": " + e.Detail }

// refuse builds a RebootRefusal.
func refuse(reason, format string, args ...any) error {
	return &RebootRefusal{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// AsRebootRefusal returns the refusal err carries, if any.
func AsRebootRefusal(err error) (*RebootRefusal, bool) {
	var refusal *RebootRefusal
	if errors.As(err, &refusal) {
		return refusal, true
	}
	return nil, false
}

// rebootContainer is one running Docker container the graceful stop acts on.
type rebootContainer struct {
	ID            string
	Name          string
	StopSignal    string
	RestartPolicy string
	SwarmTask     bool
}

// comesBackAfterBoot reports whether a normal reboot would have restarted the
// container, which is the set McHarbor starts again once the host is back: a stop
// through the API marks it manually stopped, so Docker alone would leave it down.
func (c rebootContainer) comesBackAfterBoot() bool {
	if c.SwarmTask {
		return false
	}
	return c.RestartPolicy == "always" || c.RestartPolicy == "unless-stopped"
}

// rebootGuest is one running guest on the host, of the subject type the plugin that drains
// it declares.
type rebootGuest struct {
	Kind string
	ID   string
	Name string
	// Graceful is false when the guest cannot be asked to shut itself down: a VM
	// with no QEMU monitor, a paused VM, a frozen container.
	Graceful bool
}

// label names the guest for a refusal sentence.
func (g rebootGuest) label() string {
	if g.Name != "" {
		return g.Name
	}
	return g.ID
}

// rebootContainerHost is the slice of Docker the graceful stop needs. It has no
// kill verb on purpose: the only signal it can send is the container's own stop signal.
type rebootContainerHost interface {
	RunningContainers(ctx context.Context) ([]rebootContainer, error)
	SignalStop(ctx context.Context, c rebootContainer) error
	StillRunning(ctx context.Context, ids []string) ([]string, error)
	Start(ctx context.Context, id string) error
}

// rebootGuestHost is the slice of the compute agent the graceful stop needs:
// an ACPI or init-system shutdown request, never a hard stop.
type rebootGuestHost interface {
	RunningGuests(ctx context.Context) ([]rebootGuest, error)
	RequestShutdown(ctx context.Context, g rebootGuest) error
	StillRunning(ctx context.Context, guests []rebootGuest) ([]rebootGuest, error)
}

// rebootBudget bounds each phase of the graceful stop. A phase that runs out is
// a refusal, never an escalation.
type rebootBudget struct {
	Guests     time.Duration
	Containers time.Duration
	Poll       time.Duration
}

// defaultRebootBudget is generous on purpose: a database flushing its buffers or
// a Windows guest installing updates on shutdown both legitimately take minutes.
var defaultRebootBudget = rebootBudget{
	Guests:     10 * time.Minute,
	Containers: 10 * time.Minute,
	Poll:       3 * time.Second,
}

// stopResult is what the graceful stop did, so an abort can undo the Docker
// half and a finished reboot can bring back what a normal boot would have.
type stopResult struct {
	Guests     []rebootGuest
	Containers []rebootContainer
}

// stopWorkloads shuts the host's guests and then its containers down gracefully,
// in that order, and refuses — without forcing anything — when one will not stop.
//
// guests may be nil for a host that runs none. progress is told each phase so the
// row the UI polls says what is happening.
func stopWorkloads(ctx context.Context, guests rebootGuestHost, containers rebootContainerHost, budget rebootBudget, progress func(string)) (*stopResult, error) {
	result := &stopResult{}
	if guests != nil {
		progress("guests")
		stopped, err := stopGuests(ctx, guests, budget)
		result.Guests = stopped
		if err != nil {
			return result, err
		}
	}
	if containers != nil {
		progress("containers")
		stopped, err := stopContainers(ctx, containers, budget)
		result.Containers = stopped
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// stopGuests asks every running guest to shut down and waits for all of them.
// A guest that cannot be asked politely refuses the reboot before any is touched.
func stopGuests(ctx context.Context, host rebootGuestHost, budget rebootBudget) ([]rebootGuest, error) {
	running, err := host.RunningGuests(ctx)
	if err != nil {
		return nil, refuse(RebootReasonGuestFailed, "the host's virtual machines and containers could not be listed: %v", err)
	}
	if len(running) == 0 {
		return nil, nil
	}
	var blocked []string
	for _, guest := range running {
		if !guest.Graceful {
			blocked = append(blocked, guest.label())
		}
	}
	if len(blocked) > 0 {
		sort.Strings(blocked)
		return nil, refuse(RebootReasonGuestNotGraceful,
			"these guests cannot be asked to shut down gracefully (paused, frozen or without a QEMU monitor): %s", strings.Join(blocked, ", "))
	}

	phaseCtx, cancel := context.WithTimeout(ctx, budget.Guests)
	defer cancel()

	failures := requestAll(phaseCtx, running, func(c context.Context, g rebootGuest) error {
		return host.RequestShutdown(c, g)
	}, func(g rebootGuest) string { return g.label() })
	if len(failures) > 0 {
		return running, refuse(RebootReasonGuestFailed, "the shutdown request was refused for: %s", strings.Join(failures, "; "))
	}

	remaining := running
	for {
		still, err := host.StillRunning(phaseCtx, remaining)
		if err == nil {
			remaining = still
			if len(remaining) == 0 {
				return running, nil
			}
		}
		select {
		case <-phaseCtx.Done():
			names := make([]string, 0, len(remaining))
			for _, guest := range remaining {
				names = append(names, guest.label())
			}
			sort.Strings(names)
			return running, refuse(RebootReasonGuestTimeout,
				"these guests were still running after %s and were left running, not forced off: %s",
				budget.Guests, strings.Join(names, ", "))
		case <-time.After(budget.Poll):
		}
	}
}

// stopContainers sends each running container its own stop signal and waits.
// It never sends SIGKILL: a container still up when the budget ends refuses the reboot.
func stopContainers(ctx context.Context, host rebootContainerHost, budget rebootBudget) ([]rebootContainer, error) {
	running, err := host.RunningContainers(ctx)
	if err != nil {
		return nil, refuse(RebootReasonContainerFailed, "the host's Docker containers could not be listed: %v", err)
	}
	if len(running) == 0 {
		return nil, nil
	}

	phaseCtx, cancel := context.WithTimeout(ctx, budget.Containers)
	defer cancel()

	failures := requestAll(phaseCtx, running, func(c context.Context, ctr rebootContainer) error {
		return host.SignalStop(c, ctr)
	}, func(ctr rebootContainer) string { return ctr.Name })
	if len(failures) > 0 {
		return running, refuse(RebootReasonContainerFailed, "the stop signal could not be sent to: %s", strings.Join(failures, "; "))
	}

	names := make(map[string]string, len(running))
	ids := make([]string, 0, len(running))
	for _, ctr := range running {
		names[ctr.ID] = ctr.Name
		ids = append(ids, ctr.ID)
	}
	for {
		still, err := host.StillRunning(phaseCtx, ids)
		if err == nil {
			ids = still
			if len(ids) == 0 {
				return running, nil
			}
		}
		select {
		case <-phaseCtx.Done():
			left := make([]string, 0, len(ids))
			for _, id := range ids {
				left = append(left, names[id])
			}
			sort.Strings(left)
			return running, refuse(RebootReasonContainerTimeout,
				"these containers were still running after %s and were left running, not killed: %s",
				budget.Containers, strings.Join(left, ", "))
		case <-time.After(budget.Poll):
		}
	}
}

// requestAll runs one request per item concurrently, because a sequential walk
// through the agent costs seconds per guest, and returns "name: error" per failure.
func requestAll[T any](ctx context.Context, items []T, send func(context.Context, T) error, name func(T) string) []string {
	var (
		mu       sync.Mutex
		failures []string
		wg       sync.WaitGroup
	)
	for _, item := range items {
		wg.Add(1)
		go func(item T) {
			defer wg.Done()
			if err := send(ctx, item); err != nil {
				mu.Lock()
				failures = append(failures, name(item)+": "+err.Error())
				mu.Unlock()
			}
		}(item)
	}
	wg.Wait()
	sort.Strings(failures)
	return failures
}

// restartContainers starts every container in ids and returns the names that
// would not start; it is both the abort path and the post-boot restore.
func restartContainers(ctx context.Context, host rebootContainerHost, containers []rebootContainer) []string {
	var failed []string
	for _, ctr := range containers {
		startCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
		if err := host.Start(startCtx, ctr.ID); err != nil {
			failed = append(failed, ctr.Name)
		}
		cancel()
	}
	sort.Strings(failed)
	return failed
}
