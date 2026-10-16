// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCheckToolchainGatesPluginOpen covers the guard in front of plugin.Open, because
// a stale .so reaching Open crash-looped the host after the Go 1.27.1 bump. The test
// binary itself stands in for a plugin: it is a Go file built by runtime.Version().
func TestCheckToolchainGatesPluginOpen(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locating test binary: %v", err)
	}

	if err := checkToolchain(self, runtime.Version()); err != nil {
		t.Fatalf("same toolchain refused: %v", err)
	}

	err = checkToolchain(self, "go1.0")
	if !errors.Is(err, ErrToolchainMismatch) {
		t.Fatalf("different toolchain: got %v, want ErrToolchainMismatch", err)
	}
	if !strings.Contains(err.Error(), runtime.Version()) || !strings.Contains(err.Error(), "go1.0") {
		t.Errorf("mismatch message should name both toolchains: %v", err)
	}

	junk := filepath.Join(t.TempDir(), "junk.so")
	if err := os.WriteFile(junk, []byte("not an ELF file"), 0o600); err != nil {
		t.Fatalf("writing junk .so: %v", err)
	}
	if err := checkToolchain(junk, runtime.Version()); !errors.Is(err, ErrNotGoPlugin) {
		t.Fatalf("non-Go file: got %v, want ErrNotGoPlugin", err)
	}
}
