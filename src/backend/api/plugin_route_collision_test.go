// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// pluginTree is where the in-tree plugins live, relative to this package. The
// tree is not part of every checkout, so its absence skips rather than fails.
const pluginTree = "../../plugins/Plugins"

// knownShadowedPluginRoutes are collisions found by this test and not yet moved,
// each with the reason it is still here. An entry that stops colliding fails the
// test, so the list can only shrink.
var knownShadowedPluginRoutes = map[string]string{}

// pluginManifestRoutes is the part of a plugin.yaml this test reads.
type pluginManifestRoutes struct {
	Slug        string `yaml:"slug"`
	Contributes struct {
		Routes []struct {
			Method string `yaml:"method"`
			Path   string `yaml:"path"`
		} `yaml:"routes"`
	} `yaml:"contributes"`
}

// pluginRouteRef is one declared plugin route, named for the failure message.
type pluginRouteRef struct {
	Slug   string
	Method string
	Path   string
}

// hostPathShapes returns every host path in openapi.json split into segments,
// with each {param} reduced to "{}" so shapes compare regardless of names.
func hostPathShapes(t *testing.T) [][]string {
	t.Helper()
	raw, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatalf("read %s: %v", specFile, err)
	}
	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", specFile, err)
	}
	if len(doc.Paths) < minOperations/2 {
		t.Fatalf("%s holds %d paths; regenerate it rather than trusting an empty scan", specFile, len(doc.Paths))
	}
	paths := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([][]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, pathShape(p))
	}
	return out
}

// pathShape splits a chi pattern into segments and blanks parameter names.
func pathShape(p string) []string {
	segs := strings.Split(strings.Trim(p, "/"), "/")
	for i, s := range segs {
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			segs[i] = "{}"
		}
	}
	return segs
}

// segmentsOverlap reports whether one request segment could match both a and b.
func segmentsOverlap(a, b string) bool {
	return a == b || a == "{}" || b == "{}"
}

// shapePrefixOf reports whether every segment of prefix overlaps the matching
// leading segment of full.
func shapePrefixOf(prefix, full []string) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i := range prefix {
		if !segmentsOverlap(prefix[i], full[i]) {
			return false
		}
	}
	return true
}

// hostShadow returns the host path that would answer a request meant for the
// plugin route, or "" when the plugin catch-all is reached.
//
// The plugin router is mounted behind every host route under /api, so a plugin
// path loses when the host serves the same shape (any method: chi answers 405,
// not the catch-all), or when it sits below a host path that itself has host
// children — the shape r.Route("/containers", …) leaves, which owns its subtree
// and answers 404 for anything it does not know.
func hostShadow(host [][]string, pluginPath string) string {
	p := pathShape(pluginPath)
	for _, h := range host {
		if len(h) == len(p) && shapePrefixOf(h, p) {
			return "/" + strings.Join(h, "/")
		}
	}
	for _, h := range host {
		if len(h) < 2 || len(h) >= len(p) || !shapePrefixOf(h, p) {
			continue
		}
		for _, child := range host {
			if len(child) > len(h) && shapePrefixOf(h, child) {
				return "/" + strings.Join(h, "/") + "/…"
			}
		}
	}
	return ""
}

// inTreePluginRoutes reads contributes.routes from every in-tree plugin.yaml.
func inTreePluginRoutes(t *testing.T) []pluginRouteRef {
	t.Helper()
	if _, err := os.Stat(pluginTree); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("%s is not in this checkout", pluginTree)
	}
	manifests, err := filepath.Glob(filepath.Join(pluginTree, "*", "plugin.yaml"))
	if err != nil {
		t.Fatalf("glob plugin manifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Skipf("no plugin.yaml under %s", pluginTree)
	}
	var out []pluginRouteRef
	for _, m := range manifests {
		raw, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		var man pluginManifestRoutes
		if err := yaml.Unmarshal(raw, &man); err != nil {
			t.Errorf("parse %s: %v", m, err)
			continue
		}
		for _, r := range man.Contributes.Routes {
			out = append(out, pluginRouteRef{Slug: man.Slug, Method: strings.ToUpper(r.Method), Path: r.Path})
		}
	}
	return out
}

// TestPluginRoutesAreNotShadowedByHostRoutes guards the addon-lxc bug: it
// declared /api/containers/…, the host's Docker subtree, so its routes 404'd or
// reached Docker handlers from every client except the runbook dispatcher.
func TestPluginRoutesAreNotShadowedByHostRoutes(t *testing.T) {
	host := hostPathShapes(t)
	var shadowed []string
	stillShadowed := map[string]bool{}
	for _, r := range inTreePluginRoutes(t) {
		if !strings.HasPrefix(r.Path, "/api/") {
			continue
		}
		owner := hostShadow(host, r.Path)
		if owner == "" {
			continue
		}
		key := r.Slug + ": " + r.Method + " " + r.Path
		if _, known := knownShadowedPluginRoutes[key]; known {
			stillShadowed[key] = true
			continue
		}
		shadowed = append(shadowed, key+"  ->  host "+owner)
	}
	for key := range knownShadowedPluginRoutes {
		if !stillShadowed[key] {
			t.Errorf("%s no longer collides with a host route; remove it from knownShadowedPluginRoutes", key)
		}
	}
	sort.Strings(shadowed)
	if len(shadowed) > 0 {
		t.Errorf("%d plugin routes are answered by the host before the plugin router sees them; "+
			"move them to a prefix the host does not own:\n  %s", len(shadowed), strings.Join(shadowed, "\n  "))
	}
}

// TestHostShadowDetectsTheLXCCollision keeps the detector itself honest against
// the exact routes that shipped broken, independent of what the tree holds today.
func TestHostShadowDetectsTheLXCCollision(t *testing.T) {
	host := hostPathShapes(t)
	for _, p := range []string{
		"/api/containers",
		"/api/containers/{id}/start",
		"/api/containers/{id}/freeze",
		"/api/containers/orphaned-backups",
		"/api/containers/backups/{runId}/recreate",
	} {
		if hostShadow(host, p) == "" {
			t.Errorf("%s is inside the host's /api/containers subtree but was not reported", p)
		}
	}
	for _, p := range []string{
		"/api/system-containers/{id}/freeze",
		"/api/vms/{id}/start",
		"/api/compute/pools",
	} {
		if owner := hostShadow(host, p); owner != "" {
			t.Errorf("%s reported as shadowed by %s, but the host does not serve it", p, owner)
		}
	}
}
