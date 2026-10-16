// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// consoleRingBytes bounds the recent PID-1 output a hub keeps, so a viewer who
// reconnects sees the last screenful or so while memory stays fixed per container.
const consoleRingBytes = 64 * 1024

// consoleReadChunk is how much of the pty master one hub read takes.
const consoleReadChunk = 4096

// consoleViewerQueue bounds the frames queued for one viewer. A viewer that
// cannot keep up is detached rather than allowed to grow the agent's memory.
const consoleViewerQueue = 256

// ErrConsoleUnavailable means this agent holds no PID-1 console for the
// container: it is not running, or it was started before the agent last restarted.
var ErrConsoleUnavailable = errors.New("lxc: this agent holds no console for the container; " +
	"it is not running or was started before the agent last restarted (restart it, or open a shell)")

// ErrViewerBehind ends a console viewer whose queue filled up; dropping frames
// instead would corrupt the terminal it is drawing.
var ErrViewerBehind = errors.New("lxc: the console viewer fell behind and was detached; reconnect to continue")

// ringBuffer keeps the most recent bytes written to it, dropping the oldest.
type ringBuffer struct {
	data  []byte
	start int
	size  int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{data: make([]byte, capacity)}
}

// Write appends p, overwriting the oldest bytes once the buffer is full.
func (r *ringBuffer) Write(p []byte) {
	capacity := len(r.data)
	if capacity == 0 {
		return
	}
	if len(p) >= capacity {
		copy(r.data, p[len(p)-capacity:])
		r.start, r.size = 0, capacity
		return
	}
	for _, b := range p {
		end := (r.start + r.size) % capacity
		r.data[end] = b
		if r.size < capacity {
			r.size++
		} else {
			r.start = (r.start + 1) % capacity
		}
	}
}

// Bytes returns a copy of the buffered bytes, oldest first.
func (r *ringBuffer) Bytes() []byte {
	out := make([]byte, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.data[(r.start+i)%len(r.data)]
	}
	return out
}

// consoleHub owns one container's PID-1 pty master: it drains it for the life
// of the container so the guest never blocks, records recent output, and fans
// it out to any number of viewers. Viewers come and go; the master does not.
type consoleHub struct {
	master      io.Reader
	write       func([]byte) (int, error)
	resize      func(cols, rows uint16) error
	closeMaster func() error
	onEnd       func()

	mu      sync.Mutex
	ring    *ringBuffer
	viewers map[*ConsoleAttachment]struct{}
	ended   bool
	done    chan struct{}

	// writeMu serialises keystrokes from several viewers, so two operators'
	// input can interleave only at frame boundaries.
	writeMu sync.Mutex
}

// newConsoleHub wraps a pty master. write must bound itself (a deadline), so a
// guest that stops reading its terminal cannot wedge the agent's read loop.
func newConsoleHub(master io.Reader, write func([]byte) (int, error),
	resize func(cols, rows uint16) error, closeMaster func() error) *consoleHub {
	return &consoleHub{
		master:      master,
		write:       write,
		resize:      resize,
		closeMaster: closeMaster,
		ring:        newRingBuffer(consoleRingBytes),
		viewers:     make(map[*ConsoleAttachment]struct{}),
		done:        make(chan struct{}),
	}
}

// run drains the master until it fails, which is the pty telling us every
// process holding the container's console has exited. Its lifetime is exactly
// the container's, so it needs no other stop path.
func (h *consoleHub) run() {
	buf := make([]byte, consoleReadChunk)
	for {
		n, err := h.master.Read(buf)
		if n > 0 {
			h.publish(buf[:n])
		}
		if err != nil {
			h.end()
			return
		}
	}
}

// publish records output and queues a copy for every viewer.
func (h *consoleHub) publish(p []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ring.Write(p)
	for viewer := range h.viewers {
		frame := make([]byte, len(p))
		copy(frame, p)
		select {
		case viewer.out <- frame:
		default:
			h.detachLocked(viewer, ErrViewerBehind)
		}
	}
}

// end closes the master and every viewer once the container's console is gone.
func (h *consoleHub) end() {
	h.mu.Lock()
	if h.ended {
		h.mu.Unlock()
		return
	}
	h.ended = true
	for viewer := range h.viewers {
		h.detachLocked(viewer, nil)
	}
	h.mu.Unlock()
	if h.closeMaster != nil {
		errlog.Log(h.closeMaster())
	}
	close(h.done)
	if h.onEnd != nil {
		h.onEnd()
	}
}

