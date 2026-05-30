// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	networkTypes "github.com/docker/docker/api/types/network"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

func TestIsSelfMcHarborContainerByName(t *testing.T) {
	if !coredocker.IsProtectedContainer([]string{"/mcharbor"}, "ghcr.io/therealmcsparrow/mcharbor:latest", nil) {
		t.Fatal("expected mcharbor container name to be detected")
	}
}

func TestIsSelfMcHarborContainerDoesNotMatchAgent(t *testing.T) {
	if coredocker.IsProtectedContainer([]string{"/mcharbor-agent"}, "ghcr.io/therealmcsparrow/mcharbor-agent:latest", nil) {
		t.Fatal("expected mcharbor-agent not to be detected as the app container")
	}
}

func TestReplacementNetworkingConfigDropsRuntimeEndpointFields(t *testing.T) {
	info := types.ContainerJSON{
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*networkTypes.EndpointSettings{
				"mcharbor_default": {
					IPAMConfig: &networkTypes.EndpointIPAMConfig{
						IPv4Address: "172.20.0.10",
					},
					Aliases:             []string{"agent"},
					NetworkID:           "network-id",
					EndpointID:          "endpoint-id",
					Gateway:             "172.20.0.1",
					IPAddress:           "172.20.0.5",
					IPPrefixLen:         16,
					IPv6Gateway:         "fd00::1",
					GlobalIPv6Address:   "fd00::5",
					GlobalIPv6PrefixLen: 64,
					MacAddress:          "02:42:ac:14:00:05",
					DriverOpts:          map[string]string{"com.docker.network.endpoint.sysctls": "net.ipv4.conf.IFNAME.log_martians=1"},
					GwPriority:          10,
				},
			},
		},
	}

	netConfig := replacementNetworkingConfig(info)
	if netConfig == nil {
		t.Fatal("expected networking config")
	}

	ep := netConfig.EndpointsConfig["mcharbor_default"]
	if ep == nil {
		t.Fatal("expected endpoint config")
	}
	if ep.NetworkID != "" || ep.EndpointID != "" || ep.Gateway != "" || ep.IPAddress != "" || ep.IPPrefixLen != 0 ||
		ep.IPv6Gateway != "" || ep.GlobalIPv6Address != "" || ep.GlobalIPv6PrefixLen != 0 {
		t.Fatalf("expected runtime endpoint fields to be omitted, got %#v", ep)
	}
	if ep.IPAMConfig == nil || ep.IPAMConfig.IPv4Address != "172.20.0.10" {
		t.Fatalf("expected static IPAM config to be preserved, got %#v", ep.IPAMConfig)
	}
	if ep.MacAddress != "02:42:ac:14:00:05" || ep.GwPriority != 10 {
		t.Fatalf("expected create-time endpoint options to be preserved, got %#v", ep)
	}
}

func TestNormalizeContainerRenameName(t *testing.T) {
	got := normalizeContainerRenameName("  /web-app  ")
	if got != "web-app" {
		t.Fatalf("expected leading slash and surrounding spaces to be removed, got %q", got)
	}
}

func TestIsValidContainerRenameName(t *testing.T) {
	valid := []string{"web", "web-app", "web_app.1", "App01"}
	for _, name := range valid {
		if !isValidContainerRenameName(name) {
			t.Fatalf("expected %q to be valid", name)
		}
	}

	invalid := []string{"", "-web", ".web", "web app", "web/app"}
	for _, name := range invalid {
		if isValidContainerRenameName(name) {
			t.Fatalf("expected %q to be invalid", name)
		}
	}
}
