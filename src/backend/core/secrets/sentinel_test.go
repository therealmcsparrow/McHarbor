// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// sentinelValue is a marker no legitimate output could contain by accident.
const sentinelValue = "McHarborSentinel-5f3a9c21-DO-NOT-LEAK"

// TestSentinelNeverReachesStoredText is step 6.5 of the secrets-manager plan.
//
// It creates a secret whose value is a unique marker, exercises every path
// that touches it — create, resolve, rotate, roll back, list, read references
// — and then greps everything the application persists or hands back for that
// marker. Zero hits is the requirement.
//
// The classes of leak this is built to catch, each of which has actually
// happened in some system somewhere:
//
//   - an audit payload that recorded the value instead of the name;
//   - a secret_versions row read back into a response;
//   - a list or detail response that grew a value field;
//   - a reference row that stored the value in its `field` column;
//   - the secrets table itself holding plaintext rather than ciphertext.
func TestSentinelNeverReachesStoredText(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "SENTINEL", Scope: ScopeGlobal, Kind: KindPassword, Value: sentinelValue,
	})
	if err != nil {
		t.Fatalf("creating the sentinel secret: %v", err)
	}

	// Exercise the paths.
	resolved, err := svc.ResolveValue(ctx, "conn://u:${secret:SENTINEL}@host",
		ResolveScope{ResourceType: "stack", ResourceID: "sentinel-stack"}, "DSN")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if !strings.Contains(resolved, sentinelValue) {
		t.Fatal("the resolver did not produce the value; the rest of this test would prove nothing")
	}
	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: sentinelValue + "-v2"}); err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if _, err := svc.Rollback(ctx, "user1", "", sec.ID, 1); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	if _, _, err := svc.List(ctx, ListFilter{}, 1, 25); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if _, err := svc.References(ctx, sec.ID); err != nil {
		t.Fatalf("listing references: %v", err)
	}
	if _, err := svc.Versions(ctx, sec.ID); err != nil {
		t.Fatalf("listing versions: %v", err)
	}

	// Now sweep every text column of every table for the marker. Two columns
	// are expected to contain it — `secrets.value` and `secret_versions.value`
	// — and both must hold *ciphertext*, so a hit there is itself a failure.
	tables := allTables(t, database)
	if len(tables) < 10 {
		t.Fatalf("table sweep found only %d tables; the fixture database looks wrong", len(tables))
	}

	for _, table := range tables {
		for _, column := range textColumns(t, database, table) {
			var hits int
			query := "SELECT COUNT(*) FROM " + table + " WHERE " + column + " LIKE ?"
			if err := database.QueryRow(query, "%"+sentinelValue+"%").Scan(&hits); err != nil {
				// A column the driver cannot LIKE (a blob) is not a text leak.
				continue
			}
			if hits > 0 {
				t.Errorf("the sentinel value leaked into %s.%s (%d row(s)) — see §5.6", table, column, hits)
			}
		}
	}
}

// TestStoredSecretIsCiphertext is the narrower half: the two columns that
// legitimately hold the value must hold it encrypted.
func TestStoredSecretIsCiphertext(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "SENTINEL", Scope: ScopeGlobal, Value: sentinelValue,
	})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: sentinelValue + "-v2"}); err != nil {
		t.Fatalf("rotating: %v", err)
	}

	var stored string
	if err := database.QueryRow("SELECT value FROM secrets WHERE id = ?", sec.ID).Scan(&stored); err != nil {
		t.Fatalf("reading stored value: %v", err)
	}
	if !strings.HasPrefix(stored, "enc:v1:") {
		t.Errorf("secrets.value is not ciphertext: %q", stored)
	}

	var archived string
	if err := database.QueryRow(
		"SELECT value FROM secret_versions WHERE secret_id = ? ORDER BY version DESC LIMIT 1", sec.ID,
	).Scan(&archived); err != nil {
		t.Fatalf("reading archived value: %v", err)
	}
	if !strings.HasPrefix(archived, "enc:v1:") {
		t.Errorf("secret_versions.value is not ciphertext: %q", archived)
	}
}

func allTables(t *testing.T, database *sql.DB) []string {
	t.Helper()
	rows, err := database.Query(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	defer closeRows(rows)

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating tables: %v", err)
	}
	return tables
}

func textColumns(t *testing.T, database *sql.DB, table string) []string {
	t.Helper()
	rows, err := database.Query("SELECT name, type FROM pragma_table_info(?)", table)
	if err != nil {
		return nil
	}
	defer closeRows(rows)

	var columns []string
	for rows.Next() {
		var name, columnType string
		if err := rows.Scan(&name, &columnType); err != nil {
			continue
		}
		upper := strings.ToUpper(columnType)
		if strings.Contains(upper, "CHAR") || strings.Contains(upper, "TEXT") ||
			strings.Contains(upper, "CLOB") || upper == "" {
			columns = append(columns, `"`+name+`"`)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating columns of %s: %v", table, err)
	}
	return columns
}
