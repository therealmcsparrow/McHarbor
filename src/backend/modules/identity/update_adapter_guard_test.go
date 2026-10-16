// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"errors"
	"testing"
)

// TestUpdateRefusesConfigWithoutAdapter: with the provider's plugin not loaded, its
// ConfigSchema cannot say which keys are secret, so a config change would store every
// secret in plaintext. The change is refused; an edit that leaves config alone is not.
func TestUpdateRefusesConfigWithoutAdapter(t *testing.T) {
	svc, database := openIdentityTestDB(t)
	mustExec(t, database, `INSERT INTO identity_providers (id, name, provider_type, enabled, client_id, client_secret, config)
		VALUES ('p-gone', 'Gone', 'oidc_plugin_gone', 1, '', '', '{}')`)

	_, err := svc.Update(t.Context(), "p-gone", UpdateProviderInput{Config: map[string]string{"client_secret": "plain-secret"}})
	if !errors.Is(err, ErrIdentityAdapterUnavailable) {
		t.Fatalf("config update without an adapter = %v, want ErrIdentityAdapterUnavailable", err)
	}
	var stored string
	if err := database.QueryRow(`SELECT COALESCE(config, '') FROM identity_providers WHERE id = 'p-gone'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "{}" {
		t.Fatalf("the refused update still wrote config %q", stored)
	}

	name := "Renamed"
	if _, err := svc.Update(t.Context(), "p-gone", UpdateProviderInput{Name: &name}); err != nil {
		t.Fatalf("an update that leaves config alone was refused: %v", err)
	}
}
