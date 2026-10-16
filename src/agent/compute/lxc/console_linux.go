// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// consoleWriteTimeout bounds one write to a pty. A guest that stops reading its
// terminal fills the pty's input queue, and an unbounded write would then park
// the agent's WebSocket read loop, which carries every other session too.
const consoleWriteTimeout = 10 * time.Second

// ptyWriter writes to a pty master with a deadline; the master must be pollable
// (recvFD makes it non-blocking) or the deadline is not honoured.
func ptyWriter(master *os.File) func([]byte) (int, error) {
	return func(p []byte) (int, error) {
		if err := master.SetWriteDeadline(time.Now().Add(consoleWriteTimeout)); err != nil {
			return 0, fmt.Errorf("lxc: bound the console write: %w", err)
		}
		return master.Write(p)
	}
}

// ptyResizer sets a pty's window size through SyscallConn, not Fd: Fd would put
// the file back into blocking mode and Close would stop unblocking readers.
func ptyResizer(master *os.File) func(cols, rows uint16) error {
	return func(cols, rows uint16) error {
		raw, err := master.SyscallConn()
		if err != nil {
			return fmt.Errorf("lxc: reach the console descriptor: %w", err)
		}
		var ioctlErr error
		if err := raw.Control(func(fd uintptr) {
			ioctlErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows})
		}); err != nil {
			return fmt.Errorf("lxc: reach the console descriptor: %w", err)
		}
		if ioctlErr != nil {
			return fmt.Errorf("lxc: resize the console: %w", ioctlErr)
		}
		return nil
	}
}

// serveConsole takes over a started container's PID-1 pty master and keeps it
// for the container's lifetime, replacing any hub a previous start left behind.
func (m *Manager) serveConsole(ctID string, master *os.File) {
	hub := newConsoleHub(master, ptyWriter(master), ptyResizer(master), master.Close)
	hub.onEnd = func() {
		m.mu.Lock()
		if m.consoles[ctID] == hub {
			delete(m.consoles, ctID)
		}
		m.mu.Unlock()
	}

	m.mu.Lock()
	previous := m.consoles[ctID]
	m.consoles[ctID] = hub
	m.mu.Unlock()
	if previous != nil {
		// The old container is gone or this start could not have happened; its
		// master only still reads because something inside held the slave open.
		previous.end()
	}
	// Bounded by the container: the read fails once nothing holds its console.
	go hub.run()
}

// AttachConsole adds a viewer to a running container's PID-1 console. Output
// written before the attach is replayed, up to consoleRingBytes.
func (m *Manager) AttachConsole(ctID string) (*ConsoleAttachment, error) {
	m.mu.Lock()
	hub := m.consoles[ctID]
	m.mu.Unlock()
	if hub == nil {
		return nil, ErrConsoleUnavailable
	}
	return hub.Attach()
}

// ExecOptions shapes one exec session. An empty Argv opens the container's
// shell; TTY gives the process a terminal, which an interactive shell needs.
type ExecOptions struct {
	Argv []string
	TTY  bool
	Cols int
	Rows int
}

// execEnv is the environment of an operator's exec process: a login-ish root
// environment, since the container's own init environment is not inherited.
func execEnv(tty bool) []string {
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"USER=root",
	}
	if tty {
		env = append(env, "TERM=xterm-256color")
	}
	return env
}

// OpenExec starts a process inside a running container, the `runc exec`
// equivalent. The session owns the process: closing it kills and reaps it.
func (m *Manager) OpenExec(_ context.Context, ctID string, opts ExecOptions) (*ExecSession, error) {
	container, err := m.load(ctID)
	if err != nil {
		return nil, err
	}
	if containerState(container) != StateRunning {
		return nil, fmt.Errorf("%w: %s", ErrNotRunning, ctID)
	}

	argv := opts.Argv
	if len(argv) == 0 {
		if argv, err = DetectShell(container.Config().Rootfs); err != nil {
			return nil, err
		}
	}
	process := &libcontainer.Process{
		Args: argv,
		Env:  execEnv(opts.TTY),
		Cwd:  "/",
		Init: false,
	}
	if opts.TTY {
		return m.openTTYExec(container, process, opts)
	}
	return openPipeExec(ctID, container, process)
}

