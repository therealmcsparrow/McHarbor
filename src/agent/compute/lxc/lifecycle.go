// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// stateFileName is the file libcontainer writes into a container's state
// directory, and the only thing libcontainer.Load reads. runc v1.5.1 writes it
// on the first start, never on Create.
const stateFileName = "state.json"

var (
	// ErrAlreadyRunning means a container that is running or frozen was asked
	// to be created or started again, which would kill the live one.
	ErrAlreadyRunning = errors.New("lxc: container is already running")
	// ErrNotRunning means an operation that needs a live init was asked of a
	// container that has none, including one that was created and never started.
	ErrNotRunning = errors.New("lxc: container is not running")
)

// presence is what a container's libcontainer state directory holds.
type presence int

const (
	// presenceAbsent is no state directory at all: never created on this host,
	// or created before a reboot emptied /run.
	presenceAbsent presence = iota
	// presenceUnstarted is a state directory without state.json: Create ran and
	// no start has, so libcontainer.Load reports the container as not existing.
	presenceUnstarted
	// presenceLoaded is a state directory with state.json, which Load can read.
	presenceLoaded
)

// probePresence reads which of the three shapes a state directory is in.
// Load alone cannot tell absent from unstarted; both come back as ErrNotExist.
func probePresence(stateDir string) (presence, error) {
	if _, err := os.Stat(filepath.Join(stateDir, stateFileName)); err == nil {
		return presenceLoaded, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return presenceAbsent, fmt.Errorf("lxc: inspect %s: %w", stateDir, err)
	}
	info, err := os.Stat(stateDir)
	if err == nil {
		if !info.IsDir() {
			return presenceAbsent, fmt.Errorf("lxc: %s is not a directory", stateDir)
		}
		return presenceUnstarted, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return presenceAbsent, nil
	}
	return presenceAbsent, fmt.Errorf("lxc: inspect %s: %w", stateDir, err)
}

// leftoverAction is what has to happen to an id's state directory before a
// fresh libcontainer.Create can claim it.
type leftoverAction int

const (
	// leftoverNone means the id is free.
	leftoverNone leftoverAction = iota
	// leftoverRemoveDir means a never-started directory holds the id; it has no
	// cgroup and no process, so removing the directory is the whole cleanup.
	leftoverRemoveDir
	// leftoverDestroy means a loadable container holds the id and has to be
	// destroyed through libcontainer so its cgroup goes with it.
	leftoverDestroy
)

// planLeftover decides how to free an id for Create and Start. A running or
// frozen container is refused rather than destroyed, because destroying it
// would kill a live guest to satisfy a create or a double-clicked start.
func planLeftover(p presence, state string) (leftoverAction, error) {
	switch p {
	case presenceAbsent:
		return leftoverNone, nil
	case presenceUnstarted:
		return leftoverRemoveDir, nil
	}
	switch state {
	case StateStopped, StateDefined:
		// Defined is libcontainer's "created": an init parked on the exec fifo
		// by a start that never finished. Destroying it kills that init.
		return leftoverDestroy, nil
	case StateRunning, StateFrozen:
		return leftoverNone, ErrAlreadyRunning
	}
	return leftoverNone, fmt.Errorf("lxc: a container in state %q cannot be replaced safely", state)
}

// unloadedState is the state reported for a container libcontainer cannot load,
// with ok false when the container is loadable and has to be asked instead.
// A never-started container is stopped, not missing: it exists and can start.
func unloadedState(p presence) (state string, ok bool) {
	if p == presenceLoaded {
		return "", false
	}
	return StateStopped, true
}

// reportedInitPID is the init pid to report for a container in state. state.json
// keeps the last init's pid after it exits, and a stopped container reporting it
// names a pid that may by now belong to an unrelated host process.
func reportedInitPID(state string, pid int) int {
	if state == StateRunning || state == StateFrozen {
		return pid
	}
	return 0
}
