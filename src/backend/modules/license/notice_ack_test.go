// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// TestNoticeAckIsPerUser guards the licence banner: one person's dismissal used
// to clear the shared flag, hiding an overage or revocation from every admin.
func TestNoticeAckIsPerUser(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "lic.db")+"?_pragma=foreign_keys(0)")
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
	svc := NewService(database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), "test")
	svc.snapshot = Status{NoticeKind: "overage", NoticeDue: true, noticeSince: time.Now().Add(-2 * time.Hour)}
	svc.sampled = true
	ctx := context.Background()

	recorded, err := svc.AcknowledgeNotice(ctx, "alice", "overage")
	if err != nil || !recorded {
		t.Fatalf("alice's first acknowledgement: recorded=%v err=%v", recorded, err)
	}
	if st, err := svc.StatusFor(ctx, "alice"); err != nil || st.NoticeDue {
		t.Fatalf("alice dismissed it: due=%v err=%v", st.NoticeDue, err)
	}
	if st, err := svc.StatusFor(ctx, "bob"); err != nil || !st.NoticeDue {
		t.Fatalf("bob has not seen it and must still be told: due=%v err=%v", st.NoticeDue, err)
	}
	if recorded, err := svc.AcknowledgeNotice(ctx, "alice", "overage"); err != nil || recorded {
		t.Fatalf("a repeat inside the window must not write another row: recorded=%v err=%v", recorded, err)
	}
	var rows int
	if err := database.QueryRow(`SELECT COUNT(*) FROM license_notices WHERE user_id = 'alice'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("alice's rows = %d (%v), want exactly 1, attributed", rows, err)
	}
}
