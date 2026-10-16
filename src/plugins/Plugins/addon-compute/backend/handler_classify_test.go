// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type quietLogger struct{}

func (quietLogger) Info(string, ...any)  {}
func (quietLogger) Warn(string, ...any)  {}
func (quietLogger) Error(string, ...any) {}
func (quietLogger) Debug(string, ...any) {}

// TestClassifyKeepsServiceTextServerSide guards the pool-move handlers, which
// used to send the raw error text beside their data: a mapped refusal travels as a code
// alone, and only an unmapped host failure carries the host's trimmed words.
func TestClassifyKeepsServiceTextServerSide(t *testing.T) {
	h := &Handler{logger: quietLogger{}}

	target := fmt.Errorf("%w: /srv/secret-mount is not an absolute path", ErrInvalidTarget)
	status, code, detail := h.classify("pool.move.plan", target)
	if status != http.StatusBadRequest || code != codeInvalidTarget || detail != "" {
		t.Fatalf("invalid target: got %d %q %q, want 400 %q and no detail", status, code, detail, codeInvalidTarget)
	}

	status, code, detail = h.classify("pool.move.start", fmt.Errorf("wrapped: %w", ErrNotAgentEnv))
	if status != http.StatusConflict || code != codeNotAgentEnv || detail != "" {
		t.Fatalf("not an agent env: got %d %q %q", status, code, detail)
	}

	host := errors.New("compute: rsync exited 23: some files vanished")
	status, code, detail = h.classify("pool.repair", host)
	if status != http.StatusBadGateway || code != codeAgentFailed {
		t.Fatalf("host failure: got %d %q", status, code)
	}
	if detail == "" || strings.HasPrefix(detail, "compute: ") {
		t.Fatalf("host failure detail must be the trimmed host words, got %q", detail)
	}
}
