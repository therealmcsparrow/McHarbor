// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

type fixedHasher string

func (h fixedHasher) StableHash(v string) string { return string(h) + ":" + v }

// TestKeyCheckCatchesANodeWithItsOwnKey guards the HA failure where a node that
// generated its own key joins a shared database: first boot records, a different key
// is refused, and only an explicit reset accepts it.
func TestKeyCheckCatchesANodeWithItsOwnKey(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "c.db"))
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
	ctx := context.Background()
	if r, err := VerifyEncryptionKey(ctx, database, fixedHasher("A"), false); err != nil || !r.Recorded {
		t.Fatalf("first boot: %+v %v", r, err)
	}
	if r, err := VerifyEncryptionKey(ctx, database, fixedHasher("A"), false); err != nil || !r.Match || r.Recorded {
		t.Fatalf("same key: %+v %v", r, err)
	}
	if _, err := VerifyEncryptionKey(ctx, database, fixedHasher("B"), false); !errors.Is(err, ErrEncryptionKeyMismatch) {
		t.Fatalf("other key accepted: %v", err)
	}
	if _, err := VerifyEncryptionKey(ctx, database, fixedHasher("B"), true); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := VerifyEncryptionKey(ctx, database, fixedHasher("A"), false); !errors.Is(err, ErrEncryptionKeyMismatch) {
		t.Fatalf("after reset the old key must be the stranger: %v", err)
	}
}
