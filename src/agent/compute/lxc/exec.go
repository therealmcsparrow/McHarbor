// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// execReapTimeout bounds how long Close waits for a killed exec process to be
// reaped, so a wedged process cannot hold the console session open.
const execReapTimeout = 5 * time.Second

// execProcess is the part of a libcontainer process an exec session drives.
type execProcess interface {
	Signal(os.Signal) error
	Wait() (*os.ProcessState, error)
}

// ExecSession is one process started inside a running container for an
// operator. It lives exactly as long as the session: Close kills and reaps it.
type ExecSession struct {
	proc    execProcess
	output  io.Reader
	write   func([]byte) (int, error)
	resize  func(cols, rows uint16) error
	closers []io.Closer
	// killTree kills the process and everything it started, set by the host side;
	// nil falls back to signalling the process alone.
	killTree func()

	exited   chan struct{}
	exitMu   sync.Mutex
	exitCode int

	closeOnce sync.Once
}

// newExecSession starts reaping proc at once, so a process that exits on its own
// is never left a zombie waiting for the session to be closed.
func newExecSession(proc execProcess, output io.Reader, write func([]byte) (int, error),
	resize func(cols, rows uint16) error, closers ...io.Closer) *ExecSession {
	session := &ExecSession{
		proc:    proc,
		output:  output,
		write:   write,
		resize:  resize,
		closers: closers,
		exited:  make(chan struct{}),
	}
	// Bounded by the process: Wait returns when it exits, and Close kills it.
	go session.reap()
	return session
}

func (s *ExecSession) reap() {
	state, err := s.proc.Wait()
	code := -1
	if state != nil {
		code = state.ExitCode()
	} else if err == nil {
		code = 0
	}
	s.exitMu.Lock()
	s.exitCode = code
	s.exitMu.Unlock()
	close(s.exited)
}

// Read returns the process's output. A pty reports EIO once the process and
// everything holding its terminal have gone; that is the normal end, not a fault.
func (s *ExecSession) Read(p []byte) (int, error) {
	n, err := s.output.Read(p)
	if err != nil && isTerminalHangup(err) {
		err = io.EOF
	}
	return n, err
}

// Write sends input to the process.
func (s *ExecSession) Write(p []byte) (int, error) {
	return s.write(p)
}

// Resize sets the terminal size of a tty session; a pipe session has none.
func (s *ExecSession) Resize(cols, rows int) error {
	c, r, err := winsize(cols, rows)
	if err != nil {
		return err
	}
	if s.resize == nil {
		return nil
	}
	return s.resize(c, r)
}

// Exited is closed once the process has been reaped.
func (s *ExecSession) Exited() <-chan struct{} { return s.exited }

// ExitCode is the process's exit status, -1 until it has been reaped or when it
// was killed by a signal.
func (s *ExecSession) ExitCode() int {
	s.exitMu.Lock()
	defer s.exitMu.Unlock()
	return s.exitCode
}

// Close ends the session: the process and everything it started are killed, its
// terminal or pipes are closed, and it is reaped. Safe to call more than once.
func (s *ExecSession) Close() error {
	s.closeOnce.Do(func() {
		// The tree is killed before the terminal or pipes close: a hangup would
		// let the shell exit first and its jobs would be reparented out of reach.
		// It runs even after the process exited, for the jobs it left behind.
		if s.killTree != nil {
			s.killTree()
		}
		for _, closer := range s.closers {
			errlog.Close(closer)
		}
		select {
		case <-s.exited:
		default:
			// A process reaped between the check and the signal reports it is
			// done; every outcome falls through to the bounded wait below.
			errlog.Log(s.proc.Signal(os.Kill))
		}
		select {
		case <-s.exited:
		case <-time.After(execReapTimeout):
		}
	})
	return nil
}

// isTerminalHangup reports the EIO a pty master returns once its slave side has
// no holders left.
func isTerminalHangup(err error) bool {
	return errors.Is(err, syscall.EIO)
}

// procEntry is the ancestry of one process in a container's cgroup.
type procEntry struct {
	pid  int
	ppid int
	sid  int
}

// parseProcStat reads the parent and session ids from a /proc/<pid>/stat line.
// The command name is parenthesised and may itself hold spaces or parentheses,
// so the fields are counted from the last ')'.
func parseProcStat(body string) (ppid, sid int, err error) {
	end := strings.LastIndexByte(body, ')')
	if end < 0 {
		return 0, 0, fmt.Errorf("lxc: malformed stat line")
	}
	fields := strings.Fields(body[end+1:])
	// state ppid pgrp session ...
	if len(fields) < 4 {
		return 0, 0, fmt.Errorf("lxc: short stat line")
	}
	if ppid, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, fmt.Errorf("lxc: stat ppid: %w", err)
	}
	if sid, err = strconv.Atoi(fields[3]); err != nil {
		return 0, 0, fmt.Errorf("lxc: stat session: %w", err)
	}
	return ppid, sid, nil
}

// execTree is every process an exec session owns: its root, the root's
// descendants, and anything still in the root's session, which is where a
// shell's background jobs stay after being reparented to the container's init.
func execTree(root int, entries []procEntry) []int {
	children := make(map[int][]int, len(entries))
	for _, entry := range entries {
		children[entry.ppid] = append(children[entry.ppid], entry.pid)
	}
	owned := map[int]bool{}
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if owned[pid] {
			continue
		}
		owned[pid] = true
		queue = append(queue, children[pid]...)
	}
	for _, entry := range entries {
		if entry.sid == root {
			owned[entry.pid] = true
		}
	}
	out := make([]int, 0, len(owned))
	for _, entry := range entries {
		if owned[entry.pid] {
			out = append(out, entry.pid)
		}
	}
	return out
}
