// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

// initWaiter is the part of a libcontainer process the reaper needs; a fake
// stands in for it in tests.
type initWaiter interface {
	Wait() (*os.ProcessState, error)
}

// initExit is how a container's init ended, as its parent observed it.
type initExit struct {
	// Code is the exit status; meaningful only when Signal is zero and Known.
	Code int
	// Signal is the signal that killed init, zero when it exited on its own.
	Signal syscall.Signal
	// Known is false when the wait itself failed and nothing is known.
	Known bool
	// Expected is true when the agent asked for the exit: a stop, a shutdown or
	// an abandoned start. Only an unexpected exit can be a crash.
	Expected bool
	At       time.Time
}

// initReaper waits on one container init exactly once. The container's PID 1 is
// a direct child of the agent (runc's nsexec clones it with CLONE_PARENT) and
// libcontainer never waits on it after a successful start, so without this every
// exited container stays a zombie until the agent exits.
type initReaper struct {
	done chan struct{}

	mu       sync.Mutex
	expected bool
	exit     initExit
}

func newInitReaper() *initReaper {
	return &initReaper{done: make(chan struct{})}
}

// reap blocks until proc exits, records how, and then calls onExit. It is the
// body of the reaper goroutine, which therefore ends when init does.
func (r *initReaper) reap(proc initWaiter, onExit func(*initReaper)) {
	state, err := proc.Wait()
	exit := exitFromState(state, err)
	exit.At = time.Now()

	r.mu.Lock()
	exit.Expected = r.expected
	r.exit = exit
	r.mu.Unlock()
	close(r.done)

	if onExit != nil {
		onExit(r)
	}
}

// expect marks the coming exit as requested. Called before the signal is sent,
// so an exit the signal causes can never be recorded as a crash.
func (r *initReaper) expect() {
	r.mu.Lock()
	r.expected = true
	r.mu.Unlock()
}

// await waits up to timeout for the reap and reports whether it happened.
func (r *initReaper) await(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-r.done:
		return true
	case <-timer.C:
		return false
	}
}

// result is the recorded exit, with ok false while init is still running.
func (r *initReaper) result() (initExit, bool) {
	select {
	case <-r.done:
	default:
		return initExit{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exit, true
}

// exitFromState reads an exit out of what Wait returned. The signal is read
// through an interface because WaitStatus differs per platform.
func exitFromState(state *os.ProcessState, err error) initExit {
	if state == nil {
		return initExit{Known: err == nil}
	}
	exit := initExit{Code: state.ExitCode(), Known: true}
	if status, ok := state.Sys().(interface {
		Signaled() bool
		Signal() syscall.Signal
	}); ok && status.Signaled() {
		exit.Signal = status.Signal()
		exit.Code = -1
	}
	return exit
}

// classifyInitExit decides whether an exit was a crash, and what to say about it.
// A guest's own poweroff or reboot inside a pid namespace ends init with SIGINT
// or SIGHUP (pid_namespaces(7)), and a clean exit(0) is a halt, not a crash.
func classifyInitExit(exit initExit) (crashed bool, reason string) {
	if exit.Expected || !exit.Known {
		return false, ""
	}
	switch {
	case exit.Signal == syscall.SIGINT:
		return false, ""
	case exit.Signal == syscall.SIGHUP:
		return false, "the guest asked to reboot; a container does not restart itself"
	case exit.Signal != 0:
		return true, fmt.Sprintf("init was killed by signal %d (%s)", int(exit.Signal), exit.Signal)
	case exit.Code != 0:
		return true, fmt.Sprintf("init exited with status %d", exit.Code)
	}
	return false, ""
}
