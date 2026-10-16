// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// Container condition keys.
const (
	conditionCPU         = "cpu"
	conditionMemory      = "memory"
	conditionDown        = "down"
	conditionUnhealthy   = "unhealthy"
	conditionRestartLoop = "restart_loop"
	conditionOOMKilled   = "oom_killed"
	conditionExitNonZero = "exit_nonzero"
	conditionImageUpdate = "image_update"
)

const (
	// inspectCacheTTL is deliberately shorter than the engine's 30s
	// tick and longer than one tick's work: several container
	// conditions can be enabled at once, and each would otherwise
	// re-inspect every container in the environment. Through the
	// remote agent an inspect costs real time, so one sweep per tick
	// shared across conditions is the difference between a tick that
	// finishes and one that does not.
	inspectCacheTTL = 20 * time.Second

	// inspectConcurrency bounds the parallel inspect sweep. Sequential
	// loops are what made agent environments unusable for stats; the
	// same applies here.
	inspectConcurrency = 8

	// restartSampleRetention caps how long restart-count samples are
	// kept. Anything older than the longest sane window is dead weight.
	restartSampleRetention = 6 * time.Hour
)

// containerSourceSpec declares what the built-in container source can
// alert on. Kept separate from the client so the catalog is complete
// before the engine has wired up Docker access.
func containerSourceSpec() alerting.AlertSourceAdapterSpec {
	return alerting.AlertSourceAdapterSpec{
		SourceType:  sourceContainer,
		DisplayName: "Containers",
		Description: "Resource usage, availability and health of individual containers.",
		Conditions: []alerting.AlertConditionDecl{
			{
				Key:              conditionCPU,
				DisplayName:      "CPU usage",
				Description:      "Container CPU usage as a percentage of its limit.",
				Kind:             alerting.KindThreshold,
				Unit:             "%",
				Operators:        []string{">", ">=", "<", "<="},
				DefaultOperator:  ">",
				DefaultThreshold: 80,
				SupportsFor:      true,
				TargetKind:       alerting.TargetContainer,
			},
			{
				Key:              conditionMemory,
				DisplayName:      "Memory usage",
				Description:      "Container memory usage as a percentage of its limit.",
				Kind:             alerting.KindThreshold,
				Unit:             "%",
				Operators:        []string{">", ">=", "<", "<="},
				DefaultOperator:  ">",
				DefaultThreshold: 80,
				SupportsFor:      true,
				TargetKind:       alerting.TargetContainer,
			},
			{
				Key:         conditionDown,
				DisplayName: "Container down",
				Description: "Container is not running.",
				Kind:        alerting.KindDuration,
				TargetKind:  alerting.TargetContainer,
			},
			{
				Key:         conditionUnhealthy,
				DisplayName: "Healthcheck failing",
				Description: "Container healthcheck reports unhealthy. Containers without a healthcheck are ignored.",
				Kind:        alerting.KindDuration,
				TargetKind:  alerting.TargetContainer,
			},
			{
				Key:              conditionRestartLoop,
				DisplayName:      "Restart loop",
				Description:      "Container restarted more than the given number of times within the window.",
				Kind:             alerting.KindCountInWindow,
				Unit:             "restarts",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">=",
				DefaultThreshold: 3,
				TargetKind:       alerting.TargetContainer,
			},
			{
				Key:         conditionOOMKilled,
				DisplayName: "Killed out of memory",
				Description: "Container was terminated by the kernel OOM killer.",
				Kind:        alerting.KindEvent,
				TargetKind:  alerting.TargetContainer,
			},
			{
				Key:         conditionExitNonZero,
				DisplayName: "Exited with an error",
				Description: "Container exited with a non-zero exit code.",
				Kind:        alerting.KindEvent,
				TargetKind:  alerting.TargetContainer,
				Fields: []types.ConfigFieldDecl{
					{
						Key:         "exit_code",
						Type:        "string",
						Label:       "Exit code",
						Help:        "Only alert on this specific exit code. Leave empty for any non-zero code.",
						Placeholder: "137",
					},
				},
			},
			{
				Key:         conditionImageUpdate,
				DisplayName: "Image update available",
				Description: "A newer image digest is available in the registry.",
				Kind:        alerting.KindEvent,
				TargetKind:  alerting.TargetContainer,
				// A registry round-trip per container is far too
				// expensive for the general tick; this was already a
				// separate 30-minute ticker before conditions carried
				// their own interval.
				PollInterval: 30 * time.Minute,
			},
		},
	}
}

