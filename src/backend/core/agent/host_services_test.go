// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestHostServicesGateNeedsVersionAndBuild(t *testing.T) {
	cases := []struct {
		version, build string
		want           bool
	}{
		{"3.8.0", "", false},
		{"2026.09.01", "", false},
		{"2026.09.01", "dev", false},
		{"2026.09.01", "202609271330", false},
		{"2026.09.01", HostServicesMinBuild, true},
		{"v2026.09.01", "202609301200", true},
		{"2026.10.01", "", true},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, c := range cases {
		transport := NewAgentTransport(&AgentConnection{Version: c.version, Build: c.build}, nil, logger)
		if got := transport.HostServicesSupported(); got != c.want {
			t.Errorf("agent %s build %q: HostServicesSupported = %v, want %v", c.version, c.build, got, c.want)
		}
	}
}

func TestHostServiceRefusesAnOldAgentWithoutSending(t *testing.T) {
	transport := NewAgentTransport(&AgentConnection{Version: "2026.09.01", Build: "202609250000"}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := transport.HostService(context.Background(), HostServicePayload{Op: HostServiceOpList}); err == nil {
		t.Fatal("an agent that predates host services would drop the request; it must be refused up front")
	}
}

func TestHostServicesCapabilityIsUnknownForAnOlderAgent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, known := NewAgentTransport(&AgentConnection{}, nil, logger).HostServicesCapable(); known {
		t.Error("an agent that reported no capabilities was read as having said no")
	}
	capable, known := NewAgentTransport(&AgentConnection{Capabilities: []string{HostServiceCapability}}, nil, logger).HostServicesCapable()
	if !capable || !known {
		t.Error("a reported capability was not read")
	}
	capable, known = NewAgentTransport(&AgentConnection{Capabilities: []string{}}, nil, logger).HostServicesCapable()
	if capable || !known {
		t.Error("an empty capability list must read as a known no")
	}
}
