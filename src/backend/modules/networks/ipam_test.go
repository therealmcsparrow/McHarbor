// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"encoding/json"
	"testing"
)

// TestIPAMRequestDecodesWhatTheFormSends guards the macvlan "invalid request body" bug: the
// body must decode whatever the operator typed, and a bad field must surface as its own reason.
func TestIPAMRequestDecodesWhatTheFormSends(t *testing.T) {
	cases := []struct {
		name, cfg, reason string
	}{
		{"valid", `{"Subnet":"192.168.1.0/24","Gateway":"192.168.1.1","IPRange":"192.168.1.192/27"}`, ""},
		{"empty optional fields", `{"Subnet":"192.168.1.0/24","Gateway":"","IPRange":""}`, ""},
		{"surrounding spaces", `{"Subnet":" 192.168.1.0/24 ","Gateway":"192.168.1.1 "}`, ""},
		{"subnet without mask", `{"Subnet":"192.168.1.0"}`, "invalid_subnet"},
		{"gateway with mask", `{"Subnet":"192.168.1.0/24","Gateway":"192.168.1.1/24"}`, "invalid_gateway"},
		{"range as start-end", `{"Subnet":"192.168.1.0/24","IPRange":"192.168.1.200-192.168.1.250"}`, "invalid_ip_range"},
		{"range as a single ip", `{"Subnet":"192.168.1.0/24","IPRange":"192.168.1.200"}`, "invalid_ip_range"},
		{"gateway outside subnet", `{"Subnet":"192.168.1.0/24","Gateway":"10.0.0.1"}`, "gateway_outside_subnet"},
		{"range outside subnet", `{"Subnet":"192.168.1.0/24","IPRange":"192.168.0.0/16"}`, "ip_range_outside_subnet"},
		{"bad aux address", `{"Subnet":"192.168.1.0/24","AuxiliaryAddresses":{"host":"192.168.1.5/32"}}`, "invalid_aux_address"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var req CreateRequest
			if err := json.Unmarshal([]byte(`{"name":"lan","driver":"macvlan","ipam":{"Config":[`+c.cfg+`]}}`), &req); err != nil {
				t.Fatalf("body did not decode: %v", err)
			}
			ipam, err := req.IPAM.toSDK(false)
			if c.reason == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got := ipam.Config[0].Subnet.String(); got != "192.168.1.0/24" {
					t.Errorf("subnet = %q", got)
				}
				return
			}
			reason, ok := ipamRefusalReason(err)
			if !ok || reason != c.reason {
				t.Errorf("reason = %q (%v), want %q; err = %v", reason, ok, c.reason, err)
			}
		})
	}
}

// TestIPv6IPAM covers the dual-stack macvlan: a v6 block only with IPv6 enabled, and no block
// whose gateway, range or aux address is of the other family.
func TestIPv6IPAM(t *testing.T) {
	cases := []struct {
		name, cfg string
		enable    bool
		reason    string
	}{
		{"v6 subnet with ipv6 on", `{"Subnet":"2001:db8:1::/64","Gateway":"2001:db8:1::1"}`, true, ""},
		{"v6 subnet with ipv6 off", `{"Subnet":"2001:db8:1::/64"}`, false, "ipv6_subnet_needs_enable"},
		{"v4 gateway in v6 subnet", `{"Subnet":"2001:db8:1::/64","Gateway":"192.168.1.1"}`, true, "address_family_mismatch"},
		{"v6 range in v4 subnet", `{"Subnet":"192.168.1.0/24","IPRange":"2001:db8::/80"}`, true, "address_family_mismatch"},
		{"v6 aux in v4 subnet", `{"Subnet":"192.168.1.0/24","AuxiliaryAddresses":{"host":"2001:db8::5"}}`, true, "address_family_mismatch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var req CreateRequest
			if err := json.Unmarshal([]byte(`{"name":"lan","driver":"macvlan","ipam":{"Config":[`+c.cfg+`]}}`), &req); err != nil {
				t.Fatalf("body did not decode: %v", err)
			}
			_, err := req.IPAM.toSDK(c.enable)
			if c.reason == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if reason, ok := ipamRefusalReason(err); !ok || reason != c.reason {
				t.Errorf("reason = %q (%v), want %q; err = %v", reason, ok, c.reason, err)
			}
		})
	}
}