// Attach adds a viewer whose first frame is the recent output, taken under the
// same lock that orders live frames, so the replay and the stream neither gap
// nor overlap.
func (h *consoleHub) Attach() (*ConsoleAttachment, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ended {
		return nil, ErrConsoleUnavailable
	}
	viewer := &ConsoleAttachment{
		hub:  h,
		out:  make(chan []byte, consoleViewerQueue),
		stop: make(chan struct{}),
	}
	if replay := h.ring.Bytes(); len(replay) > 0 {
		viewer.out <- replay
	}
	h.viewers[viewer] = struct{}{}
	return viewer, nil
}

// detachLocked removes a viewer. Only the hub sends on or closes out, always
// under mu, so a close can never race a send.
func (h *consoleHub) detachLocked(viewer *ConsoleAttachment, cause error) {
	if _, ok := h.viewers[viewer]; !ok {
		return
	}
	delete(h.viewers, viewer)
	viewer.setCause(cause)
	close(viewer.out)
}

// viewerCount reports how many viewers are attached.
func (h *consoleHub) viewerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.viewers)
}

// ConsoleAttachment is one viewer of a container's PID-1 console. Closing it
// detaches the viewer and leaves the console, and so PID 1, untouched.
type ConsoleAttachment struct {
	hub     *consoleHub
	out     chan []byte
	pending []byte

	stop     chan struct{}
	stopOnce sync.Once

	causeMu sync.Mutex
	cause   error
}

func (a *ConsoleAttachment) setCause(err error) {
	a.causeMu.Lock()
	a.cause = err
	a.causeMu.Unlock()
}

func (a *ConsoleAttachment) endError() error {
	a.causeMu.Lock()
	defer a.causeMu.Unlock()
	if a.cause != nil {
		return a.cause
	}
	return io.EOF
}

// Read returns console output: the replay first, then live frames. It returns
// io.EOF once the container's console has gone or the viewer was closed.
func (a *ConsoleAttachment) Read(p []byte) (int, error) {
	if len(a.pending) == 0 {
		select {
		case frame, ok := <-a.out:
			if !ok {
				return 0, a.endError()
			}
			a.pending = frame
		case <-a.stop:
			return 0, io.EOF
		}
	}
	n := copy(p, a.pending)
	a.pending = a.pending[n:]
	return n, nil
}

// Write sends keystrokes to the container's console.
func (a *ConsoleAttachment) Write(p []byte) (int, error) {
	select {
	case <-a.stop:
		return 0, fmt.Errorf("lxc: console viewer is closed")
	default:
	}
	a.hub.writeMu.Lock()
	defer a.hub.writeMu.Unlock()
	return a.hub.write(p)
}

// Resize sets the console's terminal size. With several viewers the last
// resize wins, which is how a shared physical terminal behaves too.
func (a *ConsoleAttachment) Resize(cols, rows int) error {
	c, r, err := winsize(cols, rows)
	if err != nil {
		return err
	}
	if a.hub.resize == nil {
		return nil
	}
	return a.hub.resize(c, r)
}

// Close detaches this viewer. It never closes the pty master: that would hang
// up the container's console and signal its PID 1.
func (a *ConsoleAttachment) Close() error {
	a.stopOnce.Do(func() {
		close(a.stop)
		a.hub.mu.Lock()
		a.hub.detachLocked(a, nil)
		a.hub.mu.Unlock()
	})
	return nil
}

// winsize validates a terminal size for the TIOCSWINSZ ioctl.
func winsize(cols, rows int) (uint16, uint16, error) {
	if cols <= 0 || rows <= 0 || cols > 0xffff || rows > 0xffff {
		return 0, 0, fmt.Errorf("lxc: terminal size %dx%d is out of range", cols, rows)
	}
	return uint16(cols), uint16(rows), nil
}

// The size a terminal starts at when the caller named none. A pty created with
// no size reports 0x0, and `stty size`, pagers and full-screen programs then
// fail until the first resize frame arrives.
const (
	defaultTermCols = 80
	defaultTermRows = 24
)

// initialWinsize is the size a new pty is created with: the requested one when
// it is valid, 80x24 otherwise.
func initialWinsize(cols, rows int) (uint16, uint16) {
	if c, r, err := winsize(cols, rows); err == nil {
		return c, r
	}
	return defaultTermCols, defaultTermRows
}
