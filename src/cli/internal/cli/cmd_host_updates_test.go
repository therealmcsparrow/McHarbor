// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func hostUpdatesServer(t *testing.T) *fakeServer {
	t.Helper()
	state := func(w http.ResponseWriter, _ string) {
		ok(w, map[string]any{
			"available": true, "count": 2, "manager": "apt",
			"schedule": map[string]any{"enabled": true, "time": "03:00", "days": []string{"mon", "fri"}, "withNewPackages": true, "skipUnavailable": false, "removeUnused": false},
		})
	}
	check := func(w http.ResponseWriter, _ string) {
		ok(w, map[string]any{"manager": "apt", "available": true, "updates": []string{"openssl 3.0.2", "linux-image-6.8"}})
	}
	apply := func(w http.ResponseWriter, _ string) { ok(w, map[string]any{"jobId": "job-1"}) }
	saved := func(w http.ResponseWriter, _ string) { ok(w, nil) }
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/system/os-updates/state":         state,
		"GET /api/system/os-updates/check":         check,
		"POST /api/system/os-updates/apply":        apply,
		"PUT /api/system/os-updates/schedule":      saved,
		"GET /api/system/self/os-updates/state":    state,
		"GET /api/system/self/os-updates/check":    check,
		"POST /api/system/self/os-updates/apply":   apply,
		"PUT /api/system/self/os-updates/schedule": saved,
	})
}

func lastRequest(t *testing.T, f *fakeServer) *http.Request {
	t.Helper()
	seen := f.seen()
	if len(seen) == 0 {
		t.Fatal("the server saw no request")
	}
	return seen[len(seen)-1]
}

func TestHostUpdatesCheckListsPackagesForTheEnvironment(t *testing.T) {
	f := hostUpdatesServer(t)
	code, stdout, stderr := run(t, f, goodKey, "host", "updates", "check", "--env", "e1")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"apt", "2 package(s)", "openssl 3.0.2", "linux-image-6.8"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	r := lastRequest(t, f)
	if r.Method != http.MethodGet || r.URL.Path != "/api/system/os-updates/check" || r.URL.Query().Get("env") != "e1" {
		t.Fatalf("request = %s %s", r.Method, r.URL.String())
	}
}

