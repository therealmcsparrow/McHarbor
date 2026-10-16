// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"testing"
)

// TestPlanFileModeSplitsWholeReferences is the core of file mode: a variable
// whose entire value is a reference becomes a file plus a `_FILE` pointer, and
// its plaintext never appears in the environment at all.
func TestPlanFileModeSplitsWholeReferences(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	mustCreate(t, svc, CreateRequest{Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "s3cr3t"})

	plan, err := svc.PlanFileMode(ctx, map[string]string{
		"WORDPRESS_DB_PASSWORD": "${secret:DB_PASSWORD}",
		"WORDPRESS_DB_USER":     "wordpress",
	}, ResolveScope{})
	if err != nil {
		t.Fatalf("planning: %v", err)
	}

	if got := plan.Env["WORDPRESS_DB_PASSWORD_FILE"]; got != "/run/secrets/WORDPRESS_DB_PASSWORD" {
		t.Errorf("_FILE pointer = %q", got)
	}
	if _, leaked := plan.Env["WORDPRESS_DB_PASSWORD"]; leaked {
		t.Error("the original variable must not survive — that is the whole point of file mode")
	}
	for key, value := range plan.Env {
		if value == "s3cr3t" {
			t.Errorf("the plaintext reached the environment in %s", key)
		}
	}
	if plan.Files["WORDPRESS_DB_PASSWORD"] != "s3cr3t" {
		t.Errorf("file content = %q", plan.Files["WORDPRESS_DB_PASSWORD"])
	}
	if plan.Env["WORDPRESS_DB_USER"] != "wordpress" {
		t.Errorf("a plain value should pass through, got %q", plan.Env["WORDPRESS_DB_USER"])
	}
}

// TestPlanFileModeLeavesEmbeddedReferencesInline covers the shape a file
// cannot stand in for: a reference interpolated into a larger string has no
// `_FILE` equivalent, because the image wants a whole DSN and not a password.
func TestPlanFileModeLeavesEmbeddedReferencesInline(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "s3cr3t"})

	plan, err := svc.PlanFileMode(ctx, map[string]string{
		"DATABASE_URL": "postgres://u:${secret:PW}@host/db",
	}, ResolveScope{})
	if err != nil {
		t.Fatalf("planning: %v", err)
	}

	if plan.Env["DATABASE_URL"] != "postgres://u:s3cr3t@host/db" {
		t.Errorf("embedded reference should resolve inline, got %q", plan.Env["DATABASE_URL"])
	}
	if len(plan.Files) != 0 {
		t.Errorf("nothing should be written to a file, got %v", plan.Files)
	}
	if len(plan.Replaced) != 0 {
		t.Errorf("Replaced should name nothing, got %v", plan.Replaced)
	}
}

func TestPlanFileModeWithNoReferencesCreatesNoFiles(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})

	plan, err := svc.PlanFileMode(context.Background(), map[string]string{
		"TZ": "UTC", "PORT": "8080",
	}, ResolveScope{})
	if err != nil {
		t.Fatalf("planning: %v", err)
	}
	if len(plan.Files) != 0 {
		t.Errorf("no references means no volume, got %v", plan.Files)
	}
	if plan.Env["TZ"] != "UTC" || plan.Env["PORT"] != "8080" {
		t.Errorf("plain values mangled: %v", plan.Env)
	}
}

// TestPlanFileModeSliceKeepsInheritedEntries pins the Docker-slice form: a
// bare `FOO` with no `=` means "inherit from the daemon", which has no value
// to put in a file.
func TestPlanFileModeSliceKeepsInheritedEntries(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "s3cr3t"})

	plan, env, err := svc.PlanFileModeSlice(ctx,
		[]string{"MYSQL_ROOT_PASSWORD=${secret:PW}", "TZ=UTC", "INHERIT_ME"}, ResolveScope{})
	if err != nil {
		t.Fatalf("planning: %v", err)
	}

	var sawFile, sawInherit bool
	for _, entry := range env {
		if entry == "MYSQL_ROOT_PASSWORD_FILE=/run/secrets/MYSQL_ROOT_PASSWORD" {
			sawFile = true
		}
		if entry == "INHERIT_ME" {
			sawInherit = true
		}
		if entry == "MYSQL_ROOT_PASSWORD=s3cr3t" {
			t.Error("the plaintext reached the environment")
		}
	}
	if !sawFile {
		t.Errorf("no _FILE entry in %v", env)
	}
	if !sawInherit {
		t.Errorf("the inherited entry was dropped: %v", env)
	}
	if plan.Files["MYSQL_ROOT_PASSWORD"] != "s3cr3t" {
		t.Errorf("file content = %q", plan.Files["MYSQL_ROOT_PASSWORD"])
	}
}

// TestPlanFileModeFailsOnAMiss — file mode changes where the value goes, not
// whether a missing reference is tolerated.
func TestPlanFileModeFailsOnAMiss(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})

	if _, err := svc.PlanFileMode(context.Background(),
		map[string]string{"X": "${secret:NOT_THERE}"}, ResolveScope{}); err == nil {
		t.Fatal("a missing reference must still fail the deploy")
	}
}
