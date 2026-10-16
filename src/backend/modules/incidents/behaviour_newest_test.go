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

// withRowCaps lowers the two memory row caps for one test, so the cap can be
// reached with a handful of rows instead of 200,000.
func withRowCaps(t *testing.T, containers, hosts int) {
	t.Helper()
	oldC, oldH := containerMemoryRowCap, hostMemoryRowCap
	containerMemoryRowCap, hostMemoryRowCap = containers, hosts
	t.Cleanup(func() { containerMemoryRowCap, hostMemoryRowCap = oldC, oldH })
}

func findings(t *testing.T, database *sql.DB, kind string) []string {
	t.Helper()
	rows, err := database.Query(`SELECT subject_name FROM behaviour_findings WHERE kind = ? ORDER BY subject_name`, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestMemoryClimbSurvivesTheRowCap guards that the cap drops the oldest rows,
// not the newest: twenty flat buckets from weeks ago fill a cap of twenty, and
// the climb that ended yesterday must still be found and fitted in time order.
func TestMemoryClimbSurvivesTheRowCap(t *testing.T) {
	withRowCaps(t, 20, 20)
	now := t0
	database := openDB(t)
	for i := 0; i < 20; i++ {
		at := now.AddDate(0, 0, -28).Add(time.Duration(i) * 12 * time.Hour)
		if _, err := database.Exec(`INSERT INTO container_metrics_rollup (id, environment_id, container_id, container_name, bucket_start, mem_used_avg)
			VALUES (?, 'e1', 'c0', 'noise', ?, ?)`, fmt.Sprintf("n%d", i), at.Format(metricsLayout), int64(300e6)); err != nil {
			t.Fatal(err)
		}
	}
	for d := 0; d < 12; d++ {
		at := now.AddDate(0, 0, -12+d)
		if _, err := database.Exec(`INSERT INTO container_metrics_rollup (id, environment_id, container_id, container_name, bucket_start, mem_used_avg)
			VALUES (?, 'e1', 'c1', 'api', ?, ?)`, fmt.Sprintf("a%d", d), at.Format(metricsLayout), int64(100e6+float64(d)*10e6)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (&Behaviour{DB: database, Now: func() time.Time { return now }}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := findings(t, database, FindingMemoryClimb); len(got) != 1 || got[0] != "api" {
		t.Fatalf("memory_climb findings %v under a 20-row cap, want [api]", got)
	}
}

// TestHostForecastSurvivesTheRowCap is the host-series twin: a quiet host's old
// buckets fill the cap, and the host filling up now must still be forecast.
func TestHostForecastSurvivesTheRowCap(t *testing.T) {
	withRowCaps(t, 20, 20)
	now := t0
	database := openDB(t)
	if _, err := database.Exec(`INSERT INTO environments (id, name) VALUES ('e2', 'quiet')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		at := now.AddDate(0, 0, -28).Add(time.Duration(i) * 12 * time.Hour)
		if _, err := database.Exec(`INSERT INTO host_metrics_rollup (id, environment_id, bucket_start, mem_percent_avg)
			VALUES (?, 'e2', ?, 30)`, fmt.Sprintf("q%d", i), at.Format(metricsLayout)); err != nil {
			t.Fatal(err)
		}
	}
	for d := 0; d < 12; d++ {
		at := now.AddDate(0, 0, -12+d)
		if _, err := database.Exec(`INSERT INTO host_metrics_rollup (id, environment_id, bucket_start, mem_percent_avg)
			VALUES (?, 'e1', ?, ?)`, fmt.Sprintf("h%d", d), at.Format(metricsLayout), 50+float64(d)*2.5); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (&Behaviour{DB: database, Now: func() time.Time { return now }}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := findings(t, database, FindingHostMemoryFull); len(got) != 1 || got[0] != "e1" {
		t.Fatalf("host_memory_forecast findings %v under a 20-row cap, want [e1]", got)
	}
}