func TestHostUpdatesSelfUsesTheSelfRouteWithoutAnEnvQuery(t *testing.T) {
	f := hostUpdatesServer(t)
	if code, _, stderr := run(t, f, goodKey, "host", "updates", "status", "--self"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	r := lastRequest(t, f)
	if r.URL.Path != "/api/system/self/os-updates/state" || r.URL.Query().Has("env") {
		t.Fatalf("request = %s", r.URL.String())
	}
}

func TestHostUpdatesNeedsExactlyOneHost(t *testing.T) {
	f := hostUpdatesServer(t)
	for _, args := range [][]string{
		{"host", "updates", "check"},
		{"host", "updates", "check", "--self", "--env", "e1"},
	} {
		if code, _, _ := run(t, f, goodKey, args...); code == ExitOK {
			t.Fatalf("%v succeeded", args)
		}
	}
	if n := len(f.seen()); n != 0 {
		t.Fatalf("a refused command reached the server (%d requests)", n)
	}
}

func TestHostUpdatesApplyRefusesWithoutConfirmation(t *testing.T) {
	f := hostUpdatesServer(t)
	code, _, stderr := run(t, f, goodKey, "host", "updates", "apply", "--env", "e1")
	if code == ExitOK || !strings.Contains(stderr, "--yes") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if n := len(f.seen()); n != 0 {
		t.Fatalf("an unconfirmed apply reached the server (%d requests)", n)
	}
}

func TestHostUpdatesApplySendsConfirmAndOnlyTheGivenOptions(t *testing.T) {
	f := hostUpdatesServer(t)
	code, stdout, stderr := run(t, f, goodKey, "host", "updates", "apply", "--env", "e1", "--yes", "--with-new-packages", "--remove-unused=false")
	if code != ExitOK || !strings.Contains(stdout, "job-1") {
		t.Fatalf("exit %d: %s %s", code, stdout, stderr)
	}
	r := lastRequest(t, f)
	if r.Method != http.MethodPost || r.URL.Path != "/api/system/os-updates/apply" || r.URL.Query().Get("env") != "e1" {
		t.Fatalf("request = %s %s", r.Method, r.URL.String())
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(f.bodies["POST /api/system/os-updates/apply"]), &body); err != nil {
		t.Fatal(err)
	}
	if body["confirm"] != true || body["withNewPackages"] != true || body["removeUnused"] != false {
		t.Fatalf("body = %v", body)
	}
	if _, present := body["skipUnavailable"]; present {
		t.Fatalf("an option that was not passed was sent: %v", body)
	}
}

func TestHostUpdatesApplySelfUsesTheSelfRoute(t *testing.T) {
	f := hostUpdatesServer(t)
	if code, _, stderr := run(t, f, goodKey, "host", "updates", "apply", "--self", "--yes"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if r := lastRequest(t, f); r.Method != http.MethodPost || r.URL.Path != "/api/system/self/os-updates/apply" {
		t.Fatalf("request = %s %s", r.Method, r.URL.Path)
	}
}

func TestHostUpdatesScheduleMergesOntoTheStoredSchedule(t *testing.T) {
	f := hostUpdatesServer(t)
	code, _, stderr := run(t, f, goodKey, "host", "updates", "schedule", "--env", "e1", "--time", "04:15", "--skip-unavailable")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	r := lastRequest(t, f)
	if r.Method != http.MethodPut || r.URL.Path != "/api/system/os-updates/schedule" || r.URL.Query().Get("env") != "e1" {
		t.Fatalf("request = %s %s", r.Method, r.URL.String())
	}
	var body struct {
		Enabled         bool     `json:"enabled"`
		Time            string   `json:"time"`
		Days            []string `json:"days"`
		WithNewPackages bool     `json:"withNewPackages"`
		SkipUnavailable bool     `json:"skipUnavailable"`
		RemoveUnused    bool     `json:"removeUnused"`
	}
	if err := json.Unmarshal([]byte(f.bodies["PUT /api/system/os-updates/schedule"]), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled || body.Time != "04:15" || strings.Join(body.Days, ",") != "mon,fri" || !body.WithNewPackages || !body.SkipUnavailable || body.RemoveUnused {
		t.Fatalf("body = %+v", body)
	}
}

func TestHostUpdatesScheduleDisableAndDays(t *testing.T) {
	f := hostUpdatesServer(t)
	if code, _, stderr := run(t, f, goodKey, "host", "updates", "schedule", "--self", "--disable", "--days", "Sun, sat"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if r := lastRequest(t, f); r.URL.Path != "/api/system/self/os-updates/schedule" {
		t.Fatalf("path = %s", r.URL.Path)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(f.bodies["PUT /api/system/self/os-updates/schedule"]), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != false || strings.Join(stringList(body["days"]), ",") != "sun,sat" {
		t.Fatalf("body = %v", body)
	}
}

func TestHostUpdatesScheduleRejectsBadInputBeforeTheServer(t *testing.T) {
	f := hostUpdatesServer(t)
	for _, args := range [][]string{
		{"--env", "e1"},
		{"--env", "e1", "--time", "3:5"},
		{"--env", "e1", "--time", "25:00"},
		{"--env", "e1", "--days", "mon,funday"},
		{"--env", "e1", "--enable", "--disable"},
	} {
		if code, _, _ := run(t, f, goodKey, append([]string{"host", "updates", "schedule"}, args...)...); code == ExitOK {
			t.Fatalf("%v succeeded", args)
		}
	}
	if n := len(f.seen()); n != 0 {
		t.Fatalf("invalid input reached the server (%d requests)", n)
	}
}
