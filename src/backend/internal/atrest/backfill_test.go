// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package atrest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

func migratedDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "atrest.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// tableColumn is one column of a table as PRAGMA table_info reports it.
type tableColumn struct {
	name, typ string
	notNull   bool
	hasDflt   bool
	pk        bool
}

func tableInfo(t *testing.T, database *sql.DB, table string) []tableColumn {
	t.Helper()
	rows, err := database.Query(`SELECT name, type, "notnull", dflt_value IS NOT NULL, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var out []tableColumn
	for rows.Next() {
		var c tableColumn
		var pk int
		if err := rows.Scan(&c.name, &c.typ, &c.notNull, &c.hasDflt, &pk); err != nil {
			t.Fatal(err)
		}
		c.pk = pk > 0
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// insertFixture adds one row to col.Table with col.Value set to value (nil = NULL),
// filling every other required column with a unique placeholder.
func insertFixture(t *testing.T, database *sql.DB, col Column, id string, value any, extra map[string]any) {
	t.Helper()
	names, marks, args := []string{}, []string{}, []any{}
	for _, c := range tableInfo(t, database, col.Table) {
		var v any
		switch {
		case c.name == col.ID:
			v = id
		case c.name == col.Value:
			v = value
		case extra[c.name] != nil:
			v = extra[c.name]
		case c.notNull && !c.hasDflt:
			if strings.Contains(strings.ToUpper(c.typ), "INT") {
				v = 1
			} else {
				v = c.name + "-" + id
			}
		default:
			continue
		}
		names, marks, args = append(names, `"`+c.name+`"`), append(marks, "?"), append(args, v)
	}
	query := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`, col.Table, strings.Join(names, ", "), strings.Join(marks, ", "))
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("fixture %s.%s %s: %v", col.Table, col.Value, id, err)
	}
}

func readValue(t *testing.T, database *sql.DB, col Column, id string) sql.NullString {
	t.Helper()
	var v sql.NullString
	query := `SELECT "` + col.Value + `" FROM "` + col.Table + `" WHERE "` + col.ID + `" = ?`
	if err := database.QueryRow(query, id).Scan(&v); err != nil {
		t.Fatalf("reading %s.%s %s: %v", col.Table, col.Value, id, err)
	}
	return v
}

func notNull(t *testing.T, database *sql.DB, col Column) bool {
	t.Helper()
	for _, c := range tableInfo(t, database, col.Table) {
		if c.name == col.Value {
			return c.notNull
		}
	}
	t.Fatalf("%s.%s is not in the schema", col.Table, col.Value)
	return false
}

// fixtureExtra keeps a fixture inside a column's Where filter, keyed per row so a
// unique column (settings.key) gets a distinct value each time.
func fixtureExtra(col Column, id string) map[string]any {
	if col.Table == "settings" {
		return map[string]any{"key": map[string]string{
			"plain": "tls_key", "sealed": "tls_cert", "empty": "webpush_vapid_private", "null": "webpush_vapid_private",
		}[id]}
	}
	return nil
}

