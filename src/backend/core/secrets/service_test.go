// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// fakeAuthorizer answers the one question the service asks RBAC. The service
// has to make the global-scope decision itself — route middleware cannot see
// the request body — so this is the seam that makes it testable.
type fakeAuthorizer struct {
	global bool
	err    error
}

func (f fakeAuthorizer) HasPermission(_ context.Context, _, _ string, perm rbac.Permission) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if perm == rbac.PermSecretsManageGlobal {
		return f.global, nil
	}
	return true, nil
}

func newTestService(t *testing.T, authz Authorizer) (*Service, *sql.DB) {
	t.Helper()
	dir := t.TempDir()

	database, err := coredb.OpenSQLite(filepath.Join(dir, "secrets_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	})

	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("creating encryption service: %v", err)
	}

	// secrets.created_by and secrets.environment_id are real foreign keys, so
	// the fixtures they point at have to exist before anything can be
	// inserted.
	for _, id := range []string{"user1", "user2", "admin"} {
		if _, err := database.Exec(
			"INSERT INTO users (id, username, password_hash) VALUES (?, ?, 'x')", id, id,
		); err != nil {
			t.Fatalf("seeding user %s: %v", id, err)
		}
	}
	if _, err := database.Exec(
		"INSERT INTO environments (id, name) VALUES ('envAAA', 'Test Env')",
	); err != nil {
		t.Fatalf("seeding environment: %v", err)
	}

	return NewService(database, enc, authz), database
}

func TestScopeKeyDistinguishesScopes(t *testing.T) {
	env := "envAAA"
	stack := "wordpress"

	global := scopeKey("DB_PASSWORD", nil, nil)
	perEnv := scopeKey("DB_PASSWORD", &env, nil)
	perStack := scopeKey("DB_PASSWORD", &env, &stack)

	if global == perEnv || perEnv == perStack || global == perStack {
		t.Fatal("the same name at three scopes must produce three distinct scope keys")
	}
	if len(global) != 64 {
		t.Errorf("scope key should be 64 hex characters, got %d", len(global))
	}
	if scopeKey("DB_PASSWORD", &env, nil) != perEnv {
		t.Error("scope key must be deterministic")
	}
}

// TestScopeKeyIsRecomputedOnPatch is the drift test. A stale scope_key
// silently permits a duplicate name at the same scope, which is the single
// bug the column exists to prevent, so moving a secret between scopes has to
// move its key with it.
func TestScopeKeyIsRecomputedOnPatch(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	env := "envAAA"

	sec, _, err := svc.Create(ctx, "user1", env, CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "hunter2",
	})
	if err != nil {
		t.Fatalf("creating global secret: %v", err)
	}

	before := readScopeKey(t, database, sec.ID)
	if before != scopeKey("DB_PASSWORD", nil, nil) {
		t.Fatal("scope key was not written from the canonical function on insert")
	}

	if _, err := svc.Patch(ctx, "user1", env, sec.ID, PatchRequest{
		Scope: strPtr(ScopeEnvironment), EnvironmentID: &env,
	}); err != nil {
		t.Fatalf("patching to environment scope: %v", err)
	}

	after := readScopeKey(t, database, sec.ID)
	if after == before {
		t.Fatal("scope key did not change when the secret moved scope")
	}
	if after != scopeKey("DB_PASSWORD", &env, nil) {
		t.Fatal("scope key after patch was not recomputed from the canonical function")
	}

	// The slot the secret vacated must now be free, and the one it moved into
	// must now be taken.
	if _, _, err := svc.Create(ctx, "user1", env, CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "other",
	}); err != nil {
		t.Fatalf("the vacated global slot should be free: %v", err)
	}
	_, _, err = svc.Create(ctx, "user1", env, CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeEnvironment, EnvironmentID: &env, Value: "other",
	})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("the occupied environment slot should be taken, got %v", err)
	}
}

func TestDuplicateAtSameScopeRejected(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	if _, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "API_TOKEN", Scope: ScopeGlobal, Value: "a",
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "API_TOKEN", Scope: ScopeGlobal, Value: "b",
	})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}

	// The same name at a different scope is fine — that is the whole point of
	// having a scope.
	env := "envAAA"
	if _, _, err := svc.Create(ctx, "user1", env, CreateRequest{
		Name: "API_TOKEN", Scope: ScopeEnvironment, EnvironmentID: &env, Value: "c",
	}); err != nil {
		t.Fatalf("same name at a different scope should be allowed: %v", err)
	}
}

