// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"slices"
	"testing"

	"github.com/moby/moby/api/types/container"
)

// TestComposeCreateTimeRules is Q20 for compose: each create-time key is reported for the
// service that sets it on McHarbor's host, and a service setting none of them is clean.
func TestComposeCreateTimeRules(t *testing.T) {
	compose := `services:
  dev:
    image: alpine
    devices: ["/dev/sda:/dev/sda"]
  apparmor:
    image: alpine
    security_opt: ["apparmor=unconfined"]
  label:
    image: alpine
    security_opt: ["label:disable"]
  userns:
    image: alpine
    userns_mode: host
  ipc:
    image: alpine
    ipc: host
  caps:
    image: alpine
    cap_add: [NET_ADMIN]
  hostnet:
    image: alpine
    network_mode: host
  plain:
    image: alpine
    security_opt: ["no-new-privileges:true"]
    network_mode: bridge
    ipc: shareable
`
	got, err := ComposeHostAccessReasons([]byte(compose), t.TempDir(), nil, SelfHost{Hosted: true})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"dev": HostAccessDevices, "apparmor": HostAccessSecurityOpt, "label": HostAccessSecurityOpt,
		"userns": HostAccessUsernsHost, "ipc": HostAccessIPCHost, "caps": HostAccessCapAdd, "hostnet": HostAccessNetworkHost,
	}
	seen := map[string]bool{}
	for _, f := range got {
		if f.Service == "plain" {
			t.Errorf("plain service reported %v", f.Reasons)
		}
		if reason, ok := want[f.Service]; ok {
			seen[f.Service] = true
			if !slices.Contains(f.Reasons, reason) {
				t.Errorf("service %s reasons %v; want %s", f.Service, f.Reasons, reason)
			}
		}
	}
	for svc := range want {
		if !seen[svc] {
			t.Errorf("service %s not reported", svc)
		}
	}
}

// TestCreateTimeReasonsLeaveSysAdminToItsOwnCode keeps the two capability codes apart, so an
// operator reading the refusal sees cap_sys_admin for the one that is root on the host.
func TestCreateTimeReasonsLeaveSysAdminToItsOwnCode(t *testing.T) {
	if got := CreateTimeReasons(&container.HostConfig{CapAdd: []string{"CAP_SYS_ADMIN"}}); len(got) != 0 {
		t.Errorf("SYS_ADMIN alone: %v; want nothing beyond cap_sys_admin", got)
	}
	if got := CreateTimeReasons(&container.HostConfig{CapAdd: []string{"SYS_ADMIN", "cap_net_raw"}}); !slices.Equal(got, []string{HostAccessCapAdd}) {
		t.Errorf("SYS_ADMIN and NET_RAW: %v; want [cap_add]", got)
	}
	if got := CreateTimeReasons(&container.HostConfig{}); len(got) != 0 {
		t.Errorf("empty host config: %v", got)
	}
}
