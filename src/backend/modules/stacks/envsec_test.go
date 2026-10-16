// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import "testing"

// TestEnvVarMaskRoundTrip is the test step 1.8 of the secrets-manager plan
// makes non-optional. The failure it guards against is silent credential
// destruction: fetch the masked view, save the form without touching the
// secret field, and the stored password becomes a row of bullets.
func TestEnvVarMaskRoundTrip(t *testing.T) {
	stored := []envVarRow{
		{Key: "DB_PASSWORD", Value: "hunter2", IsSecret: true},
		{Key: "API_TOKEN", Value: "tok_live_abc", IsSecret: true},
		{Key: "LOG_LEVEL", Value: "debug", IsSecret: false},
	}

	// GET, masked.
	masked := maskEnvVars(stored)
	if masked["DB_PASSWORD"] != EnvMaskSentinel {
		t.Fatalf("DB_PASSWORD not masked: %q", masked["DB_PASSWORD"])
	}
	if masked["LOG_LEVEL"] != "debug" {
		t.Fatalf("non-secret value should not be masked: %q", masked["LOG_LEVEL"])
	}

	// PUT, that exact payload back.
	current := map[string]string{}
	for _, row := range stored {
		current[row.Key] = row.Value
	}
	merged := mergeMaskedEnvVars(masked, current)

	// GET with ?reveal=true — values intact.
	for _, row := range stored {
		if merged[row.Key] != row.Value {
			t.Errorf("%s round-tripped to %q, want %q", row.Key, merged[row.Key], row.Value)
		}
	}
}

func TestMergeMaskedEnvVars(t *testing.T) {
	current := map[string]string{"DB_PASSWORD": "hunter2", "LOG_LEVEL": "debug"}

	t.Run("a real value overwrites", func(t *testing.T) {
		got := mergeMaskedEnvVars(map[string]string{"DB_PASSWORD": "newpass"}, current)
		if got["DB_PASSWORD"] != "newpass" {
			t.Errorf("got %q, want newpass", got["DB_PASSWORD"])
		}
	})

	t.Run("an absent key is a deletion", func(t *testing.T) {
		got := mergeMaskedEnvVars(map[string]string{"LOG_LEVEL": "info"}, current)
		if _, ok := got["DB_PASSWORD"]; ok {
			t.Error("DB_PASSWORD should have been dropped, not resurrected from storage")
		}
	})

	t.Run("an empty string is a real value, not a mask", func(t *testing.T) {
		got := mergeMaskedEnvVars(map[string]string{"DB_PASSWORD": ""}, current)
		if got["DB_PASSWORD"] != "" {
			t.Errorf("got %q, want the caller's empty string", got["DB_PASSWORD"])
		}
	})

	t.Run("the sentinel for an unknown key does not store bullets", func(t *testing.T) {
		got := mergeMaskedEnvVars(map[string]string{"NEW_SECRET": EnvMaskSentinel}, current)
		if got["NEW_SECRET"] == EnvMaskSentinel {
			t.Error("the mask itself must never reach the database")
		}
	})
}

func TestLooksSecret(t *testing.T) {
	secret := []string{
		"DB_PASSWORD", "db_passwd", "JWT_SECRET", "GITHUB_TOKEN", "APIKEY",
		"STRIPE_API_KEY", "TLS_PRIVATE_KEY", "DATABASE_DSN", "PG_CONNECTION_STRING",
		"AWS_CREDENTIAL_FILE", "BASIC_AUTH", "SIGNING_KEY",
	}
	for _, key := range secret {
		if !looksSecret(key) {
			t.Errorf("%s should be treated as a credential", key)
		}
	}

	plain := []string{"LOG_LEVEL", "PORT", "TZ", "NODE_ENV", "PUID", "APP_NAME"}
	for _, key := range plain {
		if looksSecret(key) {
			t.Errorf("%s should not be treated as a credential", key)
		}
	}
}
