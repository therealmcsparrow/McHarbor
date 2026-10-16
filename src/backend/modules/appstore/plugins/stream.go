// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// SSEEvent is the wire shape every stream line uses. Mirrors the
// frontend useStreamInstall InstallEvent type so a single React
// component can render the same progress bar for app-store, plugin
// install, and plugin update streams.
type SSEEvent struct {
	Phase   string `json:"phase"`
	Step    int    `json:"step"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
	Status  string `json:"status"` // "progress" | "done" | "error"
	// Error fields are populated only on the terminal error event.
	ErrorCode string `json:"error_code,omitempty"`
	ErrorText string `json:"error_text,omitempty"`
	// Result is populated only on the terminal done event with the
	// serialized payload (InstallResult for install/update).
	Result any `json:"result,omitempty"`
}

// SSEWriter adapts an http.ResponseWriter into a Server-Sent
// Events stream. It also exposes a `progress` ProgressFunc so the
// installer can be driven without knowing about HTTP framing — the
// installer emits `progress(stage, pct, msg)`, the writer turns it
// into a properly framed SSE event with status="progress".
//
// Wire format:
//
//	data: {"phase":"...","step":N,"total":100,"message":"...","status":"progress"}\n\n
//
// Two consecutive newlines terminate each event per the SSE spec.
// Each Write is flushed immediately so the client receives events
// in real time — without the explicit Flush(), Go's net/http will
// buffer until the handler returns and the user sees a flat
// progress bar that jumps from 0 to 100.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// newSSEWriter flips the response into SSE mode: sets the headers
// that disable proxy buffering and the Content-Type, and verifies
// the writer can actually flush. Returns an error if the underlying
// writer doesn't implement http.Flusher — in practice the only way
// that happens is a custom middleware that wraps the writer, which
// the host doesn't use, so this is mostly defensive.
func newSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("response writer does not support flushing (required for SSE)")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &SSEWriter{w: w, flusher: flusher}, nil
}

// progress is the ProgressFunc passed to installer.Install. It
// converts the installer's (stage, pct, msg) triple into an SSE
// progress event. Errors writing the event are swallowed: by the
// time we fail to flush, the client has most likely disconnected,
// and surfacing the error here would just abort a clean uninstall
// path. The installer's own error handling still kicks in if the
// pipeline itself returns an error.
func (s *SSEWriter) progress(stage string, pct int, message string) {
	s.send(SSEEvent{
		Phase:   stage,
		Step:    pct,
		Total:   100,
		Message: message,
		Status:  "progress",
	})
}

// writeDone emits the terminal success event with the installer's
// result payload. Total is set to step so the progress bar fills
// the track and the frontend's InstallProgress component can flip
// its icon to the success variant.
func (s *SSEWriter) writeDone(result any) {
	s.send(SSEEvent{
		Phase:   "done",
		Step:    100,
		Total:   100,
		Message: fmt.Sprintf("%v", result),
		Status:  "done",
		Result:  result,
	})
}

// writeError emits the terminal error event with the i18n code and
// the raw error text (the latter is logged server-side; clients
// should render only ErrorCode through their translation layer).
func (s *SSEWriter) writeError(code i18n.MsgCode, errText string) {
	s.send(SSEEvent{
		Phase:     "error",
		Step:      0,
		Total:     100,
		Status:    "error",
		ErrorCode: string(code),
		ErrorText: errText,
	})
}

// send writes one event and logs a failure at debug, because a client that hung up mid-stream is the usual cause and the installer must keep running.
func (s *SSEWriter) send(ev SSEEvent) {
	if err := s.write(ev); err != nil {
		slog.Debug("plugin stream: writing event failed", "phase", ev.Phase, "error", err)
	}
}

// write serializes one SSEEvent and flushes it to the client. The
// JSON encoding is best-effort — if the encoder itself fails the
// event is dropped silently (we can't really recover).
func (s *SSEWriter) write(ev SSEEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encoding sse event: %w", err)
	}
	if _, err := s.w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := s.w.Write(b); err != nil {
		return err
	}
	if _, err := s.w.Write([]byte("\n\n")); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// Close is a no-op for SSE — the stream terminates when the handler
// returns and the underlying connection closes. Provided so callers
// can `defer sw.Close()` without thinking about it; matches the
// pattern in handleInstallStream and handleUpdateStream.
func (s *SSEWriter) Close() error {
	return nil
}
