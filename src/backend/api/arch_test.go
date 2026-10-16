// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_test

import (
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// modulePackagePrefix is the import path prefix every backend feature module
// shares; moduleImportEdges strips it to get a bare module name.
const modulePackagePrefix = "github.com/therealmcsparrow/mcharbor/modules/"

// allowedEdges is the small set of modules/X -> modules/Y imports this repo
// accepts today, each with the coupling reason recorded so a new edge is a
// reviewed decision rather than an accident. "X->*" allows X to import any
// module.
var allowedEdges = map[string]string{
	"demo->*":                     "composition root for demo fixtures",
	"compliancepacks->compliance": "sub-feature of compliance",
	"containers->debugshell":      "debug shell nests under /containers/{id}",
	"terminal->debugshell":        "shares the debug shell transport",
	"deviceauth->api_keys":        "device sign-in is a second front door to API key creation and returns its CreateAPIKeyResult",
	"updates->agentreleases":      "agent releases are mounted and scheduled by updates under /updates/agents/releases",
}

// moduleImportEdges runs `go list` over the backend module tree and returns,
// for each module, the set of other modules it imports. It reads .Imports,
// which go list already excludes test-only imports from.
func moduleImportEdges(t *testing.T) map[string]map[string]bool {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./modules/...")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=auto")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./modules/...: %v", err)
	}
	edges := map[string]map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], modulePackagePrefix) {
			continue
		}
		from := moduleName(fields[0])
		for _, imp := range fields[1:] {
			if !strings.HasPrefix(imp, modulePackagePrefix) {
				continue
			}
			to := moduleName(imp)
			if to == from {
				continue
			}
			if edges[from] == nil {
				edges[from] = map[string]bool{}
			}
			edges[from][to] = true
		}
	}
	return edges
}

// moduleName reduces a package import path under modulePackagePrefix to its
// top-level module directory, so modules/x/sub imports still name "x".
func moduleName(importPath string) string {
	rest := strings.TrimPrefix(importPath, modulePackagePrefix)
	name, _, _ := strings.Cut(rest, "/")
	return name
}

// TestModuleImportGraph fails when a feature module imports another feature
// module outside allowedEdges. It started red with 22 distinct module-pair
// edges and was burned down to zero on 2026-09-29; it now runs
// unconditionally, so a new cross-module import fails the ordinary test run.
func TestModuleImportGraph(t *testing.T) {
	edges := moduleImportEdges(t)
	var bad []string
	for from, tos := range edges {
		for to := range tos {
			if _, ok := allowedEdges[from+"->"+to]; ok {
				continue
			}
			if _, ok := allowedEdges[from+"->*"]; ok {
				continue
			}
			bad = append(bad, from+" -> "+to)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("%d module import edges outside allowedEdges:\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}
