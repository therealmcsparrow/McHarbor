// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"

	coreDocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

var errRawLogStreamLimit = fmt.Errorf("raw log stream exceeds %d bytes", maxRawStreamBytes)

// Service owns the container log streams and enforces the per-environment and per-user concurrency limits.
type Service struct {
	pool        *coreDocker.ClientPool
	mu          sync.Mutex
	streamCount int
	envStreams  map[string]int
	userStreams map[string]int
}

// NewService returns a Service that resolves Docker clients through pool.
func NewService(pool *coreDocker.ClientPool) *Service {
	return &Service{pool: pool, envStreams: make(map[string]int), userStreams: make(map[string]int)}
}

func (s *Service) resolveEnvironmentID(ctx context.Context, envID string) string {
	resolved, err := s.pool.ResolveEnvironmentIDContext(ctx, envID)
	if err != nil {
		return envID
	}
	return resolved
}

func (s *Service) acquireStream(ctx context.Context, envID, userID string) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	envLimit := maxLocalEnvStreams
	if s.pool.IsAgentEnvContext(ctx, envID) {
		envLimit = maxAgentEnvStreams
	}
	if s.streamCount >= maxStreamSessions || s.envStreams[envID] >= envLimit || s.userStreams[userID] >= maxUserStreams {
		return nil, false
	}

	s.streamCount++
	s.envStreams[envID]++
	s.userStreams[userID]++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.streamCount--
			s.envStreams[envID]--
			s.userStreams[userID]--
			if s.envStreams[envID] == 0 {
				delete(s.envStreams, envID)
			}
			if s.userStreams[userID] == 0 {
				delete(s.userStreams, userID)
			}
		})
	}, true
}

type logStream struct {
	cli         *client.Client
	containerID string
	query       streamQuery
	tty         bool
	agent       bool

	mu           sync.Mutex
	reader       io.ReadCloser
	readerCancel context.CancelFunc
}

func (s *Service) prepareStream(ctx context.Context, envID, containerID string, query streamQuery) (*logStream, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}

	inspectCtx, cancelInspect := context.WithTimeout(ctx, coreDocker.CallTimeout)
	inspectRes, err := cli.ContainerInspect(inspectCtx, containerID, client.ContainerInspectOptions{})
	inspect := inspectRes.Container
	cancelInspect()
	if err != nil {
		return nil, fmt.Errorf("inspecting container for logs: %w", err)
	}

	stream := &logStream{
		cli:         cli,
		containerID: containerID,
		query:       query,
		tty:         inspect.Config != nil && inspect.Config.Tty,
		agent:       s.pool.IsAgentEnvContext(ctx, envID),
	}
	if err := stream.openReader(ctx, query.Cursor, true); err != nil {
		return nil, err
	}
	return stream, nil
}

func (s *logStream) openReader(ctx context.Context, cursor streamCursor, initial bool) error {
	requestCtx := ctx
	cancel := func() {}
	if s.agent {
		requestCtx, cancel = context.WithTimeout(ctx, coreDocker.CallTimeout)
	}

	tail := fmt.Sprintf("%d", s.query.Tail)
	since := s.query.Since
	if cursor.Timestamp != "" {
		tail = "all"
		parsed, err := time.Parse(time.RFC3339Nano, cursor.Timestamp)
		if err != nil {
			cancel()
			return fmt.Errorf("parsing log cursor timestamp: %w", err)
		}
		since = strconv.FormatInt(parsed.Unix(), 10) + "." + fmt.Sprintf("%09d", parsed.Nanosecond())
	}
	follow := s.query.Follow && !s.agent
	if !initial && s.agent {
		follow = false
	}

	reader, err := s.cli.ContainerLogs(requestCtx, s.containerID, client.ContainerLogsOptions{
		ShowStdout: s.query.ShowStdout,
		ShowStderr: s.query.ShowStderr,
		Tail:       tail,
		Since:      since,
		Follow:     follow,
		Timestamps: true,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("opening container logs: %w", err)
	}

	s.mu.Lock()
	s.reader = reader
	s.readerCancel = cancel
	s.mu.Unlock()
	return nil
}

func (s *logStream) closeReader() {
	s.mu.Lock()
	reader := s.reader
	cancel := s.readerCancel
	s.reader = nil
	s.readerCancel = nil
	s.mu.Unlock()

	if reader != nil {
		if err := reader.Close(); err != nil {
			slog.Debug("closing log stream reader failed", "error", err)
		}
	}
	if cancel != nil {
		cancel()
	}
}

func (s *logStream) Close() {
	s.closeReader()
}

func (s *logStream) run(ctx context.Context, emit func(logRecord) error) (string, error) {
	resume := s.query.Cursor
	for {
		s.mu.Lock()
		reader := s.reader
		s.mu.Unlock()
		if reader == nil {
			return "failed", fmt.Errorf("log stream reader is unavailable")
		}

		lastCursor, err := readLogReader(reader, s.tty, resume, emit)
		s.closeReader()
		if errors.Is(err, errRawLogStreamLimit) {
			if lastCursor == resume {
				return "byte_limit_no_progress", nil
			}
			return "byte_limit", nil
		}
		if err != nil && ctx.Err() == nil {
			return "failed", fmt.Errorf("reading container logs: %w", err)
		}
		if ctx.Err() != nil {
			return "cancelled", ctx.Err()
		}
		if !s.agent || !s.query.Follow {
			return "eof", nil
		}
		if lastCursor.Timestamp != "" {
			resume = lastCursor
		}

		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "cancelled", ctx.Err()
		case <-timer.C:
		}
		if err := s.openReader(ctx, resume, false); err != nil {
			return "failed", err
		}
	}
}

