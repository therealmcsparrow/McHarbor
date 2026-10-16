// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"errors"
	"os"
	"sync/atomic"
	"time"
)

// SelfRestartExitCode is the status a native McHarbor exits with to be restarted. It is
// non-zero so systemd's Restart=on-failure starts a new process; 75 is EX_TEMPFAIL.
const SelfRestartExitCode = 75

// ErrRestartNotSupervised is returned on a native install that no service manager watches:
// exiting there would stop McHarbor for good instead of restarting it.
var ErrRestartNotSupervised = errors.New("mcharbor self-restart needs systemd to start the new process")

var selfExit atomic.Value // func()

// SetSelfExit registers how this process ends itself for a native restart. main wires it to a
// graceful shutdown followed by an exit with SelfRestartExitCode.
func SetSelfExit(fn func()) {
	selfExit.Store(fn)
}

// Seams for tests: a real host answers these from the filesystem and the environment.
var (
	// Podman marks its containers with /run/.containerenv, which inContainer does not read.
	restartInContainer = func() bool {
		if _, err := os.Stat("/run/.containerenv"); err == nil {
			return true
		}
		return inContainer()
	}
	// systemd sets INVOCATION_ID for every unit it starts.
	underSystemd      = func() bool { return os.Getenv("INVOCATION_ID") != "" }
	nativeExitDelay   = 2 * time.Second
	nativeServiceName = "mcharbor.service"
)

// RunningNatively reports whether McHarbor runs directly on its host rather than in a
// container, which decides whether Docker-secret and container-recreate flows apply at all.
func RunningNatively() bool {
	return !restartInContainer()
}

// restartNative schedules the process exit that systemd turns into a restart. Docker is never
// consulted: on a native host the only McHarbor-looking container it could find is someone
// else's, and restarting that was what left the operator waiting for a restart that never came.
func (s *Service) restartNative() (RestartResult, error) {
	fn, _ := selfExit.Load().(func())
	if fn == nil || !underSystemd() {
		s.restartPending.Store(false)
		return RestartResult{}, ErrRestartNotSupervised
	}
	scheduled := time.Now().UTC()
	// goroutine lifecycle: ends after one timer; the process exits from it.
	go func() {
		// The delay lets the handler flush its response before the listeners close.
		time.Sleep(nativeExitDelay)
		s.logger.Warn("system: self-restart requested; exiting so systemd starts a new process",
			"exit_code", SelfRestartExitCode)
		fn()
	}()
	return RestartResult{ContainerName: nativeServiceName, ScheduledAt: scheduled}, nil
}