// containerSource evaluates the built-in container conditions.
type containerSource struct {
	deps EngineDeps

	mu           sync.Mutex
	inspectCache map[string]inspectSnapshot
	restarts     map[string][]restartSample
}

type inspectSnapshot struct {
	takenAt    time.Time
	containers []inspectedContainer
}

type inspectedContainer struct {
	summary ContainerSummary
	name    string
	inspect *ContainerInspect
}

type restartSample struct {
	at    time.Time
	count int
}

func newContainerSource(deps EngineDeps) *containerSource {
	return &containerSource{
		deps:         deps,
		inspectCache: make(map[string]inspectSnapshot),
		restarts:     make(map[string][]restartSample),
	}
}

// Evaluate implements alerting.AlertSourceAdapterClient.
func (s *containerSource) Evaluate(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	switch req.ConditionKey {
	case conditionCPU, conditionMemory:
		return s.evaluateMetric(ctx, req)
	case conditionImageUpdate:
		return s.evaluateImageUpdate(ctx, req)
	case conditionDown, conditionUnhealthy, conditionRestartLoop, conditionOOMKilled, conditionExitNonZero:
		return s.evaluateFromInspect(ctx, req)
	default:
		return nil, fmt.Errorf("container source: unknown condition %q", req.ConditionKey)
	}
}

func (s *containerSource) evaluateMetric(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	if s.deps.Metrics == nil {
		return nil, fmt.Errorf("container source: metrics unavailable")
	}

	stats, err := s.deps.Metrics.AllContainerStats(ctx, req.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("container stats: %w", err)
	}

	out := make([]alerting.AlertObservation, 0, len(stats))
	for _, stat := range stats {
		value := stat.CPUPercent
		if req.ConditionKey == conditionMemory {
			value = stat.MemPercent
		}

		out = append(out, alerting.AlertObservation{
			SubjectID:   stat.ID,
			SubjectName: stat.Name,
			Value:       value,
			ValueLabel:  formatFloat(value) + "%",
		})
	}

	return out, nil
}

func (s *containerSource) evaluateImageUpdate(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	if s.deps.Containers == nil {
		return nil, fmt.Errorf("container source: container access unavailable")
	}

	results, err := s.deps.Containers.CheckImageUpdates(ctx, req.EnvironmentID, nil)
	if err != nil {
		return nil, fmt.Errorf("image update check: %w", err)
	}

	out := make([]alerting.AlertObservation, 0, len(results))
	for _, result := range results {
		observation := alerting.AlertObservation{
			SubjectID:   result.ContainerID,
			SubjectName: result.ContainerName,
			Active:      result.UpdateAvailable,
		}
		if result.CurrentDigest != "" && result.RemoteDigest != "" {
			observation.Context = map[string]string{
				"current_digest": shortDigest(result.CurrentDigest),
				"remote_digest":  shortDigest(result.RemoteDigest),
			}
			observation.ValueLabel = shortDigest(result.CurrentDigest) + " -> " + shortDigest(result.RemoteDigest)
		}
		out = append(out, observation)
	}

	return out, nil
}

