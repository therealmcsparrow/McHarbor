// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
)

type fakeGuest struct {
	output chan []byte
	closed chan struct{}
	once   sync.Once

	mu      sync.Mutex
	input   []byte
	resizes [][2]int
}

func newFakeGuest() *fakeGuest {
	return &fakeGuest{output: make(chan []byte, 8), closed: make(chan struct{})}
}

func (g *fakeGuest) Read(p []byte) (int, error) {
	select {
	case frame, ok := <-g.output:
		if !ok {
			return 0, io.EOF
		}
		return copy(p, frame), nil
	case <-g.closed:
		return 0, io.EOF
	}
}

func (g *fakeGuest) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.input = append(g.input, p...)
	return len(p), nil
}

func (g *fakeGuest) Resize(cols, rows int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resizes = append(g.resizes, [2]int{cols, rows})
	return nil
}

func (g *fakeGuest) Close() error {
	g.once.Do(func() { close(g.closed) })
	return nil
}

func (g *fakeGuest) isClosed() bool {
	select {
	case <-g.closed:
		return true
	default:
		return false
	}
}

type fakeOpener struct {
	attached []string
	execs    []lxc.ExecOptions
	guest    *fakeGuest
}

func (f *fakeOpener) AttachConsole(ctID string) (guestConsole, error) {
	f.attached = append(f.attached, ctID)
	return f.guest, nil
}

func (f *fakeOpener) OpenExec(_ context.Context, _ string, opts lxc.ExecOptions) (guestConsole, error) {
	f.execs = append(f.execs, opts)
	return f.guest, nil
}

func TestContainerConsoleKindsMapOntoTheManager(t *testing.T) {
	opener := &fakeOpener{guest: newFakeGuest()}
	ctx := context.Background()

	if _, err := openContainerConsole(ctx, opener, ConsolePayload{Kind: "tty", TargetID: "ct1", Cols: 90, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(opener.attached, []string{"ct1"}) || len(opener.execs) != 0 {
		t.Fatalf("tty opened %v / %v, want PID 1's console and no new process", opener.attached, opener.execs)
	}
	if !reflect.DeepEqual(opener.guest.resizes, [][2]int{{90, 30}}) {
		t.Fatalf("initial size = %v, want 90x30", opener.guest.resizes)
	}

	if _, err := openContainerConsole(ctx, opener, ConsolePayload{Kind: "exec", TargetID: "ct1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := openContainerConsole(ctx, opener, ConsolePayload{Kind: "exec", TargetID: "ct1", Command: []string{"uptime"}}); err != nil {
		t.Fatal(err)
	}
	want := []lxc.ExecOptions{{TTY: true}, {Argv: []string{"uptime"}, TTY: false}}
	if !reflect.DeepEqual(opener.execs, want) {
		t.Fatalf("exec options = %+v, want a pty shell then a plain command", opener.execs)
	}

	if _, err := openContainerConsole(ctx, opener, ConsolePayload{Kind: "serial", TargetID: "ct1"}); err == nil {
		t.Fatal("a VM console kind was accepted for a container")
	}
}

type sentMessages struct {
	mu   sync.Mutex
	msgs []WSMessage
}

func (s *sentMessages) send(msg WSMessage) error {
	s.mu.Lock()
	s.msgs = append(s.msgs, msg)
	s.mu.Unlock()
	return nil
}

func (s *sentMessages) types() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.msgs))
	for _, msg := range s.msgs {
		out = append(out, msg.Type)
	}
	return out
}

func testConsoleRuntime(sent *sentMessages) *computeRuntime {
	return &computeRuntime{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		console: newConsoleRegistry(),
		send:    sent.send,
	}
}

func waitRegistryEmpty(t *testing.T, rt *computeRuntime, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := rt.console.get(id); !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the session never left the registry")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestGuestOutputThenEndReachesTheServer(t *testing.T) {
	sent := &sentMessages{}
	rt := testConsoleRuntime(sent)
	guest := newFakeGuest()
	rt.startConsoleSession(context.Background(), "s1", "exec", guest)

	guest.output <- []byte("hello")
	close(guest.output)
	waitRegistryEmpty(t, rt, "s1")

	if got := sent.types(); !reflect.DeepEqual(got, []string{MsgConsoleOutput, MsgConsoleEnd}) {
		t.Fatalf("sent %v, want output then end", got)
	}
	if string(sent.msgs[0].Console.Data) != "hello" {
		t.Fatalf("output = %q", sent.msgs[0].Console.Data)
	}
	if !guest.isClosed() {
		t.Fatal("an ended exec session was not closed, so its process is never killed")
	}
}

func TestServerEndClosesTheGuestAndInputAndResizeReachIt(t *testing.T) {
	sent := &sentMessages{}
	rt := testConsoleRuntime(sent)
	guest := newFakeGuest()
	rt.startConsoleSession(context.Background(), "s2", "tty", guest)

	rt.consoleFrame(MsgConsoleInput, "s2", ConsolePayload{Data: []byte("ls\n")})
	rt.consoleFrame(MsgConsoleResize, "s2", ConsolePayload{Cols: 132, Rows: 43})
	guest.mu.Lock()
	input, resizes := string(guest.input), append([][2]int(nil), guest.resizes...)
	guest.mu.Unlock()
	if input != "ls\n" || !reflect.DeepEqual(resizes, [][2]int{{132, 43}}) {
		t.Fatalf("input %q resizes %v, want ls and 132x43", input, resizes)
	}

	rt.consoleFrame(MsgConsoleEnd, "s2", ConsolePayload{})
	if !guest.isClosed() {
		t.Fatal("console_end from the server left the guest side open")
	}
	waitRegistryEmpty(t, rt, "s2")
	for _, kind := range sent.types() {
		if kind == MsgConsoleEnd {
			t.Fatal("a session the server ended was echoed back as ended")
		}
	}
}

func TestTransportLossClosesTheGuest(t *testing.T) {
	sent := &sentMessages{}
	rt := testConsoleRuntime(sent)
	guest := newFakeGuest()
	ctx, cancel := context.WithCancel(context.Background())
	rt.startConsoleSession(ctx, "s3", "exec", guest)
	cancel()
	waitRegistryEmpty(t, rt, "s3")
	if !guest.isClosed() {
		t.Fatal("losing the transport left an exec process running")
	}
}
