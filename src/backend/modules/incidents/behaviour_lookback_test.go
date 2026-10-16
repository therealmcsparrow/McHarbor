// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestMemoryLookbackFollowsRetentionWithinBounds(t *testing.T) {
	cases := []struct{ retention, want int }{
		{1, 14}, {7, 14}, {14, 14}, {15, 15}, {30, 30}, {90, 90}, {91, 90}, {3650, 90},
	}
	for _, c := range cases {
		if got := memoryLookbackDays(c.retention); got != c.want {
			t.Errorf("retention %d days: look-back %d, want %d", c.retention, got, c.want)
		}
	}
}

// seedOldClimb writes a steady daily memory climb that ends 16 days ago, so only
// a look-back longer than the old fixed 14 days can see it.
func seedOldClimb(t *testing.T, database *sql.DB, now time.Time) {
	t.Helper()
	for d := 0; d < 12; d++ {
		at := now.AddDate(0, 0, -27+d)
		if _, err := database.Exec(`INSERT INTO container_metrics_rollup (id, environment_id, container_id, container_name, bucket_start, mem_used_avg)
			VALUES (?, 'e1', 'c1', 'api', ?, ?)`, fmt.Sprintf("r%d", d), at.Format(metricsLayout), int64(100e6+float64(d)*10e6)); err != nil {
			t.Fatal(err)
		}
	}
}

func climbFindings(t *testing.T, database *sql.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM behaviour_findings WHERE kind = ?`, FindingMemoryClimb).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMemoryDetectorsReadTheConfiguredRetention(t *testing.T) {
	now := t0
	b := func(database *sql.DB) *Behaviour {
		return &Behaviour{DB: database, Now: func() time.Time { return now }}
	}

	database := openDB(t)
	seedOldClimb(t, database, now)
	if _, err := b(database).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := climbFindings(t, database); got != 1 {
		t.Fatalf("with the default 30-day retention the climb 16-27 days ago gave %d findings, want 1", got)
	}

	short := openDB(t)
	if _, err := short.Exec(`INSERT INTO settings (id, key, value, category, updated_at)
		VALUES ('s1', 'retention_metrics_rollup_days', '7', 'retention', ?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	seedOldClimb(t, short, now)
	if _, err := b(short).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := climbFindings(t, short); got != 0 {
		t.Fatalf("with 7 days of retention the look-back is the 14-day floor, yet the old climb gave %d findings", got)
	}
}
