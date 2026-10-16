// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"io"
	"log/slog"
	"testing"
)

func TestStatsPushGateNeedsVersionAndBuild(t *testing.T) {
	cases := []struct {
		version, build string
		want           bool
	}{
		{"3.8.0", "", false},
		{"2026.09.01", "", false},
		{"2026.09.01", "202609250000", false},
		{"2026.09.01", StatsPushMinBuild, true},
		{"2026.09.01", "202609301200", true},
		{"2026.10.01", "", true},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, c := range cases {
		transport := NewAgentTransport(&AgentConnection{Version: c.version, Build: c.build}, nil, logger)
		if got := transport.StatsPushSupported(); got != c.want {
			t.Errorf("agent %s build %q: StatsPushSupported = %v, want %v", c.version, c.build, got, c.want)
		}
	}
}

func TestSubscribeStatsRefusesAnOldAgent(t *testing.T) {
	transport := NewAgentTransport(&AgentConnection{Version: "3.8.0"}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := transport.SubscribeStats(0); err == nil {
		t.Fatal("subscribing an agent that predates stats push must fail rather than send a message it drops")
	}
}
