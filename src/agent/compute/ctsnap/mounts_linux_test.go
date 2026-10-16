// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package ctsnap

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/compute/hostmnt/hostmnttest"
)

func init() {
	runtime.LockOSThread()
}

var strandedMarker string

func TestMain(m *testing.M) {
	if os.Geteuid() != 0 {
		os.Exit(m.Run())
	}
	var release func()
	var err error
	strandedMarker, release, err = hostmnttest.StrandLeader()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	release()
	os.Exit(code)
}

func TestMountsIgnoresANamespaceRuncLeftTheLeaderIn(t *testing.T) {
	if strandedMarker == "" {
		t.Skip("needs root to strand the thread-group leader in another mount namespace")
	}
	mounts, err := Mounts()
	if err != nil {
		t.Fatalf("Mounts: %v", err)
	}
	root := false
	for _, entry := range mounts {
		if entry.MountPoint == strandedMarker {
			t.Fatalf("Mounts returned the stranded leader's mount at %s", strandedMarker)
		}
		root = root || entry.MountPoint == "/"
	}
	if !root {
		t.Fatal("Mounts has no root mount")
	}
}
