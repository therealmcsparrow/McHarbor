// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"
)

// followRun is the one pattern for long-running operations: re-read the
// operation until it is terminal, printing progress to stderr whenever it
// changes, and return the final state for stdout. --no-follow skips it and the
// caller prints the id, which `get` reads later.
func followRun(ctx context.Context, errOut io.Writer, get func(context.Context) (map[string]any, error),
	terminal func(map[string]any) bool, progress func(map[string]any) string) (map[string]any, error) {
	last := ""
	for {
		state, err := get(ctx)
		if err != nil {
			return nil, err
		}
		if line := progress(state); line != "" && line != last {
			emitln(errOut, line)
			last = line
		}
		if terminal(state) {
			return state, nil
		}
		select {
		case <-ctx.Done():
			return state, &Error{Code: ExitTimeout, Message: "stopped following before the operation finished; it is still running on the server"}
		case <-time.After(pollInterval):
		}
	}
}

// sseEvent is one server-sent event.
type sseEvent struct {
	ID    string
	Event string
	Data  string
}

// readSSE parses a text/event-stream, calling emit per event; it returns when
// the stream ends or emit returns false.
func readSSE(r io.Reader, emit func(sseEvent) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var ev sseEvent
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(data) > 0 || ev.Event != "" {
				ev.Data = strings.Join(data, "\n")
				if !emit(ev) {
					return nil
				}
			}
			ev, data = sseEvent{}, nil
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			ev.ID = value
		case "event":
			ev.Event = value
		case "data":
			data = append(data, value)
		}
	}
	return scanner.Err()
}
