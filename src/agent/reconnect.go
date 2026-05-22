// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
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
	jitter := 0.5 + rand.Float64()
	return time.Duration(base * jitter)
}
