// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

func widgetServer(t *testing.T) *fakeServer {
	t.Helper()
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/widgets/catalog": func(w http.ResponseWriter, _ string) {
			ok(w, []map[string]any{
				{"key": "weather", "label": "Weather", "version": "1.2.0", "signed": true, "installed": false},
				{"key": "clock", "label": "Clock", "version": "1.0.0", "signed": true, "installed": true, "installedVersion": "1.0.0"},
			})
		},
		"POST /api/widgets/catalog/weather/install": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"key": "weather"})
		},
		"DELETE /api/widgets/weather": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"code": "msg.widget.uninstalled"})
		},
		"DELETE /api/widgets/clock": func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusBadRequest, "err.widget.builtin", "Built-in widgets cannot be removed")
		},
	})
}

func TestWidgetsCatalogListsInstallState(t *testing.T) {
	f := widgetServer(t)
	code, stdout, stderr := run(t, f, goodKey, "widgets", "catalog")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"weather", "Weather", "clock", "1.2.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("catalog output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestWidgetsInstallReachesItsRoute(t *testing.T) {
	f := widgetServer(t)
	if code, _, stderr := run(t, f, goodKey, "widgets", "install", "weather"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, r := range f.seen() {
		if r.Method == http.MethodPost && r.URL.Path == "/api/widgets/catalog/weather/install" {
			return
		}
	}
	t.Fatal("install never reached POST /api/widgets/catalog/weather/install")
}

func TestWidgetsRemoveIsIrreversibleAndShowsTheServerMessage(t *testing.T) {
	f := widgetServer(t)
	if code, _, _ := run(t, f, goodKey, "widgets", "remove", "weather", "--yes"); code == ExitOK {
		t.Error("an irreversible remove ran on --yes alone")
	}
	for _, r := range f.seen() {
		if r.Method == http.MethodDelete {
			t.Fatalf("an unconfirmed remove still sent %s %s", r.Method, r.URL.Path)
		}
	}
	if code, _, stderr := run(t, f, goodKey, "widgets", "remove", "weather", "--confirm", "weather"); code != ExitOK {
		t.Fatalf("confirmed remove: exit %d: %s", code, stderr)
	}
	code, _, stderr := run(t, f, goodKey, "widgets", "remove", "clock", "--confirm", "clock")
	if code == ExitOK || !strings.Contains(stderr, "Built-in widgets cannot be removed") {
		t.Fatalf("built-in remove: exit %d: %s", code, stderr)
	}
	if code, _, _ := run(t, f, goodKey, "widgets", "remove", "../x", "--confirm", "../x"); code != ExitUsage {
		t.Errorf("bad key: exit %d, want %d", code, ExitUsage)
	}
}
