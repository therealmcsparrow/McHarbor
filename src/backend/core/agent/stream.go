// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"errors"
	"io"
	"sync"
)

const (
	maxAgentStreamChunkBytes    = 256 * 1024
	maxAgentStreamBufferedBytes = 8 * 1024 * 1024
)

var errAgentStreamOverflow = errors.New("agent stream buffer capacity exceeded")

// StreamReader is an io.ReadCloser backed by a channel.
// Used for streaming Docker responses (logs, stats, exec).
// The agent sends chunks which are pushed to the channel;
// the Docker SDK reads them via Read().
type StreamReader struct {
	ch          chan []byte
	buf         []byte
	closed      bool
	buffered    int
	terminalErr error
	mu          sync.Mutex
	done        chan struct{}
}

// NewStreamReader creates a new StreamReader.
func NewStreamReader() *StreamReader {
	return &StreamReader{
		ch:   make(chan []byte, 256),
		done: make(chan struct{}),
	}
}

// Push sends a chunk of data to the stream reader.
// Returns false if the reader has been closed.
func (sr *StreamReader) Push(data []byte) bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.closed {
		return false
	}
	if len(data) > maxAgentStreamChunkBytes || sr.buffered+len(data) > maxAgentStreamBufferedBytes {
		sr.failLocked(errAgentStreamOverflow)
		return false
	}

	select {
	case sr.ch <- data:
		sr.buffered += len(data)
		return true
	default:
		sr.failLocked(errAgentStreamOverflow)
		return false
	}
}

func (sr *StreamReader) failLocked(err error) {
	sr.closed = true
	sr.terminalErr = err
	close(sr.done)
}

// Fail terminates the stream with an error after buffered data is drained.
func (sr *StreamReader) Fail(err error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if !sr.closed {
		sr.failLocked(err)
	}
}

// End signals that no more data will be sent.
func (sr *StreamReader) End() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if !sr.closed {
		sr.closed = true
		close(sr.done)
	}
}

// Done is closed when the stream ends, fails, or is closed by the consumer.
func (sr *StreamReader) Done() <-chan struct{} {
	return sr.done
}

// Read implements io.Reader.
func (sr *StreamReader) Read(p []byte) (int, error) {
	// Drain leftover buffer first
	if len(sr.buf) > 0 {
		n := copy(p, sr.buf)
		sr.buf = sr.buf[n:]
		return n, nil
	}

	var chunk []byte
	select {
	case chunk = <-sr.ch:
	case <-sr.done:
		select {
		case chunk = <-sr.ch:
		default:
			sr.mu.Lock()
			err := sr.terminalErr
			sr.mu.Unlock()
			if err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
	}
	sr.mu.Lock()
	sr.buffered -= len(chunk)
	sr.mu.Unlock()

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
	}
	return nil
}