type cursorTracker struct {
	resume           streamCursor
	resumeTime       time.Time
	currentTimestamp string
	ordinal          int
	last             streamCursor
	writeSequence    int
}

func (t *cursorTracker) next(timestamp string) (streamCursor, bool, error) {
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return streamCursor{}, false, fmt.Errorf("parsing Docker log timestamp: %w", err)
	}
	if timestamp != t.currentTimestamp {
		t.currentTimestamp = timestamp
		t.ordinal = 0
	}
	t.ordinal++
	cursor := streamCursor{Version: 1, Timestamp: timestamp, Ordinal: t.ordinal}
	if t.resume.Timestamp != "" {
		if parsed.Before(t.resumeTime) || (parsed.Equal(t.resumeTime) && t.ordinal <= t.resume.Ordinal) {
			return cursor, false, nil
		}
	}
	t.last = cursor
	return cursor, true, nil
}

type lineWriter struct {
	stream            string
	buffer            bytes.Buffer
	truncated         bool
	tracker           *cursorTracker
	emit              func(logRecord) error
	err               error
	lastWriteSequence int
}

type boundedReader struct {
	reader    io.Reader
	remaining int64
}

func (r *boundedReader) Read(payload []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		read, err := r.reader.Read(probe[:])
		if read > 0 {
			return 0, errRawLogStreamLimit
		}
		return 0, err
	}
	if int64(len(payload)) > r.remaining {
		payload = payload[:r.remaining]
	}
	read, err := r.reader.Read(payload)
	r.remaining -= int64(read)
	return read, err
}

func (w *lineWriter) Write(payload []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if len(payload) > 0 {
		w.tracker.writeSequence++
		w.lastWriteSequence = w.tracker.writeSequence
	}
	for _, value := range payload {
		if value == '\n' {
			w.flush()
			if w.err != nil {
				return 0, w.err
			}
			continue
		}
		if w.buffer.Len() < maxLogLineBytes {
			w.buffer.WriteByte(value)
		} else {
			w.truncated = true
		}
	}
	return len(payload), nil
}

func (w *lineWriter) flush() {
	line := strings.TrimSuffix(w.buffer.String(), "\r")
	w.buffer.Reset()
	timestamp, message, err := splitDockerTimestamp(line)
	if err != nil {
		w.err = err
		return
	}
	cursor, shouldEmit, err := w.tracker.next(timestamp)
	if err != nil {
		w.err = err
		return
	}
	if shouldEmit {
		id, err := encodeCursor(cursor)
		if err != nil {
			w.err = err
			return
		}
		w.err = w.emit(logRecord{
			ID:        id,
			Timestamp: timestamp,
			Stream:    w.stream,
			Message:   message,
			Truncated: w.truncated,
		})
	}
	w.truncated = false
}

func (w *lineWriter) flushPartial() {
	if w.buffer.Len() > 0 || w.truncated {
		w.flush()
	}
}

func splitDockerTimestamp(line string) (string, string, error) {
	timestamp, message, found := strings.Cut(line, " ")
	if found {
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), message, nil
		}
	}
	return "", "", fmt.Errorf("docker log record is missing a timestamp")
}

func readLogReader(reader io.Reader, tty bool, resume streamCursor, emit func(logRecord) error) (streamCursor, error) {
	tracker := &cursorTracker{resume: resume}
	if resume.Timestamp != "" {
		resumeTime, err := time.Parse(time.RFC3339Nano, resume.Timestamp)
		if err != nil {
			return streamCursor{}, fmt.Errorf("parsing resume timestamp: %w", err)
		}
		tracker.resumeTime = resumeTime
	}
	stdout := &lineWriter{stream: "stdout", tracker: tracker, emit: emit}
	stderr := &lineWriter{stream: "stderr", tracker: tracker, emit: emit}
	bounded := &boundedReader{reader: reader, remaining: maxRawStreamBytes}

	var err error
	if tty {
		_, err = io.Copy(stdout, bounded)
	} else {
		err = coreDocker.DemuxReader(bounded, stdout, stderr)
	}
	if err == nil {
		if stdout.lastWriteSequence <= stderr.lastWriteSequence {
			stdout.flushPartial()
			stderr.flushPartial()
		} else {
			stderr.flushPartial()
			stdout.flushPartial()
		}
	}
	if stdout.err != nil {
		return tracker.last, stdout.err
	}
	if stderr.err != nil {
		return tracker.last, stderr.err
	}
	return tracker.last, err
}
