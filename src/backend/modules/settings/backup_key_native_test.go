// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testBackupKey(fill byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(fill), 32)))
}

// TestNativeBackupKeyInstallWritesTheFileAndRestarts guards the native install: the key lands
// in the data directory (never a Docker secret), an older different key is kept rather than
// stranding the backups it encrypted, and McHarbor is restarted to load it.
func TestNativeBackupKeyInstallWritesTheFileAndRestarts(t *testing.T) {
	dataDir := t.TempDir()
	restarts := 0
	SetNativeHost(dataDir, func(context.Context) error { restarts++; return nil })
	t.Cleanup(func() { nativeHost.set = false })

	svc := &Service{}
	ctx := context.Background()
	first, second := testBackupKey('a'), testBackupKey('b')
	if _, err := svc.InstallBackupKeyWithPath(ctx, first, ""); err != nil {
		t.Fatalf("native install failed: %v", err)
	}
	res, err := svc.InstallBackupKeyWithPath(ctx, second, "")
	if err != nil {
		t.Fatal(err)
	}

	path := NativeBackupKeyPath(dataDir)
	if res.SecretPath != path {
		t.Errorf("secret path = %q, want %q", res.SecretPath, path)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != second {
		t.Fatalf("installed key = %q, %v; want the second key", got, err)
	}
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("key file mode = %v, %v; want 0600", st.Mode().Perm(), err)
		}
	}
	kept, err := filepath.Glob(path + ".*.bak")
	if err != nil || len(kept) != 1 {
		t.Fatalf("previous key copies = %v, %v; want exactly one", kept, err)
	}
	if old, err := os.ReadFile(kept[0]); err != nil || string(old) != first {
		t.Errorf("kept key = %q, %v; want the first key", old, err)
	}
	if restarts != 2 {
		t.Errorf("restarts = %d, want one per install", restarts)
	}
	if !svc.BackupKeyStatus(path).Native {
		t.Error("status does not report the native host")
	}
}

// TestNativeBackupKeyInstallWithoutSupervisorSaysSo: the key is written, but nothing would start
// McHarbor again, and the operator must hear that rather than wait out a restart timeout.
func TestNativeBackupKeyInstallWithoutSupervisorSaysSo(t *testing.T) {
	SetNativeHost(t.TempDir(), func(context.Context) error { return ErrNativeRestartUnsupervised })
	t.Cleanup(func() { nativeHost.set = false })

	if _, err := (&Service{}).InstallBackupKeyWithPath(context.Background(), testBackupKey('c'), ""); !errors.Is(err, ErrNativeRestartUnsupervised) {
		t.Fatalf("err = %v, want ErrNativeRestartUnsupervised", err)
	}
}

func TestResolveBackupKeyFileKeepsAnOperatorChoice(t *testing.T) {
	dataDir := t.TempDir()
	if got := ResolveBackupKeyFile("/srv/keys/backup.key", dataDir); got != "/srv/keys/backup.key" {
		t.Errorf("operator path replaced: %q", got)
	}
	if got := ResolveBackupKeyFile(containerBackupKeyFile, dataDir); got != NativeBackupKeyPath(dataDir) {
		t.Errorf("container default kept on a native host: %q", got)
	}
	if got := ResolveBackupKeyFile("", dataDir); got != NativeBackupKeyPath(dataDir) {
		t.Errorf("empty path not resolved: %q", got)
	}
}
