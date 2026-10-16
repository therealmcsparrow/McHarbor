// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package hostmnt

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

// ErrForeignNamespace reports that the thread a read ran on is not in the agent's
// mount namespace, so its table would describe some other namespace's mounts.
var ErrForeignNamespace = errors.New("hostmnt: the reading thread is not in the agent's mount namespace")

// agentNS is the mount namespace the agent started in. Package initialisation runs
// on the main thread before main, so no runc code can have moved it yet.
var agentNS, agentNSErr = threadNamespace()

// Read returns /proc/thread-self/<name> ("mountinfo" or "mounts") for the agent's
// own mount namespace, or ErrForeignNamespace rather than another namespace's table.
func Read(name string) ([]byte, error) {
	if name != "mountinfo" && name != "mounts" {
		return nil, fmt.Errorf("hostmnt: %q is not a mount table", name)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return readLocked(name)
}

// readLocked must run on a locked thread: the namespace check and the read have to
// happen on the same thread for the check to say anything about the read.
func readLocked(name string) ([]byte, error) {
	if agentNSErr != nil {
		return nil, agentNSErr
	}
	if err := checkThread(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile("/proc/thread-self/" + name)
	if err != nil {
		return nil, fmt.Errorf("hostmnt: reading the mount table: %w", err)
	}
	if err := checkThread(); err != nil {
		return nil, err
	}
	return data, nil
}

func checkThread() error {
	ns, err := threadNamespace()
	if err != nil {
		return err
	}
	if ns != agentNS {
		return fmt.Errorf("%w: thread in %s, agent in %s", ErrForeignNamespace, ns, agentNS)
	}
	return nil
}

func threadNamespace() (string, error) {
	link, err := os.Readlink("/proc/thread-self/ns/mnt")
	if err != nil {
		return "", fmt.Errorf("hostmnt: reading this thread's mount namespace: %w", err)
	}
	return link, nil
}