// TestGlobalScopeSplit is the check the plan calls out as impossible to do in
// middleware: a user with secrets.manage but not secrets.manage_global can
// create an environment secret, cannot create a global one, and cannot patch
// the former into the latter.
func TestGlobalScopeSplit(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: false})
	ctx := context.Background()
	env := "envAAA"

	_, _, err := svc.Create(ctx, "user1", env, CreateRequest{
		Name: "GLOBAL_KEY", Scope: ScopeGlobal, Value: "x",
	})
	if !errors.Is(err, ErrGlobalDenied) {
		t.Fatalf("creating a global secret without manage_global should be denied, got %v", err)
	}

	sec, _, err := svc.Create(ctx, "user1", env, CreateRequest{
		Name: "ENV_KEY", Scope: ScopeEnvironment, EnvironmentID: &env, Value: "x",
	})
	if err != nil {
		t.Fatalf("creating an environment secret should be allowed: %v", err)
	}

	_, err = svc.Patch(ctx, "user1", env, sec.ID, PatchRequest{Scope: strPtr(ScopeGlobal)})
	if !errors.Is(err, ErrGlobalDenied) {
		t.Fatalf("promoting to global without manage_global should be denied, got %v", err)
	}
}

// TestGlobalDemotionAlsoNeedsAuthority covers the other direction: a holder of
// plain secrets.manage must not be able to pull a global secret out from under
// every environment that depends on it.
func TestGlobalDemotionAlsoNeedsAuthority(t *testing.T) {
	privileged, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	env := "envAAA"

	sec, _, err := privileged.Create(ctx, "admin", env, CreateRequest{
		Name: "SHARED_KEY", Scope: ScopeGlobal, Value: "x",
	})
	if err != nil {
		t.Fatalf("creating global secret: %v", err)
	}

	limited := NewService(database, privileged.enc, fakeAuthorizer{global: false})
	_, err = limited.Patch(ctx, "user1", env, sec.ID, PatchRequest{
		Scope: strPtr(ScopeEnvironment), EnvironmentID: &env,
	})
	if !errors.Is(err, ErrGlobalDenied) {
		t.Fatalf("demoting a global secret without manage_global should be denied, got %v", err)
	}
}

// TestSystemSecretImmutability pins §5.10: a system = 1 row is not deletable
// through the ordinary route, force or not, and is not revealable at all.
func TestSystemSecretImmutability(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "SMTP_PASSWORD", Scope: ScopeGlobal, Value: "x",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	if _, err := database.Exec(
		"UPDATE secrets SET system = 1, origin_module = 'communications' WHERE id = ?", sec.ID,
	); err != nil {
		t.Fatalf("marking secret as system-owned: %v", err)
	}

	for _, force := range []bool{false, true} {
		if err := svc.Delete(ctx, sec.ID, force); !errors.Is(err, ErrSystemImmutable) {
			t.Errorf("Delete(force=%v) should be refused, got %v", force, err)
		}
	}
	if _, err := svc.Reveal(ctx, sec.ID); !errors.Is(err, ErrSystemImmutable) {
		t.Errorf("Reveal on a system secret should be refused, got %v", err)
	}
	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: "y"}); !errors.Is(err, ErrSystemImmutable) {
		t.Errorf("Rotate on a system secret should be refused, got %v", err)
	}
}

func TestDeleteRefusesWhileReferenced(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "x",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	field := "WORDPRESS_DB_PASSWORD"
	if err := svc.TouchReference(ctx, SecretReference{
		SecretID: sec.ID, ResourceType: "stack", ResourceID: "wordpress", Field: &field,
	}); err != nil {
		t.Fatalf("recording reference: %v", err)
	}

	// The same reference twice is one row: re-resolving on every deploy must
	// not accumulate.
	if err := svc.TouchReference(ctx, SecretReference{
		SecretID: sec.ID, ResourceType: "stack", ResourceID: "wordpress", Field: &field,
	}); err != nil {
		t.Fatalf("re-recording reference: %v", err)
	}
	refs, err := svc.References(ctx, sec.ID)
	if err != nil {
		t.Fatalf("listing references: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference after two touches, got %d", len(refs))
	}

	if err := svc.Delete(ctx, sec.ID, false); !errors.Is(err, ErrInUse) {
		t.Fatalf("expected ErrInUse, got %v", err)
	}
	if err := svc.Delete(ctx, sec.ID, true); err != nil {
		t.Fatalf("forced delete should succeed: %v", err)
	}
}

