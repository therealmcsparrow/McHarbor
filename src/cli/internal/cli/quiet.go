// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"strings"
)

// emitf writes formatted text to a terminal stream. A failed write to stdout or
// stderr has nowhere left to be reported, so it is the one discard in the CLI.
func emitf(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, format, a...) //nolint:errcheck // a failed terminal write cannot be reported anywhere else
}

// emit writes its operands to a terminal stream without a newline, discarding a
// failed write for the same reason as emitf.
func emit(w io.Writer, a ...any) {
	fmt.Fprint(w, a...) //nolint:errcheck // a failed terminal write cannot be reported anywhere else
}

// emitln writes its operands and a newline to a terminal stream, discarding a
// failed write for the same reason as emitf.
func emitln(w io.Writer, a ...any) {
	fmt.Fprintln(w, a...) //nolint:errcheck // a failed terminal write cannot be reported anywhere else
}

// warnErr reports a best-effort cleanup failure (close, remove, late write) on
// stderr instead of dropping it, skipping outcomes that only mean the resource
// was already gone.
func warnErr(err error) {
	if err == nil || benignCleanupError(err) {
		return
	}
	emitf(os.Stderr, "warning: %v\n", err)
}

// valueOr returns v and reports err through warnErr, for best-effort reads
// whose zero value is an acceptable answer when the call fails.
func valueOr[T any](v T, err error) T {
	warnErr(err)
	return v
}

func benignCleanupError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, fs.ErrNotExist) {
		return true
	}
	msg := err.Error()
	for _, frag := range []string{"use of closed", "close sent", "already closed"} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	return false
}
