// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

type fakeHostFS struct {
	usage HostFSUsage
	err   error
}

func (f fakeHostFS) HostFS(context.Context, string) (HostFSUsage, error) { return f.usage, f.err }

type fakeHostFSHealth struct {
	health HostFSHealth
	err    error
}

func (f fakeHostFSHealth) HostFSHealth(context.Context, string) (HostFSHealth, error) {
	return f.health, f.err
}

func hostFSRequest(key string) alerting.EvaluateRequest {
	return alerting.EvaluateRequest{ConditionKey: key, EnvironmentID: "env1", EnvironmentName: "Production"}
}

func TestHostFSPercentReportsUsedShare(t *testing.T) {
	src := newHostSource(EngineDeps{HostFS: fakeHostFS{usage: HostFSUsage{Path: "/", Total: 200, Used: 190}}})
	obs, err := src.Evaluate(context.Background(), hostFSRequest(conditionHostFSPercent))
	if err != nil || len(obs) != 1 {
		t.Fatalf("obs=%v err=%v", obs, err)
	}
	if obs[0].Value != 95 || obs[0].Context["path"] != "/" || obs[0].Context["total_bytes"] != "200" {
		t.Fatalf("unexpected observation %+v", obs[0])
	}
}

func TestHostFSPercentIsAnErrorNeverZero(t *testing.T) {
	for name, deps := range map[string]EngineDeps{
		"no source":    {},
		"read failed":  {HostFS: fakeHostFS{err: errors.New("agent offline")}},
		"total is 0":   {HostFS: fakeHostFS{usage: HostFSUsage{Total: 0, Used: 0}}},
		"negative use": {HostFS: fakeHostFS{usage: HostFSUsage{Total: 100, Used: -1}}},
	} {
		obs, err := newHostSource(deps).Evaluate(context.Background(), hostFSRequest(conditionHostFSPercent))
		if err == nil {
			t.Fatalf("%s: got observations %+v, want an error so the tick is skipped", name, obs)
		}
	}
}

func TestHostFSStaleFiresWithLastSeenAsSince(t *testing.T) {
	seen := time.Now().Add(-24 * time.Hour).UTC()
	src := newHostSource(EngineDeps{HostFSHealth: fakeHostFSHealth{health: HostFSHealth{
		State: HostFSStateStale, LastSeen: seen, Reason: "agent_offline",
	}}})
	obs, err := src.Evaluate(context.Background(), hostFSRequest(conditionHostFSStale))
	if err != nil || len(obs) != 1 {
		t.Fatalf("obs=%v err=%v", obs, err)
	}
	if !obs[0].Active || !obs[0].Since.Equal(seen) || obs[0].Context["reason"] != "agent_offline" {
		t.Fatalf("unexpected observation %+v", obs[0])
	}
}

func TestHostFSStaleStatesMapToActivity(t *testing.T) {
	cases := map[string]struct {
		state  string
		count  int
		active bool
	}{
		"unknown fires":        {HostFSStateUnknown, 1, true},
		"ok recovers":          {HostFSStateOK, 1, false},
		"unsupported is quiet": {HostFSStateUnsupported, 0, false},
	}
	for name, c := range cases {
		src := newHostSource(EngineDeps{HostFSHealth: fakeHostFSHealth{health: HostFSHealth{State: c.state}}})
		obs, err := src.Evaluate(context.Background(), hostFSRequest(conditionHostFSStale))
		if err != nil || len(obs) != c.count || (c.count == 1 && obs[0].Active != c.active) {
			t.Fatalf("%s: obs=%+v err=%v", name, obs, err)
		}
	}
}

func TestHostFSStaleWithoutHealthSourceUsesHostFS(t *testing.T) {
	src := newHostSource(EngineDeps{HostFS: fakeHostFS{err: errors.New("down")}})
	obs, err := src.Evaluate(context.Background(), hostFSRequest(conditionHostFSStale))
	if err != nil || len(obs) != 1 || !obs[0].Active {
		t.Fatalf("a failed read must be an active stale observation: obs=%+v err=%v", obs, err)
	}
	if _, err := newHostSource(EngineDeps{}).Evaluate(context.Background(), hostFSRequest(conditionHostFSStale)); err == nil {
		t.Fatal("no source at all must be an error, not silence")
	}
}

func TestDockerDiskPercentWithoutAnyCapacityIsAnError(t *testing.T) {
	src := newHostSource(EngineDeps{
		Metrics:    diskFullMetrics{host: HostMetrics{DiskTotal: 40 << 30}},
		SystemInfo: diskFullSystemInfo{info: SystemInfo{}},
		HostFS:     fakeHostFS{err: errors.New("down")},
	})
	req := hostFSRequest(conditionDockerDisk)
	req.Unit = "%"
	if _, err := src.Evaluate(context.Background(), req); err == nil {
		t.Fatal("no driver capacity and no host filesystem must be an error")
	}
}

func TestDockerDiskPercentUsesHostFSTotalOnOverlay2(t *testing.T) {
	src := newHostSource(EngineDeps{
		Metrics:    diskFullMetrics{host: HostMetrics{DiskTotal: 10 << 30}},
		SystemInfo: diskFullSystemInfo{info: SystemInfo{}},
		HostFS:     fakeHostFS{usage: HostFSUsage{Total: 40 << 30, Used: 39 << 30}},
	})
	req := hostFSRequest(conditionDockerDisk)
	req.Unit = "%"
	obs, err := src.Evaluate(context.Background(), req)
	if err != nil || len(obs) != 1 {
		t.Fatalf("obs=%v err=%v", obs, err)
	}
	if obs[0].Value != 25 || obs[0].Context["capacity_source"] != "host_fs" {
		t.Fatalf("unexpected observation %+v", obs[0])
	}
}