// evaluateFromInspect answers the four conditions that need per-container
// inspect data, off one shared sweep.
func (s *containerSource) evaluateFromInspect(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	snapshot, err := s.inspectAll(ctx, req.EnvironmentID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	out := make([]alerting.AlertObservation, 0, len(snapshot))

	for _, item := range snapshot {
		observation := alerting.AlertObservation{
			SubjectID:   item.summary.ID,
			SubjectName: item.name,
		}

		state := (*ContainerInspectState)(nil)
		if item.inspect != nil {
			state = item.inspect.State
		}
		running := strings.EqualFold(strings.TrimSpace(item.summary.State), "running")

		switch req.ConditionKey {
		case conditionDown:
			observation.Active = !running
			if state != nil && state.FinishedAt != "" {
				if finishedAt, parseErr := time.Parse(time.RFC3339Nano, state.FinishedAt); parseErr == nil {
					observation.Since = finishedAt
				}
			}

		case conditionUnhealthy:
			// A container with no healthcheck reports an empty status.
			// Treating that as healthy rather than skipping it keeps
			// the subject present in the result set, which is what
			// lets the engine resolve a container that has just had
			// its healthcheck removed.
			if state != nil && state.Health != nil {
				observation.Active = strings.EqualFold(state.Health.Status, "unhealthy")
				observation.ValueLabel = state.Health.Status
			}
			if observation.Active && state != nil && state.StartedAt != "" {
				if startedAt, parseErr := time.Parse(time.RFC3339Nano, state.StartedAt); parseErr == nil {
					observation.Since = startedAt
				}
			}

		case conditionRestartLoop:
			restarts := 0
			if item.inspect != nil {
				restarts = item.inspect.RestartCount
			}
			window := time.Duration(req.WindowSeconds) * time.Second
			delta := s.restartsInWindow(item.summary.ID, restarts, now, window)
			observation.Value = float64(delta)
			observation.ValueLabel = strconv.Itoa(delta) + " restarts"

		case conditionOOMKilled:
			observation.Active = state != nil && state.OOMKilled

		case conditionExitNonZero:
			if state != nil && !running && state.ExitCode != 0 {
				wanted := strings.TrimSpace(req.Params["exit_code"])
				observation.Active = wanted == "" || wanted == strconv.Itoa(state.ExitCode)
				observation.ValueLabel = "exit " + strconv.Itoa(state.ExitCode)
				observation.Value = float64(state.ExitCode)
			}
		}

		out = append(out, observation)
	}

	return out, nil
}

// inspectAll returns one inspect sweep of the environment, cached for
// inspectCacheTTL so several conditions in the same tick share it.
func (s *containerSource) inspectAll(ctx context.Context, envID string) ([]inspectedContainer, error) {
	if s.deps.Containers == nil {
		return nil, fmt.Errorf("container source: container access unavailable")
	}

	s.mu.Lock()
	cached, ok := s.inspectCache[envID]
	s.mu.Unlock()
	if ok && time.Since(cached.takenAt) < inspectCacheTTL {
		return cached.containers, nil
	}

	items, err := s.deps.Containers.ListContainers(ctx, envID, true)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	out := make([]inspectedContainer, len(items))
	semaphore := make(chan struct{}, inspectConcurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		out[i] = inspectedContainer{summary: item, name: containerName(item)}

		wg.Add(1)
		go func(index int, containerID string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// An inspect that fails leaves that container's entry with
			// a nil inspect rather than failing the whole sweep: one
			// container disappearing mid-tick must not blind every
			// other rule in the environment.
			inspect, inspectErr := s.deps.Containers.InspectContainer(ctx, envID, containerID)
			if inspectErr == nil {
				out[index].inspect = inspect
			}
		}(i, item.ID)
	}

	wg.Wait()

	s.mu.Lock()
	s.inspectCache[envID] = inspectSnapshot{takenAt: time.Now(), containers: out}
	s.mu.Unlock()

	return out, nil
}

// restartsInWindow converts Docker's cumulative RestartCount into
// "restarts since now-window".
//
// Docker exposes a lifetime counter, not a rate, so the delta has to be
// measured against an earlier reading. Samples live in memory: this is
// a rate over a short window, and a restarted host has no restart
// history worth reconstructing anyway. The practical consequence is
// that the condition needs one window's worth of ticks after a restart
// before it can fire, which is the safe direction — it under-reports
// while warming up rather than firing on a count it never observed.
func (s *containerSource) restartsInWindow(containerID string, count int, now time.Time, window time.Duration) int {
	if window <= 0 {
		window = time.Hour
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.restarts[containerID]
	samples := make([]restartSample, 0, len(previous)+1)
	samples = append(samples, previous...)
	samples = append(samples, restartSample{at: now, count: count})

	cutoff := now.Add(-window)
	retention := now.Add(-restartSampleRetention)
	kept := samples[:0]
	baseline := count
	for _, sample := range samples {
		if sample.at.Before(retention) {
			continue
		}
		kept = append(kept, sample)
		// The baseline is the oldest sample still inside the window;
		// anything older is kept only so a longer window on another
		// rule can still find its own baseline.
		if !sample.at.Before(cutoff) && sample.count < baseline {
			baseline = sample.count
		}
	}
	s.restarts[containerID] = kept

	delta := count - baseline
	if delta < 0 {
		// The counter went backwards, which means the container was
		// recreated. Start again rather than reporting a negative.
		delta = 0
	}
	return delta
}

func containerName(item ContainerSummary) string {
	if len(item.Names) == 0 {
		if len(item.ID) >= 12 {
			return item.ID[:12]
		}
		return item.ID
	}
	return strings.TrimPrefix(item.Names[0], "/")
}

func shortDigest(value string) string {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) == 2 {
		value = parts[1]
	}
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
