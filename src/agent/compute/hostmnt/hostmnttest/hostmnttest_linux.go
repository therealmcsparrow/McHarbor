// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

// Package hostmnttest reproduces what runc leaves behind after an unprivileged
// start, for the tests of every mount-table reader.
package hostmnttest

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// StrandLeader moves the calling thread into a private mount namespace with a tmpfs
// at the returned directory, as runc's goCreateMountSources does to a locked thread.
// Called from a TestMain whose goroutine init locked to the main thread, it strands
// the thread-group leader there, which is the case /proc/self then reports.
func StrandLeader() (marker string, release func(), err error) {
	marker, err = os.MkdirTemp("", "hostmnt-foreign-")
	if err != nil {
		return "", nil, fmt.Errorf("hostmnttest: marker directory: %w", err)
	}
	if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
		return "", nil, fmt.Errorf("hostmnttest: unshare: %w", err)
	}
	// Private first, or the marker would propagate back into the agent's namespace
	// and every reader would see it for the right reason.
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return "", nil, fmt.Errorf("hostmnttest: making / private: %w", err)
	}
	if err := unix.Mount("hostmnt-foreign", marker, "tmpfs", 0, "size=64k"); err != nil {
		return "", nil, fmt.Errorf("hostmnttest: mounting the marker: %w", err)
	}
	release = func() {
		if err := unix.Unmount(marker, unix.MNT_DETACH); err != nil {
			fmt.Fprintf(os.Stderr, "hostmnttest: unmounting %s: %v\n", marker, err)
		}
		if err := os.Remove(marker); err != nil {
			fmt.Fprintf(os.Stderr, "hostmnttest: removing %s: %v\n", marker, err)
		}
	}
	return marker, release, nil
}
