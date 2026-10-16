// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

type fakeInit struct {
	exit  chan struct{}
	waits int
}

func (f *fakeInit) Wait() (*os.ProcessState, error) {
	f.waits++
	<-f.exit
	return nil, nil
}

func TestReaperWaitsOnceAndRecordsTheExitBeforeCallingBack(t *testing.T) {
	proc := &fakeInit{exit: make(chan struct{})}
	reaper := newInitReaper()
	called := make(chan bool, 1)
	go reaper.reap(proc, func(r *initReaper) {
		_, ok := r.result()
		called <- ok
	})

	if _, ok := reaper.result(); ok {
		t.Fatal("a running init reported a result")
	}
	if reaper.await(20 * time.Millisecond) {
		t.Fatal("await returned true before init exited")
	}

	close(proc.exit)
	if !reaper.await(time.Second) {
		t.Fatal("init was never reaped")
	}
	select {
	case ok := <-called:
		if !ok {
			t.Fatal("onExit ran before the exit was recorded")
		}
	case <-time.After(time.Second):
		t.Fatal("onExit was never called")
	}
	if proc.waits != 1 {
		t.Fatalf("Wait called %d times, want 1", proc.waits)
	}
	exit, ok := reaper.result()
	if !ok || !exit.Known || exit.Expected {
		t.Fatalf("result = %+v, %v", exit, ok)
	}
}

func TestReaperMarksAnExitExpectedWhenAskedBeforeIt(t *testing.T) {
	proc := &fakeInit{exit: make(chan struct{})}
	reaper := newInitReaper()
	go reaper.reap(proc, nil)
	reaper.expect()
	close(proc.exit)
	if !reaper.await(time.Second) {
		t.Fatal("init was never reaped")
	}
	if exit, _ := reaper.result(); !exit.Expected {
		t.Fatal("an exit asked for by a stop was not recorded as expected")
	}
}

func TestExitFromStateWithoutAProcessState(t *testing.T) {
	if exit := exitFromState(nil, nil); !exit.Known {
		t.Fatal("a clean wait with no state should be a known exit")
	}
	if exit := exitFromState(nil, errors.New("wait: no child processes")); exit.Known {
		t.Fatal("a failed wait must not be a known exit")
	}
}

func TestClassifyInitExit(t *testing.T) {
	cases := []struct {
		name    string
		exit    initExit
		crashed bool
		reason  string
	}{
		{"clean halt", initExit{Known: true}, false, ""},
		{"requested kill", initExit{Known: true, Signal: syscall.SIGKILL, Code: -1, Expected: true}, false, ""},
		{"requested non-zero", initExit{Known: true, Code: 143, Expected: true}, false, ""},
		{"unknown", initExit{Code: 1}, false, ""},
		{"guest poweroff", initExit{Known: true, Signal: syscall.SIGINT, Code: -1}, false, ""},
		{"guest reboot", initExit{Known: true, Signal: syscall.SIGHUP, Code: -1}, false,
			"the guest asked to reboot; a container does not restart itself"},
		{"killed", initExit{Known: true, Signal: syscall.SIGKILL, Code: -1}, true,
			"init was killed by signal 9 (" + syscall.SIGKILL.String() + ")"},
		{"failed", initExit{Known: true, Code: 3}, true, "init exited with status 3"},
	}
	for _, tc := range cases {
		crashed, reason := classifyInitExit(tc.exit)
		if crashed != tc.crashed || reason != tc.reason {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", tc.name, crashed, reason, tc.crashed, tc.reason)
		}
	}
}
