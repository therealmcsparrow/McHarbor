// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// legacyHashRow is HashRow exactly as it was before APIKeyID existed.
type legacyHashRow struct {
	ID            string  `json:"id"`
	PrevHash      string  `json:"prevHash"`
	UserID        *string `json:"userId,omitempty"`
	Username      *string `json:"username,omitempty"`
	Action        string  `json:"action"`
	Severity      string  `json:"severity"`
	EntityType    *string `json:"entityType,omitempty"`
	EntityID      *string `json:"entityId,omitempty"`
	EntityName    *string `json:"entityName,omitempty"`
	Details       *string `json:"details,omitempty"`
	StateBefore   *string `json:"stateBefore,omitempty"`
	StateAfter    *string `json:"stateAfter,omitempty"`
	ChangedKeys   *string `json:"changedKeys,omitempty"`
	IPAddress     *string `json:"ipAddress,omitempty"`
	EnvironmentID *string `json:"environmentId,omitempty"`
	Timestamp     string  `json:"timestamp"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// TestRowsWithoutAKeyHashAsBefore guards every audit row sealed before the API
// key column existed: if its hash changed, the verifier would report the whole
// historic chain as tampered.
func TestRowsWithoutAKeyHashAsBefore(t *testing.T) {
	user, env := "u1", "e1"
	current := HashRow{ID: "r1", PrevHash: "abc", UserID: &user, Action: "create", Severity: "success",
		EnvironmentID: &env, Timestamp: "2026-09-01T00:00:00Z", CreatedAt: "2026-09-01T00:00:00Z", UpdatedAt: "2026-09-01T00:00:00Z"}
	legacy := legacyHashRow{ID: "r1", PrevHash: "abc", UserID: &user, Action: "create", Severity: "success",
		EnvironmentID: &env, Timestamp: "2026-09-01T00:00:00Z", CreatedAt: "2026-09-01T00:00:00Z", UpdatedAt: "2026-09-01T00:00:00Z"}

	got, err := ComputeRowHash("secret", current)
	if err != nil {
		t.Fatal(err)
	}
	want := legacyHash(t, "secret", legacy)
	if got != want {
		t.Fatalf("a row without an API key hashes to %s, want the pre-change %s", got, want)
	}

	key := "k1"
	current.APIKeyID = &key
	withKey, err := ComputeRowHash("secret", current)
	if err != nil {
		t.Fatal(err)
	}
	if withKey == want {
		t.Fatal("the API key is not part of the sealed payload")
	}
}

func TestRequestsWithAKeyAreAttributedToIt(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTest(t, database) })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	SetAPIKeyResolver(func(ctx context.Context) (string, string) {
		if ctx.Value(testKeyCtx{}) != nil {
			return "key-1", "ci-deploy"
		}
		return "", ""
	})
	t.Cleanup(func() { SetAPIKeyResolver(nil) })

	logger := NewLogger(database, "secret")
	withKey := httptest.NewRequest("POST", "/api/volumes", nil)
	withKey = withKey.WithContext(context.WithValue(withKey.Context(), testKeyCtx{}, true))
	logger.Log(withKey, Entry{Action: "create", EntityType: "volume", EntityName: "data"})
	logger.Log(httptest.NewRequest("POST", "/api/volumes", nil), Entry{Action: "create", EntityType: "volume", EntityName: "cache"})

	rows, err := database.Query(`SELECT entity_name, api_key_id, api_key_name, row_hash FROM audit_logs ORDER BY entity_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close rows: %v", err)
		}
	}()
	got := map[string][2]sql.NullString{}
	for rows.Next() {
		var name string
		var id, keyName, hash sql.NullString
		if err := rows.Scan(&name, &id, &keyName, &hash); err != nil {
			t.Fatal(err)
		}
		if hash.String == "" {
			t.Fatalf("row %s was not sealed", name)
		}
		got[name] = [2]sql.NullString{id, keyName}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got["data"][0].String != "key-1" || got["data"][1].String != "ci-deploy" {
		t.Fatalf("keyed row = %+v, want key-1 / ci-deploy", got["data"])
	}
	if got["cache"][0].Valid || got["cache"][1].Valid {
		t.Fatalf("session row carries a key: %+v", got["cache"])
	}
}

type testKeyCtx struct{}

func legacyHash(t *testing.T, secret string, row legacyHashRow) string {
	t.Helper()
	h, err := computeAnyHash(secret, row)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func computeAnyHash(secret string, row any) (string, error) {
	payload, err := json.Marshal(row)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
