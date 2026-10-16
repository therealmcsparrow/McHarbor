// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestFacetKeyMigration runs 0007 against the schema 0001 and 0004 create:
// schedules of a removed plugin report move onto the replacing built-in,
// container-inventory becomes workload-inventory in schedules and stored
// documents, and stored documents of a removed type are marked retired.
func TestFacetKeyMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	for _, f := range []string{"0001_init.sql", "0004_schedules.sql"} {
		runMigrationFile(t, db, f)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for id, typ := range map[string]string{
		"s-inv": "kvm-inventory", "s-prot": "lxc-protection", "s-cap": "kvm-allocation",
		"s-health": "swarm-cluster-health", "s-ci": "container-inventory", "s-keep": "backup-recovery",
	} {
		exec(`INSERT INTO plugin_reporting_schedules (id, name, report_type, subject_kind, subject_id, cron, created_at, updated_at)
			VALUES (?, ?, ?, 'kvm-host', 'h1', '0 6 * * *', 'x', 'x')`, id, id, typ)
	}
	for id, typ := range map[string]string{"r-old": "compute-inventory", "r-ci": "container-inventory", "r-keep": "access-review"} {
		exec(`INSERT INTO plugin_reporting_reports (id, report_type, title, period_start, period_end, document, created_at, updated_at)
			VALUES (?, ?, ?, 'a', 'b', ?, 'x', 'x')`, id, typ, id, `{"meta":{"report_type":"`+typ+`"}}`)
	}

	runMigrationFile(t, db, "0007_facet_report_keys.sql")

	schedules := map[string][2]string{}
	rows, err := db.Query(`SELECT id, report_type, subject_kind FROM plugin_reporting_schedules`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, typ, kind string
		if err := rows.Scan(&id, &typ, &kind); err != nil {
			t.Fatal(err)
		}
		schedules[id] = [2]string{typ, kind}
	}
	_ = rows.Close()
	want := map[string][2]string{
		"s-inv":    {"workload-inventory", ""},
		"s-prot":   {"data-protection-rpo", ""},
		"s-cap":    {"capacity-utilisation", ""},
		"s-health": {"environment-health", ""},
		"s-ci":     {"workload-inventory", "kvm-host"},
		"s-keep":   {"backup-recovery", "kvm-host"},
	}
	for id, w := range want {
		if schedules[id] != w {
			t.Errorf("schedule %s = %v, want %v", id, schedules[id], w)
		}
	}

	type rep struct{ typ, status, metaType string }
	reports := map[string]rep{}
	rows, err = db.Query(`SELECT id, report_type, status, json_extract(document, '$.meta.report_type') FROM plugin_reporting_reports`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var r rep
		if err := rows.Scan(&id, &r.typ, &r.status, &r.metaType); err != nil {
			t.Fatal(err)
		}
		reports[id] = r
	}
	_ = rows.Close()
	if r := reports["r-old"]; r.status != "retired" || r.typ != "compute-inventory" {
		t.Errorf("removed-type document = %+v, want retired and its type kept", r)
	}
	if r := reports["r-ci"]; r.typ != "workload-inventory" || r.metaType != "workload-inventory" || r.status != "ready" {
		t.Errorf("renamed document = %+v", r)
	}
	if r := reports["r-keep"]; r.status != "ready" || r.typ != "access-review" {
		t.Errorf("untouched document = %+v", r)
	}
}

func runMigrationFile(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(raw)); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