// TestBackfillSealsEveryColumn runs every column in Columns against the migrated
// schema: plaintext becomes ciphertext that decrypts to the original, ciphertext, NULL
// and empty values are untouched, and a second run changes nothing.
func TestBackfillSealsEveryColumn(t *testing.T) {
	database := migratedDB(t)
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := enc.Encrypt("already-sealed")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, col := range Columns {
		if seen[col.Table] {
			continue
		}
		seen[col.Table] = true
		// One table at a time: the fixtures of a table fill all its listed columns at once.
		var cols []Column
		for _, c := range Columns {
			if c.Table == col.Table {
				cols = append(cols, c)
			}
		}
		base := cols[0]
		for _, id := range []string{"plain", "sealed", "empty", "null"} {
			extra := fixtureExtra(base, id)
			if extra == nil {
				extra = map[string]any{}
			}
			for _, c := range cols[1:] {
				switch id {
				case "plain":
					extra[c.Value] = "plain-" + c.Value
				case "sealed":
					extra[c.Value] = sealed
				case "empty", "null":
					extra[c.Value] = ""
				}
			}
			var value any
			switch id {
			case "plain":
				value = "plain-" + base.Value
			case "sealed":
				value = sealed
			case "empty":
				value = ""
			case "null":
				if notNull(t, database, base) {
					continue
				}
			}
			if id == "null" && col.Table == "settings" {
				continue
			}
			insertFixture(t, database, base, id, value, extra)
		}
	}

	results, err := Backfill(context.Background(), database, enc, Columns)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		if res.Missing {
			t.Errorf("%s.%s is listed but not in the schema", res.Table, res.Column)
		}
		if res.Encrypted != 1 || res.Skipped != 0 {
			t.Errorf("%s.%s: encrypted %d skipped %d, want exactly the plaintext row", res.Table, res.Column, res.Encrypted, res.Skipped)
		}
	}
	for _, col := range Columns {
		got := readValue(t, database, col, "plain")
		if !encryption.IsEncrypted(got.String) {
			t.Errorf("%s.%s plaintext row is still %q", col.Table, col.Value, got.String)
			continue
		}
		if plain, err := enc.Decrypt(got.String); err != nil || plain != "plain-"+col.Value {
			t.Errorf("%s.%s decrypts to (%q, %v)", col.Table, col.Value, plain, err)
		}
		if got := readValue(t, database, col, "sealed"); got.String != sealed {
			t.Errorf("%s.%s already-sealed row changed", col.Table, col.Value)
		}
		if got := readValue(t, database, col, "empty"); got.String != "" {
			t.Errorf("%s.%s empty row became %q", col.Table, col.Value, got.String)
		}
	}

	again, err := Backfill(context.Background(), database, enc, Columns)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range again {
		if res.Encrypted != 0 || res.Skipped != 0 {
			t.Errorf("second run touched %s.%s: %+v", res.Table, res.Column, res)
		}
	}
}

// TestBackfillLeavesOrdinarySettingsAndKeptValues: the settings filter keeps a
// non-credential row in plaintext, and a value the writer stores unencrypted on
// purpose ("{}" storage config) is not wrapped.
func TestBackfillLeavesOrdinarySettingsAndKeptValues(t *testing.T) {
	database := migratedDB(t)
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	var settingsCol, storageConfig Column
	for _, c := range Columns {
		switch {
		case c.Table == "settings":
			settingsCol = c
		case c.Table == "storage_locations" && c.Value == "config":
			storageConfig = c
		}
	}
	insertFixture(t, database, settingsCol, "theme", "dark", map[string]any{"key": "theme"})
	insertFixture(t, database, storageConfig, "loc", "{}", nil)
	if _, err := Backfill(context.Background(), database, enc, []Column{settingsCol, storageConfig}); err != nil {
		t.Fatal(err)
	}
	if got := readValue(t, database, settingsCol, "theme"); got.String != "dark" {
		t.Fatalf("an ordinary setting was encrypted: %q", got.String)
	}
	if got := readValue(t, database, storageConfig, "loc"); got.String != "{}" {
		t.Fatalf("the writer's unencrypted {} was wrapped: %q", got.String)
	}
}

// TestBackfillNeverOverwritesAConcurrentWrite: a row changed after the backfill read
// it keeps the newer value and is counted as skipped.
func TestBackfillNeverOverwritesAConcurrentWrite(t *testing.T) {
	database := migratedDB(t)
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	col := Column{Table: "registries", ID: "id", Value: "password"}
	insertFixture(t, database, col, "r1", "old-password", nil)
	page, err := pendingPage(context.Background(), database, col, "")
	if err != nil || len(page) != 1 {
		t.Fatalf("page = %v (%v)", page, err)
	}
	newer, err := enc.Encrypt("new-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE registries SET password = ? WHERE id = 'r1'`, newer); err != nil {
		t.Fatal(err)
	}
	encrypted, skipped, err := writePage(context.Background(), database, enc, col, page)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted != 0 || skipped != 1 {
		t.Fatalf("encrypted %d skipped %d, want the row skipped", encrypted, skipped)
	}
	if got := readValue(t, database, col, "r1"); got.String != newer {
		t.Fatal("the backfill overwrote a concurrent write")
	}
}

// TestBackfillRefusesWithoutEncryption: no encryption service, no writes.
func TestBackfillRefusesWithoutEncryption(t *testing.T) {
	database := migratedDB(t)
	if _, err := Backfill(context.Background(), database, nil, Columns); !errors.Is(err, ErrEncryptionUnavailable) {
		t.Fatalf("Backfill without encryption = %v", err)
	}
}
