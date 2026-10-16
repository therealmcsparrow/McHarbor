// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// TestListEnvironmentScopeKeepsInstanceWideRows pins which rows an environment
// filter shows: that environment's rows plus every instance-wide row, whether
// stored as NULL or as the empty string, and never another environment's.
func TestListEnvironmentScopeKeepsInstanceWideRows(t *testing.T) {
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id  string
		env any
	}{
		{"a1", "env-a"}, {"a2", "env-a"}, {"b1", "env-b"}, {"n1", nil}, {"n2", nil}, {"e1", ""},
	}
	for i, r := range rows {
		if _, err := database.Exec(`INSERT INTO audit_logs (id, action, severity, environment_id, timestamp)
			VALUES (?, 'x.test', 'info', ?, ?)`, r.id, r.env, "2026-09-27T10:00:0"+string(rune('0'+i))+"Z"); err != nil {
			t.Fatalf("seeding %s: %v", r.id, err)
		}
	}
	svc := NewService(database, nil, nil, "test-secret")

	cases := []struct {
		name   string
		filter listFilter
		want   string
	}{
		{"scoped", listFilter{Page: 1, PerPage: 50, EnvironmentID: "env-a"}, "a1,a2,e1,n1,n2"},
		{"other environment", listFilter{Page: 1, PerPage: 50, EnvironmentID: "env-b"}, "b1,e1,n1,n2"},
		{"exact", listFilter{Page: 1, PerPage: 50, EnvironmentID: "env-a", EnvironmentExact: true}, "a1,a2"},
		{"unscoped", listFilter{Page: 1, PerPage: 50}, "a1,a2,b1,e1,n1,n2"},
	}
	for _, c := range cases {
		logs, total, err := svc.List(context.Background(), c.filter)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		ids := make([]string, 0, len(logs))
		for _, l := range logs {
			ids = append(ids, l.ID)
		}
		sort.Strings(ids)
		if got := strings.Join(ids, ","); got != c.want {
			t.Errorf("%s: rows %s, want %s", c.name, got, c.want)
		}
		if int(total) != len(strings.Split(c.want, ",")) {
			t.Errorf("%s: total %d, want %d", c.name, total, len(strings.Split(c.want, ",")))
		}
	}
}
