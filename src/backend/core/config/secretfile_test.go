// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSecret(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSecretFilesLoadIntoTheConfig is the Docker-secrets path: the values arrive from files,
// so they never sit in the container's Config.Env where docker inspect shows them.
func TestSecretFilesLoadIntoTheConfig(t *testing.T) {
	t.Setenv("MCHARBOR_SECRET", "")
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("MCHARBOR_SECRET_FILE", writeSecret(t, "mcharbor_secret", "file-secret-value\n"))
	t.Setenv("ENCRYPTION_KEY_FILE", writeSecret(t, "mcharbor_encryption_key", "  file-key-value  "))
	t.Setenv("MCHARBOR_DB_DSN", "")
	t.Setenv("MCHARBOR_DB_DSN_FILE", writeSecret(t, "mcharbor_db_dsn", "postgres://mch:pw@db:5432/mch\n"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Secret != "file-secret-value" || cfg.EncryptionKey != "file-key-value" || cfg.DBDSN != "postgres://mch:pw@db:5432/mch" {
		t.Fatalf("Secret=%q EncryptionKey=%q DBDSN=%q, want the trimmed file contents", cfg.Secret, cfg.EncryptionKey, cfg.DBDSN)
	}
}

// TestSecretFileRefusals keeps a node from starting on an ambiguous or missing key: either
// would leave it running with a different key from its peers without saying so.
func TestSecretFileRefusals(t *testing.T) {
	cases := []struct {
		name, value, file string
		want              error
	}{
		{name: "both set", value: "env-value", file: writeSecret(t, "s", "file-value"), want: ErrSecretSourceConflict},
		{name: "unreadable", file: filepath.Join(t.TempDir(), "missing"), want: os.ErrNotExist},
		{name: "empty", file: writeSecret(t, "e", " \n"), want: ErrSecretFileEmpty},
	}
	for _, variable := range []string{"MCHARBOR_SECRET", "ENCRYPTION_KEY", "MCHARBOR_DB_DSN"} {
		for _, tc := range cases {
			t.Run(variable+"/"+tc.name, func(t *testing.T) {
				t.Setenv("MCHARBOR_SECRET", "")
				t.Setenv("ENCRYPTION_KEY", "")
				t.Setenv("MCHARBOR_SECRET_FILE", "")
				t.Setenv("ENCRYPTION_KEY_FILE", "")
				t.Setenv("MCHARBOR_DB_DSN", "")
				t.Setenv("MCHARBOR_DB_DSN_FILE", "")
				t.Setenv(variable, tc.value)
				t.Setenv(variable+"_FILE", tc.file)
				_, err := Load()
				if !errors.Is(err, tc.want) {
					t.Fatalf("Load error = %v, want %v", err, tc.want)
				}
				if !strings.Contains(err.Error(), variable+"_FILE") {
					t.Errorf("the error does not name %s_FILE: %v", variable, err)
				}
			})
		}
	}
}
