// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/bootstrapfs"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// secretInEnviron reports whether MCHARBOR_SECRET is in the process environment in any form.
func secretInEnviron() bool {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, secretEnv+"=") {
			return true
		}
	}
	_, set := os.LookupEnv(secretEnv)
	return set
}

// TestLoadConfigRemovesTheSecretFromTheEnvironment: once the config has loaded, the secret
// lives in cfg.Secret only, whether the operator pinned it or startup generated it into the
// environment from the data directory, so no child process inherits it.
func TestLoadConfigRemovesTheSecretFromTheEnvironment(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv(secretEnv, "pinned-secret")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Secret != "pinned-secret" {
		t.Fatalf("the loaded config must carry the pinned secret, got %q", cfg.Secret)
	}
	if secretInEnviron() {
		t.Fatal("MCHARBOR_SECRET must be gone from the environment after config has loaded")
	}

	// The generated path, as main runs it: nothing pinned, so the data directory's secret is
	// exported for config.Load to read, and must be gone again afterwards.
	dataDir := t.TempDir()
	generated, err := bootstrapfs.EnsureSecret(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secretEnv, generated)
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Secret != generated || generated == "" {
		t.Fatalf("the loaded config must carry the generated secret, got %q", cfg.Secret)
	}
	if secretInEnviron() {
		t.Fatal("a generated MCHARBOR_SECRET must be gone from the environment after config has loaded")
	}
}

// TestLoadConfigRemovesEveryRootOfTrustVariable is H4 of the 2026-09-30 review: only
// MCHARBOR_SECRET was unset, so ENCRYPTION_KEY and MCHARBOR_DB_DSN stayed in the environment
// every compose and helper child inherited.
func TestLoadConfigRemovesEveryRootOfTrustVariable(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv(secretEnv, "pinned-secret-value")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("MCHARBOR_BACKUP_KEY", "backup-key-value")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EncryptionKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("the loaded config must keep the encryption key, got %q", cfg.EncryptionKey)
	}
	for _, name := range rootOfTrustEnv {
		if _, set := os.LookupEnv(name); set {
			t.Errorf("%s is still in the process environment after config has loaded", name)
		}
	}
	if !(envredact.Policy{Reveal: true}).Withhold("ANY", "x-0123456789abcdef0123456789abcdef-x") {
		t.Error("the loaded encryption key was not registered as a pinned value")
	}
}

// TestWhitespaceSecretIsNotOperatorSupplied is L7: MCHARBOR_SECRET="  " with no file used to
// count as operator-pinned, which skipped generating the data-directory secret.
func TestWhitespaceSecretIsNotOperatorSupplied(t *testing.T) {
	t.Setenv(secretEnv, "   ")
	t.Setenv(secretFileEnv, "")
	if secretFromOperator() {
		t.Fatal("a whitespace-only MCHARBOR_SECRET counted as supplied by the operator")
	}
	t.Setenv(secretEnv, "real-secret")
	if !secretFromOperator() {
		t.Fatal("a real MCHARBOR_SECRET must count as supplied by the operator")
	}
}
