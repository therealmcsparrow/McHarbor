// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseReferences(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantNames []string
		wantText  string
	}{
		{
			name:     "plain value is untouched",
			in:       "postgres",
			wantText: "postgres",
		},
		{
			name:      "a bare reference",
			in:        "${secret:DB_PASSWORD}",
			wantNames: []string{"DB_PASSWORD"},
			wantText:  "${secret:DB_PASSWORD}",
		},
		{
			name:      "a reference embedded mid-string",
			in:        "postgres://u:${secret:PW}@host/db",
			wantNames: []string{"PW"},
			wantText:  "postgres://u:${secret:PW}@host/db",
		},
		{
			name:      "two distinct references",
			in:        "${secret:USER}:${secret:PW}",
			wantNames: []string{"USER", "PW"},
			wantText:  "${secret:USER}:${secret:PW}",
		},
		{
			name:      "the same reference twice is listed once",
			in:        "${secret:PW}-${secret:PW}",
			wantNames: []string{"PW"},
			wantText:  "${secret:PW}-${secret:PW}",
		},
		{
			name:     "$$ escapes a reference to a literal",
			in:       "$${secret:DB_PASSWORD}",
			wantText: "${secret:DB_PASSWORD}",
		},
		{
			name:      "an escape and a live reference side by side",
			in:        "$${secret:LITERAL} ${secret:LIVE}",
			wantNames: []string{"LIVE"},
			wantText:  "${secret:LITERAL} ${secret:LIVE}",
		},
		{
			name:     "a lowercase name is not a reference",
			in:       "${secret:db_password}",
			wantText: "${secret:db_password}",
		},
		{
			name:     "an unterminated reference is passed through",
			in:       "${secret:DB_PASSWORD",
			wantText: "${secret:DB_PASSWORD",
		},
		{
			name:     "a compose variable is not a secret reference",
			in:       "${POSTGRES_PASSWORD}",
			wantText: "${POSTGRES_PASSWORD}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names, text := parseReferences(tc.in)
			if len(names) == 0 && len(tc.wantNames) == 0 {
				// both empty — fine
			} else if !reflect.DeepEqual(names, tc.wantNames) {
				t.Errorf("names = %v, want %v", names, tc.wantNames)
			}
			if text != tc.wantText {
				t.Errorf("text = %q, want %q", text, tc.wantText)
			}
		})
	}
}

func TestContainsReference(t *testing.T) {
	if !ContainsReference("a ${secret:X} b") {
		t.Error("a live reference should be detected")
	}
	if ContainsReference("$${secret:X}") {
		t.Error("an escaped reference is a literal, not a reference")
	}
	if ContainsReference("${PLAIN_VAR}") {
		t.Error("a compose variable is not a secret reference")
	}
}

