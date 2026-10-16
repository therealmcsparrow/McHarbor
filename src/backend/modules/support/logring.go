// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LogLine is one server log record kept in memory for the support bundle.
type LogLine struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Attrs   string `json:"attrs,omitempty"`
}

// LogRing keeps the most recent server log records in memory. McHarbor logs to stdout only,
// so without it a bundle could say nothing about what the server did before it was made.
type LogRing struct {
	mu    sync.Mutex
	lines []LogLine
	next  int
	full  bool
}

// NewLogRing returns a ring holding at most size records.
func NewLogRing(size int) *LogRing {
	if size < 1 {
		size = 1
	}
	return &LogRing{lines: make([]LogLine, size)}
}

// Handler wraps inner so every record it accepts is also copied into the ring; inner stays
// the one deciding the level, so the ring never sees what stdout would not.
func (l *LogRing) Handler(inner slog.Handler) slog.Handler {
	return &ringHandler{ring: l, inner: inner}
}

func (l *LogRing) add(line LogLine) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines[l.next] = line
	l.next = (l.next + 1) % len(l.lines)
	if l.next == 0 {
		l.full = true
	}
}

// Snapshot returns the kept records, oldest first.
func (l *LogRing) Snapshot() []LogLine {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.full {
		return append([]LogLine(nil), l.lines[:l.next]...)
	}
	out := make([]LogLine, 0, len(l.lines))
	out = append(out, l.lines[l.next:]...)
	return append(out, l.lines[:l.next]...)
}

type ringHandler struct {
	ring   *LogRing
	inner  slog.Handler
	prefix string
	attrs  string
}

func (h *ringHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *ringHandler) Handle(ctx context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&b, h.prefix, a)
		return true
	})
	h.ring.add(LogLine{
		Time:    r.Time.UTC().Format(time.RFC3339),
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   strings.TrimSpace(b.String()),
	})
	return h.inner.Handle(ctx, r)
}

func (h *ringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var b strings.Builder
	b.WriteString(h.attrs)
	for _, a := range attrs {
		writeAttr(&b, h.prefix, a)
	}
	return &ringHandler{ring: h.ring, inner: h.inner.WithAttrs(attrs), prefix: h.prefix, attrs: b.String()}
}

func (h *ringHandler) WithGroup(name string) slog.Handler {
	return &ringHandler{ring: h.ring, inner: h.inner.WithGroup(name), prefix: h.prefix + name + ".", attrs: h.attrs}
}

func writeAttr(b *strings.Builder, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		for _, g := range v.Group() {
			writeAttr(b, prefix+a.Key+".", g)
		}
		return
	}
	fmt.Fprintf(b, " %s%s=%v", prefix, a.Key, v.Any())
}
