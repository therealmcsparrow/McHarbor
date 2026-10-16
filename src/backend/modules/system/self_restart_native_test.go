// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func withNativeHost(t *testing.T, systemd bool) {
	t.Helper()
	prevContainer, prevSystemd, prevDelay := restartInContainer, underSystemd, nativeExitDelay
	restartInContainer = func() bool { return false }
	underSystemd = func() bool { return systemd }
	nativeExitDelay = 0
	t.Cleanup(func() {
		restartInContainer, underSystemd, nativeExitDelay = prevContainer, prevSystemd, prevDelay
		selfExit.Store(func() {})
	})
}

// TestNativeRestartExitsForSystemd guards the native restart: it must end this process for
// systemd to restart, and never go looking for a Docker container. A nil pool proves the
// latter — the container path would have panicked or failed on it.
func TestNativeRestartExitsForSystemd(t *testing.T) {
	withNativeHost(t, true)
	exited := make(chan struct{}, 1)
	SetSelfExit(func() { exited <- struct{}{} })

	svc := NewService(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := svc.RestartSelf(context.Background())
	if err != nil {
		t.Fatalf("native restart refused: %v", err)
	}
	if res.ContainerName != nativeServiceName {
		t.Errorf("result names %q, want %q", res.ContainerName, nativeServiceName)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the process was never asked to exit")
	}
}

// TestNativeRestartWithoutSystemdIsRefused: exiting with nothing to start the process again
// would stop McHarbor for good, so the request is refused and can be retried.
func TestNativeRestartWithoutSystemdIsRefused(t *testing.T) {
	withNativeHost(t, false)
	SetSelfExit(func() { t.Error("exited without a supervisor") })

	svc := NewService(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.RestartSelf(context.Background()); !errors.Is(err, ErrRestartNotSupervised) {
		t.Fatalf("err = %v, want ErrRestartNotSupervised", err)
	}
	if svc.restartPending.Load() {
		t.Error("a refused restart left the pending flag set, blocking every later attempt")
	}
}
