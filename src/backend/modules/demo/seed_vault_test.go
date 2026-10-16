// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

func vaultSpec(t *testing.T, name string) vaultSecret {
	t.Helper()
	for _, s := range demoVaultSecrets() {
		if s.name == name {
			return s
		}
	}
	t.Fatalf("no demo secret %s", name)
	return vaultSecret{}
}

// TestSeedVaultWritesAResolvableVault checks the seed against the vault's own
// code rather than its tables: a secret the resolver cannot find, or a stale
// badge that never shows, would look fine in SQL and wrong on the page.
func TestSeedVaultWritesAResolvableVault(t *testing.T) {
	m := activateDemo(t)
	f := newGitSeedFixture(t, m)
	ctx := context.Background()

	got, err := seedVault(ctx, f.conn, f.enc, f.estate, f.envIDs, m, f.now)
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if want := len(demoVaultSecrets()); got.Secrets != want {
		t.Fatalf("seeded %d secrets, want all %d: a scope the estate lacks was skipped", got.Secrets, want)
	}
	if got.References == 0 || got.Versions == 0 {
		t.Fatalf("seed wrote no references or history: %+v", got)
	}

	again, err := seedVault(ctx, f.conn, f.enc, f.estate, f.envIDs, m, f.now)
	if err != nil {
		t.Fatalf("re-seeding: %v", err)
	}
	if again != (vaultSeedCounts{}) {
		t.Fatalf("a second boot re-seeded: %+v", again)
	}
	if n := count(t, f.conn, "SELECT COUNT(*) FROM secrets"); n != got.Secrets {
		t.Fatalf("secrets after re-seed = %d, want %d", n, got.Secrets)
	}

	stripe := vaultSpec(t, "STRIPE_API_KEY")
	var stackName, envID string
	if err := f.conn.QueryRowContext(ctx, "SELECT stack_name, environment_id FROM secrets WHERE name = ?", stripe.name).
		Scan(&stackName, &envID); err != nil {
		t.Fatalf("reading seeded stack secret: %v", err)
	}
	if !strings.HasPrefix(stackName, "payments") {
		t.Fatalf("STRIPE_API_KEY scoped to stack %q, want a payments stack", stackName)
	}
	svc := secrets.NewService(f.conn, f.enc, nil)
	resolved, err := svc.ResolveValue(ctx, "${secret:STRIPE_API_KEY}",
		secrets.ResolveScope{EnvironmentID: envID, StackName: stackName}, "STRIPE_API_KEY")
	if err != nil {
		t.Fatalf("resolving through the vault: %v", err)
	}
	if want := vaultValue(m, stripe, stripe.versions); resolved != want || !strings.HasPrefix(resolved, "sk_live_") {
		t.Fatalf("resolved %q, want the current version %q", resolved, want)
	}
	if vaultValue(m, stripe, 1) == resolved {
		t.Fatal("every version has the same value, so a rollback would show nothing changing")
	}
	if n := count(t, f.conn, `SELECT COUNT(*) FROM secret_versions v JOIN secrets s ON s.id = v.secret_id
		WHERE s.name = 'STRIPE_API_KEY'`); n != stripe.versions-1 {
		t.Fatalf("STRIPE_API_KEY history rows = %d, want %d", n, stripe.versions-1)
	}

	for name, want := range map[string]int{"REGISTRY_TOKEN": 1, "STRIPE_API_KEY": 1, "SMTP_PASSWORD": 0} {
		stale := count(t, f.conn, `SELECT COUNT(*) FROM secret_references r JOIN secrets s ON s.id = r.secret_id
			WHERE s.name = '`+name+`' AND r.applied_version < s.version`)
		if stale != want {
			t.Errorf("%s stale references = %d, want %d", name, stale, want)
		}
	}
	if n := count(t, f.conn, `SELECT COUNT(*) FROM secret_references r JOIN secrets s ON s.id = r.secret_id
		WHERE s.name = 'LEGACY_FTP_PASSWORD'`); n != 0 {
		t.Errorf("the legacy secret should be unused, has %d references", n)
	}
	if n := count(t, f.conn, "SELECT COUNT(*) FROM secrets WHERE scope = 'environment'"); n == 0 {
		t.Error("no environment-scoped secret was seeded")
	}
}
