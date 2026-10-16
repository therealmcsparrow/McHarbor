// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

// offlineLogHandler is a slog.Handler that forwards every record to the real
// stdout handler and, while the agent is offline, also appends it to the local
// store. Records are buffered at Info and above regardless of LOG_LEVEL: the
// point of the buffer is to show an operator what happened while McHarbor
// could not see the host, and debug chatter would crowd the bounded table.
type offlineLogHandler struct {
	inner  slog.Handler
	buffer *OfflineBuffer
	attrs  []slog.Attr
	group  string
}

// newOfflineLogHandler wraps inner. buffer may be swapped in later via
// SetBuffer because the store is opened after the logger exists.
func newOfflineLogHandler(inner slog.Handler) *offlineLogHandler {
	return &offlineLogHandler{inner: inner}
}

// SetBuffer attaches the buffer once it is open.
func (h *offlineLogHandler) SetBuffer(b *OfflineBuffer) { h.buffer = b }

func (h *offlineLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.buffer != nil && h.buffer.IsOffline() && level >= slog.LevelInfo {
		return true
	}
	return h.inner.Enabled(ctx, level)
}

func (h *offlineLogHandler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	if h.inner.Enabled(ctx, r.Level) {
		err = h.inner.Handle(ctx, r)
	}
	if h.buffer == nil || !h.buffer.IsOffline() || r.Level < slog.LevelInfo {
		return err
	}

	attrs := make(map[string]any, r.NumAttrs()+len(h.attrs))
	for _, a := range h.attrs {
		attrs[h.qualify(a.Key)] = attrValue(a.Value)
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[h.qualify(a.Key)] = attrValue(a.Value)
		return true
	})
	var attrsJSON string
	if len(attrs) > 0 {
		if b, marshalErr := json.Marshal(attrs); marshalErr == nil {
			attrsJSON = string(b)
		}
	}
	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	if storeErr := h.buffer.Store().AddLog(context.WithoutCancel(ctx), OfflineLogEntry{
		Level:    strings.ToLower(r.Level.String()),
		Message:  r.Message,
		Attrs:    attrsJSON,
		LoggedAt: ts.UTC().Format(time.RFC3339Nano),
	}); storeErr != nil && err == nil {
		err = storeErr
	}
	return err
}

func (h *offlineLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &offlineLogHandler{inner: h.inner.WithAttrs(attrs), buffer: h.buffer, attrs: merged, group: h.group}
}

func (h *offlineLogHandler) WithGroup(name string) slog.Handler {
	group := name
	if h.group != "" {
		group = h.group + "." + name
	}
	return &offlineLogHandler{inner: h.inner.WithGroup(name), buffer: h.buffer, attrs: h.attrs, group: group}
}

func (h *offlineLogHandler) qualify(key string) string {
	if h.group == "" {
		return key
	}
	return h.group + "." + key
}

// attrValue flattens a slog value into something json.Marshal handles. Errors
// become their message; everything else is resolved and passed through.
func attrValue(v slog.Value) any {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return v.Any()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindGroup:
		out := make(map[string]any, len(v.Group()))
		for _, a := range v.Group() {
			out[a.Key] = attrValue(a.Value)
		}
		return out
	default:
		return v.Any()
	}
}
