// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"io"
	"os"
	"reflect"
	"sort"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

type fakeExecProcess struct {
	mu      sync.Mutex
	signals []os.Signal
	events  *[]string
	exit    chan struct{}
	once    sync.Once
}

func newFakeExecProcess(events *[]string) *fakeExecProcess {
	return &fakeExecProcess{events: events, exit: make(chan struct{})}
}

func (f *fakeExecProcess) Signal(sig os.Signal) error {
	f.mu.Lock()
	f.signals = append(f.signals, sig)
	*f.events = append(*f.events, "signal")
	f.mu.Unlock()
	if sig == os.Kill {
		f.once.Do(func() { close(f.exit) })
	}
	return nil
}

func (f *fakeExecProcess) Wait() (*os.ProcessState, error) {
	<-f.exit
	return nil, nil
}

func (f *fakeExecProcess) exitOnItsOwn() { f.once.Do(func() { close(f.exit) }) }

func (f *fakeExecProcess) signalled() []os.Signal {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]os.Signal(nil), f.signals...)
}

type recordingCloser struct {
	events *[]string
	mu     *sync.Mutex
}

func (c recordingCloser) Close() error {
	c.mu.Lock()
	*c.events = append(*c.events, "close")
	c.mu.Unlock()
	return nil
}

func TestClosingAnExecSessionKillsTheTreeThenReapsTheProcess(t *testing.T) {
	var events []string
	proc := newFakeExecProcess(&events)
	session := newExecSession(proc, eofReader{}, nil, nil, recordingCloser{&events, &proc.mu})
	session.killTree = func() {
		proc.mu.Lock()
		events = append(events, "killTree")
		proc.mu.Unlock()
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Exited():
	default:
		t.Fatal("Close returned before the process was reaped")
	}
	if got := proc.signalled(); len(got) != 1 || got[0] != os.Kill {
		t.Fatalf("signals = %v, want one SIGKILL", got)
	}
	proc.mu.Lock()
	defer proc.mu.Unlock()
	if !reflect.DeepEqual(events, []string{"killTree", "close", "signal"}) {
		t.Fatalf("order = %v; the tree must die before the terminal hangs up", events)
	}
	errlog.Close(session)
}

func TestAnExecProcessThatExitsOnItsOwnIsReapedWithoutAClose(t *testing.T) {
	var events []string
	proc := newFakeExecProcess(&events)
	session := newExecSession(proc, eofReader{}, nil, nil)
	jobsKilled := false
	session.killTree = func() { jobsKilled = true }

	proc.exitOnItsOwn()
	select {
	case <-session.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("a process that exited was never reaped")
	}
	if code := session.ExitCode(); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	errlog.Close(session)
	if len(proc.signalled()) != 0 {
		t.Fatal("an already reaped process was signalled; its pid may have been reused")
	}
	if !jobsKilled {
		t.Fatal("jobs the exited shell left in its session were not killed")
	}
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestExecReadTreatsATerminalHangupAsTheEnd(t *testing.T) {
	var events []string
	proc := newFakeExecProcess(&events)
	defer proc.exitOnItsOwn()
	hangup := &os.PathError{Op: "read", Path: "/dev/ptmx", Err: syscall.EIO}
	session := newExecSession(proc, errReader{hangup}, nil, nil)
	if _, err := session.Read(make([]byte, 4)); !errors.Is(err, io.EOF) {
		t.Fatalf("read = %v, want io.EOF for EIO", err)
	}
	other := newExecSession(proc, errReader{os.ErrPermission}, nil, nil)
	if _, err := other.Read(make([]byte, 4)); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("read = %v, want the real error passed through", err)
	}
}

func TestParseProcStatSurvivesAHostileCommandName(t *testing.T) {
	ppid, sid, err := parseProcStat("4242 (evil) 1 2 3) S 17 4242 4240 34816 0")
	if err != nil || ppid != 17 || sid != 4240 {
		t.Fatalf("ppid=%d sid=%d err=%v, want 17 4240", ppid, sid, err)
	}
	if _, _, err := parseProcStat("garbage"); err == nil {
		t.Fatal("a line with no command name parsed")
	}
}

func TestExecTreeIsTheRootItsDescendantsAndItsSession(t *testing.T) {
	entries := []procEntry{
		{pid: 1, ppid: 0, sid: 1},
		{pid: 10, ppid: 1, sid: 10},
		{pid: 11, ppid: 10, sid: 10},
		{pid: 12, ppid: 11, sid: 10},
		{pid: 13, ppid: 1, sid: 10},
		{pid: 14, ppid: 11, sid: 14},
		{pid: 20, ppid: 1, sid: 1},
	}
	got := execTree(10, entries)
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{10, 11, 12, 13, 14}) {
		t.Fatalf("tree = %v, want the shell, its descendants and its reparented job", got)
	}
	for _, pid := range got {
		if pid == 1 || pid == 20 {
			t.Fatalf("tree includes %d, which the exec session does not own", pid)
		}
	}
}
