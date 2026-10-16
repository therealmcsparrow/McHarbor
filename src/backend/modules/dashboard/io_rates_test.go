// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dashboard

import (
	"database/sql"
	"testing"
)

func counter(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

func sample(at string, rx, tx, read, write sql.NullInt64) ioSample {
	return ioSample{at: at, label: at[11:16], netRx: rx, netTx: tx, blockRead: read, blockWrite: write}
}

// TestIORatesAreBytesPerSecond guards the bug that made the environment charts
// look frozen: cumulative counters were charted as-is, a line that only rises.
func TestIORatesAreBytesPerSecond(t *testing.T) {
	samples := []ioSample{
		sample("2026-10-02 08:00:00", counter(1_000), counter(500), counter(10_000), counter(0)),
		sample("2026-10-02 08:00:30", counter(4_000), counter(800), counter(10_000), counter(3_000)),
		// Counter reset: a rebooted host starts again from zero.
		sample("2026-10-02 08:01:00", counter(100), counter(900), counter(10_600), counter(3_000)),
	}
	rx, tx, read, write := ioRates(samples)

	wantRx := []float64{100, 0}
	wantTx := []float64{10, 100.0 / 30}
	wantRead := []float64{0, 20}
	wantWrite := []float64{100, 0}
	for name, pair := range map[string]struct {
		got  []TimeValue
		want []float64
	}{"rx": {rx, wantRx}, "tx": {tx, wantTx}, "read": {read, wantRead}, "write": {write, wantWrite}} {
		if len(pair.got) != len(pair.want) {
			t.Fatalf("%s: %d points, want %d", name, len(pair.got), len(pair.want))
		}
		for i, want := range pair.want {
			if diff := pair.got[i].Value - want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("%s[%d] = %v, want %v", name, i, pair.got[i].Value, want)
			}
		}
	}
	if rx[0].Timestamp != "08:00" || rx[1].Timestamp != "08:01" {
		t.Errorf("labels = %q, %q; want the later sample's time", rx[0].Timestamp, rx[1].Timestamp)
	}
}

// TestIORatesSkipMissingCounters keeps a NULL row (no host counters) from being
// read as zero, which would chart the whole counter as one interval's traffic.
func TestIORatesSkipMissingCounters(t *testing.T) {
	missing := sql.NullInt64{}
	samples := []ioSample{
		sample("2026-10-02 08:00:00", missing, missing, missing, missing),
		sample("2026-10-02 08:00:30", counter(9_000_000), counter(1), counter(1), counter(1)),
		sample("2026-10-02 08:01:00", counter(9_000_300), counter(1), counter(1), counter(1)),
	}
	rx, _, _, _ := ioRates(samples)
	if len(rx) != 1 || rx[0].Value != 10 {
		t.Fatalf("rx = %+v, want one point of 10 B/s", rx)
	}
}