// TestRotationArchivesAndPrunes checks the history window and that a rollback
// restores the value without ever displaying it.
func TestRotationArchivesAndPrunes(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "ROTATING", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	for i := 2; i <= 9; i++ {
		if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{
			Value: "v" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
	}

	versions, err := svc.Versions(ctx, sec.ID)
	if err != nil {
		t.Fatalf("listing versions: %v", err)
	}
	if len(versions) > versionRetention {
		t.Fatalf("history should be pruned to %d, got %d", versionRetention, len(versions))
	}

	current, err := svc.Reveal(ctx, sec.ID)
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if current != "v9" {
		t.Fatalf("current value should be v9, got %q", current)
	}

	// Roll back to the newest archived version and confirm the value moved.
	target := versions[0].Version
	if _, err := svc.Rollback(ctx, "user1", "", sec.ID, target); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	restored, err := svc.Reveal(ctx, sec.ID)
	if err != nil {
		t.Fatalf("reveal after rollback: %v", err)
	}
	if restored == current {
		t.Fatal("rollback did not change the current value")
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"A", "DB_PASSWORD", "API_KEY_2", "X9_Z"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("%q should be valid: %v", name, err)
		}
	}
	invalid := []string{
		"", "db_password", "1LEADING", "_LEADING", "HAS-HYPHEN", "HAS SPACE",
		"HAS.DOT", strings.Repeat("A", 65),
	}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("%q should be rejected", name)
		}
	}
}

func TestGenerateValue(t *testing.T) {
	a, err := GenerateValue(0)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	b, err := GenerateValue(0)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if a == b {
		t.Fatal("two generated values must not be equal")
	}
	if len(a) < 32 {
		t.Errorf("default generated value looks too short: %d characters", len(a))
	}

	short, err := GenerateValue(1)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if len(short) < 20 {
		t.Errorf("a tiny length request should be clamped up, got %d characters", len(short))
	}
}

// TestRevealRateLimit walks the limiter over the cap using the audit rows the
// reveal handler writes.
func TestRevealRateLimit(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	if err := svc.RevealRateLimit(ctx, "user1"); err != nil {
		t.Fatalf("a user with no reveals should be under the limit: %v", err)
	}

	for i := 0; i < revealsPerHour; i++ {
		if _, err := database.Exec(`
			INSERT INTO audit_logs (id, user_id, action, timestamp, created_at, updated_at)
			VALUES (?, 'user1', ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'),
			        strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		`, "audit"+strconv.Itoa(i), RevealAuditAction); err != nil {
			t.Fatalf("seeding audit row %d: %v", i, err)
		}
	}

	if err := svc.RevealRateLimit(ctx, "user1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited at the cap, got %v", err)
	}
	if err := svc.RevealRateLimit(ctx, "user2"); err != nil {
		t.Fatalf("the limit is per user, not global: %v", err)
	}
}

// TestSecretTypeCarriesNoValueField is the structural half of "a list
// response cannot leak a credential". The rule is not "we remembered to omit
// it" — the field does not exist, so no future change can reintroduce it by
// accident without failing here.
func TestSecretTypeCarriesNoValueField(t *testing.T) {
	for _, banned := range []string{"Value", "Plaintext", "Ciphertext", "Secret"} {
		if hasField(Secret{}, banned) {
			t.Errorf("Secret must not carry a %s field", banned)
		}
		if hasField(SecretVersion{}, banned) {
			t.Errorf("SecretVersion must not carry a %s field", banned)
		}
	}
}

// readScopeKey reads the stored scope_key straight from the table, so the
// test asserts on what was written rather than on what the service says it
// wrote.
func readScopeKey(t *testing.T, database *sql.DB, id string) string {
	t.Helper()
	var key string
	if err := database.QueryRow("SELECT scope_key FROM secrets WHERE id = ?", id).Scan(&key); err != nil {
		t.Fatalf("reading scope_key: %v", err)
	}
	return key
}

func strPtr(s string) *string { return &s }

// hasField reports whether a struct type declares a field by name.
func hasField(v any, name string) bool {
	_, ok := reflect.TypeOf(v).FieldByName(name)
	return ok
}
