// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
)

func TestMarkerLifecycle(t *testing.T) {
	dir := t.TempDir()
	if m, err := Load(dir); err != nil || m != nil || Active() {
		t.Fatalf("empty data dir: marker %v, err %v, active %v", m, err, Active())
	}
	m, err := NewMarker("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteMarker(dir, m); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(MarkerPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("marker permissions %v expose the secret", perm)
	}
	loaded, err := Load(dir)
	if err != nil || loaded == nil || !Active() || loaded.Secret != m.Secret {
		t.Fatalf("load after write: %+v, %v", loaded, err)
	}
	if err := ParkMarker(dir); err != nil {
		t.Fatal(err)
	}
	if again, err := Load(dir); err != nil || again != nil || Active() {
		t.Fatalf("a parked marker must not activate demo mode: %v %v", again, err)
	}
	parked, err := ParkedMarker(dir)
	if err != nil || parked == nil || parked.Secret != m.Secret {
		t.Fatalf("parked marker lost its secret: %+v, %v", parked, err)
	}
}

func TestTokensHaveTheAgentShapeAndDifferPerEnvironment(t *testing.T) {
	m := &Marker{Secret: "0123456789abcdef0123456789abcdef"}
	shape := regexp.MustCompile(`^agt_[0-9a-f]{64}$`)
	a, b := m.AgentToken("prod-eu-1"), m.AgentToken("prod-eu-2")
	if !shape.MatchString(a) || !shape.MatchString(b) || a == b {
		t.Fatalf("tokens %q %q", a, b)
	}
	if a != m.AgentToken("prod-eu-1") {
		t.Fatal("token derivation is not stable")
	}
	other := &Marker{Secret: "fedcba9876543210fedcba9876543210"}
	if other.AgentToken("prod-eu-1") == a {
		t.Fatal("a new secret must mint new tokens")
	}
}

func TestPrepareDatabaseInstallsAStagedReset(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(DatabasePath(dir), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DatabasePath(dir)+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(NextDatabasePath(dir), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareDatabase(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(DatabasePath(dir))
	if err != nil || string(got) != "new" {
		t.Fatalf("demo.db = %q, %v", got, err)
	}
	if _, err := os.Stat(DatabasePath(dir) + "-wal"); !os.IsNotExist(err) {
		t.Fatal("stale WAL survived the swap and would corrupt the new database")
	}
}

func TestGuardRefusesOnlyInDemoMode(t *testing.T) {
	cases := []struct {
		method, path string
		blocked      bool
	}{
		{http.MethodGet, "/api/system/self/os-info", true},
		{http.MethodPost, "/api/system/self/os-updates/apply", true},
		{http.MethodPost, "/api/system/reset", true},
		{http.MethodPost, "/api/system/restart", false},
		{http.MethodGet, "/api/updates/check", true},
		{http.MethodGet, "/api/updates/state", false},
		{http.MethodPost, "/api/instance-backups/restore", true},
		{http.MethodGet, "/api/instance-backups", false},
		{http.MethodPost, "/api/database/migration", true},
		{http.MethodPut, "/api/settings/tls", true},
		{http.MethodPut, "/api/settings/branding", false},
		{http.MethodGet, "/api/containers", false},
		{http.MethodPost, "/api/demo/exit", false},
		{http.MethodGet, "/api/host-services/self", true},
		{http.MethodGet, "/api/host-services", false},
		{http.MethodPost, "/api/appstore/plugins/uninstall", true},
		{http.MethodGet, "/api/appstore/plugins/addon-kvm", false},
	}
	for _, c := range cases {
		if got := Blocked(c.method, c.path); got != c.blocked {
			t.Errorf("Blocked(%s %s) = %v, want %v", c.method, c.path, got, c.blocked)
		}
	}

	refused := 0
	h := Guard(func(w http.ResponseWriter, r *http.Request) { refused++; w.WriteHeader(http.StatusForbidden) })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	active.Store(nil)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/system/reset", nil))
	if refused != 0 {
		t.Fatal("guard refused a request outside demo mode")
	}
	active.Store(&Marker{Secret: "x"})
	defer active.Store(nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/system/reset", nil))
	if refused != 1 || rec.Code != http.StatusForbidden {
		t.Fatalf("guard let a factory reset through in demo mode (code %d)", rec.Code)
	}
}
