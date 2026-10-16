// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The conditions this plugin contributes to the host's alert engine.
//
// The engine owns the thresholds, the schedule, the firing state and the
// delivery. This file only answers "what is true right now", which is why every
// evaluation is a plain read with no memory of the last one.
const (
	alertCTCrashed             = "ct_crashed"
	alertCTStoppedUnexpectedly = "ct_stopped_unexpectedly"
	alertCTUnknownState        = "ct_unknown_state"
	alertCTMemoryPressure      = "ct_memory_pressure"
	alertCTPidsPressure        = "ct_pids_pressure"
	alertCTAutostartFailed     = "ct_autostart_failed"
)

const (
	alertPollInterval = 60 * time.Second
	alertEvalTimeout  = 45 * time.Second

	// Defaults for the two pressure conditions, as a percentage of the
	// container's own limit. 90% of memory.max is close enough to the OOM
	// killer to be worth knowing about and far enough that there is still time
	// to act; 80% of pids.max is lower on purpose, because a process count
	// climbing toward its limit is usually a leak and by 90% it is often too
	// late to log in and look.
	warnAtMemoryPercent = 90.0
	warnAtPidsPercent   = 80.0
)

// lxcAlertSpec declares what this plugin can alert on.
//
// Subjects are named `host/container`, which is what a rule's target pattern
// matches on — so `prod/*`, `*/db-*` and `prod/api-*` all work with the
// wildcard matcher the host already has.
func lxcAlertSpec() plugin.AlertSourceAdapterSpec {
	return plugin.AlertSourceAdapterSpec{
		SourceType:  "lxc",
		DisplayName: "System Containers",
		Slug:        "addon-lxc",
		Icon:        "plugin.svg",
		Description: "Container health, memory and process pressure against each container's own limits.",
		Conditions: []plugin.AlertConditionDecl{
			{
				Key:         alertCTCrashed,
				DisplayName: "Container crashed",
				Description: "A container's init exited abnormally — the OOM killer took it, it was signalled, or its rootfs returned an IO error. The reason is read from the unit's result and carried in the message.",
				// An event: a crash is a moment, not a state to wait out, and
				// the operator wants to know immediately.
				Kind:         plugin.AlertKindEvent,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertCTStoppedUnexpectedly,
				DisplayName:  "Container stopped unexpectedly",
				Description:  "A container that should be running is stopped, and nobody asked it to stop through McHarbor.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
			{
				Key:         alertCTUnknownState,
				DisplayName: "Container state unknown",
				Description: "A container's host cannot be reached, so its state is genuinely unknown. Its containers are supervised by systemd and are probably still running, but nothing can confirm that until the agent returns.",
				// Duration, not event: an agent restart or a brief network
				// blip should not page anyone.
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
			{
				// Measured against the container's own memory.max, not against
				// the host's memory. A container at 90% of a 512 MB limit is in
				// trouble on a host with 128 GB free, and a host-relative
				// figure would say it is fine right up until the OOM killer
				// runs.
				Key:              alertCTMemoryPressure,
				DisplayName:      "Container near its memory limit",
				Description:      "A container is using most of the memory it is allowed. Past its limit the kernel's OOM killer takes a process inside the container — usually not the one at fault.",
				Kind:             plugin.AlertKindThreshold,
				TargetKind:       plugin.AlertTargetNone,
				Unit:             "%",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: warnAtMemoryPercent,
				PollInterval:     alertPollInterval,
			},
			{
				Key:              alertCTPidsPressure,
				DisplayName:      "Container near its process limit",
				Description:      "A container is approaching pids.max. At the limit it can no longer fork, which presents as everything inside it failing at once for no visible reason.",
				Kind:             plugin.AlertKindThreshold,
				TargetKind:       plugin.AlertTargetNone,
				Unit:             "%",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: warnAtPidsPercent,
				PollInterval:     alertPollInterval,
			},
			{
				Key:          alertCTAutostartFailed,
				DisplayName:  "Autostart container is not running",
				Description:  "A container marked to start automatically is not running. Usually noticed after a host reboot.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
		},
	}
}

// alertSource answers the engine's evaluations.
type alertSource struct {
	plugin *LXCPlugin
}

var _ plugin.AlertSourceAdapterClient = (*alertSource)(nil)

// Evaluate reports the observations for one condition.
func (a *alertSource) Evaluate(
	ctx context.Context,
	req plugin.AlertEvaluateRequest,
) ([]plugin.AlertObservation, error) {
	svc := a.plugin.svc
	if svc == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, alertEvalTimeout)
	defer cancel()

	containers, err := svc.List(ctx, "", false)
	if err != nil {
		return nil, fmt.Errorf("lxc: list containers for alerting: %w", err)
	}

	var out []plugin.AlertObservation
	for _, ct := range containers {
		// Templates are not running and are not supposed to be. Alerting on
		// them would be noise on every condition here.
		if ct.IsTemplate {
			continue
		}
		subject := ct.EnvironmentID + "/" + ct.Name
		base := map[string]string{"container": ct.Name, "host": ct.EnvironmentID}

		switch req.ConditionKey {
		case alertCTCrashed:
			out = append(out, plugin.AlertObservation{
				SubjectID:   ct.ID,
				SubjectName: subject,
				Active:      ct.State == StateCrashed,
				Context: withContext(base,
					// The unit's own result, which is what turns "the container
					// died" into "the OOM killer took it".
					"reason", firstNonEmpty(ct.ExitReason, ct.LastError)),
			})

		case alertCTStoppedUnexpectedly:
			// Only meaningful for a container that has run before. One defined
			// and never started is not one that stopped.
			if ct.LastStartedAt == nil {
				continue
			}
			out = append(out, plugin.AlertObservation{
				SubjectID:   ct.ID,
				SubjectName: subject,
				Active:      ct.HostAvailable && ct.State == StateStopped,
				Context:     base,
			})

		case alertCTUnknownState:
			out = append(out, plugin.AlertObservation{
				SubjectID:   ct.ID,
				SubjectName: subject,
				Active:      !ct.HostAvailable,
				Context:     withContext(base, "reason", ct.UnavailableReason),
			})

		case alertCTMemoryPressure:
			stats, ok := a.sample(ctx, ct)
			if !ok || stats.MemoryMaxBytes <= 0 {
				// No limit means nothing to be near. A container with no
				// memory.max is bounded by the host, which is the host's own
				// alert to raise.
				continue
			}
			percent := float64(stats.MemoryBytes) / float64(stats.MemoryMaxBytes) * 100
			out = append(out, plugin.AlertObservation{
				SubjectID:   ct.ID,
				SubjectName: subject,
				Value:       percent,
				ValueLabel:  fmt.Sprintf("%.0f%%", percent),
				Context: withContext(base,
					"usage", humanBytes(stats.MemoryBytes),
					"limit", humanBytes(stats.MemoryMaxBytes)),
			})

		case alertCTPidsPressure:
			stats, ok := a.sample(ctx, ct)
			if !ok || stats.PidsMax <= 0 {
				continue
			}
			percent := float64(stats.PidsCurrent) / float64(stats.PidsMax) * 100
			out = append(out, plugin.AlertObservation{
				SubjectID:   ct.ID,
				SubjectName: subject,
				Value:       percent,
				ValueLabel:  fmt.Sprintf("%.0f%%", percent),
				Context: withContext(base,
					"processes", fmt.Sprintf("%d", stats.PidsCurrent),
					"limit", fmt.Sprintf("%d", stats.PidsMax)),
			})

		case alertCTAutostartFailed:
			if !ct.Autostart {
				continue
			}
			out = append(out, plugin.AlertObservation{
				SubjectID:   ct.ID,
				SubjectName: subject,
				Active:      ct.HostAvailable && ct.State != StateRunning && ct.State != StateFrozen,
				Context:     withContext(base, "state", ct.State),
			})
		}
	}
	return out, nil
}

// sample reads one container's cgroup, tolerating a failure.
//
// A container that cannot be sampled is skipped rather than reported at zero.
// Zero would read as "no pressure" and would silently suppress the alert this
// evaluation exists to raise.
func (a *alertSource) sample(ctx context.Context, ct Container) (Stats, bool) {
	if ct.State != StateRunning && ct.State != StateFrozen {
		return Stats{}, false
	}
	stats, err := a.plugin.svc.Stats(ctx, ct.ID)
	if err != nil {
		a.plugin.logger.Debug("system containers: sampling for an alert",
			"container", ct.ID, "error", err)
		return Stats{}, false
	}
	return stats, true
}

// withContext copies the base context and adds pairs, so one container's extra
// fields never land in another's observation.
func withContext(base map[string]string, pairs ...string) map[string]string {
	out := make(map[string]string, len(base)+len(pairs)/2)
	for key, value := range base {
		out[key] = value
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = pairs[i+1]
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
