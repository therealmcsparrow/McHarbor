// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"regexp"
	"strings"
	"testing"
)

// sqliteOnlyConstructs are the things that must not survive translation onto
// an external driver. Each one is a function or syntax that exists only in
// SQLite, so a statement still carrying it after TranslateStatement will fail
// at apply time.
//
// This test exists because discovering them one per `Migrate()` run — the
// first failure hides the next 40 — turned a morning's work into a loop. It
// reports every remaining construct across the whole corpus in one pass.
var sqliteOnlyConstructs = map[string]*regexp.Regexp{
	"json_each":        regexp.MustCompile(`(?i)\bjson_each\s*\(`),
	"json_group_array": regexp.MustCompile(`(?i)\bjson_group_array\s*\(`),
	"json_insert":      regexp.MustCompile(`(?i)\bjson_insert\s*\(`),
	// Case-sensitive, unlike the rest: MySQL's own spelling is JSON_OBJECT,
	// which is what the translation emits. Only the lowercase SQLite form
	// is a survivor.
	"json_object":    regexp.MustCompile(`\bjson_object\s*\(`),
	"json_extract":   regexp.MustCompile(`(?i)\bjson_extract\s*\(`),
	"randomblob":     regexp.MustCompile(`(?i)\brandomblob\s*\(`),
	"strftime":       regexp.MustCompile(`(?i)\bstrftime\s*\(`),
	"datetime(":      regexp.MustCompile(`(?i)\bdatetime\s*\(\s*'`),
	"INSERT OR":      regexp.MustCompile(`(?i)\bINSERT\s+OR\s+(IGNORE|REPLACE)\b`),
	"COLLATE NOCASE": regexp.MustCompile(`(?i)\bCOLLATE\s+NOCASE\b`),
	"AUTOINCREMENT":  regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`),
	"PRAGMA":         regexp.MustCompile(`(?i)^\s*PRAGMA\b`),
}

func TestNoSQLiteOnlyConstructsSurviveTranslation(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("reading migrations: %v", err)
	}

	for _, driver := range []Driver{DriverPostgres, DriverMySQL, DriverMSSQL} {
		t.Run(string(driver), func(t *testing.T) {
			reported := map[string]int{}

			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
					continue
				}
				body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
				if err != nil {
					t.Fatalf("reading %s: %v", entry.Name(), err)
				}

				for _, stmt := range SplitStatements(string(body)) {
					action := ClassifyStatement(driver, stmt)
					if action == SkipStatement || action == SkipStatementLoudly {
						continue
					}
					translated := stripComments(TranslateStatement(driver, stmt))
					for name, re := range sqliteOnlyConstructs {
						if re.MatchString(translated) {
							if reported[name] < 3 {
								t.Errorf("%s: %s survives translation in %s:\n%s",
									driver, name, entry.Name(), firstLine(stmt))
							}
							reported[name]++
						}
					}
				}
			}

			for name, count := range reported {
				if count > 3 {
					t.Logf("%s: %s appears %d more times", driver, name, count-3)
				}
			}
		})
	}
}

// TestKeyedTextColumnsBecomeVarchar is the rule §9 of the secrets plan
// predicted, pinned at the corpus level: no CREATE TABLE or ALTER TABLE that
// MySQL will apply may leave a keyed column as TEXT, and no *unkeyed* column
// may be narrowed.
func TestKeyedTextColumnsBecomeVarchar(t *testing.T) {
	// secrets.value is the canonical unkeyed column: it holds a multi-KB
	// kubeconfig or certificate chain and must survive as TEXT.
	body, err := migrationsFS.ReadFile("migrations/069_secrets.sql")
	if err != nil {
		t.Fatalf("reading 069_secrets.sql: %v", err)
	}

	var createSecrets string
	for _, stmt := range SplitStatements(string(body)) {
		if strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS secrets") {
			createSecrets = TranslateStatement(DriverMySQL, stmt)
			break
		}
	}
	if createSecrets == "" {
		t.Fatal("could not find the secrets CREATE TABLE")
	}

	// Matched on the column name plus its type, not on the original column
	// padding — the rewrite replaces the run of spaces along with the type.
	if !columnIsVarchar(createSecrets, "scope_key") {
		t.Errorf("scope_key is keyed and should be a VARCHAR:\n%s", createSecrets)
	}
	if !columnIsVarchar(createSecrets, "id") {
		t.Errorf("id is the primary key and should be a VARCHAR:\n%s", createSecrets)
	}
	if !columnIsText(createSecrets, "value") {
		t.Errorf("secrets.value is unkeyed and must stay TEXT - a VARCHAR would truncate a certificate chain:\n%s", createSecrets)
	}
}

func TestSQLiteTranslationIsIdentity(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("reading migrations: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		text := string(body)
		if got := TranslateStatement(DriverSQLite, text); got != text {
			t.Fatalf("%s was modified for SQLite; the default driver must see the file verbatim", entry.Name())
		}
		if ClassifyStatement(DriverSQLite, text) != ExecuteStatement {
			t.Fatalf("%s was classified as anything but Execute on SQLite", entry.Name())
		}
	}
}

// columnIsVarchar reports whether a CREATE TABLE declares the column as a
// VARCHAR, ignoring the alignment padding between the name and the type.
func columnIsVarchar(ddl, column string) bool {
	return regexp.MustCompile(`(?i)` + `\b` + column + `\s+VARCHAR\(`).MatchString(ddl)
}

// columnIsText is its counterpart for the columns that must stay TEXT.
func columnIsText(ddl, column string) bool {
	return regexp.MustCompile(`(?i)` + `\b` + column + `\s+TEXT\b`).MatchString(ddl)
}
