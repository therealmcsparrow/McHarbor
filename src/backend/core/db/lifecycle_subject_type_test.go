// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const lifecycleOpenSubjectMigration = "130_lifecycle_events_open_subject_type.sql"

// TestLifecycleEventsSubjectTypeIsOpen pins migration 130: a subject type only a plugin
// declares is storable, while every other constraint, column and index of 077 (and the
// index 120 added) survives the rebuild, and re-running it keeps the rows it copies.
func TestLifecycleEventsSubjectTypeIsOpen(t *testing.T) {
	database, err := openSQLite(filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	}()
	if err := Migrate(database, DriverSQLite); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	insert := func(id, subjectType, severity, source string) error {
		_, err := database.Exec(`INSERT INTO lifecycle_events
			(id, subject_type, subject_id, event_type, action, severity, source)
			VALUES (?, ?, 's1', 'guest', 'start', ?, ?)`, id, subjectType, severity, source)
		return err
	}
	if err := insert("e1", "widget", "info", "addon-demo"); err != nil {
		t.Fatalf("a plugin-declared subject type must be storable: %v", err)
	}
	if err := insert("e2", "container", "success", "docker"); err != nil {
		t.Fatalf("a host subject type must stay storable: %v", err)
	}
	if err := insert("e3", "widget", "bogus", "docker"); err == nil {
		t.Error("the severity CHECK must survive the rebuild")
	}
	if err := insert("e4", "widget", "info", ""); err == nil {
		t.Error("the source CHECK must survive the rebuild")
	}
	if _, err := database.Exec(`INSERT INTO lifecycle_events (id, subject_id, event_type, action, severity)
		VALUES ('e5', 's1', 'guest', 'start', 'info')`); err == nil {
		t.Error("subject_type must stay NOT NULL")
	}

	var ddl string
	if err := database.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'lifecycle_events'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ddl, "subject_type IN") {
		t.Errorf("the fixed subject_type CHECK is still present: %s", ddl)
	}
	if !strings.Contains(ddl, "REFERENCES environments(id) ON DELETE SET NULL") {
		t.Errorf("the environment foreign key must survive the rebuild: %s", ddl)
	}

	wantIndexes := []string{
		"idx_lifecycle_events_env_sev_time", "idx_lifecycle_events_env_time",
		"idx_lifecycle_events_severity", "idx_lifecycle_events_subject", "idx_lifecycle_events_timestamp",
	}
	if got := lifecycleIndexes(t, database); !slices.Equal(got, wantIndexes) {
		t.Errorf("indexes: got %v, want %v", got, wantIndexes)
	}

	body, err := MigrationSQL(lifecycleOpenSubjectMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(body); err != nil {
		t.Fatalf("re-running the rebuild: %v", err)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM lifecycle_events WHERE id IN ('e1', 'e2')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("the rebuild must copy every row, kept %d of 2", count)
	}
	if got := lifecycleIndexes(t, database); !slices.Equal(got, wantIndexes) {
		t.Errorf("indexes after a rebuild: got %v, want %v", got, wantIndexes)
	}
}

func lifecycleIndexes(t *testing.T, database *sql.DB) []string {
	t.Helper()
	rows, err := database.Query(`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'lifecycle_events' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Logf("closing rows: %v", err)
		}
	}()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
