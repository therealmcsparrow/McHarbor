// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

// Host condition keys.
const (
	conditionHostCPU     = "cpu"
	conditionHostMemory  = "memory"
	conditionLoad1       = "load1"
	conditionLoad5       = "load5"
	conditionLoad15      = "load15"
	conditionDockerDisk  = "docker_disk"
	conditionUnreachable = "unreachable"
)

// hostSourceSpec declares what the built-in host source can alert on.
// Every condition here is environment-scoped: the subject is the
// environment itself, not a container inside it.
func hostSourceSpec() alerting.AlertSourceAdapterSpec {
	return alerting.AlertSourceAdapterSpec{
		SourceType:  sourceHost,
		DisplayName: "Hosts & environments",
		Description: "Resource pressure and reachability of the machine behind an environment.",
		Conditions: []alerting.AlertConditionDecl{
			{
				Key:              conditionHostCPU,
				DisplayName:      "Host CPU usage",
				Description:      "CPU usage of the whole host, not a single container.",
				Kind:             alerting.KindThreshold,
				Unit:             "%",
				Operators:        []string{">", ">=", "<", "<="},
				DefaultOperator:  ">",
				DefaultThreshold: 85,
				SupportsFor:      true,
				TargetKind:       alerting.TargetEnvironment,
			},
			{
				Key:              conditionHostMemory,
				DisplayName:      "Host memory usage",
				Description:      "Memory usage of the whole host.",
				Kind:             alerting.KindThreshold,
				Unit:             "%",
				Operators:        []string{">", ">=", "<", "<="},
				DefaultOperator:  ">",
				DefaultThreshold: 85,
				SupportsFor:      true,
				TargetKind:       alerting.TargetEnvironment,
			},
			{
				Key:              conditionLoad1,
				DisplayName:      "Load average (1 min)",
				Description:      "One-minute load average. Compare against your core count.",
				Kind:             alerting.KindThreshold,
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: 4,
				SupportsFor:      true,
				TargetKind:       alerting.TargetEnvironment,
			},
			{
				Key:              conditionLoad5,
				DisplayName:      "Load average (5 min)",
				Description:      "Five-minute load average — steadier than the one-minute figure.",
				Kind:             alerting.KindThreshold,
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: 4,
				SupportsFor:      true,
				TargetKind:       alerting.TargetEnvironment,
			},
			{
				Key:              conditionLoad15,
				DisplayName:      "Load average (15 min)",
				Description:      "Fifteen-minute load average — sustained pressure only.",
				Kind:             alerting.KindThreshold,
				Operators:        []string{">", ">="},
				DefaultOperator:  ">",
				DefaultThreshold: 4,
				SupportsFor:      true,
				TargetKind:       alerting.TargetEnvironment,
			},
			{
				Key:              conditionDockerDisk,
				DisplayName:      "Docker disk usage",
				Description:      "Space used by Docker's storage driver, as a percentage of capacity or as an absolute size.",
				Kind:             alerting.KindThreshold,
				Unit:             "%",
				Operators:        []string{">", ">=", "<", "<="},
				DefaultOperator:  ">",
				DefaultThreshold: 80,
				SupportsFor:      true,
				TargetKind:       alerting.TargetEnvironment,
			},
			capacityConditionDecl(),
			{
				Key:         conditionUnreachable,
				DisplayName: "Environment unreachable",
				Description: "The Docker daemon or its remote agent stopped answering.",
				Kind:        alerting.KindDuration,
				TargetKind:  alerting.TargetEnvironment,
			},
		},
	}
}

// hostSource evaluates the built-in host conditions.
type hostSource struct {
	deps EngineDeps
}

func newHostSource(deps EngineDeps) *hostSource {
	return &hostSource{deps: deps}
}

