// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/opencontainers/runc/libcontainer"

	// This blank import is load-bearing and must not be removed.
	//
	// nsenter carries a C constructor (nsexec.c) that runs before the Go
	// runtime starts. It has to: the setns/clone sequence for user namespaces
	// cannot be performed safely from Go once the runtime has spawned threads,
	// because those threads are already in the wrong namespaces. Importing the
	// package is what gets the constructor linked in.
	//
	// Dropping it compiles perfectly and then fails at runtime with an error
	// that does not name the cause — every container start fails with an
	// obscure message about the init process. It is the second most likely
	// mistake in this package, after getting the re-exec branch in the wrong
	// place.
	_ "github.com/opencontainers/runc/libcontainer/nsenter"

	// Also load-bearing. Its init registers the cgroup v1/v2 device writers and
	// systemd's device properties; without it every start fails with "cgroup
	// manager is not configured to set device rules", because BuildConfig always
	// sets device rules. runc's own main.go carries the same import.
	_ "github.com/opencontainers/cgroups/devices"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// InitArg is the argv[1] that means "this process is a container's init helper,
// not the agent". It is libcontainer's literal, not ours to choose: runc v1.5.1
// re-executes the binary as exec.Command(exePath, "init") (container_linux.go).
const InitArg = "init"

// initPipeEnv is set by libcontainer on every init re-exec. Requiring it keeps
// an operator's `mcharbor-agent init` from being taken for a container init.
const initPipeEnv = "_LIBCONTAINER_INITPIPE"

// RunInit is the container init entry point.
//
// # Why this exists and where it must be called from
//
// libcontainer works by re-executing the host binary as the container's init
// helper — the same pattern as `runc init`. When Start creates a container, the
// kernel ends up running *this* binary again inside the new namespaces, and
// that second process must hand control to libcontainer immediately.
//
// So main() must branch to this before it does anything else: before flag
// parsing, before logging setup, before reading configuration, before opening a
// database or dialling a WebSocket. Not "early in main" — first.
//
// The reason is not tidiness. The re-executed process is already inside a
// half-constructed container: its namespaces are set up but its root has not
// been pivoted, and the C constructor has already run. Any file or socket the
// normal agent startup opens in that state is opened in the wrong namespace,
// against the wrong root, and the failure that results is remote from its
// cause. Getting this wrong produces containers that fail to start with opaque
// errors, and it is the single most likely implementation mistake here.
//
// libcontainer.Init never returns. It either becomes the container's init or
// exits the process.
func RunInit() {
	libcontainer.Init()
	// Unreachable. Init calls os.Exit on every path; this exists so a future
	// change to that contract fails loudly rather than silently continuing
	// into agent startup inside a container.
	panicUnreachable()
}

// panicUnreachable makes the impossible case visible.
//
// This is the one place a panic is right: reaching it means libcontainer's
// contract changed underneath us and a container's init is about to run the
// agent's normal startup path, which would be far worse than a crash.
func panicUnreachable() {
	fmt.Fprintln(os.Stderr, "mcharbor-agent: libcontainer.Init returned; refusing to continue "+
		"as a container init process")
	os.Exit(1)
}

// IsInitInvocation reports whether this process was re-executed as a container
// init.
//
// main() calls this as its first statement.
func IsInitInvocation(args []string) bool {
	return len(args) > 1 && args[1] == InitArg && os.Getenv(initPipeEnv) != ""
}

// consoleReceiver holds the socket pair libcontainer uses to hand back the
// container's controlling terminal.
//
// libcontainer writes the pty master's file descriptor over a unix socket as
// out-of-band data. The agent keeps one end; the container's init gets the
// other and sends the descriptor through it once it has allocated the terminal.
type consoleReceiver struct {
	agentEnd     *os.File
	containerEnd *os.File
	path         string
}

// newConsoleReceiver creates the socket pair.
//
// A socketpair is used rather than a listening unix socket on disk because
// there is no window in which anything else could connect to it — the
// descriptor is passed directly to the child, and a container's controlling
// terminal is not something to leave reachable through the filesystem even
// briefly.
func newConsoleReceiver(path string) (*consoleReceiver, error) {
	parent, child, err := socketPair()
	if err != nil {
		return nil, fmt.Errorf("lxc: create console socket pair: %w", err)
	}
	return &consoleReceiver{agentEnd: parent, containerEnd: child, path: path}, nil
}

// AgentEnd is the descriptor handed to libcontainer as Process.ConsoleSocket.
func (c *consoleReceiver) AgentEnd() *os.File { return c.containerEnd }

// Receive waits for the container to send its pty master descriptor.
//
// The caller must call this before the container produces output, or the first
// bytes of boot are lost — the same ordering hazard the console gateway has,
// and worth stating twice because losing early boot output means losing exactly
// the messages that explain a failed boot.
func (c *consoleReceiver) Receive() (*os.File, error) {
	conn, err := net.FileConn(c.agentEnd)
	if err != nil {
		return nil, fmt.Errorf("lxc: wrap console socket: %w", err)
	}
	defer func() { errlog.Close(conn) }()

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("lxc: console socket is not a unix socket")
	}
	// libcontainer sends the master before Start returns, so a wait longer than
	// this means it never will, and an unbounded read would wedge the start.
	if err := unixConn.SetReadDeadline(time.Now().Add(consoleReceiveTimeout)); err != nil {
		return nil, fmt.Errorf("lxc: bound the console receive: %w", err)
	}
	return recvFD(unixConn)
}

// consoleReceiveTimeout bounds how long Receive waits for the pty master.
const consoleReceiveTimeout = 10 * time.Second

// Close releases both ends.
func (c *consoleReceiver) Close() {
	if c.agentEnd != nil {
		errlog.Close(c.agentEnd)
	}
	if c.containerEnd != nil {
		errlog.Close(c.containerEnd)
	}
}
