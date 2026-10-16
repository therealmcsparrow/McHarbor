// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateNICIPv6(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		ip6, gw6 string
		wantCode string
	}{
		{"nothing set on a bridge", "", "", "", ""},
		{"nothing set on a macvlan", "macvlan", "", "", ""},
		{"v6 cidr accepted", "macvlan", "2001:db8::50/64", "", ""},
		{"v6 cidr with a gateway", "macvlan", "2001:db8::50/64", "2001:db8::1", ""},
		{"link-local gateway outside the prefix", "macvlan", "2001:db8::50/64", "fe80::1", ""},
		{"v4 cidr in the address field", "macvlan", "10.0.0.5/24", "", codeNICIPv6Invalid},
		{"v4-mapped address", "macvlan", "::ffff:10.0.0.5/120", "", codeNICIPv6Invalid},
		{"address without a prefix length", "macvlan", "2001:db8::50", "", codeNICIPv6Invalid},
		{"garbage", "macvlan", "not-an-address", "", codeNICIPv6Invalid},
		{"v4 gateway", "macvlan", "2001:db8::50/64", "10.0.0.1", codeNICIPv6Invalid},
		{"gateway with a zone", "macvlan", "2001:db8::50/64", "fe80::1%eth0", codeNICIPv6Invalid},
		{"gateway without an address", "macvlan", "", "2001:db8::1", codeNICIPv6Invalid},
		{"bridge attachment refused", "", "2001:db8::50/64", "", codeNICIPv6Macvlan},
		{"explicit bridge attachment refused", "bridge", "2001:db8::50/64", "2001:db8::1", codeNICIPv6Macvlan},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNICIPv6(tc.kind, tc.ip6, tc.gw6)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("validateNICIPv6 = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateNICIPv6 accepted %q / %q on %q", tc.ip6, tc.gw6, tc.kind)
			}
			if got := requestCode(err); got != tc.wantCode {
				t.Fatalf("code = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

func TestCreateRefusesAV4ValueInTheV6FieldWithItsOwnCode(t *testing.T) {
	req := createRequest()
	req.NICs = []CreateNICRequest{{
		NetworkID: "net-a", AttachKind: "macvlan", ParentLink: "eth0", StaticIPv6: "10.0.0.5/24",
	}}
	err := validateCreate(req)
	if err == nil || requestCode(err) != codeNICIPv6Invalid {
		t.Fatalf("validateCreate = %v (code %q), want %s", err, requestCode(err), codeNICIPv6Invalid)
	}
}

func TestAddNICHandlerAnswersWithTheIPv6Code(t *testing.T) {
	h := &Handler{logger: quietLogger{}}
	rec := httptest.NewRecorder()
	h.respondErr(rec, httptest.NewRequest(http.MethodPost, "/", nil), "add an interface",
		validateNICIPv6("", "2001:db8::50/64", ""))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), codeNICIPv6Macvlan) {
		t.Fatalf("status %d body %s, want 400 with %s", rec.Code, rec.Body.String(), codeNICIPv6Macvlan)
	}
}

func TestAgentNICCarriesTheIPv6PairUnderTheWireKeys(t *testing.T) {
	raw, err := json.Marshal(agentNIC{Slot: 1, MAC: "aa", StaticIPv6: "2001:db8::50/64", StaticGW6: "2001:db8::1"})
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if keys["staticIpv6"] != "2001:db8::50/64" || keys["staticGw6"] != "2001:db8::1" {
		t.Fatalf("wire keys = %s", raw)
	}
	bare, _ := json.Marshal(agentNIC{Slot: 1, MAC: "aa"})
	if strings.Contains(string(bare), "Ipv6") || strings.Contains(string(bare), "Gw6") {
		t.Fatalf("empty v6 fields must be omitted: %s", bare)
	}
}

func TestCreateStoresTheIPv6PairAndSendsItToTheAgentOnDHCPv4(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	req := createRequest()
	req.NICs = []CreateNICRequest{{
		NetworkID: "net-a", AttachKind: "macvlan", ParentLink: "eth0",
		AddressMode: "dhcp", StaticIPv6: "2001:db8::50/64", StaticGW6: "2001:db8::1",
	}}
	ct, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	nics, err := svc.NICs(context.Background(), ct.ID)
	if err != nil || len(nics) != 1 {
		t.Fatalf("NICs = %+v, %v", nics, err)
	}
	if nics[0].StaticIPv6 != "2001:db8::50/64" || nics[0].StaticGW6 != "2001:db8::1" || nics[0].AddressMode != "dhcp" {
		t.Fatalf("stored nic = %+v", nics[0])
	}
	sent, ok := rpc.last(msgCTCreate)
	if !ok || sent.Spec == nil || len(sent.Spec.NICs) != 1 {
		t.Fatalf("ct_create = %+v", sent)
	}
	if sent.Spec.NICs[0].StaticIPv6 != "2001:db8::50/64" || sent.Spec.NICs[0].StaticGW6 != "2001:db8::1" {
		t.Fatalf("spec nic = %+v", sent.Spec.NICs[0])
	}
}

func TestAddNICRefusesTheIPv6PairOnABridge(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ct, err := svc.Create(context.Background(), createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = svc.AddNIC(context.Background(), ct.ID, CreateNICRequest{
		NetworkID: "net-a", Bridge: "br0", StaticIPv6: "2001:db8::50/64",
	})
	if err == nil || requestCode(err) != codeNICIPv6Macvlan {
		t.Fatalf("AddNIC = %v, want %s", err, codeNICIPv6Macvlan)
	}
}

func TestBackupRecreateKeepsTheIPv6Pair(t *testing.T) {
	manifest, err := parseBackupManifest([]byte(`{"version": 1, "name": "web", "environmentId": "env-a",
		"nics": [{"id": "n1", "slot": 0, "networkId": "net-a", "attachKind": "macvlan", "parentLink": "eth0",
		"guestName": "eth0", "mac": "bb", "addressMode": "dhcp",
		"staticIpv6": "2001:db8::50/64", "staticGw6": "2001:db8::1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	create := draftFromManifest(manifest).Create
	if len(create.NICs) != 1 || create.NICs[0].StaticIPv6 != "2001:db8::50/64" || create.NICs[0].StaticGW6 != "2001:db8::1" {
		t.Fatalf("recreate nics = %+v", create.NICs)
	}
}