// Evaluate implements alerting.AlertSourceAdapterClient.
func (s *hostSource) Evaluate(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	// Capacity is answered before the live-metrics guard: the projection is read
	// from the retained series, and a daemon that is down is exactly when
	// somebody wants to know how much room is left.
	if req.ConditionKey == conditionCapacityDays {
		return s.evaluateCapacity(ctx, req)
	}

	if s.deps.Metrics == nil {
		return nil, fmt.Errorf("host source: metrics unavailable")
	}

	// Reachability is the one condition where a failed host call is
	// the answer rather than a reason to skip the tick.
	if req.ConditionKey == conditionUnreachable {
		_, err := s.deps.Metrics.HostInfo(ctx, req.EnvironmentID)
		observation := alerting.AlertObservation{
			SubjectID:   req.EnvironmentID,
			SubjectName: req.EnvironmentName,
			Active:      err != nil,
		}
		if err != nil {
			observation.ValueLabel = "unreachable"
		}
		return []alerting.AlertObservation{observation}, nil
	}

	hostInfo, err := s.deps.Metrics.HostInfo(ctx, req.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("host info: %w", err)
	}

	observation := alerting.AlertObservation{
		SubjectID:   req.EnvironmentID,
		SubjectName: req.EnvironmentName,
	}

	switch req.ConditionKey {
	case conditionHostCPU:
		observation.Value = hostInfo.CPUPercent
		observation.ValueLabel = formatFloat(hostInfo.CPUPercent) + "%"
	case conditionHostMemory:
		observation.Value = hostInfo.MemPercent
		observation.ValueLabel = formatFloat(hostInfo.MemPercent) + "%"
	case conditionLoad1:
		observation.Value = hostInfo.Load1
		observation.ValueLabel = formatFloat(hostInfo.Load1)
	case conditionLoad5:
		observation.Value = hostInfo.Load5
		observation.ValueLabel = formatFloat(hostInfo.Load5)
	case conditionLoad15:
		observation.Value = hostInfo.Load15
		observation.ValueLabel = formatFloat(hostInfo.Load15)
	case conditionDockerDisk:
		return s.evaluateDockerDisk(ctx, req, hostInfo)
	default:
		return nil, fmt.Errorf("host source: unknown condition %q", req.ConditionKey)
	}

	return []alerting.AlertObservation{observation}, nil
}

// evaluateDockerDisk reports Docker's storage usage either as a
// percentage of capacity or as an absolute size, depending on the unit
// the operator chose.
//
// The unit changes the measurement rather than the comparison, which
// is why it travels on the request: "> 80%" and "> 10GB" are the same
// operator against two different numbers.
func (s *hostSource) evaluateDockerDisk(ctx context.Context, req alerting.EvaluateRequest, hostInfo *HostMetrics) ([]alerting.AlertObservation, error) {
	if s.deps.SystemInfo == nil {
		return nil, fmt.Errorf("host source: docker info unavailable")
	}

	unit := strings.ToLower(strings.TrimSpace(req.Unit))
	if divisor, ok := byteUnits[unit]; ok && divisor > 0 {
		value := float64(hostInfo.DiskTotal) / divisor
		return []alerting.AlertObservation{{
			SubjectID:   req.EnvironmentID,
			SubjectName: req.EnvironmentName,
			Value:       value,
			ValueLabel:  formatFloat(value) + " " + strings.ToUpper(unit),
		}}, nil
	}

	systemInfo, err := s.deps.SystemInfo.SystemInfo(ctx, req.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("docker info: %w", err)
	}

	capacityBytes, ok := resolveDiskCapacity(systemInfo, hostInfo.DiskTotal)
	if !ok || capacityBytes <= 0 {
		// Without a capacity there is no percentage to report. An
		// empty result is wrong here — it would read as "no subjects,
		// everything recovered" — so this is an error, which the
		// engine logs and skips.
		return nil, fmt.Errorf("docker disk capacity unavailable")
	}

	value := (float64(hostInfo.DiskTotal) / float64(capacityBytes)) * 100
	return []alerting.AlertObservation{{
		SubjectID:   req.EnvironmentID,
		SubjectName: req.EnvironmentName,
		Value:       value,
		ValueLabel:  formatFloat(value) + "%",
		Context: map[string]string{
			"used_bytes":     strconv.FormatInt(hostInfo.DiskTotal, 10),
			"capacity_bytes": strconv.FormatInt(capacityBytes, 10),
		},
	}}, nil
}

// resolveDiskCapacity reads Docker's storage driver status for a total
// capacity, falling back to used + available when the driver reports
// only the latter.
func resolveDiskCapacity(info *SystemInfo, usedBytes int64) (int64, bool) {
	if total, ok := readDriverStatusBytes(info, "data space total", "space total", "total space"); ok && total > 0 {
		return total, true
	}

	if available, ok := readDriverStatusBytes(info, "data space available", "space available", "available space"); ok && available >= 0 {
		return usedBytes + available, true
	}

	return 0, false
}

func readDriverStatusBytes(info *SystemInfo, keys ...string) (int64, bool) {
	if info == nil {
		return 0, false
	}

	lookup := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		lookup[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}

	for _, pair := range info.DriverStatus {
		if len(pair) < 2 {
			continue
		}

		label := strings.ToLower(strings.TrimSpace(pair[0]))
		if _, ok := lookup[label]; !ok {
			continue
		}

		parsed, ok := parseByteCount(pair[1])
		if ok && parsed > 0 {
			return parsed, true
		}
	}

	return 0, false
}

func parseByteCount(value string) (int64, bool) {
	match := comparisonConditionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) == 0 {
		return 0, false
	}

	amount, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return 0, false
	}

	unit := strings.ToLower(strings.TrimSpace(match[3]))
	divisor, ok := byteUnits[unit]
	if !ok {
		return 0, false
	}

	return int64(amount * divisor), true
}
