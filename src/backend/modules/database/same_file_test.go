// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameFileRecognisesTheRunningDatabaseUnderAnotherSpelling(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "mcharbor.db")
	if err := os.WriteFile(live, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	spelled := dir + string(os.PathSeparator) + "sub" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "mcharbor.db"
	if !sameFile(spelled, live) {
		t.Fatal("a relative spelling of the live file was treated as a different target")
	}
	if sameFile(filepath.Join(dir, "moved.db"), live) {
		t.Fatal("a new file path was treated as the live database")
	}
	if !sameFile(" "+live+" ", live) {
		t.Fatal("surrounding whitespace hid the live file")
	}
}
