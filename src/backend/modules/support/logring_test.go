// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"io"
	"log/slog"
	"strconv"
	"testing"
)

// TestLogRingKeepsTheNewestInOrder guards what a bundle calls "recent logs": the latest
// records, oldest first, with attributes bound by With still attached.
func TestLogRingKeepsTheNewestInOrder(t *testing.T) {
	ring := NewLogRing(3)
	logger := slog.New(ring.Handler(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))).With("node", "a")
	logger.Debug("dropped by the level")
	for i := 0; i < 5; i++ {
		logger.Info("line " + strconv.Itoa(i))
	}
	got := ring.Snapshot()
	if len(got) != 3 {
		t.Fatalf("kept %d lines, want 3", len(got))
	}
	for i, want := range []string{"line 2", "line 3", "line 4"} {
		if got[i].Message != want || got[i].Attrs != "node=a" {
			t.Errorf("line %d = %+v, want %q with node=a", i, got[i], want)
		}
	}
}
