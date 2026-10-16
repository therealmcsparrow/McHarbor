// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pluginFixture writes a minimal but valid plugin tree into a throwaway
// checkout and returns the repository root. Each test then breaks exactly one
// thing, so a finding can only come from what that test changed.
func pluginFixture(t *testing.T, slug string) string {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, "src", "plugins", "Plugins", slug)
	write := func(rel, body string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "backend", "go.mod"),
		[]byte("module github.com/therealmcsparrow/mcharbor\n\ngo 1.27.2\n\nrequire github.com/rs/xid v1.6.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	write("plugin.yaml", `schema: "mcharbor.plugin/v1"
slug: "`+slug+`"
name: "Fixture"
version: "2026.09.01"
mcharbor: ">=2026.09.01"
adds_permissions:
  - key: "fixture.manage"
    scope: "env"
contributes:
  routes:
    - method: "POST"
      path: "/api/`+slug+`/send"
      requires_permission: "fixture.manage"
`)
	write("go.mod", "module github.com/therealmcsparrow/mcharbor-addons/plugins/"+slug+
		"\n\ngo 1.27.2\n\nrequire github.com/rs/xid v1.6.0\n")
	write("backend/main.go", `package main

import "net/http"

var meta = struct{ Version string }{
	Version:  "2026.09.01",
}

func register(reg registry) {
	reg.Protected(http.MethodPost, "/api/`+slug+`/send", nil)
}
`)
	write("icons/plugin.svg", "<svg/>")
	return repo
}

// findingsFor returns only the errors, which are the findings that mean the
// plugin would misbehave once installed.
func errorsFor(t *testing.T, repo, slug string) []pluginFinding {
	t.Helper()
	all, err := validatePlugin(repo, slug)
	if err != nil {
		t.Fatalf("validatePlugin: %v", err)
	}
	var out []pluginFinding
	for _, f := range all {
		if f.Severity == "error" {
			out = append(out, f)
		}
	}
	return out
}

func joinDetails(findings []pluginFinding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, f.Check+": "+f.Detail)
	}
	return strings.Join(parts, " | ")
}

// TestValidateCleanPluginHasNoErrors is the baseline: without it, every test
// below could be passing because the fixture is broken in some other way.
func TestValidateCleanPluginHasNoErrors(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	if found := errorsFor(t, repo, "comm-fixture"); len(found) != 0 {
		t.Fatalf("a clean plugin must produce no errors, got: %s", joinDetails(found))
	}
}

// TestValidateFailsOnUndeclaredRoute is the reason plugin dev validate exists.
// A non-public route the manifest never declares gets no permission middleware:
// the loader returns an empty permission on a miss and an empty permission is a
// pass-through, so the route is reachable by any authenticated user.
func TestValidateFailsOnUndeclaredRoute(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	manifest := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "plugin.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	// Drop the declaration, keep the registration.
	stripped := strings.Split(string(body), "contributes:")[0] + "contributes:\n  routes: []\n"
	if err := os.WriteFile(manifest, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	found := errorsFor(t, repo, "comm-fixture")
	if len(found) == 0 {
		t.Fatal("an undeclared non-public route must be an error")
	}
	detail := joinDetails(found)
	if !strings.Contains(detail, "POST /api/comm-fixture/send") {
		t.Fatalf("the finding must name the route: %s", detail)
	}
	if !strings.Contains(detail, "no permission middleware") {
		t.Fatalf("the finding must say why it matters: %s", detail)
	}
}

// TestValidateFailsOnEmptyRequiresPermission covers the other half of the same
// bug: a declaration whose requires_permission is blank short-circuits the
// middleware to a pass-through just as a missing declaration does.
func TestValidateFailsOnEmptyRequiresPermission(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	manifest := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "plugin.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	blanked := strings.Replace(string(body), `requires_permission: "fixture.manage"`, `requires_permission: ""`, 1)
	if err := os.WriteFile(manifest, []byte(blanked), 0o644); err != nil {
		t.Fatal(err)
	}
	if detail := joinDetails(errorsFor(t, repo, "comm-fixture")); !strings.Contains(detail, "pass-through") {
		t.Fatalf("an empty requires_permission must be an error naming the pass-through: %s", detail)
	}
}

// TestValidateAcceptsAPublicRoute: a route the manifest declares public is legal
// and the loader honours it, so it must not be reported.
func TestValidateAcceptsAPublicRoute(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	manifest := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "plugin.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	public := strings.Replace(string(body), `requires_permission: "fixture.manage"`,
		"requires_permission: \"\"\n      public: true", 1)
	if err := os.WriteFile(manifest, []byte(public), 0o644); err != nil {
		t.Fatal(err)
	}
	if found := errorsFor(t, repo, "comm-fixture"); len(found) != 0 {
		t.Fatalf("a declared-public route must not be reported: %s", joinDetails(found))
	}
}

// TestValidateFailsOnCRLFGoMod: the packager reads go.mod as text, so CRLF
// endings stage a .so that cannot load, with nothing in the build output saying so.
func TestValidateFailsOnCRLFGoMod(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	goMod := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "go.mod")
	body, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goMod, []byte(strings.ReplaceAll(string(body), "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	detail := joinDetails(errorsFor(t, repo, "comm-fixture"))
	if !strings.Contains(detail, "CRLF") {
		t.Fatalf("a CRLF go.mod must be an error naming CRLF: %s", detail)
	}
}

// TestValidateFailsOnDependencySkew: a dependency at one version in the plugin
// and another in the host stages a .so the host refuses to open.
func TestValidateFailsOnDependencySkew(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	goMod := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "go.mod")
	body, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goMod,
		[]byte(strings.Replace(string(body), "v1.6.0", "v1.5.0", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if detail := joinDetails(errorsFor(t, repo, "comm-fixture")); !strings.Contains(detail, "github.com/rs/xid") {
		t.Fatalf("a skewed dependency must be an error naming the module: %s", detail)
	}
}

// TestValidateFailsOnVersionSymbolDrift: the loader asserts the manifest, the
// install directory and the .so symbol all agree, and a source edit moves two.
func TestValidateFailsOnVersionSymbolDrift(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	main := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "backend", "main.go")
	body, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main,
		[]byte(strings.Replace(string(body), `Version:  "2026.09.01"`, `Version:  "2026.09.02"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if detail := joinDetails(errorsFor(t, repo, "comm-fixture")); !strings.Contains(detail, "2026.09.02") {
		t.Fatalf("a drifted Version symbol must be an error: %s", detail)
	}
}

