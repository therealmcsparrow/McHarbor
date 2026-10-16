// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	coreinstancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
)

// TestVerifierReportsRestoreAnchorsInsteadOfHidingThem guards the third
// verdict: a chain through a restore verifies, and says it went through one.
func TestVerifierReportsRestoreAnchorsInsteadOfHidingThem(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "anchor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	logger := coreAudit.NewLogger(database, "secret")
	svc := NewService(database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), "secret")

	if err := logger.Record("", "admin", coreAudit.Entry{Action: "create", EntityType: "volume"}); err != nil {
		t.Fatal(err)
	}
	clean, err := svc.VerifyHashChain(context.Background())
	if err != nil || !clean.Verified || len(clean.Anchors) != 0 {
		t.Fatalf("a chain with no restore: %+v %v", clean, err)
	}

	if err := logger.Record("", "local:root@harbor", coreAudit.Entry{
		Action: coreinstancebackup.AnchorAction, EntityType: "instance_backup", Details: `{"preRestoreTerminalHash":"abc"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Record("", "admin", coreAudit.Entry{Action: "delete", EntityType: "volume"}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.VerifyHashChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || len(result.Anchors) != 1 || result.Anchors[0].Actor != "local:root@harbor" || result.Anchors[0].Details == "" {
		t.Fatalf("a chain through a restore: %+v", result)
	}
}
