// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"math"
	"time"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 60 * time.Second
)

// RunWithReconnect connects the agent and reconnects with exponential backoff on disconnect.
func RunWithReconnect(ctx context.Context, agent *Agent, logger *slog.Logger) {
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := agent.Connect(ctx)
		if ctx.Err() != nil {
			return // Intentional shutdown
		}

		// From here until the next successful auth the agent is on its own:
		// buffer Docker events, stats and its own log locally.
		agent.MarkOffline(ctx)

		// A session that authenticated is proof the server is reachable, so
		// the ladder starts from the bottom again. Without this reset the
		// counter measured lifetime disconnects rather than consecutive
		// failures, and an agent up for hours sat at the 60s cap — taking a
		// minute or more to return from a one-second server restart.
		if agent.Authenticated() {
			attempt = 0
		}

		attempt++
		backoff := calcBackoff(attempt)
		logger.Warn("disconnected, reconnecting",
			"error", err,
			"attempt", attempt,
			"backoff", backoff,
		)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// calcBackoff returns the wait duration with exponential backoff and jitter.
func calcBackoff(attempt int) time.Duration {
	base := float64(initialBackoff) * math.Pow(2, float64(attempt-1))
	if base > float64(maxBackoff) {
		base = float64(maxBackoff)
	}
	// Add jitter: 0.5x to 1.5x
	jitter := 0.5 + jitterFraction()
	return time.Duration(base * jitter)
}

// jitterFraction returns a value in [0,1) from the OS entropy source, and 0.5
// (no net jitter) if that source fails.
func jitterFraction() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0.5
	}
	return float64(binary.BigEndian.Uint64(b[:])>>11) / (1 << 53)
}