// TestValidateRefusesABadPermissionKey: install rejects a key with a hyphen or a
// colon, which is what blocked every comm plugin until it was found by hand.
func TestValidateRefusesABadPermissionKey(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	manifest := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "plugin.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest,
		[]byte(strings.Replace(string(body), `key: "fixture.manage"`, `key: "comm-fixture.manage"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if detail := joinDetails(errorsFor(t, repo, "comm-fixture")); !strings.Contains(detail, "comm-fixture.manage") {
		t.Fatalf("a hyphenated permission key must be an error: %s", detail)
	}
}

// TestScanRecognisesAllThreeRegistrationShapes: the drift check is only as good
// as its ability to see a registration, and the host's own test recognises three
// shapes. A plugin using the literal forms must be scanned too.
func TestScanRecognisesAllThreeRegistrationShapes(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "net/http"

var routes = []route{
	{http.MethodGet, "/api/x/bool-public", true, (*h).Public},
	{http.MethodPut, "/api/x/bool-protected", false, (*h).Protected},
	{http.MethodDelete, "/api/x/plain", (*h).Plain},
}

func register(reg registry) {
	reg.Protected(http.MethodPost, "/api/x/call", nil)
	reg.Public(http.MethodGet, "/api/x/open", nil)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	routes, err := scanPluginRegistrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"POST /api/x/call":          false,
		"GET /api/x/open":           true,
		"GET /api/x/bool-public":    true,
		"PUT /api/x/bool-protected": false,
		"DELETE /api/x/plain":       false,
	}
	if len(routes) != len(want) {
		t.Fatalf("expected %d registrations, got %d: %+v", len(want), len(routes), routes)
	}
	for _, rt := range routes {
		key := rt.method + " " + rt.path
		public, known := want[key]
		if !known {
			t.Fatalf("unexpected registration %s", key)
		}
		if public != rt.public {
			t.Fatalf("%s: public should be %v, got %v", key, public, rt.public)
		}
	}
}
