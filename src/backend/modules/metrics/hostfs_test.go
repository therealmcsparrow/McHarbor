// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"errors"
	"testing"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

type fakeFSBackend struct {
	envID      string
	kindOf     hostKind
	kindErr    error
	stats      coreagent.HostStatsPayload
	statsErr   error
	localTotal int64
	localUsed  int64
	localOK    bool
	localCalls int
	agentCalls int
	unknownEnv bool
}

func (f *fakeFSBackend) exists(context.Context, string) (bool, error) { return !f.unknownEnv, nil }

func (f *fakeFSBackend) resolve(_ context.Context, envID string) (string, error) {
	if f.envID != "" {
		return f.envID, nil
	}
	return envID, nil
}

func (f *fakeFSBackend) kind(context.Context, string) (hostKind, error) { return f.kindOf, f.kindErr }

func (f *fakeFSBackend) agentStats(context.Context, string) (coreagent.HostStatsPayload, error) {
	f.agentCalls++
	return f.stats, f.statsErr
}

func (f *fakeFSBackend) localRoot() (int64, int64, bool) {
	f.localCalls++
	return f.localTotal, f.localUsed, f.localOK
}

func fsService(b *fakeFSBackend) *Service { return &Service{fsb: b} }

func TestHostFSAgentEnvironmentNeverReadsMcHarborsOwnDisk(t *testing.T) {
	b := &fakeFSBackend{kindOf: hostKindAgent, localTotal: 100, localUsed: 1, localOK: true,
		stats: coreagent.HostStatsPayload{Supported: true, FSTotal: 1000, FSUsed: 980}}
	got, err := fsService(b).HostFS(context.Background(), "agent-env")
	if err != nil {
		t.Fatalf("HostFS: %v", err)
	}
	if b.localCalls != 0 {
		t.Fatalf("an agent environment read McHarbor's own filesystem %d time(s)", b.localCalls)
	}
	if got.Total != 1000 || got.Used != 980 || got.Percent != 98 {
		t.Fatalf("got %+v, want the agent host's 980/1000", got)
	}
}

func TestHostFSLocalEnvironmentReadsLocalRoot(t *testing.T) {
	b := &fakeFSBackend{kindOf: hostKindLocal, localTotal: 200, localUsed: 50, localOK: true}
	got, err := fsService(b).HostFS(context.Background(), "local")
	if err != nil || got.Total != 200 || got.Used != 50 || got.Percent != 25 || got.Path != "/" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if b.agentCalls != 0 {
		t.Fatal("a local environment asked an agent")
	}
}

func TestHostFSFailuresAreErrorsNeverZeroUsage(t *testing.T) {
	cases := []struct {
		name string
		b    *fakeFSBackend
		want error
	}{
		{"agent offline", &fakeFSBackend{kindOf: hostKindAgent, statsErr: ErrHostFSAgentOffline}, ErrHostFSAgentOffline},
		{"agent too old", &fakeFSBackend{kindOf: hostKindAgent, statsErr: ErrHostFSAgentTooOld}, ErrHostFSAgentTooOld},
		{"agent unsupported payload", &fakeFSBackend{kindOf: hostKindAgent, stats: coreagent.HostStatsPayload{Supported: false}}, ErrHostFSAgentTooOld},
		{"agent zero total", &fakeFSBackend{kindOf: hostKindAgent, stats: coreagent.HostStatsPayload{Supported: true}}, ErrHostFSReadFailed},
		{"local unreadable", &fakeFSBackend{kindOf: hostKindLocal}, ErrHostFSReadFailed},
		{"local zero total", &fakeFSBackend{kindOf: hostKindLocal, localOK: true}, ErrHostFSReadFailed},
		{"remote tcp host", &fakeFSBackend{kindOf: hostKindOther}, ErrHostFSUnsupported},
		{"kind lookup fails", &fakeFSBackend{kindErr: errors.New("db")}, ErrHostFSReadFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := fsService(c.b).HostFS(context.Background(), "e")
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if got != (HostFSUsage{}) {
				t.Fatalf("a failed read returned usage %+v", got)
			}
		})
	}
}

func TestHostFSReasonClassifiesEverySentinel(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrHostFSAgentOffline, HostFSReasonAgentOffline},
		{ErrHostFSAgentTooOld, HostFSReasonAgentTooOld},
		{ErrHostFSReadFailed, HostFSReasonReadFailed},
		{errors.New("other"), HostFSReasonReadFailed},
	}
	for _, c := range cases {
		if got := hostFSReason(c.err); got != c.want {
			t.Errorf("hostFSReason(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
