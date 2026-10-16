// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

func hostDevicesServer(t *testing.T) *fakeServer {
	t.Helper()
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/system/host-devices": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"truncated": true,
				"devices": []map[string]any{
					{"path": "/dev/ttyUSB0", "type": "char", "major": 188, "minor": 0, "category": "serial", "label": "Silicon Labs CP2102", "stablePath": "/dev/serial/by-id/usb-Silicon_Labs_CP2102_0001-if00-port0", "dangerous": false},
					{"path": "/dev/sda", "type": "block", "major": 8, "minor": 0, "category": "block", "label": "Samsung SSD 870", "dangerous": true},
				},
			})
		},
	})
}

func TestHostDevicesListsTheHostsNodes(t *testing.T) {
	f := hostDevicesServer(t)
	code, stdout, stderr := run(t, f, goodKey, "host", "devices", "--env", "e1")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"/dev/ttyUSB0", "serial", "188", "Silicon Labs CP2102", "/dev/serial/by-id/usb-Silicon_Labs_CP2102_0001-if00-port0", "/dev/sda", "block", "yes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "truncated") {
		t.Errorf("a truncated listing was not flagged: %q", stderr)
	}
	if q := f.seen()[len(f.seen())-1].URL.Query().Get("env"); q != "e1" {
		t.Fatalf("env query = %q", q)
	}
}

func TestHostDevicesJSONKeepsTheWholeAnswer(t *testing.T) {
	f := hostDevicesServer(t)
	code, stdout, stderr := run(t, f, goodKey, "host", "devices", "--env", "e1", "-o", "json")
	if code != ExitOK || !strings.Contains(stdout, `"stablePath"`) || !strings.Contains(stdout, `"truncated"`) {
		t.Fatalf("exit %d: %s %s", code, stdout, stderr)
	}
}

func TestHostDevicesRequiresAnEnvironment(t *testing.T) {
	f := hostDevicesServer(t)
	if code, _, _ := run(t, f, goodKey, "host", "devices"); code == ExitOK {
		t.Fatal("host devices without --env succeeded")
	}
	for _, r := range f.seen() {
		if strings.HasPrefix(r.URL.Path, "/api/system/host-devices") {
			t.Fatalf("a refused command reached the server: %s", r.URL.Path)
		}
	}
}
