// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"io"
	"sync"
)

// StreamReader is an io.ReadCloser backed by a channel.
// Used for streaming Docker responses (logs, stats, exec).
// The agent sends chunks which are pushed to the channel;
// the Docker SDK reads them via Read().
type StreamReader struct {
	ch     chan []byte
	buf    []byte
	closed bool
	mu     sync.Mutex
	done   chan struct{}
}

// NewStreamReader creates a new StreamReader.
func NewStreamReader() *StreamReader {
	return &StreamReader{
		ch:   make(chan []byte, 64),
		done: make(chan struct{}),
	}
}

// Push sends a chunk of data to the stream reader.
// Returns false if the reader has been closed.
func (sr *StreamReader) Push(data []byte) bool {
	sr.mu.Lock()
	if sr.closed {
		sr.mu.Unlock()
		return false
	}
	sr.mu.Unlock()

	select {
	case sr.ch <- data:
		return true
	case <-sr.done:
		return false
	}
}

// End signals that no more data will be sent.
func (sr *StreamReader) End() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if !sr.closed {
		sr.closed = true
		close(sr.ch)
	}
}

// Read implements io.Reader.
func (sr *StreamReader) Read(p []byte) (int, error) {
	// Drain leftover buffer first
	if len(sr.buf) > 0 {
		n := copy(p, sr.buf)
		sr.buf = sr.buf[n:]
		return n, nil
	}

	chunk, ok := <-sr.ch
	if !ok {
		return 0, io.EOF
	}

	n := copy(p, chunk)
	if n < len(chunk) {
		sr.buf = chunk[n:]
	}
	return n, nil
}

// Close implements io.Closer.
func (sr *StreamReader) Close() error {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if !sr.closed {
		sr.closed = true
		close(sr.done)
		// Drain remaining data
		go func() {
			for range sr.ch {
			}
		}()
	}
	return nil
}
