// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

func lifecycleDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "lifecycle.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// TestInsertValidatesSubjectTypeAgainstDeclarations guards the check migration 130 moved
// out of the table: host subjects always write, a plugin subject writes only while a loaded
// plugin declares it, and anything else is refused before it reaches the database.
func TestInsertValidatesSubjectTypeAgainstDeclarations(t *testing.T) {
	reg := plugincaps.NewRegistry()
	svc := NewService(lifecycleDB(t))
	svc.declared = reg.IsLifecycleSubject
	ctx := context.Background()

	write := func(subjectType string) error {
		return svc.Insert(ctx, LifecycleEvent{SubjectType: subjectType, SubjectID: "s1", EventType: "guest", Action: "start"})
	}

	if err := write("container"); err != nil {
		t.Fatalf("a host subject type must write: %v", err)
	}
	if err := write("widget"); !errors.Is(err, ErrUnknownSubjectType) {
		t.Fatalf("an undeclared subject type must be refused, got %v", err)
	}

	reg.Set("addon-demo", []plugincaps.Capability{{Kind: plugincaps.KindLifecycleSubject, SubjectType: "widget", Page: "/plugins/addon-demo"}})
	if err := write("widget"); err != nil {
		t.Fatalf("a declared subject type must write: %v", err)
	}

	reg.Remove("addon-demo")
	if err := write("widget"); !errors.Is(err, ErrUnknownSubjectType) {
		t.Fatalf("a subject type must stop writing once its plugin unloads, got %v", err)
	}
	if err := write(""); !errors.Is(err, ErrUnknownSubjectType) {
		t.Fatalf("an empty subject type must be refused, got %v", err)
	}

	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM lifecycle_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("want exactly the two accepted rows, got %d", count)
	}
}

// TestBridgeRefusesAnUndeclaredPluginSubject proves the plugin path goes through the same check.
func TestBridgeRefusesAnUndeclaredPluginSubject(t *testing.T) {
	bridge := NewHostBridge(lifecycleDB(t))
	bridge.svc.declared = plugincaps.NewRegistry().IsLifecycleSubject
	err := bridge.Record(context.Background(), "addon-demo", pluginruntime.LifecycleRecord{SubjectType: "widget", SubjectID: "s1", EventType: "guest", Action: "start"})
	if !errors.Is(err, ErrUnknownSubjectType) {
		t.Fatalf("want ErrUnknownSubjectType, got %v", err)
	}
}
