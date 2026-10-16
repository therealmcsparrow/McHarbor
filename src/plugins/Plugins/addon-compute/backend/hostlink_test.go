// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func directReq(mutate func(*NetworkCreateRequest)) NetworkCreateRequest {
	req := NetworkCreateRequest{
		EnvironmentID: "env-1", Name: "lan", Mode: "macvlan",
		UplinkLink: "eth0", LinkMode: "bridge", HostAccess: true,
	}
	if mutate != nil {
		mutate(&req)
	}
	return req
}

// TestValidateHostLinkFields pins the name rules the agent enforces again and
// the IPv6 dependencies the dialog relies on, so a bad value is refused before
// an agent is asked.
func TestValidateHostLinkFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*NetworkCreateRequest)
		valid  bool
	}{
		{"defaults", nil, true},
		{"plain name", func(r *NetworkCreateRequest) { r.HostLinkName = "lanshim" }, true},
		{"15 chars", func(r *NetworkCreateRequest) { r.HostLinkName = "abcdefghijklmno" }, true},
		{"dots dashes underscores", func(r *NetworkCreateRequest) { r.HostLinkName = "a.b-c_d" }, true},
		{"16 chars", func(r *NetworkCreateRequest) { r.HostLinkName = "abcdefghijklmnop" }, false},
		{"leading dash", func(r *NetworkCreateRequest) { r.HostLinkName = "-lan" }, false},
		{"space", func(r *NetworkCreateRequest) { r.HostLinkName = "my lan" }, false},
		{"slash", func(r *NetworkCreateRequest) { r.HostLinkName = "a/b" }, false},
		{"reserved mcharbor-", func(r *NetworkCreateRequest) { r.HostLinkName = "mcharbor-h12345" }, false},
		{"reserved mcht", func(r *NetworkCreateRequest) { r.HostLinkName = "mcht1" }, false},
		{"reserved mchv upper", func(r *NetworkCreateRequest) { r.HostLinkName = "MCHV5" }, false},
		{"reserved mchd", func(r *NetworkCreateRequest) { r.HostLinkName = "mchd0" }, false},
		{"name needs host access", func(r *NetworkCreateRequest) { r.HostAccess = false; r.HostLinkName = "lanshim" }, false},
		{"ipv6 on, slaac", func(r *NetworkCreateRequest) { r.IPv6Enabled = true }, true},
		{"static v6", func(r *NetworkCreateRequest) { r.IPv6Enabled = true; r.HostAccessIPv6 = "fd00::9/64" }, true},
		{"v6 needs ipv6 enabled", func(r *NetworkCreateRequest) { r.HostAccessIPv6 = "fd00::9/64" }, false},
		{"v6 needs host access", func(r *NetworkCreateRequest) {
			r.HostAccess = false
			r.IPv6Enabled = true
			r.HostAccessIPv6 = "fd00::9/64"
		}, false},
		{"v6 without prefix", func(r *NetworkCreateRequest) { r.IPv6Enabled = true; r.HostAccessIPv6 = "fd00::9" }, false},
		{"v4 cidr as v6", func(r *NetworkCreateRequest) { r.IPv6Enabled = true; r.HostAccessIPv6 = "192.168.1.9/24" }, false},
		{"garbage v6", func(r *NetworkCreateRequest) { r.IPv6Enabled = true; r.HostAccessIPv6 = "nope" }, false},
	}
	for _, tc := range cases {
		err := validateNetworkRequest(directReq(tc.mutate))
		if tc.valid && err != nil {
			t.Errorf("%s: unexpected refusal: %v", tc.name, err)
		}
		if !tc.valid {
			if err == nil {
				t.Errorf("%s: accepted, want a refusal", tc.name)
			} else if !errors.Is(err, ErrInvalidHostLink) {
				t.Errorf("%s: refused with %v, want ErrInvalidHostLink", tc.name, err)
			}
		}
	}
}

// TestDirectNetworkFromCarriesHostLinkFields guards the wire names the host
// decodes into ComputeNetPayload; a renamed tag would be dropped silently.
func TestDirectNetworkFromCarriesHostLinkFields(t *testing.T) {
	req := directReq(func(r *NetworkCreateRequest) {
		r.HostLinkName = "lanshim"
		r.IPv6Enabled = true
		r.HostAccessIPv6 = "fd00::9/64"
	})
	raw, err := json.Marshal(directNetworkFrom("net-1", req))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"hostLinkName": "lanshim", "ipv6Enabled": true, "hostAccessIpv6": "fd00::9/64"}
	for key, value := range want {
		if wire[key] != value {
			t.Errorf("wire[%q] = %v, want %v (payload %s)", key, wire[key], value, raw)
		}
	}

	raw, _ = json.Marshal(directNetworkFrom("net-1", directReq(nil)))
	for _, key := range []string{"hostLinkName", "ipv6Enabled", "hostAccessIpv6"} {
		if strings.Contains(string(raw), key) {
			t.Errorf("%s must be omitted when unset, got %s", key, raw)
		}
	}
}

