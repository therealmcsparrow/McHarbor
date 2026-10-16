// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"fmt"
	"net"
	"os"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// File-descriptor passing over a unix socket.
//
// This is how the container's controlling terminal reaches the agent: the pty
// master lives inside the container's namespaces, so it cannot be opened by
// path from outside — only the descriptor itself can cross.

// socketPair creates a connected pair of unix sockets as *os.File handles.
//
// SOCK_CLOEXEC matters: without it the descriptors leak into every process the
// agent later forks, including guests, which would hand a container a handle to
// another container's console socket.
func socketPair() (parent, child *os.File, err error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("lxc: socketpair: %w", err)
	}
	parent = os.NewFile(uintptr(fds[0]), "console-parent")
	child = os.NewFile(uintptr(fds[1]), "console-child")
	if parent == nil || child == nil {
		return nil, nil, fmt.Errorf("lxc: socketpair returned an unusable descriptor")
	}
	return parent, child, nil
}

// recvFD reads a single file descriptor sent as out-of-band data.
//
// The payload carries a name alongside the descriptor, which is ignored: the
// descriptor is the whole point and the name is libcontainer's own debugging
// aid.
func recvFD(conn *net.UnixConn) (*os.File, error) {
	// One descriptor's worth of control-message space. Sizing this exactly is
	// what stops a sender from passing more descriptors than expected and
	// leaving extras open in this process.
	oob := make([]byte, unix.CmsgSpace(4))
	buf := make([]byte, 4096)

	n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, fmt.Errorf("lxc: receive console descriptor: %w", err)
	}
	_ = n

	messages, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, fmt.Errorf("lxc: parse console control message: %w", err)
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("lxc: the container sent no console descriptor")
	}

	fds, err := unix.ParseUnixRights(&messages[0])
	if err != nil {
		return nil, fmt.Errorf("lxc: parse console descriptor: %w", err)
	}
	if len(fds) == 0 {
		return nil, fmt.Errorf("lxc: the container sent an empty console descriptor set")
	}
	// Anything beyond the first is closed rather than leaked. A well-behaved
	// libcontainer sends exactly one; a misbehaving one must not be able to
	// accumulate descriptors in a long-lived root process.
	for _, extra := range fds[1:] {
		errlog.Log(unix.Close(extra))
	}
	// Non-blocking before NewFile, so the file joins Go's poller: only then does
	// Close unblock a pending Read, and write deadlines work on the master.
	if err := unix.SetNonblock(fds[0], true); err != nil {
		errlog.Log(unix.Close(fds[0]))
		return nil, fmt.Errorf("lxc: make the console descriptor non-blocking: %w", err)
	}
	return os.NewFile(uintptr(fds[0]), "container-console"), nil
}

// unixSignal converts a signal number into the os.Signal the container API
// takes.
func unixSignal(sig int) os.Signal {
	return unix.Signal(sig)
}
