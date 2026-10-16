// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// TestBundleReportsSecretConfiguredFromConfig guards I-6: main unsets MCHARBOR_SECRET once the
// configuration loads and a _FILE secret never enters the environment, so reading the
// environment alone told support engineers a configured secret was missing.
func TestBundleReportsSecretConfiguredFromConfig(t *testing.T) {
	env := map[string]string{"PORT": "8705"}
	prev := processEnv
	processEnv = func() ([]string, func(string) (string, bool)) {
		return []string{"PORT=8705"}, func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	}
	t.Cleanup(func() { processEnv = prev })

	svc := NewService(context.Background(), Deps{
		DB: migratedDB(t), DataDir: t.TempDir(), EncryptionKeySet: true, SecretSet: true, DBDSNSet: true,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(svc.Close)
	raw, err := svc.collect(context.Background(), CatSystem)
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := raw.(map[string]any)["environment"].([]EnvEntry)
	if !ok {
		t.Fatalf("system category has no environment list: %#v", raw)
	}
	got := map[string]EnvEntry{}
	for _, e := range entries {
		got[e.Name] = e
	}
	for _, name := range []string{"MCHARBOR_SECRET", "ENCRYPTION_KEY", "MCHARBOR_DB_DSN"} {
		if e := got[name]; !e.Set || e.Value != Redacted {
			t.Errorf("%s = %+v, want reported as set and redacted", name, e)
		}
	}
	if e := got["DOCKER_CERT_PATH"]; e.Set {
		t.Errorf("an unconfigured variable reads as set: %+v", e)
	}
}
