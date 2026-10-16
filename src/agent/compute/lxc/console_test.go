// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

type fakeMaster struct {
	guest  *io.PipeWriter
	mu     sync.Mutex
	input  bytes.Buffer
	sizes  [][2]uint16
	closes int
}

func newTestHub(t *testing.T) (*consoleHub, *fakeMaster) {
	t.Helper()
	reader, writer := io.Pipe()
	fake := &fakeMaster{guest: writer}
	hub := newConsoleHub(reader,
		func(p []byte) (int, error) {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			return fake.input.Write(p)
		},
		func(cols, rows uint16) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.sizes = append(fake.sizes, [2]uint16{cols, rows})
			return nil
		},
		func() error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.closes++
			return reader.Close()
		})
	go hub.run()
	t.Cleanup(func() { errlog.Close(writer) })
	return hub, fake
}

func (f *fakeMaster) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

// guestWrite writes as the guest; the pipe returns only once the hub has read it.
func guestWrite(t *testing.T, f *fakeMaster, text string) {
	t.Helper()
	if _, err := f.guest.Write([]byte(text)); err != nil {
		t.Fatalf("guest write: %v", err)
	}
}

// readUntil reads a viewer until the collected output contains want.
func readUntil(t *testing.T, r io.Reader, want string) string {
	t.Helper()
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var got strings.Builder
		buf := make([]byte, 1024)
		for !strings.Contains(got.String(), want) {
			n, err := r.Read(buf)
			got.Write(buf[:n])
			if err != nil {
				done <- result{got.String(), err}
				return
			}
		}
		done <- result{got.String(), nil}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("read ended with %v before %q; got %q", res.err, want, res.text)
		}
		return res.text
	case <-time.After(5 * time.Second):
		t.Fatalf("never read %q", want)
		return ""
	}
}

func TestRingBufferKeepsOnlyTheNewestBytes(t *testing.T) {
	ring := newRingBuffer(8)
	ring.Write([]byte("abcdef"))
	ring.Write([]byte("ghij"))
	if got := string(ring.Bytes()); got != "cdefghij" {
		t.Fatalf("ring = %q, want the newest 8 bytes", got)
	}
	ring.Write([]byte("0123456789XYZ"))
	if got := string(ring.Bytes()); got != "56789XYZ" {
		t.Fatalf("ring after an oversized write = %q", got)
	}
}

func TestConsoleReplayIsBoundedToTheNewestOutput(t *testing.T) {
	hub, fake := newTestHub(t)
	head := strings.Repeat("h", 1000)
	tail := strings.Repeat("t", consoleRingBytes)
	guestWrite(t, fake, head)
	for i := 0; i < len(tail); i += consoleReadChunk {
		guestWrite(t, fake, tail[i:i+consoleReadChunk])
	}
	// An empty write is taken only after the previous chunk was published.
	guestWrite(t, fake, "")

	viewer, err := hub.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer errlog.Close(viewer)
	first := <-viewer.out
	if len(first) != consoleRingBytes {
		t.Fatalf("replay is %d bytes, want the %d-byte bound", len(first), consoleRingBytes)
	}
	if string(first) != tail {
		t.Fatal("replay is not the newest output; the oldest was kept instead")
	}
}

func TestReconnectReplaysRecentOutputAndKeepsTheConsoleOpen(t *testing.T) {
	hub, fake := newTestHub(t)
	first, err := hub.Attach()
	if err != nil {
		t.Fatal(err)
	}
	guestWrite(t, fake, "boot line\n")
	readUntil(t, first, "boot line")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	if fake.closeCount() != 0 {
		t.Fatal("closing a viewer closed the pty master, which hangs up PID 1")
	}
	// The hub still drains the guest with nobody attached.
	guestWrite(t, fake, "while away\n")

	second, err := hub.Attach()
	if err != nil {
		t.Fatalf("reattach: %v", err)
	}
	defer errlog.Close(second)
	got := readUntil(t, second, "while away")
	if !strings.Contains(got, "boot line") {
		t.Fatalf("reconnect replay = %q, want the output from before the disconnect", got)
	}
}

func TestViewerInputAndResizeReachTheConsole(t *testing.T) {
	hub, fake := newTestHub(t)
	viewer, err := hub.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer errlog.Close(viewer)

	if _, err := viewer.Write([]byte("ls\n")); err != nil {
		t.Fatal(err)
	}
	if err := viewer.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	if err := viewer.Resize(0, 40); err == nil {
		t.Fatal("a zero-width resize was accepted")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.input.String() != "ls\n" {
		t.Fatalf("console input = %q", fake.input.String())
	}
	if len(fake.sizes) != 1 || fake.sizes[0] != [2]uint16{120, 40} {
		t.Fatalf("resizes = %v, want exactly 120x40", fake.sizes)
	}
}

func TestASlowViewerIsDetachedRatherThanBuffered(t *testing.T) {
	hub, fake := newTestHub(t)
	slow, err := hub.Attach()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < consoleViewerQueue+10; i++ {
		guestWrite(t, fake, "x")
	}
	if hub.viewerCount() != 0 {
		t.Fatal("a viewer that never read is still attached with an unbounded backlog")
	}

	buf := make([]byte, 64)
	var err2 error
	for err2 == nil {
		_, err2 = slow.Read(buf)
	}
	if !errors.Is(err2, ErrViewerBehind) {
		t.Fatalf("slow viewer ended with %v, want ErrViewerBehind", err2)
	}
	if fake.closeCount() != 0 {
		t.Fatal("detaching a slow viewer closed the console")
	}
	fresh, err := hub.Attach()
	if err != nil {
		t.Fatalf("the console did not survive a slow viewer: %v", err)
	}
	errlog.Close(fresh)
}

func TestConsoleEndClosesViewersAndRefusesNewOnes(t *testing.T) {
	hub, fake := newTestHub(t)
	viewer, err := hub.Attach()
	if err != nil {
		t.Fatal(err)
	}
	guestWrite(t, fake, "last words\n")
	errlog.Close(fake.guest)

	readUntil(t, viewer, "last words")
	if _, err := viewer.Read(make([]byte, 8)); !errors.Is(err, io.EOF) {
		t.Fatalf("read after the console ended = %v, want io.EOF", err)
	}
	<-hub.done
	if fake.closeCount() != 1 {
		t.Fatalf("master closed %d times, want once", fake.closeCount())
	}
	if _, err := hub.Attach(); !errors.Is(err, ErrConsoleUnavailable) {
		t.Fatalf("attach after the end = %v, want ErrConsoleUnavailable", err)
	}
}

func TestANewTerminalStartsAtTheRequestedSizeOr80x24(t *testing.T) {
	cases := []struct {
		cols, rows int
		wantC      uint16
		wantR      uint16
	}{
		{120, 40, 120, 40},
		{0, 0, 80, 24},
		{0, 40, 80, 24},
		{-1, 30, 80, 24},
		{70000, 30, 80, 24},
	}
	for _, c := range cases {
		cols, rows := initialWinsize(c.cols, c.rows)
		if cols != c.wantC || rows != c.wantR {
			t.Errorf("%dx%d started at %dx%d, want %dx%d", c.cols, c.rows, cols, rows, c.wantC, c.wantR)
		}
	}
}
