// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrapfs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestEnsureSecretGeneratesOnce covers the first boot: a secret is written and
// every later boot reads the same one back, because regenerating it would
// invalidate every session on restart.
func TestEnsureSecretGeneratesOnce(t *testing.T) {
	dir := t.TempDir()

	first, err := EnsureSecret(dir)
	if err != nil {
		t.Fatalf("first EnsureSecret: %v", err)
	}
	if first == "" {
		t.Fatal("generated secret is empty")
	}

	second, err := EnsureSecret(dir)
	if err != nil {
		t.Fatalf("second EnsureSecret: %v", err)
	}
	if second != first {
		t.Errorf("secret changed across boots: %q then %q", first, second)
	}
}

// TestEnsureSecretIsNotWorldReadable guards the file mode: the session secret
// signs cookies, and a native install shares its data directory with a real
// multi-user filesystem rather than a container's private one.
func TestEnsureSecretIsNotWorldReadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root makes the mode check meaningless")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not honor Unix file modes")
	}
	dir := t.TempDir()

	if _, err := EnsureSecret(dir); err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, ".secret"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
}

// TestEnsureDirsCreatesTLSDir covers the boot ordering: certgen writes into
// $DATA_DIR/tls and would fail if nothing had created it first.
func TestEnsureDirsCreatesTLSDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	if err := EnsureDirs(dir); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "tls"))
	if err != nil {
		t.Fatalf("stat tls dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("tls path is not a directory")
	}
}
