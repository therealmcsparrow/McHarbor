// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apppaths

import "testing"

// TestDataDirPrefersTheEnvironment covers the native install, where systemd
// sets DATA_DIR to /var/lib/mcharbor and the container default must not win.
func TestDataDirPrefersTheEnvironment(t *testing.T) {
	t.Setenv("DATA_DIR", "/var/lib/mcharbor")
	if got := DataDir(); got != "/var/lib/mcharbor" {
		t.Errorf("DataDir() = %q, want /var/lib/mcharbor", got)
	}
}

// TestDataDirFallsBackToTheContainerPath pins the historical default. Six
// call sites used to hardcode it; if this changes, every one of them moves.
func TestDataDirFallsBackToTheContainerPath(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	if got := DataDir(); got != DefaultDataDir {
		t.Errorf("DataDir() = %q, want %q", got, DefaultDataDir)
	}
}

// TestDataDirIgnoresWhitespaceOnly guards against an env file written with a
// trailing space, which would otherwise point the data directory at "".
func TestDataDirIgnoresWhitespaceOnly(t *testing.T) {
	t.Setenv("DATA_DIR", "   ")
	if got := DataDir(); got != DefaultDataDir {
		t.Errorf("DataDir() = %q, want %q", got, DefaultDataDir)
	}
}