// TestResolvePrecedence walks the whole scope ladder. The same name exists at
// three scopes; each lookup must take the narrowest one visible.
func TestResolvePrecedence(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	env := "envAAA"
	stack := "wordpress"

	mustCreate(t, svc, CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "global-value",
	})
	mustCreate(t, svc, CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeEnvironment, EnvironmentID: &env, Value: "env-value",
	})
	mustCreate(t, svc, CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeStack, EnvironmentID: &env, StackName: &stack, Value: "stack-value",
	})

	cases := []struct {
		name  string
		scope ResolveScope
		want  string
	}{
		{"stack scope wins", ResolveScope{EnvironmentID: env, StackName: stack}, "stack-value"},
		{"environment scope next", ResolveScope{EnvironmentID: env}, "env-value"},
		{"global is the fallback", ResolveScope{}, "global-value"},
		{"an unknown stack falls back to the environment", ResolveScope{EnvironmentID: env, StackName: "other"}, "env-value"},
		{"an unknown environment falls back to global", ResolveScope{EnvironmentID: "envZZZ"}, "global-value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ResolveValue(ctx, "${secret:DB_PASSWORD}", tc.scope, "DB_PASSWORD")
			if err != nil {
				t.Fatalf("resolving: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveMissFailsLoudly is the one the plan flags as blast-radius item
// number four. Substituting empty instead of failing starts databases with
// blank passwords.
func TestResolveMissFailsLoudly(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	got, err := svc.ResolveValue(ctx, "${secret:NOT_THERE}", ResolveScope{}, "X")
	if err == nil {
		t.Fatalf("a missing secret must fail the deploy, got %q", got)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("the error should map onto ErrSecretNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "NOT_THERE") {
		t.Errorf("the error must name the reference, got %q", err.Error())
	}
	if got != "" {
		t.Errorf("nothing should be returned on a miss, got %q", got)
	}
}

func TestResolveEmbeddedMidString(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "s3cr3t"})

	got, err := svc.ResolveValue(ctx, "postgres://u:${secret:PW}@host/db", ResolveScope{}, "DSN")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if got != "postgres://u:s3cr3t@host/db" {
		t.Errorf("got %q", got)
	}
}

func TestResolveMapDoesNotMutateInput(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "s3cr3t"})

	stored := map[string]string{"DB_PASSWORD": "${secret:PW}", "LOG_LEVEL": "debug"}
	resolved, err := svc.ResolveMap(ctx, stored, ResolveScope{})
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if resolved["DB_PASSWORD"] != "s3cr3t" {
		t.Errorf("resolved value = %q", resolved["DB_PASSWORD"])
	}
	if stored["DB_PASSWORD"] != "${secret:PW}" {
		t.Fatal("the caller's map was mutated — the stored reference would be overwritten with plaintext on its next save")
	}
	if resolved["LOG_LEVEL"] != "debug" {
		t.Errorf("a plain value should pass through, got %q", resolved["LOG_LEVEL"])
	}
}

func TestResolveSlice(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "s3cr3t"})

	got, err := svc.ResolveSlice(ctx, []string{
		"DB_PASSWORD=${secret:PW}",
		"LOG_LEVEL=debug",
		"NO_EQUALS_SIGN",
		"EMPTY=",
	}, ResolveScope{})
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	want := []string{"DB_PASSWORD=s3cr3t", "LOG_LEVEL=debug", "NO_EQUALS_SIGN", "EMPTY="}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestResolveRecordsUse covers the bookkeeping half: a successful resolution
// stamps last_used_at and leaves exactly one reference row per field, however
// many times the stack is deployed.
func TestResolveRecordsUse(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec := mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "s3cr3t"})
	if sec.LastUsedAt != nil {
		t.Fatal("a fresh secret should have no last_used_at")
	}

	scope := ResolveScope{ResourceType: "stack", ResourceID: "wordpress"}
	for i := 0; i < 3; i++ {
		if _, err := svc.ResolveValue(ctx, "${secret:PW}", scope, "DB_PASSWORD"); err != nil {
			t.Fatalf("resolving: %v", err)
		}
	}

	after, err := svc.ByID(ctx, sec.ID)
	if err != nil {
		t.Fatalf("reloading secret: %v", err)
	}
	if after.LastUsedAt == nil {
		t.Error("last_used_at was not stamped")
	}
	if after.ReferenceCount != 1 {
		t.Errorf("three deploys should leave one reference row, got %d", after.ReferenceCount)
	}
}

// TestResolveRecordsNothingWithoutAResource pins that a validation pass or a
// dry run can resolve without writing reference rows.
func TestResolveRecordsNothingWithoutAResource(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec := mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "s3cr3t"})
	if _, err := svc.ResolveValue(ctx, "${secret:PW}", ResolveScope{}, "X"); err != nil {
		t.Fatalf("resolving: %v", err)
	}

	after, err := svc.ByID(ctx, sec.ID)
	if err != nil {
		t.Fatalf("reloading secret: %v", err)
	}
	if after.ReferenceCount != 0 {
		t.Errorf("expected no reference rows, got %d", after.ReferenceCount)
	}
}

// TestRotationTakesEffectOnNextResolve is step 3.6 at the unit level: rotate,
// resolve again, get the new value. There is no cache to go stale.
func TestRotationTakesEffectOnNextResolve(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec := mustCreate(t, svc, CreateRequest{Name: "PW", Scope: ScopeGlobal, Value: "old"})

	before, err := svc.ResolveValue(ctx, "${secret:PW}", ResolveScope{}, "X")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if before != "old" {
		t.Fatalf("got %q, want old", before)
	}

	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: "new"}); err != nil {
		t.Fatalf("rotating: %v", err)
	}

	after, err := svc.ResolveValue(ctx, "${secret:PW}", ResolveScope{}, "X")
	if err != nil {
		t.Fatalf("resolving after rotation: %v", err)
	}
	if after != "new" {
		t.Errorf("got %q, want new — a rotation must take effect on the next deploy", after)
	}
}

func mustCreate(t *testing.T, svc *Service, req CreateRequest) *Secret {
	t.Helper()
	sec, _, err := svc.Create(context.Background(), "user1", "", req)
	if err != nil {
		t.Fatalf("creating secret %s: %v", req.Name, err)
	}
	return sec
}
