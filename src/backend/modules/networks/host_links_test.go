// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import "testing"

// TestLocalLinkClassificationMatchesTheAgent keeps the native listing's kinds aligned with the
// agent's, because the same parentInterfaceKinds filter runs over both.
func TestLocalLinkClassificationMatchesTheAgent(t *testing.T) {
	cases := []struct {
		name, devtype string
		device        bool
		want          string
	}{
		{"eth0", "", true, "physical"},
		{"enp3s0", "", true, "physical"},
		{"wlan0", "wlan", true, "physical"},
		{"bond0", "bond", false, "bond"},
		{"eth0.20", "vlan", false, "vlan"},
		{"br0", "bridge", false, "bridge"},
		{"veth1a2b", "", false, "unknown"},
		{"lo", "", false, "loopback"},
	}
	for _, c := range cases {
		if got := classifyLink(c.name, c.devtype, c.device); got != c.want {
			t.Errorf("classifyLink(%q, %q, %v) = %q, want %q", c.name, c.devtype, c.device, got, c.want)
		}
	}
	if got := ueventDevtype("INTERFACE=eth0.20\nIFINDEX=7\nDEVTYPE=vlan\n"); got != "vlan" {
		t.Errorf("ueventDevtype = %q, want vlan", got)
	}
}

func TestDefaultRouteInterfaceReadsProcNetRoute(t *testing.T) {
	routes := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"docker0\t000011AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	if got := defaultRouteInterface(routes); got != "eth0" {
		t.Errorf("defaultRouteInterface = %q, want eth0", got)
	}
	if got := defaultRouteInterface("Iface\tDestination\n"); got != "" {
		t.Errorf("no default route reported as %q", got)
	}
}
