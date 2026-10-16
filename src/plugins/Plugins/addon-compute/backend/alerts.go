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
// delivery. This source only answers "is it true right now, and for which
// objects?" when asked — which is why there is no route, no settings page and
// no notification code anywhere in this plugin.
const (
	alertPoolNearFull      = "compute_pool_near_full"
	alertPoolOvercommitted = "compute_pool_overcommitted"
	alertHostLostKVM       = "compute_host_lost_kvm"
	alertHostUnreachable   = "compute_host_unreachable"
	alertImageFetchFailed  = "compute_image_fetch_failed"
)

// alertPollInterval is how often the host asks.
//
// Slower than the engine's default on purpose: every evaluation costs a round
// trip to each agent host, and pool capacity does not move fast enough to
// justify a tighter cadence. A pool going from 84% to 91% inside two minutes
// was going to fill regardless of how often it was sampled.
const alertPollInterval = 2 * time.Minute

// alertEvalTimeout bounds one evaluation across every host.
const alertEvalTimeout = 60 * time.Second

// computeAlertSpec declares what this plugin can alert on.
//
// # Why every condition is TargetNone
//
// The engine evaluates an environment-scoped condition once per host
// environment. Pools are per host, but this source already walks every host in
// one pass — so asking once and letting it enumerate is one round trip rather
// than N. Subjects are named `host/pool`, which is what a rule's target pattern
// matches on, so `prod/*`, `*/fast` and `prod/tank-*` all work with the
// wildcard matcher the host already has.
func computeAlertSpec() plugin.AlertSourceAdapterSpec {
	return plugin.AlertSourceAdapterSpec{
		SourceType:  "compute",
		DisplayName: "Compute",
		Slug:        "addon-compute",
		Icon:        "plugin.svg",
		Description: "Storage pool capacity and host readiness for virtual machines and system containers.",
		Conditions: []plugin.AlertConditionDecl{
			{
				Key:              alertPoolNearFull,
				DisplayName:      "Storage pool near full",
				Description:      "A guest storage pool is above the threshold. When one actually fills, every guest on it is paused with an IO error — which presents as \"all my VMs froze\" and points nowhere near the pool.",
				Kind:             plugin.AlertKindThreshold,
				TargetKind:       plugin.AlertTargetNone,
				Unit:             "%",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: 85,
				SupportsFor:      true,
				PollInterval:     alertPollInterval,
			},
			{
				Key:              alertPoolOvercommitted,
				DisplayName:      "Storage pool overcommitted",
				Description:      "Guests on a thin pool are provisioned beyond its real capacity. Legitimate in itself, and the usual reason a pool fills unexpectedly.",
				Kind:             plugin.AlertKindThreshold,
				TargetKind:       plugin.AlertTargetNone,
				Unit:             "%",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: 150,
				PollInterval:     alertPollInterval,
			},
			{
				Key:         alertHostLostKVM,
				DisplayName: "Host lost hardware virtualization",
				Description: "A host that could run virtual machines no longer can. Usually a kernel upgrade that did not rebuild the kvm module, or a firmware setting reset by a BIOS update.",
				// Duration rather than event: a host is briefly not-ready
				// during a reboot, and firing on the instant would page
				// somebody for every planned restart.
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertHostUnreachable,
				DisplayName:  "Compute host unreachable",
				Description:  "A host with compute enabled is not answering. Its guests may still be running — systemd supervises them — but nothing can be started, stopped or inspected until the agent reconnects.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertImageFetchFailed,
				DisplayName:  "Image download failed",
				Description:  "An image download failed or did not verify. A guest created from a partially downloaded image will not boot.",
				Kind:         plugin.AlertKindEvent,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
		},
	}
}

// alertSource answers the engine's evaluations.
type alertSource struct {
	plugin *ComputePlugin
}

var _ plugin.AlertSourceAdapterClient = (*alertSource)(nil)

// Evaluate reports the observations for one condition.
//
// It never fails the whole evaluation because one host could not be reached: an
// unreachable host is itself one of the conditions, and erroring out would let
// a single offline agent silence the alerts for every other host.
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

	environments, err := svc.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("compute: list hosts for alerting: %w", err)
	}

	var out []plugin.AlertObservation
	for _, env := range environments {
		if !env.ComputeEnabled {
			continue
		}
		name := env.Name
		if name == "" {
			name = env.EnvironmentID
		}

		switch req.ConditionKey {
		case alertHostUnreachable:
			out = append(out, plugin.AlertObservation{
				SubjectID:   env.EnvironmentID,
				SubjectName: name,
				Active:      !env.Available,
				Context: map[string]string{
					"reason": env.UnavailableReason,
					"detail": unreachableDetail(env.UnavailableReason),
				},
			})

		case alertHostLostKVM:
			// Only meaningful for a host that has been probed. A host that was
			// never able to run VMs did not "lose" anything, and firing on it
			// is noise the operator learns to ignore.
			if env.Capability == nil {
				continue
			}
			out = append(out, plugin.AlertObservation{
				SubjectID:   env.EnvironmentID,
				SubjectName: name,
				Active:      env.Available && !env.Capability.CanRunVMs,
				Context:     map[string]string{"detail": kvmDetail(env)},
			})

		case alertPoolNearFull, alertPoolOvercommitted:
			// A host that cannot be reached has no current capacity to report.
			// Reporting its last known numbers would be presenting stale data
			// as a live measurement.
			if !env.Available {
				continue
			}
			pools, err := svc.ListPools(ctx, env.EnvironmentID)
			if err != nil {
				continue
			}
			for _, pool := range pools {
				if pool.CapacityBytes <= 0 {
					continue
				}
				subject := name + "/" + pool.Name

				if req.ConditionKey == alertPoolNearFull {
					percent := float64(pool.UsedBytes) * 100 / float64(pool.CapacityBytes)
					out = append(out, plugin.AlertObservation{
						SubjectID:   pool.ID,
						SubjectName: subject,
						Value:       percent,
						ValueLabel:  fmt.Sprintf("%.0f%%", percent),
						Context: map[string]string{
							"pool":    pool.Name,
							"backend": pool.Backend,
							"host":    name,
						},
					})
					continue
				}

				provisioned := float64(pool.AllocatedBytes) * 100 / float64(pool.CapacityBytes)
				out = append(out, plugin.AlertObservation{
					SubjectID:   pool.ID,
					SubjectName: subject,
					Value:       provisioned,
					ValueLabel:  fmt.Sprintf("%.0f%%", provisioned),
					Context: map[string]string{
						"pool":    pool.Name,
						"backend": pool.Backend,
						"host":    name,
					},
				})
			}

		case alertImageFetchFailed:
			images, err := svc.ListImages(ctx, env.EnvironmentID, "")
			if err != nil {
				continue
			}
			for _, image := range images {
				if image.Cache == nil {
					continue
				}
				out = append(out, plugin.AlertObservation{
					SubjectID:   image.Cache.ID,
					SubjectName: name + "/" + image.DisplayName,
					Active:      image.Cache.State == CacheStateFailed,
					Context: map[string]string{
						"image": image.DisplayName,
						"host":  name,
						"error": image.Cache.LastError,
					},
				})
			}
		}
	}
	return out, nil
}

// unreachableDetail turns the reason code into a sentence with the fix in it.
func unreachableDetail(reason string) string {
	switch reason {
	case "not_agent_environment":
		return "This environment is not connected through an agent, so it cannot host guests."
	case "agent_too_old":
		return "This agent is too old to run guests. Update it."
	case "agent_offline":
		return "The agent is not connected. Guests already running are supervised by systemd and keep running, but nothing can be started or stopped until it returns."
	default:
		return ""
	}
}

// kvmDetail says what specifically went missing, taken from the probe's own
// evidence rather than restated here.
func kvmDetail(env ComputeEnvironment) string {
	if env.Capability == nil {
		return ""
	}
	for _, item := range env.Capability.Items {
		if item.Key == "kvm_device" && !item.Present {
			if item.Remediation != "" {
				return item.Detail + " " + item.Remediation
			}
			return item.Detail
		}
	}
	return "This host can no longer run virtual machines."
}