// TestDirectRemovalCarriesTheHostLinkName makes sure a delete or rename tears
// down the shim the operator named, not the generated default.
func TestDirectRemovalCarriesTheHostLinkName(t *testing.T) {
	got := directRemovalFrom(Network{ID: "n1", Name: "lan", Mode: "macvlan", UplinkLink: "eth0", HostLinkName: "lanshim"})
	if got.HostLinkName != "lanshim" || got.NetworkID != "n1" || got.UplinkLink != "eth0" {
		t.Fatalf("removal payload = %+v", got)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), `"hostLinkName":"lanshim"`) {
		t.Fatalf("removal wire = %s", raw)
	}
}

// TestDirectNeedsTeardown covers the edits that must remove the old shim first.
func TestDirectNeedsTeardown(t *testing.T) {
	existing := Network{
		UplinkLink: "eth0", LinkMode: "bridge", HostAccess: true,
		HostLinkName: "lanshim", IPv6Enabled: true,
	}
	same := directReq(func(r *NetworkCreateRequest) {
		r.HostLinkName = "lanshim"
		r.IPv6Enabled = true
	})
	cases := []struct {
		name   string
		mutate func(*NetworkCreateRequest)
		want   bool
	}{
		{"unchanged", nil, false},
		{"new v6 address only", func(r *NetworkCreateRequest) { r.HostAccessIPv6 = "fd00::9/64" }, false},
		{"host link renamed", func(r *NetworkCreateRequest) { r.HostLinkName = "other" }, true},
		{"host link name cleared", func(r *NetworkCreateRequest) { r.HostLinkName = "" }, true},
		{"ipv6 turned off", func(r *NetworkCreateRequest) { r.IPv6Enabled = false }, true},
		{"host access turned off", func(r *NetworkCreateRequest) { r.HostAccess = false; r.HostLinkName = "" }, true},
		{"parent moved", func(r *NetworkCreateRequest) { r.UplinkLink = "eth1" }, true},
	}
	for _, tc := range cases {
		req := same
		if tc.mutate != nil {
			tc.mutate(&req)
		}
		if got := directNeedsTeardown(existing, req); got != tc.want {
			t.Errorf("%s: teardown = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestClassifyMapsHostLinkRefusals keeps an agent's host-link code reaching the
// operator as the plugin's own specific message.
func TestClassifyMapsHostLinkRefusals(t *testing.T) {
	h := &Handler{logger: quietLogger{}}
	cases := []struct {
		code   string
		status int
		want   string
	}{
		{rpcNameTaken, http.StatusConflict, codeHostLinkNameTaken},
		{rpcParentNotFound, http.StatusBadRequest, codeHostLinkParentMissing},
		{rpcPermissionDenied, http.StatusBadGateway, codeHostLinkPermission},
	}
	for _, tc := range cases {
		err := fmt.Errorf("create network: %w", &codedErr{code: tc.code, text: "agent failed: eth9 is gone"})
		status, code, _ := h.classify("create network", err)
		if status != tc.status || code != tc.want {
			t.Errorf("%s: got %d %q, want %d %q", tc.code, status, code, tc.status, tc.want)
		}
	}
	status, code, detail := h.classify("create network", fmt.Errorf("%w: bad", ErrInvalidHostLink))
	if status != http.StatusBadRequest || code != codeHostLinkInvalid || detail != "" {
		t.Errorf("invalid host link: got %d %q %q", status, code, detail)
	}
}

// TestHostLinkMigrationAddsTheColumns applies every migration in order to a
// real SQLite database and checks the new columns exist with the defaults the
// scan relies on, and that EnsureColumns knows the same three.
func TestHostLinkMigrationAddsTheColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	files, err := filepath.Glob(filepath.Join("..", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", filepath.Base(file), err)
		}
	}

	if _, err := db.Exec(`INSERT INTO plugin_addon_compute_networks
		(id, environment_id, name, mode, bridge, state, created_at, updated_at)
		VALUES ('n1', 'e1', 'lan', 'macvlan', '', 'ready', 'x', 'x')`); err != nil {
		t.Fatalf("a row without the new columns must still insert: %v", err)
	}
	var enabled int
	var addr, link sql.NullString
	if err := db.QueryRow(`SELECT ipv6_enabled, host_access_ipv6, host_link_name
		FROM plugin_addon_compute_networks WHERE id = 'n1'`).Scan(&enabled, &addr, &link); err != nil {
		t.Fatalf("new columns missing: %v", err)
	}
	if enabled != 0 || addr.Valid || link.Valid {
		t.Errorf("defaults = %d %v %v, want 0 and NULLs", enabled, addr, link)
	}

	have := map[string]bool{}
	for _, c := range lateColumns {
		have[c.column] = true
	}
	for _, column := range []string{"ipv6_enabled", "host_access_ipv6", "host_link_name"} {
		if !have[column] {
			t.Errorf("lateColumns lacks %s, so an installed instance would never get it", column)
		}
	}
}