// openTTYExec runs process with its own pty, received the same way PID 1's is.
func (m *Manager) openTTYExec(container *libcontainer.Container, process *libcontainer.Process, opts ExecOptions) (*ExecSession, error) {
	receiver, err := newConsoleReceiver("")
	if err != nil {
		return nil, err
	}
	defer receiver.Close()
	process.ConsoleSocket = receiver.AgentEnd()
	process.ConsoleWidth, process.ConsoleHeight = initialWinsize(opts.Cols, opts.Rows)

	if err := container.Run(process); err != nil {
		return nil, fmt.Errorf("lxc: exec in %s: %w", container.ID(), err)
	}
	master, err := receiver.Receive()
	if err != nil {
		errlog.Log(process.Signal(os.Kill))
		recvErr := fmt.Errorf("lxc: receive the exec terminal of %s: %w", container.ID(), err)
		// Wait reports the SIGKILL just sent as an exit error; only a failure
		// to reap at all is worth adding to the error.
		if _, waitErr := process.Wait(); waitErr != nil {
			var exitErr *exec.ExitError
			if !errors.As(waitErr, &exitErr) {
				return nil, errors.Join(recvErr, fmt.Errorf("lxc: reap the killed exec of %s: %w", container.ID(), waitErr))
			}
		}
		return nil, recvErr
	}
	session := newExecSession(process, master, ptyWriter(master), ptyResizer(master), master)
	session.killTree = treeKiller(container, process)
	return session, nil
}

// openPipeExec runs process with plain pipes: stdout and stderr share one
// stream, as they would on a terminal, and there is nothing to resize.
func openPipeExec(ctID string, container *libcontainer.Container, process *libcontainer.Process) (*ExecSession, error) {
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("lxc: exec stdin pipe for %s: %w", ctID, err)
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		errlog.Close(stdinRead)
		errlog.Close(stdinWrite)
		return nil, fmt.Errorf("lxc: exec output pipe for %s: %w", ctID, err)
	}
	process.Stdin, process.Stdout, process.Stderr = stdinRead, outWrite, outWrite

	runErr := container.Run(process)
	// The child holds its own copies now; keeping ours would stop the output
	// pipe from ever reporting EOF.
	errlog.Close(stdinRead)
	errlog.Close(outWrite)
	if runErr != nil {
		errlog.Close(stdinWrite)
		errlog.Close(outRead)
		return nil, fmt.Errorf("lxc: exec in %s: %w", ctID, runErr)
	}
	write := func(p []byte) (int, error) {
		if err := stdinWrite.SetWriteDeadline(time.Now().Add(consoleWriteTimeout)); err != nil {
			return 0, fmt.Errorf("lxc: bound the exec write: %w", err)
		}
		return stdinWrite.Write(p)
	}
	session := newExecSession(process, outRead, write, nil, stdinWrite, outRead)
	session.killTree = treeKiller(container, process)
	return session, nil
}

// treeKiller returns the kill for an exec session's process tree. The tree is
// taken from the container's own cgroup, so a pid can never name a host process.
func treeKiller(container *libcontainer.Container, process *libcontainer.Process) func() {
	return func() {
		root, err := process.Pid()
		if err != nil {
			return
		}
		pids, err := container.Processes()
		if err != nil {
			return
		}
		entries := make([]procEntry, 0, len(pids))
		for _, pid := range pids {
			body, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
			if err != nil {
				continue
			}
			ppid, sid, err := parseProcStat(string(body))
			if err != nil {
				continue
			}
			entries = append(entries, procEntry{pid: pid, ppid: ppid, sid: sid})
		}
		for _, pid := range execTree(root, entries) {
			errlog.Log(unix.Kill(pid, unix.SIGKILL))
		}
	}
}
