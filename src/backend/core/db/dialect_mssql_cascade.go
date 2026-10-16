// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// SQL Server refuses a foreign key whose ON DELETE action would give a table a
// second cascade path from some ancestor, or a cycle (error 1785). SQLite has no
// such rule, and the corpus relies on it: container_backup_runs cascades from
// environments both directly and through container_backup_plans. The action is
// kept by moving it into an AFTER DELETE trigger on the parent, which the rule
// does not govern, and dropping the constraint that would have carried it.

var (
	// mssqlCreateTableHeadRE finds the table a translated CREATE TABLE creates.
	mssqlCreateTableHeadRE = regexp.MustCompile(`(?is)\bCREATE\s+TABLE\s+\[?"?([a-z_][a-z0-9_]*)"?\]?\s*\(`)
	// mssqlReferencesRE reads one REFERENCES clause and its optional delete action.
	mssqlReferencesRE = regexp.MustCompile(`(?is)\s*\bREFERENCES\s+\[?"?([a-z_][a-z0-9_]*)"?\]?\s*\(\s*\[?"?([a-z_][a-z0-9_]*)"?\]?\s*\)` +
		`(\s+ON\s+DELETE\s+(CASCADE|SET\s+NULL|SET\s+DEFAULT|NO\s+ACTION|RESTRICT))?(\s+ON\s+UPDATE\s+(?:CASCADE|SET\s+NULL|SET\s+DEFAULT|NO\s+ACTION|RESTRICT))?`)
	// mssqlTableForeignKeyRE is a table-level FOREIGN KEY item, optionally named.
	mssqlTableForeignKeyRE = regexp.MustCompile(`(?is)^\s*(?:CONSTRAINT\s+\S+\s+)?FOREIGN\s+KEY\s*\(\s*\[?"?([a-z_][a-z0-9_]*)"?\]?\s*\)`)
	// mssqlAlterAddRE is an ALTER TABLE … ADD, capturing the table and the added
	// definition up to the statement end.
	mssqlAlterAddRE = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+\[?"?([a-z_][a-z0-9_]*)"?\]?\s+ADD\s+(.*?)\s*;?\s*$`)
	// mssqlColumnNameRE is the leading name of a column definition item.
	mssqlColumnNameRE = regexp.MustCompile(`(?is)^\s*\[?"?([a-z_][a-z0-9_]*)"?\]?\s`)
)

// cascadeQuerier is the read half of a migration executor; both *sql.Tx and
// *sql.DB satisfy it.
type cascadeQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// mssqlResolveCascadePaths rewrites a translated CREATE TABLE, or an ALTER TABLE
// that adds a column, so every foreign key SQL Server would refuse for a second
// cascade path loses its constraint, and returns the triggers that carry those
// delete actions instead.
func mssqlResolveCascadePaths(ctx context.Context, exec migrationExecutor, original string) (string, []string, error) {
	// Comments are masked so an apostrophe or a comma inside one cannot shift the
	// literal and item boundaries the scan below relies on.
	stmt, comments := maskComments(original)
	if !strings.Contains(strings.ToUpper(stmt), "ON DELETE") {
		return original, nil, nil
	}
	var table, body string
	var from, to int
	if head := mssqlCreateTableHeadRE.FindStringSubmatchIndex(stmt); head != nil {
		open := head[1] - 1
		closeAt := matchingParen(stmt, open)
		if closeAt < 0 {
			return original, nil, nil
		}
		table, from, to = stmt[head[2]:head[3]], open+1, closeAt
	} else if add := mssqlAlterAddRE.FindStringSubmatchIndex(stmt); add != nil {
		table, from, to = stmt[add[2]:add[3]], add[4], add[5]
	} else {
		return original, nil, nil
	}
	table, body = strings.ToLower(table), stmt[from:to]
	q, ok := exec.(cascadeQuerier)
	if !ok {
		return original, nil, nil
	}
	parents, err := mssqlCascadeParents(ctx, q)
	if err != nil {
		return "", nil, err
	}

	items := splitTopLevel(body)
	kept := make([]string, 0, len(items))
	var triggers []string
	for _, item := range items {
		ref := mssqlReferencesRE.FindStringSubmatchIndex(item)
		action := ""
		if ref != nil && ref[8] >= 0 {
			action = strings.ToUpper(strings.Join(strings.Fields(item[ref[8]:ref[9]]), " "))
		}
		if ref == nil || action == "" || action == "NO ACTION" || action == "RESTRICT" {
			kept = append(kept, item)
			continue
		}
		parent := strings.ToLower(item[ref[2]:ref[3]])
		if !cascadeConflicts(parents, parent, table) {
			parents[table] = append(parents[table], parent)
			kept = append(kept, item)
			continue
		}
		column := ""
		if m := mssqlTableForeignKeyRE.FindStringSubmatch(item); m != nil {
			column = m[1]
		} else if m := mssqlColumnNameRE.FindStringSubmatch(item); m != nil {
			column = m[1]
		}
		if column == "" {
			return "", nil, fmt.Errorf("sql server cascade path: cannot read the column of %q", strings.TrimSpace(item))
		}
		child := canonicalTableName(table)
		triggers = append(triggers, mssqlDropForeignTrigger(parent, child, column),
			mssqlCascadeTrigger(parent, item[ref[4]:ref[5]], child, column, action))
		if mssqlTableForeignKeyRE.MatchString(item) {
			continue
		}
		kept = append(kept, item[:ref[0]]+item[ref[1]:])
	}
	if triggers == nil {
		return original, nil, nil
	}
	return restoreComments(stmt[:from]+strings.Join(kept, ",")+stmt[to:], comments), triggers, nil
}

// mssqlCascadeParents reads the existing cascade graph as child → parents, the
// same edges SQL Server walks when it evaluates error 1785.
func mssqlCascadeParents(ctx context.Context, q cascadeQuerier) (map[string][]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT LOWER(OBJECT_NAME(parent_object_id)), LOWER(OBJECT_NAME(referenced_object_id)) "+
		"FROM sys.foreign_keys WHERE delete_referential_action <> 0")
	if err != nil {
		return nil, fmt.Errorf("reading the cascade graph: %w", err)
	}
	defer CloseRows(rows)
	out := map[string][]string{}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			return nil, fmt.Errorf("reading the cascade graph: %w", err)
		}
		out[child] = append(out[child], parent)
	}
	return out, rows.Err()
}

// cascadeConflicts reports whether a new cascading edge parent → child would give
// child, or any table below it, a second path from some table, or close a cycle.
func cascadeConflicts(parents map[string][]string, parent, child string) bool {
	from := ancestors(parents, parent)
	from[parent] = true
	if from[child] {
		return true
	}
	below := map[string]bool{child: true}
	for changed := true; changed; {
		changed = false
		for c, ps := range parents {
			for _, p := range ps {
				if below[p] && !below[c] {
					below[c], changed = true, true
				}
			}
		}
	}
	for d := range below {
		for a := range ancestors(parents, d) {
			if from[a] {
				return true
			}
		}
	}
	return false
}

// ancestors returns every table that reaches table through cascade edges.
func ancestors(parents map[string][]string, table string) map[string]bool {
	seen := map[string]bool{}
	stack := append([]string(nil), parents[table]...)
	for len(stack) > 0 {
		t := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[t] {
			continue
		}
		seen[t] = true
		stack = append(stack, parents[t]...)
	}
	return seen
}

// mssqlCascadeTrigger emulates one ON DELETE action. It is CREATE OR ALTER so a
// table rebuild that redeclares the key replaces the trigger rather than failing.
func mssqlCascadeTrigger(parent, parentColumn, child, column, action string) string {
	body := "DELETE FROM " + bracket(child) + " WHERE " + bracket(column) +
		" IN (SELECT " + bracket(parentColumn) + " FROM deleted)"
	switch action {
	case "SET NULL":
		body = "UPDATE " + bracket(child) + " SET " + bracket(column) + " = NULL WHERE " + bracket(column) +
			" IN (SELECT " + bracket(parentColumn) + " FROM deleted)"
	case "SET DEFAULT":
		body = "UPDATE " + bracket(child) + " SET " + bracket(column) + " = DEFAULT WHERE " + bracket(column) +
			" IN (SELECT " + bracket(parentColumn) + " FROM deleted)"
	}
	return "CREATE OR ALTER TRIGGER " + bracket("trg_mch_fk_"+child+"_"+column) + " ON " + bracket(parent) +
		" AFTER DELETE AS BEGIN SET NOCOUNT ON; " + body + "; END"
}

// mssqlDropForeignTrigger removes a same-named trigger left on another table. A
// rebuild renames the old table away with its triggers, and CREATE OR ALTER
// refuses a name that already belongs to a different object.
func mssqlDropForeignTrigger(parent, child, column string) string {
	name := "trg_mch_fk_" + child + "_" + column
	return "IF EXISTS (SELECT 1 FROM sys.triggers WHERE name = N'" + name + "' AND parent_id <> ISNULL(OBJECT_ID(N'" +
		parent + "'), 0)) DROP TRIGGER " + bracket(name)
}

// matchingParen returns the index of the parenthesis closing the one at open,
// skipping string literals, or -1.
func matchingParen(s string, open int) int {
	depth := 0
	quoted := false
	for i := open; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'':
			quoted = !quoted
		case quoted:
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevel splits a CREATE TABLE body on the commas between its items,
// leaving the commas inside a type, a default or a literal alone.
func splitTopLevel(body string) []string {
	var out []string
	depth, start := 0, 0
	quoted := false
	for i := 0; i < len(body); i++ {
		switch c := body[i]; {
		case c == '\'':
			quoted = !quoted
		case quoted:
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, body[start:i])
			start = i + 1
		}
	}
	return append(out, body[start:])
}
