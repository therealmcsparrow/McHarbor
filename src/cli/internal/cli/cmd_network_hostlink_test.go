// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

// TestNetworkCreateSendsTheHostLink pins the wire shape the backend validates: the flags become
// enableIPv6, an IPAM block and a hostLink object, and "auto" becomes ipv6Auto rather than a CIDR.
func TestNetworkCreateSendsTheHostLink(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	var applied int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/health":
			ok(w, map[string]string{"status": "ok"})
		case r.URL.Path == "/api/about":
			ok(w, map[string]any{"version": "2026.09.01", "build": "202609250000"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/networks":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
			ok(w, map[string]any{"Id": "net1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/networks/lan/host-link/apply":
			mu.Lock()
			applied++
			mu.Unlock()
			ok(w, map[string]any{"state": "present"})
		default:
			fail(w, http.StatusNotFound, "err.not_found", "no route "+r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	f := &fakeServer{Server: srv}

	args := []string{
		"network", "create", "lan", "--driver", "macvlan", "--ipv6", "--opt", "parent=eth0",
		"--subnet", "192.168.1.0/24", "--gateway", "192.168.1.1", "--subnet", "2001:db8:1::/64",
		"--host-link-name", "mcvlan0", "--host-link-ipv4", "192.168.1.10/24", "--host-link-ipv6", "auto",
	}
	if code, _, stderr := run(t, f, goodKey, args...); code != ExitOK {
		t.Fatalf("network create: exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, f, goodKey, "network", "host-link", "apply", "lan"); code != ExitOK {
		t.Fatalf("host-link apply: exit %d: %s", code, stderr)
	}
	if code, _, _ := run(t, f, goodKey, "network", "create", "bad", "--host-link-ipv4", "192.168.1.10/24"); code != ExitUsage {
		t.Errorf("a host link address without a name: exit %d, want %d", code, ExitUsage)
	}

	mu.Lock()
	defer mu.Unlock()
	if applied != 1 || len(bodies) != 1 {
		t.Fatalf("applied %d, creates %d; want 1 and 1", applied, len(bodies))
	}
	b := bodies[0]
	if b["enableIPv6"] != true {
		t.Errorf("enableIPv6 = %v", b["enableIPv6"])
	}
	if !reflect.DeepEqual(b["hostLink"], map[string]any{"name": "mcvlan0", "ipv4": "192.168.1.10/24", "ipv6Auto": true}) {
		t.Errorf("hostLink = %v", b["hostLink"])
	}
	if !reflect.DeepEqual(b["options"], map[string]any{"parent": "eth0"}) {
		t.Errorf("options = %v", b["options"])
	}
	want := map[string]any{"Config": []any{
		map[string]any{"Subnet": "192.168.1.0/24", "Gateway": "192.168.1.1"},
		map[string]any{"Subnet": "2001:db8:1::/64"},
	}}
	if !reflect.DeepEqual(b["ipam"], want) {
		t.Errorf("ipam = %v", b["ipam"])
	}
}
