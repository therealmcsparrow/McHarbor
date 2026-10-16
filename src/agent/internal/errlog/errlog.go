// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package errlog reports the errors of best-effort cleanup calls (close,
// remove, rollback, late writes) that have no caller left to return them to.
package errlog

import (
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Log records err at warn level with the caller's source position, skipping
// the outcomes that only mean the resource was already gone.
func Log(err error) {
	if err == nil || benign(err) {
		return
	}
	slog.Warn("ignored cleanup error", "error", err, "at", caller())
}

// Close closes c and logs a failure through Log; it is the deferred-close form
// so callers keep their one-line defer.
func Close(c io.Closer) {
	if err := c.Close(); err != nil && !benign(err) {
		slog.Warn("ignored cleanup error", "error", err, "at", caller())
	}
}

// ReadAll reads r to the end and logs a read failure instead of dropping it;
// whatever was read is still returned, and callers bound r themselves.
func ReadAll(r io.Reader) []byte {
	body, err := io.ReadAll(r)
	if err != nil && !benign(err) {
		slog.Warn("ignored read error", "error", err, "at", caller())
	}
	return body
}

// Value returns v and logs err, for best-effort reads whose zero value is an
// acceptable answer when the call fails.
func Value[T any](v T, err error) T {
	if err != nil && !benign(err) {
		slog.Warn("ignored error", "error", err, "at", caller())
	}
	return v
}

// Discard drains up to limit bytes of r so the connection can be reused and
// logs a failure instead of dropping it.
func Discard(r io.Reader, limit int64) {
	if _, err := io.Copy(io.Discard, io.LimitReader(r, limit)); err != nil && !benign(err) {
		slog.Warn("ignored drain error", "error", err, "at", caller())
	}
}

func benign(err error) bool {
	if errors.Is(err, sql.ErrTxDone) || errors.Is(err, os.ErrClosed) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrProcessDone) || errors.Is(err, fs.ErrNotExist) {
		return true
	}
	msg := err.Error()
	for _, frag := range []string{"use of closed", "close sent", "broken pipe", "connection reset", "already closed"} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	return false
}

func caller() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		file = file[i+1:]
	}
	return file + ":" + strconv.Itoa(line)
}
