// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"fmt"
	"strconv"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

// Host filesystem condition keys.
const (
	conditionHostFSPercent = "host_fs_percent"
	conditionHostFSStale   = "host_fs_stale"
)

func hostFSPercentDecl() alerting.AlertConditionDecl {
	return alerting.AlertConditionDecl{
		Key:              conditionHostFSPercent,
		DisplayName:      "Host filesystem usage",
		Description:      "How full the filesystem under the host is, read without asking Docker. A host whose disk cannot be read skips the check; pair it with Host disk data missing.",
		Kind:             alerting.KindThreshold,
		Unit:             "%",
		Operators:        []string{">", ">=", "<", "<="},
		DefaultOperator:  ">=",
		DefaultThreshold: 85,
		SupportsFor:      true,
		TargetKind:       alerting.TargetEnvironment,
	}
}

func hostFSStaleDecl() alerting.AlertConditionDecl {
	return alerting.AlertConditionDecl{
		Key:         conditionHostFSStale,
		DisplayName: "Host disk data missing",
		Description: "McHarbor has had no usable disk reading for this host, so the disk alerts cannot fire. Counts from the last good reading.",
		Kind:        alerting.KindDuration,
		TargetKind:  alerting.TargetEnvironment,
	}
}

// evaluateHostFSPercent reports used/total of the host filesystem. A failed read or
// a zero total is an error rather than 0%, because the engine skips the tick on an
// error and "unknown" must never read as healthy.
func (s *hostSource) evaluateHostFSPercent(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	if s.deps.HostFS == nil {
		return nil, fmt.Errorf("host filesystem source unavailable")
	}
	usage, err := s.deps.HostFS.HostFS(ctx, req.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("host filesystem: %w", err)
	}
	if usage.Total <= 0 || usage.Used < 0 {
		return nil, fmt.Errorf("host filesystem reported total %d used %d", usage.Total, usage.Used)
	}
	value := float64(usage.Used) / float64(usage.Total) * 100
	return []alerting.AlertObservation{{
		SubjectID:   req.EnvironmentID,
		SubjectName: req.EnvironmentName,
		Value:       value,
		ValueLabel:  formatFloat(value) + "%",
		Context: map[string]string{
			"path":        usage.Path,
			"used_bytes":  strconv.FormatInt(usage.Used, 10),
			"total_bytes": strconv.FormatInt(usage.Total, 10),
		},
	}}, nil
}

// evaluateHostFSStale fires while no usable disk reading exists. It asks the
// freshness tracker when one is wired, and otherwise derives the answer from a
// live read, so deleting the percentage rules never silences it.
func (s *hostSource) evaluateHostFSStale(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	observation := alerting.AlertObservation{SubjectID: req.EnvironmentID, SubjectName: req.EnvironmentName}

	var health HostFSHealth
	switch {
	case s.deps.HostFSHealth != nil:
		var err error
		if health, err = s.deps.HostFSHealth.HostFSHealth(ctx, req.EnvironmentID); err != nil {
			return nil, fmt.Errorf("host filesystem health: %w", err)
		}
	case s.deps.HostFS != nil:
		if _, err := s.deps.HostFS.HostFS(ctx, req.EnvironmentID); err != nil {
			health = HostFSHealth{State: HostFSStateStale, Reason: "read_failed"}
		} else {
			health = HostFSHealth{State: HostFSStateOK}
		}
	default:
		return nil, fmt.Errorf("host filesystem source unavailable")
	}

	switch health.State {
	case HostFSStateUnsupported:
		return nil, nil
	case HostFSStateStale, HostFSStateUnknown:
		observation.Active = true
		observation.Since = health.LastSeen
		observation.ValueLabel = "no disk data"
		observation.Context = map[string]string{"reason": health.Reason}
		if !health.LastSeen.IsZero() {
			observation.Context["last_seen"] = health.LastSeen.UTC().Format("2006-01-02 15:04 UTC")
		}
	}
	return []alerting.AlertObservation{observation}, nil
}
