// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

type diskFullMetrics struct{ host HostMetrics }

func (m diskFullMetrics) AllContainerStats(context.Context, string) ([]MetricSample, error) {
	return nil, nil
}

func (m diskFullMetrics) HostInfo(context.Context, string) (*HostMetrics, error) {
	h := m.host
	return &h, nil
}

type diskFullSystemInfo struct{ info SystemInfo }

func (s diskFullSystemInfo) SystemInfo(context.Context, string) (*SystemInfo, error) {
	i := s.info
	return &i, nil
}

// A root filesystem that already crossed the red line is the case an operator
// most needs the alert for. The capacity projection refuses it as "already_over",
// the source then emits no observation, and the engine reads silence as healthy.
func TestCapacityConditionReportsAFilesystemThatIsAlreadyOverTheRedLine(t *testing.T) {
	db := openCapacityAlertDB(t)
	seedCapacity(t, db, "full", 14, 80, 1.4)

	src := newHostSource(EngineDeps{DB: db})
	obs, err := src.Evaluate(context.Background(), capacityRequest())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("a filesystem at ~98%% produced %d observations, want 1 (a full disk must not read as healthy): %+v", len(obs), obs)
	}
	if obs[0].Value > 14 {
		t.Fatalf("value = %v days, want 0 or less for a disk past the red line", obs[0].Value)
	}
}

// docker_disk is the only instantaneous disk condition, and on overlay2 (every
// native install) the daemon reports no "data space total", so the percentage can
// never be computed and the rule never evaluates. The engine logs the error and
// skips the tick, which leaves the operator with a rule that looks armed.
func TestDockerDiskPercentEvaluatesOnOverlay2(t *testing.T) {
	src := newHostSource(EngineDeps{
		Metrics: diskFullMetrics{host: HostMetrics{DiskTotal: 40 << 30}},
		HostFS:  fakeHostFS{usage: HostFSUsage{Path: "/", Total: 100 << 30, Used: 40 << 30}},
		SystemInfo: diskFullSystemInfo{info: SystemInfo{DriverStatus: [][]string{
			{"Backing Filesystem", "extfs"},
			{"Supports d_type", "true"},
			{"Native Overlay Diff", "true"},
		}}},
	})
	obs, err := src.Evaluate(context.Background(), alerting.EvaluateRequest{
		ConditionKey:    conditionDockerDisk,
		EnvironmentID:   "env1",
		EnvironmentName: "Production",
		Unit:            "%",
	})
	if err != nil {
		t.Fatalf("docker_disk %% on overlay2 returned an error and the tick is skipped: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("got %d observations, want 1", len(obs))
	}
}
