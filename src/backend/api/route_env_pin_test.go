// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_test

import (
	"regexp"
	"strings"
	"testing"
)

// environmentsItem matches the /api/environments/{id} family, whose {id} is an
// environment; elsewhere {id} names some other resource and is not checked.
var environmentsItem = regexp.MustCompile(`^/api/environments/\{id\}(/|$)`)

// envPathParam returns the path parameter that names the environment a route acts
// on, or "" when the path names none.
func envPathParam(path string) string {
	switch {
	case strings.Contains(path, "{envId}"):
		return "envId"
	case strings.Contains(path, "{environmentId}"):
		return "environmentId"
	case environmentsItem.MatchString(path):
		return "id"
	}
	return ""
}

// envPinViolations lists every environment-scoped gate on a path that names an
// environment but is not pinned to that path parameter. Such a gate checks
// whichever `?env=` the caller sends while the handler acts on the path, so a
// grant on one environment reaches another.
func envPinViolations(ops []specOp) []string {
	var out []string
	for _, s := range ops {
		param := envPathParam(s.Path)
		if param == "" {
			continue
		}
		for _, gate := range s.Op.Permission {
			if gate.Scope == "environment" && gate.EnvParam != param {
				out = append(out, s.String()+": env_param="+gate.EnvParam+", want "+param+" (mount rbac.PinEnvToPath)")
			}
		}
	}
	return out
}

func TestEnvironmentScopedGatesArePinnedToThePath(t *testing.T) {
	if bad := envPinViolations(loadOperations(t)); len(bad) > 0 {
		t.Fatalf("%d environment-scoped gates are not pinned to the environment in their path:\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// TestEnvPinDetectorCatchesAnUnpinnedGate proves the check above can fail: it
// strips the pin from a real pinned operation and expects a violation.
func TestEnvPinDetectorCatchesAnUnpinnedGate(t *testing.T) {
	ops := loadOperations(t)
	for i, s := range ops {
		if envPathParam(s.Path) == "" || len(s.Op.Permission) == 0 || s.Op.Permission[0].Scope != "environment" {
			continue
		}
		gates := append([]permissionGate(nil), s.Op.Permission...)
		gates[0].EnvParam = ""
		ops[i].Op.Permission = gates
		if len(envPinViolations(ops)) == 0 {
			t.Fatalf("stripping the pin from %s went undetected", s)
		}
		return
	}
	t.Fatal("no environment-scoped operation on an environment path found; the spec is not what this test expects")
}
