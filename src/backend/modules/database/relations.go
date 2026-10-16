// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// relation is one child-to-parent link the orphan check will test.
type relation struct {
	Table     string
	Column    string
	RefTable  string
	RefColumn string
	// Declared is true when a FOREIGN KEY constraint backs this relation.
	Declared bool
}

// relationOverrides maps a column name to the parent table convention gets
// wrong. Seeded from environments.envColumnNames: roughly 46 columns hold an
// environments.id and almost none of them are constrained.
var relationOverrides = map[string]string{
	"env_id":                 "environments",
	"environment_id":         "environments",
	"target_environment_id":  "environments",
	"preview_environment_id": "environments",
}

// governedTables hold records McHarbor must not offer to delete: an audit
// trail's rows are meant to outlive the user or environment they name.
//
// Their orphans are still reported, because an operator should be able to see
// them — only the destructive remedy is withheld.
var governedTables = map[string]bool{
	"audit_logs": true,
}

// isGovernedTable reports whether a table's rows are a governed record rather
// than data the check may propose pruning.
func isGovernedTable(table string) bool {
	return governedTables[strings.ToLower(table)]
}

// relationExclusions are columns that look like a relation and are not:
// an external identity provider's subject, a self-reference, or a pointer
// that is allowed to dangle once the row it named is revoked.
//
// Keyed "table.column"; a bare "column" excludes that name everywhere.
var relationExclusions = map[string]bool{
	"parent_id":   true,
	"external_id": true,
	"provider_id": true,
	"subject_id":  true,
	"message_id":  true,
	"request_id":  true,
	"session_id":  true,
	"secret_id":   true,
	"trace_id":    true,
}

// discoverRelations returns every relation worth testing for orphans, the
// candidate columns it could not map, and the columns the exclusion map
// suppressed.
//
// The unresolved list is returned rather than dropped on purpose: discovery
// that silently discards what it cannot resolve has the same blind spot as a
// hand-maintained map and hides it better.
func discoverRelations(ctx context.Context, database *sql.DB, driver coredb.Driver, tables []string) ([]relation, []string, []string, error) {
	known := make(map[string]bool, len(tables))
	for _, t := range tables {
		known[strings.ToLower(t)] = true
	}

	var (
		found      []relation
		unresolved []string
		excluded   []string
		seen       = map[string]bool{}
	)

	for _, table := range tables {
		keys, err := coredb.ForeignKeys(ctx, database, driver, table)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("reading declared keys of %s: %w", table, err)
		}
		for _, k := range keys {
			rel := relation{
				Table:     table,
				Column:    k.Column,
				RefTable:  k.RefTable,
				RefColumn: k.RefColumn,
				Declared:  true,
			}
			id := relationKey(rel)
			if seen[id] {
				continue
			}
			seen[id] = true
			found = append(found, rel)
		}
	}

	for _, table := range tables {
		cols, err := coredb.TableColumns(ctx, database, driver, table)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("reading columns of %s: %w", table, err)
		}
		for _, col := range cols {
			lower := strings.ToLower(col)
			if !isRelationCandidate(lower) {
				continue
			}
			if relationExclusions[lower] || relationExclusions[strings.ToLower(table)+"."+lower] {
				excluded = append(excluded, table+"."+col)
				continue
			}
			parent, ok := resolveParent(lower, known)
			if !ok {
				unresolved = append(unresolved, table+"."+col)
				continue
			}
			rel := relation{Table: table, Column: col, RefTable: parent, RefColumn: "id"}
			// A declared key already covers this column; the declared form
			// wins because it carries the real parent column.
			if seen[relationKey(rel)] || seenColumn(seen, table, col) {
				continue
			}
			seen[relationKey(rel)] = true
			found = append(found, rel)
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].Table != found[j].Table {
			return found[i].Table < found[j].Table
		}
		return found[i].Column < found[j].Column
	})
	sort.Strings(unresolved)
	sort.Strings(excluded)
	return found, unresolved, excluded, nil
}

// isRelationCandidate reports whether a column name looks like it holds
// another row's id.
func isRelationCandidate(col string) bool {
	if col == "id" {
		return false
	}
	return strings.HasSuffix(col, "_id")
}

// resolveParent maps a candidate column to a real table, applying the
// override map first and the strip-and-pluralise convention second.
func resolveParent(col string, known map[string]bool) (string, bool) {
	if parent, ok := relationOverrides[col]; ok && known[parent] {
		return parent, true
	}
	base := strings.TrimSuffix(col, "_id")
	if base == "" {
		return "", false
	}
	for _, candidate := range []string{base + "s", base, base + "es"} {
		if known[candidate] {
			return candidate, true
		}
	}
	// "policy_id" -> "policies"
	if strings.HasSuffix(base, "y") {
		plural := strings.TrimSuffix(base, "y") + "ies"
		if known[plural] {
			return plural, true
		}
	}
	return "", false
}

// relationKey is the dedupe key for a relation.
func relationKey(r relation) string {
	return strings.ToLower(r.Table + "." + r.Column + "->" + r.RefTable + "." + r.RefColumn)
}

// seenColumn reports whether any relation already covers this child column,
// whatever parent it resolved to.
func seenColumn(seen map[string]bool, table, col string) bool {
	prefix := strings.ToLower(table + "." + col + "->")
	for k := range seen {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}
