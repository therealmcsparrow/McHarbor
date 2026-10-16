// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// netdevExecCommand is the argv[1] that turns this binary into the shim rather
// than into an agent.
const netdevExecCommand = "netdev-exec"

// isNetdevExecInvocation reports whether this process was started as the
// macvtap shim rather than as the agent.
func isNetdevExecInvocation(args []string) bool {
	return len(args) > 1 && args[1] == netdevExecCommand
}

// runNetdevExec opens each macvtap character device, puts it on the file
// descriptor QEMU was told to expect, and execs QEMU. It never returns.
//
// It exists because QEMU is started detached — as a transient systemd unit —
// so the agent cannot hand it an inherited descriptor, and a macvtap has no
// name for QEMU to reopen the way a bridged tap does. The alternative was a
// shell in the launch path, which would put guest configuration back into a
// quoted string where one space in a volume path becomes two arguments.
func runNetdevExec(args []string) {
	devices, argv, err := parseNetdevExec(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netdev-exec: %v\n", err)
		os.Exit(2)
	}

	for fd, path := range devices {
		opened, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netdev-exec: open %s: %v\n", path, err)
			os.Exit(2)
		}
		// Dup3 refuses a no-op duplication, and the descriptor has to lose
		// CLOEXEC either way or QEMU would be handed a closed fd.
		if opened == fd {
			if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, 0); err != nil {
				fmt.Fprintf(os.Stderr, "netdev-exec: clear cloexec on %d: %v\n", fd, err)
				os.Exit(2)
			}
			continue
		}
		if err := unix.Dup3(opened, fd, 0); err != nil {
			fmt.Fprintf(os.Stderr, "netdev-exec: place %s on fd %d: %v\n", path, fd, err)
			os.Exit(2)
		}
		errlog.Log(unix.Close(opened))
	}

	if err := syscall.Exec(argv[0], argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "netdev-exec: exec %s: %v\n", argv[0], err)
		os.Exit(2)
	}
}

// parseNetdevExec splits the shim's own flags from the command it wraps.
//
// The form is deliberately rigid — repeated --fd <n>=<path> pairs, then a bare
// --, then the command — so that nothing in the wrapped argv can be mistaken
// for a flag of the shim's own.
func parseNetdevExec(args []string) (map[int]string, []string, error) {
	devices := make(map[int]string)

	index := 2 // argv[0] is the binary, argv[1] is the subcommand.
	for index < len(args) {
		switch args[index] {
		case "--":
			argv := args[index+1:]
			if len(argv) == 0 {
				return nil, nil, fmt.Errorf("no command to run after --")
			}
			if len(devices) == 0 {
				return nil, nil, fmt.Errorf("no --fd pairs given")
			}
			return devices, argv, nil

		case "--fd":
			if index+1 >= len(args) {
				return nil, nil, fmt.Errorf("--fd needs a <number>=<path> pair")
			}
			fd, path, err := parseFDPair(args[index+1])
			if err != nil {
				return nil, nil, err
			}
			devices[fd] = path
			index += 2

		default:
			return nil, nil, fmt.Errorf("unexpected argument %q", args[index])
		}
	}
	return nil, nil, fmt.Errorf("missing -- before the command to run")
}

// parseFDPair reads one <number>=<path> pair, refusing the descriptors the
// process already needs.
func parseFDPair(pair string) (int, string, error) {
	number, path, ok := strings.Cut(pair, "=")
	if !ok {
		return 0, "", fmt.Errorf("%q is not a <number>=<path> pair", pair)
	}
	fd, err := strconv.Atoi(number)
	if err != nil {
		return 0, "", fmt.Errorf("%q is not a file descriptor number", number)
	}
	// Standard streams belong to the unit's journal, and overwriting one would
	// lose every message QEMU writes about why it failed.
	if fd < 3 {
		return 0, "", fmt.Errorf("refusing to place a device on fd %d", fd)
	}
	if path == "" {
		return 0, "", fmt.Errorf("no device path for fd %d", fd)
	}
	return fd, path, nil
}
