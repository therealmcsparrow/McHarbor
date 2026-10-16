// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

type chainKeyCtx struct{}

// TestChainWithKeyedRowsVerifiesAndSealsTheKey guards both halves of API key
// attribution: rows with and without a key verify together, and rewriting which
// key made a change breaks the chain like any other edit.
func TestChainWithKeyedRowsVerifiesAndSealsTheKey(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "chain.db"))
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
	coreAudit.SetAPIKeyResolver(func(ctx context.Context) (string, string) {
		if ctx.Value(chainKeyCtx{}) != nil {
			return "key-1", "ci-deploy"
		}
		return "", ""
	})
	t.Cleanup(func() { coreAudit.SetAPIKeyResolver(nil) })

	logger := coreAudit.NewLogger(database, "secret")
	session := httptest.NewRequest("POST", "/api/volumes", nil)
	keyed := httptest.NewRequest("POST", "/api/volumes", nil)
	keyed = keyed.WithContext(context.WithValue(keyed.Context(), chainKeyCtx{}, true))
	logger.Log(session, coreAudit.Entry{Action: "create", EntityType: "volume", EntityName: "a"})
	logger.Log(keyed, coreAudit.Entry{Action: "create", EntityType: "volume", EntityName: "b"})
	logger.Log(session, coreAudit.Entry{Action: "delete", EntityType: "volume", EntityName: "a"})

	svc := NewService(database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), "secret")
	result, err := svc.VerifyHashChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.HashedRows != 3 {
		t.Fatalf("mixed chain: %+v, want 3 verified rows", result)
	}

	logs, _, err := svc.List(context.Background(), listFilter{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	var sawKey bool
	for _, l := range logs {
		if l.APIKeyName != nil && *l.APIKeyName == "ci-deploy" && l.APIKeyID != nil && *l.APIKeyID == "key-1" {
			sawKey = true
		}
	}
	if !sawKey {
		t.Fatal("the audit list does not return which key made the change")
	}

	if _, err := database.Exec(`UPDATE audit_logs SET api_key_id = 'someone-elses-key' WHERE api_key_id = 'key-1'`); err != nil {
		t.Fatal(err)
	}
	result, err = svc.VerifyHashChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Verified {
		t.Fatal("rewriting which key made a change did not break the chain")
	}
}
