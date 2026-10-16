// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestSupportsHostLinksNeedsVersionAndBuild(t *testing.T) {
	cases := []struct {
		version, build string
		want           bool
	}{
		{"2026.09.01", "209912312359", false},
		{"2026.10.01", "", false},
		{"2026.10.01", "dev", false},
		{"2026.10.01", "202610080154", false},
		{"2026.10.01", HostLinksMinBuild, true},
		{"v2026.10.01", "202610091200", true},
		{"2026.10.02", "", true},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, c := range cases {
		if got := SupportsHostLinks(c.version, c.build); got != c.want {
			t.Errorf("SupportsHostLinks(%s, %q) = %v, want %v", c.version, c.build, got, c.want)
		}
		transport := NewAgentTransport(&AgentConnection{Version: c.version, Build: c.build}, nil, logger)
		// The transport reads the version through VersionEquals, which does not
		// strip a leading v; only the plain form must agree with SupportsHostLinks.
		if c.version[0] != 'v' && transport.HostLinksSupported() != c.want {
			t.Errorf("agent %s build %q: HostLinksSupported = %v, want %v", c.version, c.build, !c.want, c.want)
		}
	}
}

// oldHostLinkTransport is an agent that has compute and networking but
// predates host links, so every refusal below is the host-link floor's.
func oldHostLinkTransport() *AgentTransport {
	return NewAgentTransport(&AgentConnection{Version: "2026.10.01", Build: "202610080154"}, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHostLinkRefusesAnOldAgentWithoutSending(t *testing.T) {
	_, err := oldHostLinkTransport().HostLink(context.Background(), HostLinkPayload{Op: HostLinkOpEnsure, Owner: HostLinkOwnerDocker, OwnerID: "d1"})
	if !errors.Is(err, ErrHostLinksUnsupported) || !errors.Is(err, ErrComputeUnsupported) {
		t.Fatalf("err = %v; an agent that predates host links drops the request and must be refused up front", err)
	}
}

func TestNetEnsureRefusesHostLinkFieldsOnAnOldAgent(t *testing.T) {
	cases := map[string]ComputeNetPayload{
		"name":      {Mode: "macvlan", HostAccess: true, HostLinkName: "lan0"},
		"ipv6":      {Mode: "macvlan", HostAccess: true, IPv6Enabled: true},
		"ipv6 addr": {Mode: "macvlan", HostAccess: true, HostAccessIPv6: "fd00::2/64"},
	}
	for label, payload := range cases {
		err := oldHostLinkTransport().NetEnsure(context.Background(), payload)
		if !errors.Is(err, ErrHostLinksUnsupported) {
			t.Errorf("%s: NetEnsure = %v, want ErrHostLinksUnsupported", label, err)
		}
	}
}

func TestIPv4OnlyShimIsNotGatedOnHostLinks(t *testing.T) {
	// An IPv4-only shim worked on every agent before host links, so it must
	// not start being refused on the agents that already run it.
	if netUsesHostLinks(ComputeNetPayload{Mode: "macvlan", HostAccess: true, HostAccessIP: "192.168.1.2/24"}) {
		t.Fatal("an IPv4-only shim was classed as needing host links")
	}
}

func TestCTCommandRefusesStaticIPv6OnAnOldAgent(t *testing.T) {
	for _, nic := range []CTNICSpec{{StaticIPv6: "fd00::10/64"}, {StaticGW6: "fe80::1"}} {
		payload := CTPayload{Spec: &CTSpec{NICs: []CTNICSpec{{StaticIP: "192.168.1.10/24"}, nic}}}
		_, err := oldHostLinkTransport().CTCommand(context.Background(), MsgCTCreate, payload, time.Second)
		if !errors.Is(err, ErrHostLinksUnsupported) {
			t.Errorf("nic %+v: CTCommand = %v, want ErrHostLinksUnsupported", nic, err)
		}
	}
	if ctUsesHostLinks(CTPayload{Spec: &CTSpec{NICs: []CTNICSpec{{StaticIP: "192.168.1.10/24"}}}}) {
		t.Error("an IPv4-only container was classed as needing host links")
	}
}
