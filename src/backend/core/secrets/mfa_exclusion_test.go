// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"strings"
	"testing"
)

// TestMFAStaysOutOfTheVault is step 5b.12 of the secrets-manager plan, shipped
// as a permanent assertion rather than a one-off check.
//
// TOTP seeds and recovery codes (`modules/mfa/service.go:132,140,286`) are NOT
// operator credentials — they are the user's second factor. A vault that has a
// reveal endpoint would let an administrator with an approved elevation read a
// user's TOTP seed and thereafter impersonate their MFA indefinitely, without
// the user ever knowing. That is a straight downgrade of the authentication
// system, and the inconsistency with the other eleven modules is deliberate.
//
// Confirmed by the project owner on 2026-09-13 as a settled decision, not a
// default to be revisited.
//
// **If this test fails, do not fix it by updating the expectation.** The
// realistic way it regresses is a future implementer "finishing the
// unification" — which is exactly the change this test exists to stop. Read
// §5.11 of `.references/todo/secrets-manager.md` first.
func TestMFAStaysOutOfTheVault(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	if err := svc.AdoptAll(ctx); err != nil {
		t.Fatalf("running the adoption pass: %v", err)
	}

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM secrets WHERE origin_module = 'mfa'",
	).Scan(&count); err != nil {
		t.Fatalf("counting mfa-origin secrets: %v", err)
	}
	if count != 0 {
		t.Fatalf("%d MFA credential(s) were adopted into the vault; see §5.11 — this is a settled exclusion, not an oversight", count)
	}
}

// TestLegacyCatalogExcludesMFAAndRotatingTokens pins the exclusions at the
// declaration rather than at the outcome, so a bad entry is caught even on a
// database with no MFA rows in it.
func TestLegacyCatalogExcludesMFAAndRotatingTokens(t *testing.T) {
	banned := map[string]string{
		"user_mfa":         "MFA seeds and recovery codes are the user's second factor (§5.11)",
		"mfa_recovery":     "MFA recovery codes are the user's second factor (§5.11)",
		"access_token":     "machine-rotated OAuth tokens are excluded (§5.11)",
		"refresh_token":    "machine-rotated OAuth tokens are excluded (§5.11)",
		"oauth_access_tok": "machine-rotated OAuth tokens are excluded (§5.11)",
	}

	for _, spec := range LegacyCatalog() {
		for fragment, why := range banned {
			if strings.Contains(spec.Table, fragment) || strings.Contains(spec.ValueColumn, fragment) {
				t.Errorf("%s.%s must not be adopted: %s", spec.Table, spec.ValueColumn, why)
			}
		}
		if spec.Module == "mfa" {
			t.Error("no catalogue entry may name the mfa module (§5.11)")
		}
	}
}

// TestLegacyCatalogIsWellFormed catches the copy-paste mistakes the shape of
// this catalogue invites: a duplicated pointer column would have two specs
// fighting over one row, and a missing field would build invalid SQL.
func TestLegacyCatalogIsWellFormed(t *testing.T) {
	seen := map[string]string{}
	for _, spec := range LegacyCatalog() {
		if spec.Table == "" || spec.IDColumn == "" || spec.ValueColumn == "" ||
			spec.SecretIDColumn == "" || spec.NamePrefix == "" || spec.Module == "" {
			t.Errorf("incomplete catalogue entry: %+v", spec)
			continue
		}
		key := spec.Table + "." + spec.ValueColumn + "|" + spec.Where
		if previous, exists := seen[key]; exists {
			t.Errorf("%s is declared twice (%s and %s)", key, previous, spec.NamePrefix)
		}
		seen[key] = spec.NamePrefix

		if err := validateKind(spec.Kind); err != nil {
			t.Errorf("%s.%s declares an invalid kind %q", spec.Table, spec.ValueColumn, spec.Kind)
		}
		// The generated name has to survive the vault's own validation.
		if err := ValidateName(systemSecretName(spec.NamePrefix, "Example Name", "d1f2g3h4")); err != nil {
			t.Errorf("%s produces an invalid secret name: %v", spec.NamePrefix, err)
		}
		if err := ValidateName(systemSecretName(spec.NamePrefix, "", "d1f2g3h4")); err != nil {
			t.Errorf("%s produces an invalid secret name with no label: %v", spec.NamePrefix, err)
		}
	}
}
