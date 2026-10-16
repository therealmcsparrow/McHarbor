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
const (
	alertVMCrashed             = "vm_crashed"
	alertVMStoppedUnexpectedly = "vm_stopped_unexpectedly"
	alertVMUnknownState        = "vm_unknown_state"
	alertVMSnapshotChain       = "vm_snapshot_chain_deep"
	alertVMGuestAgentGone      = "vm_guest_agent_gone"
	alertVMAutostartFailed     = "vm_autostart_failed"
)

const (
	alertPollInterval = 60 * time.Second
	alertEvalTimeout  = 45 * time.Second
)

// kvmAlertSpec declares what this plugin can alert on.
//
// Subjects are named `host/guest`, which is what a rule's target pattern
// matches on — so `prod/*`, `*/db-*` and `prod/api-*` all work with the
// wildcard matcher the host already has.
func kvmAlertSpec() plugin.AlertSourceAdapterSpec {
	return plugin.AlertSourceAdapterSpec{
		SourceType:  "kvm",
		DisplayName: "Virtual Machines",
		Slug:        "addon-kvm",
		Icon:        "plugin.svg",
		Description: "Guest health, snapshot chain depth and guest agent availability.",
		Conditions: []plugin.AlertConditionDecl{
			{
				Key:         alertVMCrashed,
				DisplayName: "Virtual machine crashed",
				Description: "A guest exited abnormally — the OOM killer took its QEMU process, it panicked, or its storage returned an IO error. The reason is read from the host's journal and carried in the message.",
				// An event: a crash is a moment, not a state to wait out, and
				// the operator wants to know immediately.
				Kind:         plugin.AlertKindEvent,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertVMStoppedUnexpectedly,
				DisplayName:  "Virtual machine stopped unexpectedly",
				Description:  "A guest that should be running is stopped, and nobody asked it to stop through McHarbor.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
			{
				Key:         alertVMUnknownState,
				DisplayName: "Virtual machine state unknown",
				Description: "A guest's host cannot be reached, so its state is genuinely unknown. Its guests are supervised by systemd and are probably still running, but nothing can confirm that until the agent returns.",
				// Duration, not event: an agent restart or a brief network
				// blip should not page anyone.
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
			{
				Key:              alertVMSnapshotChain,
				DisplayName:      "Snapshot chain too deep",
				Description:      "A guest has accumulated snapshots. On a directory pool each one adds a link to a read chain, and a deep chain is the classic reason a virtual machine becomes mysteriously slow.",
				Kind:             plugin.AlertKindThreshold,
				TargetKind:       plugin.AlertTargetNone,
				Unit:             "snapshots",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: warnAtChain,
				PollInterval:     alertPollInterval,
			},
			{
				Key:          alertVMGuestAgentGone,
				DisplayName:  "Guest agent not responding",
				Description:  "A running guest's QEMU guest agent is not answering. Snapshots of it fall back to crash-consistent, and its reported addresses stop updating.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				SupportsFor:  true,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertVMAutostartFailed,
				DisplayName:  "Autostart guest is not running",
				Description:  "A guest marked to start automatically is not running. Usually noticed after a host reboot.",
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
	plugin *KVMPlugin
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

	guests, err := svc.List(ctx, "", false)
	if err != nil {
		return nil, fmt.Errorf("kvm: list guests for alerting: %w", err)
	}

	var out []plugin.AlertObservation
	for _, vm := range guests {
		// Templates are not running and are not supposed to be. Alerting on
		// them would be noise on every condition here.
		if vm.IsTemplate {
			continue
		}
		subject := vm.EnvironmentID + "/" + vm.Name

		switch req.ConditionKey {
		case alertVMCrashed:
			out = append(out, plugin.AlertObservation{
				SubjectID:   vm.ID,
				SubjectName: subject,
				Active:      vm.State == StateCrashed,
				Context: map[string]string{
					"vm": vm.Name, "host": vm.EnvironmentID,
					// The journal's own reason, which is what turns "the VM
					// died" into "the OOM killer took it".
					"reason": vm.LastError,
				},
			})

		case alertVMStoppedUnexpectedly:
			// Only meaningful for a guest that has run before. A guest defined
			// and never started is not one that stopped.
			if vm.LastStartedAt == nil {
				continue
			}
			out = append(out, plugin.AlertObservation{
				SubjectID:   vm.ID,
				SubjectName: subject,
				Active:      vm.HostAvailable && vm.State == StateStopped,
				Context:     map[string]string{"vm": vm.Name, "host": vm.EnvironmentID},
			})

		case alertVMUnknownState:
			out = append(out, plugin.AlertObservation{
				SubjectID:   vm.ID,
				SubjectName: subject,
				Active:      !vm.HostAvailable,
				Context: map[string]string{
					"vm": vm.Name, "host": vm.EnvironmentID,
					"reason": vm.UnavailableReason,
				},
			})

		case alertVMSnapshotChain:
			snapshots, err := svc.Snapshots(ctx, vm.ID)
			if err != nil {
				continue
			}
			out = append(out, plugin.AlertObservation{
				SubjectID:   vm.ID,
				SubjectName: subject,
				Value:       float64(len(snapshots)),
				ValueLabel:  fmt.Sprintf("%d", len(snapshots)),
				Context: map[string]string{
					"vm": vm.Name, "host": vm.EnvironmentID,
					// The backend matters: a CoW pool does not suffer from
					// chain depth the way a directory pool does.
					"pool": poolBackendOf(vm),
				},
			})

		case alertVMGuestAgentGone:
			// Only for a running guest that has an agent to lose. A guest
			// created without one has not lost anything.
			if !vm.Running() || vm.GuestAgentState == "" {
				continue
			}
			out = append(out, plugin.AlertObservation{
				SubjectID:   vm.ID,
				SubjectName: subject,
				Active:      vm.GuestAgentState == "absent",
				Context:     map[string]string{"vm": vm.Name, "host": vm.EnvironmentID},
			})

		case alertVMAutostartFailed:
			if !vm.Autostart {
				continue
			}
			out = append(out, plugin.AlertObservation{
				SubjectID:   vm.ID,
				SubjectName: subject,
				Active:      vm.HostAvailable && !vm.Running(),
				Context: map[string]string{
					"vm": vm.Name, "host": vm.EnvironmentID, "state": vm.State,
				},
			})
		}
	}
	return out, nil
}

// poolBackendOf names the storage a guest's boot disk sits on.
func poolBackendOf(vm VM) string {
	for _, disk := range vm.Disks {
		if disk.IsBoot {
			return disk.PoolBackend
		}
	}
	if len(vm.Disks) > 0 {
		return vm.Disks[0].PoolBackend
	}
	return ""
}
