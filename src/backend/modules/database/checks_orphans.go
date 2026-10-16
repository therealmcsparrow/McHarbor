// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// checkOrphans counts rows whose parent no longer exists, across declared
// foreign keys and the relations convention discovery found.
func (c *Checker) checkOrphans(ctx context.Context, driver coredb.Driver, tables []string) {
	c.setPhase(checkPhaseOrphans)

	relations, unresolved, excluded, err := discoverRelations(ctx, c.source, driver, tables)
	if err != nil {
		c.add(Finding{Key: "orphan_discovery_failed", Family: familyOrphans, Severity: severityFail, Detail: sanitize(err, "")})
		return
	}

	if len(unresolved) > 0 {
		c.add(Finding{
			Key:      "orphan_unmapped",
			Family:   familyOrphans,
			Severity: severitySkip,
			Reason:   "unresolved_relation",
			Count:    int64(len(unresolved)),
			Detail:   strings.Join(unresolved, ", "),
		})
	}

	if len(excluded) > 0 {
		// The exclusion map is hand-maintained, so it is reported for the same
		// reason the unresolved list is: a column suppressed by mistake is
		// otherwise invisible.
		c.add(Finding{
			Key:      "orphan_excluded",
			Family:   familyOrphans,
			Severity: severityInfo,
			Count:    int64(len(excluded)),
			Detail:   strings.Join(excluded, ", "),
		})
	}

	total := int64(0)
	checked := int64(0)
	for i, rel := range relations {
		if ctx.Err() != nil {
			return
		}
		c.setTable(rel.Table+"."+rel.Column, i+1, len(relations))

		n, err := orphanCount(ctx, c.source, driver, rel)
		if err != nil {
			c.add(Finding{
				Key:       "orphan_query_failed",
				Family:    familyOrphans,
				Severity:  severityWarn,
				Table:     rel.Table,
				Column:    rel.Column,
				RefTable:  rel.RefTable,
				RefColumn: rel.RefColumn,
				Detail:    sanitize(err, ""),
			})
			continue
		}
		checked++
		if n == 0 {
			continue
		}
		total += n

		// A governed table's rows are meant to outlive the row they name, so the
		// dangling reference is reported but never offered for deletion.
		severity, reason, remedy := severityWarn, "", remedyDeleteOrphans
		if isGovernedTable(rel.Table) {
			severity, reason, remedy = severityInfo, "governed_table", ""
		}

		c.add(Finding{
			Key:         "orphan_rows",
			Family:      familyOrphans,
			Severity:    severity,
			Table:       rel.Table,
			Column:      rel.Column,
			RefTable:    rel.RefTable,
			RefColumn:   rel.RefColumn,
			Count:       n,
			Reason:      reason,
			Remedy:      remedy,
			Destructive: remedy != "",
		})
	}

	// A relation whose count errored was never verified, so it must not be
	// folded into a "checked, clean" claim — that is the one severity a
	// summary card renders green.
	if total == 0 && checked > 0 {
		c.add(Finding{
			Key:      "orphans_none",
			Family:   familyOrphans,
			Severity: severityOK,
			Count:    checked,
		})
	}
}

// orphanCount counts child rows whose parent row is gone.
//
// Every identifier is quoted and originates from catalog introspection, never
// from request input, so there is no interpolation of caller-controlled text.
func orphanCount(ctx context.Context, database *sql.DB, driver coredb.Driver, rel relation) (int64, error) {
	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	child := coredb.QuoteIdent(driver, rel.Table)
	parent := coredb.QuoteIdent(driver, rel.RefTable)
	childCol := coredb.QuoteIdent(driver, rel.Column)
	parentCol := coredb.QuoteIdent(driver, rel.RefColumn)

	query := fmt.Sprintf( // SQL identifier: every %s is a coredb.QuoteIdent result or Placeholder
		`SELECT COUNT(*) FROM %s AS c LEFT JOIN %s AS p ON c.%s = p.%s
		  WHERE c.%s IS NOT NULL AND p.%s IS NULL`,
		child, parent, childCol, parentCol, childCol, parentCol,
	)

	var n int64
	if err := database.QueryRowContext(runCtx, query).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting orphans in %s.%s: %w", rel.Table, rel.Column, err)
	}
	return n, nil
}
