// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package elevations

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// TestPendingRequestsAreCappedPerRequester guards the approvers' inbox: every
// request notifies all of them, so one user cannot queue an unbounded number,
// nor ask twice for the same permission while the first is still open.
func TestPendingRequestsAreCappedPerRequester(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "cap.db")+"?_pragma=foreign_keys(1)")
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
	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash) VALUES ('req', 'req', 'x')`); err != nil {
		t.Fatal(err)
	}
	if err := rbac.UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	// Real environment-scoped keys, marked elevatable, one more than the cap.
	keys := []string{"containers.delete", "images.delete", "volumes.delete", "networks.delete", "scans.manage", "incidents.manage"}
	if len(keys) != maxPendingPerRequester+1 {
		t.Fatalf("the test needs %d keys", maxPendingPerRequester+1)
	}
	for _, key := range keys {
		if err := rbac.SetElevatable(context.Background(), database, key, true, "test"); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewService(database)
	ctx := context.Background()
	ask := func(key string) error {
		_, err := svc.Create(ctx, "req", CreateRequest{PermissionKey: key, Justification: "j", RequestedMinutes: 30})
		return err
	}

	for _, key := range keys[:maxPendingPerRequester] {
		if err := ask(key); err != nil {
			t.Fatalf("request %s under the cap: %v", key, err)
		}
	}
	if err := ask(keys[0]); !errors.Is(err, ErrAlreadyPending) {
		t.Fatalf("a second pending request for the same key: got %v, want ErrAlreadyPending", err)
	}
	if err := ask(keys[maxPendingPerRequester]); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("the request past the cap: got %v, want ErrTooManyPending", err)
	}
}
