// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// newHostServiceExecutor returns the executor that drives the real host, or an
// error naming why this agent cannot: a container agent sees its own systemd
// namespace at best, and acting on it would be acting on the wrong machine.
func newHostServiceExecutor() (*hostServiceExecutor, error) {
	if !hostServicesCapable() {
		return nil, errors.New("host services need a native agent on a host running systemd")
	}
	return &hostServiceExecutor{run: runHostCommand, base: systemdEtcDir, chown: chownRoot}, nil
}

// hostServicesCapable reports whether this agent can manage systemd units: it
// runs natively, systemd is PID 1, and systemctl is on PATH.
func hostServicesCapable() bool {
	if detectInstallKind() != InstallKindNative {
		return false
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

// runHostCommand runs name with args under timeout and returns its combined
// output, capped at hostServiceOutputCap.
func runHostCommand(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, bool, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out := &cappedBuffer{limit: hostServiceOutputCap}
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	// SYSTEMD_COLORS and the pager would otherwise decorate what is parsed.
	cmd.Env = append(os.Environ(), "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=", "LC_ALL=C")
	err := cmd.Run()
	if runCtx.Err() != nil && err != nil {
		err = errors.Join(err, runCtx.Err())
	}
	return out.buf.Bytes(), out.truncated, err
}

// chownRoot hands a drop-in to root, as `systemctl edit` leaves it. An agent
// not running as root could not have written into /etc/systemd/system anyway.
func chownRoot(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return os.Chown(path, 0, 0)
}

const (
	// hostServiceOutputCap bounds what any one systemctl call may return, so a
	// host with thousands of units cannot push a message past the WS limit.
	hostServiceOutputCap = 4 << 20
	// systemdEtcDir is where administrator drop-ins live.
	systemdEtcDir = "/etc/systemd/system"
)

// cappedBuffer keeps the first limit bytes written and counts the rest, so a
// runaway command is bounded without failing the write and killing it early.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.limit - c.buf.Len()
	if room <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) > room {
		c.buf.Write(p[:room])
		c.truncated = true
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}
