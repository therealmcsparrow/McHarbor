// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_test

import (
	"regexp"
	"strings"
	"testing"
)

// rowScopedFamilies are routes that address a row living in one environment.
// Their gates must decide on that row's environment (rbac.PinEnvToResource),
// or a grant on one environment reads and changes another's rows by naming it
// in `?env=`. Found by the 2026-09-29 cross-environment audit.
var rowScopedFamilies = []*regexp.Regexp{
	regexp.MustCompile(`^/api/secrets/\{id\}`),
	regexp.MustCompile(`^/api/container-backups/runs/\{runId\}`),
	regexp.MustCompile(`^/api/container-backups/\{planId\}`),
	regexp.MustCompile(`^/api/container-backups/snapshot-configs/\{id\}`),
	regexp.MustCompile(`^/api/reconciler/\{id\}`),
	regexp.MustCompile(`^/api/reconciler/proposals/\{pid\}`),
	regexp.MustCompile(`^/api/reconciler/sources/\{sid\}`),
	regexp.MustCompile(`^/api/scans/\{id\}`),
	regexp.MustCompile(`^/api/incidents/\{id\}`),
	regexp.MustCompile(`^/api/incidents/findings/\{fid\}`),
	regexp.MustCompile(`^/api/incidents/by-alert/\{eventId\}`),
	regexp.MustCompile(`^/api/runbooks/\{id\}`),
	regexp.MustCompile(`^/api/runbooks/runs/\{id\}`),
	regexp.MustCompile(`^/api/updates/\{id\}`),
}

func resourcePinViolations(ops []specOp) []string {
	var out []string
	for _, s := range ops {
		inFamily := false
		for _, re := range rowScopedFamilies {
			inFamily = inFamily || re.MatchString(s.Path)
		}
		if !inFamily {
			continue
		}
		for _, gate := range s.Op.Permission {
			if gate.Scope == "environment" && gate.EnvParam != "resource" {
				out = append(out, s.String()+": env_param="+gate.EnvParam+", want resource (mount rbac.PinEnvToResource)")
			}
		}
	}
	return out
}

func TestRowScopedRoutesDecideOnTheRowsEnvironment(t *testing.T) {
	if bad := resourcePinViolations(loadOperations(t)); len(bad) > 0 {
		t.Fatalf("%d row-scoped gates decide on the caller's ?env= instead of the row's environment:\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// TestResourcePinDetectorCatchesAnUnpinnedGate proves the check above can fail.
func TestResourcePinDetectorCatchesAnUnpinnedGate(t *testing.T) {
	ops := loadOperations(t)
	for i, s := range ops {
		if !strings.HasPrefix(s.Path, "/api/secrets/{id}") || len(s.Op.Permission) == 0 {
			continue
		}
		gates := append([]permissionGate(nil), s.Op.Permission...)
		gates[0].EnvParam = ""
		ops[i].Op.Permission = gates
		if len(resourcePinViolations(ops)) == 0 {
			t.Fatalf("stripping the pin from %s went undetected", s)
		}
		return
	}
	t.Fatal("no /api/secrets/{id} operation found; the spec is not what this test expects")
}

// stackItem matches every route under one named stack. Stacks are addressed by name while
// each lives in one environment, so their gates must decide on the stack's environment:
// the 2026-09-30 review found reveal, PUT /compose and PUT /env-vars reaching a stack in
// another environment through a grant on the caller's ?env=.
var stackItem = regexp.MustCompile(`^/api/stacks/\{name\}(/|$)`)

func stackPinViolations(ops []specOp) []string {
	var out []string
	for _, s := range ops {
		if !stackItem.MatchString(s.Path) {
			continue
		}
		for _, gate := range s.Op.Permission {
			if (gate.Scope == "environment" || gate.Scope == "stack") && gate.EnvParam != "resource" {
				out = append(out, s.String()+": scope="+gate.Scope+" env_param="+gate.EnvParam+", want resource (mount rbac.PinEnvToResource)")
			}
		}
	}
	return out
}

func TestStackRoutesDecideOnTheStacksEnvironment(t *testing.T) {
	if bad := stackPinViolations(loadOperations(t)); len(bad) > 0 {
		t.Fatalf("%d stack gates decide on the caller's ?env= instead of the stack's environment:\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// TestStackPinDetectorCatchesAnUnpinnedGate proves the check above can fail.
func TestStackPinDetectorCatchesAnUnpinnedGate(t *testing.T) {
	ops := loadOperations(t)
	for i, s := range ops {
		if s.Path != "/api/stacks/{name}/env-vars" || len(s.Op.Permission) == 0 {
			continue
		}
		gates := append([]permissionGate(nil), s.Op.Permission...)
		gates[0].EnvParam = ""
		ops[i].Op.Permission = gates
		if len(stackPinViolations(ops)) == 0 {
			t.Fatalf("stripping the pin from %s went undetected", s)
		}
		return
	}
	t.Fatal("no /api/stacks/{name}/env-vars operation found; the spec is not what this test expects")
}
