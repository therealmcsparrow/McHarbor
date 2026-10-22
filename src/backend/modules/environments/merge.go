// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// Errors the merge path returns.
var (
	ErrMergeIntoSelf   = errors.New("an environment cannot be merged into itself")
	ErrMergeSourceGone = errors.New("source environment not found")
	ErrMergeTargetGone = errors.New("target environment not found")
	// ErrMergeUnsupportedDriver guards a database driver the schema walk has no
	// catalog query for, rather than half-applying a merge it cannot plan.
	ErrMergeUnsupportedDriver = errors.New("merging environments is not supported on this database driver")
)

// mergePageSize bounds every cursor the merge opens. The merge walks rows in
// pages by key so a skipped row is never read twice and no SELECT is unbounded.
var mergePageSize = 1000

// envColumnNames are the column names that hold an environments.id.
//
// Discovered against the live schema rather than hardcoded per table: fifty
// tables reference an environment today and the list grows with every feature.
// A merge that silently misses the table someone added last week orphans its
// rows, and orphaned rows are deleted by the cascade at the end of the merge.
var envColumnNames = map[string]bool{
	"environment_id":         true,
	"env_id":                 true,
	"target_environment_id":  true,
	"preview_environment_id": true,
}

// MergePlanEntry is one table's share of a merge.
type MergePlanEntry struct {
	Table  string `json:"table"`
	Column string `json:"column"`
	Rows   int    `json:"rows"`
}

// MergeResult is the plan, or what the plan turned out to do.
//
// Discarded counts rows a unique constraint refused: the target already has the
// equivalent row — the same pool name, the same compute settings row, the same
// role grant, the same secret name, the same alert state — and the target's
// copy is the one that survives.
type MergeResult struct {
	SourceID    string           `json:"sourceId"`
	SourceName  string           `json:"sourceName"`
	TargetID    string           `json:"targetId"`
	TargetName  string           `json:"targetName"`
	DryRun      bool             `json:"dryRun"`
	Moved       []MergePlanEntry `json:"moved"`
	Discarded   []MergePlanEntry `json:"discarded,omitempty"`
	TotalRows   int              `json:"totalRows"`
	DefaultMove bool             `json:"defaultMoved,omitempty"`
}

// MergeEnvironments folds source into target and deletes source.
//
// Every row keyed on the source id is re-pointed at the target, so the history
// an operator would otherwise lose by deleting a duplicate environment — its
// metrics, its activity, its audit trail, its stacks — survives the
// consolidation. dryRun returns the same plan without writing anything.
func (s *Service) MergeEnvironments(ctx context.Context, sourceID, targetID string, dryRun bool) (*MergeResult, error) {
	if sourceID == targetID {
		return nil, ErrMergeIntoSelf
	}
	source, err := s.ByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, ErrMergeSourceGone
	}
	target, err := s.ByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrMergeTargetGone
	}

	// Introspection runs before the transaction opens: SQLite runs on one
	// connection, and a catalog query issued beside an open tx would wait on it.
	discoverCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	schema, err := discoverMergeSchema(discoverCtx, s.db, mergeDriver())
	if err != nil {
		return nil, err
	}

	result := &MergeResult{
		SourceID:   sourceID,
		SourceName: source.Name,
		TargetID:   targetID,
		TargetName: target.Name,
		DryRun:     dryRun,
	}
	for _, col := range schema.columns {
		count, err := s.countRows(ctx, schema.driver, col, sourceID)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			continue
		}
		result.Moved = append(result.Moved, MergePlanEntry{Table: col.table, Column: col.column, Rows: count})
		result.TotalRows += count
	}
	result.DefaultMove = source.IsDefault && !target.IsDefault

	if dryRun {
		return result, nil
	}

	if err := inTx(ctx, s.db, func(tx *sql.Tx) error {
		return applyMerge(ctx, tx, schema, source, target, result)
	}); err != nil {
		return nil, err
	}

	// The source is gone; both ids must stop resolving to a cached client, and
	// an agent still attached to the source has nothing left to serve.
	s.disconnectAgent(sourceID)
	s.dockerPool.Remove(sourceID)
	s.dockerPool.Remove(targetID)
	notifyMerged(sourceID, targetID)

	return result, nil
}

var mergedHook atomic.Pointer[func(sourceID, targetID string)]

// SetMergedHook installs the callback run after a merge commits, so in-memory
// caches keyed by environment id (alert states, host disk tracking) can drop
// the ids the merge just re-keyed. It is package-level because every handler
// and tool builds its own Service; nil removes the hook.
func SetMergedHook(fn func(sourceID, targetID string)) {
	if fn == nil {
		mergedHook.Store(nil)
		return
	}
	mergedHook.Store(&fn)
}

func notifyMerged(sourceID, targetID string) {
	if fn := mergedHook.Load(); fn != nil {
		(*fn)(sourceID, targetID)
	}
}

// mergeDriver names the active driver, treating "not opened yet" as SQLite
// because that is what every caller without db.Open is running against.
func mergeDriver() db.Driver {
	if driver := db.DriverOf(); driver != "" {
		return driver
	}
	return db.DriverSQLite
}

// applyMerge performs the re-pointing inside one transaction.
func applyMerge(
	ctx context.Context,
	tx *sql.Tx,
	schema *mergeSchema,
	source, target *Environment,
	result *MergeResult,
) error {
	// Secrets carry a digest of their own scope, so the environment id has to
	// be rewritten in two places at once or the moved secret resolves to
	// nothing. Done before the generic pass, which would otherwise move the
	// row and leave the digest pointing at the dead environment.
	if schema.tables["secrets"] {
		skipped, err := mergeSecrets(ctx, tx, source.ID, target.ID)
		if err != nil {
			return err
		}
		result.addDiscarded("secrets", "environment_id", skipped)
	}

	for _, col := range schema.columns {
		if col.table == "secrets" {
			continue
		}
		// Skipping a collision is the merge rule, not a shortcut: a row the
		// target already has an equivalent of — the same pool name, the same
		// per-environment settings row — cannot be moved onto it without
		// violating a unique constraint, and the target's copy is the one to
		// keep. What stays behind is removed with the source.
		_, skipped, err := repointColumn(ctx, tx, schema.driver, col, source.ID, target.ID)
		if err != nil {
			return fmt.Errorf("merging %s.%s: %w", col.table, col.column, err)
		}
		result.addDiscarded(col.table, col.column, skipped)
	}

	if err := mergeEmbeddedIDs(ctx, tx, schema, source.ID, target.ID, result); err != nil {
		return err
	}

	if result.DefaultMove {
		if _, err := tx.ExecContext(ctx, `UPDATE environments SET is_default = 1 WHERE id = ?`, target.ID); err != nil {
			return fmt.Errorf("moving default flag: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM environments WHERE id = ?`, source.ID); err != nil {
		return fmt.Errorf("deleting merged environment: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE environments SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), target.ID); err != nil {
		return fmt.Errorf("touching merged environment: %w", err)
	}
	return nil
}

// addDiscarded records rows a merge left behind because the target already had
// their equivalent; zero is not recorded so the report lists only real skips.
func (r *MergeResult) addDiscarded(table, column string, rows int) {
	if rows <= 0 {
		return
	}
	r.Discarded = append(r.Discarded, MergePlanEntry{Table: table, Column: column, Rows: rows})
}

// repointColumn moves one column's rows from source to target, skipping every
// row whose move would collide with a unique key the target already holds.
//
// The skip is decided in Go rather than by SQLite's UPDATE OR IGNORE, which no
// other backend accepts: collisions are read first, and only the rest move.
func repointColumn(ctx context.Context, tx *sql.Tx, driver db.Driver, col envColumn, sourceID, targetID string) (moved, skipped int, err error) {
	table, column := db.QuoteIdent(driver, col.table), db.QuoteIdent(driver, col.column)

	colliding, err := countCollisions(ctx, tx, driver, col, sourceID, targetID)
	if err != nil {
		return 0, 0, err
	}
	if colliding == 0 {
		res, err := tx.ExecContext(ctx, `UPDATE `+table+` SET `+column+` = ? WHERE `+column+` = ?`, targetID, sourceID) // SQL identifier: table and column come from db.QuoteIdent
		if err != nil {
			return 0, 0, err
		}
		return int(db.RowsAffected(res)), 0, nil
	}
	rowKey := rowIdentity(col)
	if len(rowKey) == 0 {
		// The environment column is a unique key on its own: the target already
		// has its one row, so every source row stays behind.
		return 0, colliding, nil
	}

	skip, err := collidingRows(ctx, tx, driver, col, rowKey, sourceID, targetID)
	if err != nil {
		return 0, 0, err
	}

	selectList, match := quotedList(driver, rowKey, ", "), matchList(driver, rowKey)
	for {
		// Moved rows leave the source set and skipped rows stay in it, ahead of
		// everything unread in this order, so the offset is the count skipped.
		page, err := pageTuples(ctx, tx, len(rowKey), `SELECT `+selectList+` FROM `+table+
			` WHERE `+column+` = ? ORDER BY `+selectList+` LIMIT ? OFFSET ?`,
			sourceID, mergePageSize, skipped)
		if err != nil {
			return moved, skipped, err
		}
		for _, tuple := range page {
			if skip[tupleKey(tuple)] {
				skipped++
				continue
			}
			args := append([]any{targetID, sourceID}, tupleArgs(tuple)...)
			res, err := tx.ExecContext(ctx, `UPDATE `+table+` SET `+column+` = ? WHERE `+column+` = ? AND `+match, args...) // SQL identifier: table and column come from db.QuoteIdent; match is built from quoted names
			if err != nil {
				return moved, skipped, err
			}
			if n := int(db.RowsAffected(res)); n > 0 {
				moved += n
			} else {
				// A NULL in the row key matches nothing; the row stays behind
				// and must be counted, or the offset would re-read it forever.
				skipped++
			}
		}
		if len(page) < mergePageSize {
			return moved, skipped, nil
		}
	}
}

// rowIdentity names the columns that single out one source row: its id when
// the table has one, otherwise the rest of its first unique key.
func rowIdentity(col envColumn) []string {
	if col.hasID {
		return []string{"id"}
	}
	if len(col.keys) == 0 {
		return nil
	}
	var out []string
	for _, k := range col.keys[0] {
		if !strings.EqualFold(k, col.column) {
			out = append(out, k)
		}
	}
	return out
}

// quotedList joins quoted column names, optionally prefixed with an alias.
func quotedList(driver db.Driver, cols []string, sep string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = db.QuoteIdent(driver, c)
	}
	return strings.Join(parts, sep)
}

// matchList is the `a = ? AND b = ?` predicate that selects one row by its key.
func matchList(driver db.Driver, cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = db.QuoteIdent(driver, c) + " = ?"
	}
	return strings.Join(parts, " AND ")
}

// collisionPredicate is the WHERE clause, over source alias s and target alias
// o, that is true when moving s onto the target would duplicate key.
//
// SQL Server's unique indexes treat NULLs as equal where the other three
// backends do not, so only there does a NULL pair count as a collision.
func collisionPredicate(driver db.Driver, col envColumn, key []string) string {
	column := db.QuoteIdent(driver, col.column)
	parts := []string{"o." + column + " = ?"}
	for _, k := range key {
		if strings.EqualFold(k, col.column) {
			continue
		}
		q := db.QuoteIdent(driver, k)
		if driver == db.DriverMSSQL {
			parts = append(parts, "(o."+q+" = s."+q+" OR (o."+q+" IS NULL AND s."+q+" IS NULL))")
		} else {
			parts = append(parts, "o."+q+" = s."+q)
		}
	}
	return strings.Join(parts, " AND ")
}

// countCollisions counts source rows that would duplicate a target row on any
// unique key containing the column. A row matching two keys is counted twice,
// which only matters as "zero or not".
func countCollisions(ctx context.Context, tx *sql.Tx, driver db.Driver, col envColumn, sourceID, targetID string) (int, error) {
	table, column := db.QuoteIdent(driver, col.table), db.QuoteIdent(driver, col.column)
	total := 0
	// bounded by the unique keys declared on one column, a handful per table.
	for _, key := range col.keys {
		var n int
		err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` s WHERE s.`+column+` = ? AND EXISTS (SELECT 1 FROM `+
			table+` o WHERE `+collisionPredicate(driver, col, key)+`)`, sourceID, targetID).Scan(&n)
		if err != nil {
			return 0, fmt.Errorf("counting collisions: %w", err)
		}
		total += n
	}
	return total, nil
}

// collidingRows returns the row keys of the source rows that must stay behind.
// Nothing is written while it reads, so plain offset paging is stable here.
func collidingRows(ctx context.Context, tx *sql.Tx, driver db.Driver, col envColumn, rowKey []string, sourceID, targetID string) (map[string]bool, error) {
	table, column := db.QuoteIdent(driver, col.table), db.QuoteIdent(driver, col.column)
	selectList := "s." + quotedList(driver, rowKey, ", s.")
	out := map[string]bool{}
	for _, key := range col.keys {
		for offset := 0; ; offset += mergePageSize {
			page, err := pageTuples(ctx, tx, len(rowKey), `SELECT `+selectList+` FROM `+table+` s WHERE s.`+column+
				` = ? AND EXISTS (SELECT 1 FROM `+table+` o WHERE `+collisionPredicate(driver, col, key)+
				`) ORDER BY `+selectList+` LIMIT ? OFFSET ?`,
				sourceID, targetID, mergePageSize, offset)
			if err != nil {
				return nil, fmt.Errorf("reading collisions: %w", err)
			}
			for _, tuple := range page {
				out[tupleKey(tuple)] = true
			}
			if len(page) < mergePageSize {
				break
			}
		}
	}
	return out, nil
}

// tupleKey renders a row key as one map key; NUL cannot occur in the text the
// schema stores, so it marks SQL NULL without colliding with a real value.
func tupleKey(tuple []sql.NullString) string {
	parts := make([]string, len(tuple))
	for i, v := range tuple {
		if v.Valid {
			parts[i] = v.String
		} else {
			parts[i] = "\x00"
		}
	}
	return strings.Join(parts, "\x1f")
}

// tupleArgs binds a row key back into a statement, NULL as nil.
func tupleArgs(tuple []sql.NullString) []any {
	out := make([]any, len(tuple))
	for i, v := range tuple {
		if v.Valid {
			out[i] = v.String
		}
	}
	return out
}

// pageTuples runs a query of width columns and returns every row, closing the
// cursor before returning so the caller can write inside the same transaction.
func pageTuples(ctx context.Context, tx *sql.Tx, width int, query string, args ...any) ([][]sql.NullString, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { logCloseErr(rows.Close()) }()
	var out [][]sql.NullString
	for rows.Next() {
		tuple := make([]sql.NullString, width)
		dest := make([]any, width)
		for i := range tuple {
			dest[i] = &tuple[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		out = append(out, tuple)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// pageIDs runs a one-column query and returns its non-NULL values.
func pageIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	page, err := pageTuples(ctx, tx, 1, query, args...)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(page))
	for _, tuple := range page {
		if tuple[0].Valid {
			ids = append(ids, tuple[0].String)
		}
	}
	return ids, nil
}

// mergeSecrets re-points environment-scoped secrets and recomputes the scope
// digest each one is deduplicated by, returning how many it left behind.
//
// A name that already exists in the target's scope is left behind rather than
// overwritten: the target's secret is the one in use, and silently replacing
// its value during a merge is the kind of surprise that ends in a failed
// deploy nobody can explain.
func mergeSecrets(ctx context.Context, tx *sql.Tx, sourceID, targetID string) (int, error) {
	pending, err := pendingSecrets(ctx, tx, sourceID)
	if err != nil {
		return 0, err
	}

	keys := make([]string, len(pending))
	for i, row := range pending {
		var stack *string
		if row.stackName.Valid {
			value := row.stackName.String
			stack = &value
		}
		keys[i] = secrets.ScopeKey(row.name, &targetID, stack)
	}
	owners, err := scopeKeyOwners(ctx, tx, keys)
	if err != nil {
		return 0, err
	}

	skipped := 0
	// bounded by the secrets of the one environment being merged, a single operator-run pass.
	for i, row := range pending {
		key := keys[i]
		if owner, held := owners[key]; held && owner != row.id {
			skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE secrets
			   SET environment_id = ?, scope_key = ?
			 WHERE id = ?`, targetID, key, row.id); err != nil {
			return 0, fmt.Errorf("merging secret %s: %w", row.id, err)
		}
		owners[key] = row.id
	}
	return skipped, nil
}

// scopeKeyOwners maps each of keys that a secret already holds to that secret's
// id, in one query per chunk instead of one per row being merged.
func scopeKeyOwners(ctx context.Context, tx *sql.Tx, keys []string) (map[string]string, error) {
	owners := make(map[string]string, len(keys))
	for start := 0; start < len(keys); start += mergePageSize {
		chunk := keys[start:min(start+mergePageSize, len(keys))]
		args := make([]any, len(chunk))
		for i, key := range chunk {
			args[i] = key
		}
		query := scopeKeysSelect + placeholders(len(chunk)) + scopeKeysTail // SQL identifier: placeholders(n) emits only "?" markers
		if err := readScopeKeyOwners(ctx, tx, query, args, owners); err != nil {
			return nil, err
		}
	}
	return owners, nil
}

// placeholders returns n comma-separated bind markers for an IN (...) list.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// mergeEmbeddedIDs rewrites the environment ids that live inside a JSON array
// or a composite key rather than in a column of their own.
//
// These are invisible to the schema walk, and each one is a real feature: an
// alert scoped to the source environment would silently stop firing, and an
// alert's stored state would reset.
func mergeEmbeddedIDs(ctx context.Context, tx *sql.Tx, schema *mergeSchema, sourceID, targetID string, result *MergeResult) error {
	if schema.tables["alerts"] {
		if err := mergeJSONIDList(ctx, tx, schema.driver, "alerts", "id", "environment_ids", sourceID, targetID); err != nil {
			return err
		}
	}
	if schema.tables["api_key_tombstones"] {
		if err := mergeJSONIDList(ctx, tx, schema.driver, "api_key_tombstones", "id", "environments_snapshot", sourceID, targetID); err != nil {
			return err
		}
	}
	if schema.tables["alert_states"] {
		skipped, err := mergeAlertStates(ctx, tx, sourceID, targetID)
		if err != nil {
			return err
		}
		result.addDiscarded("alert_states", "state_key", skipped)
	}
	return nil
}

// mergeAlertStates re-keys alert_states rows, whose key is
// `alertID:environmentID:subjectID`, and returns how many it left behind.
//
// A row whose new key already exists for the target is dropped: both describe
// the same condition on what is now one environment, and the target's is current.
func mergeAlertStates(ctx context.Context, tx *sql.Tx, sourceID, targetID string) (int, error) {
	from, to := ":"+sourceID+":", ":"+targetID+":"
	skipped := 0
	for {
		// Re-keyed rows stop matching and skipped rows sort ahead of everything
		// unread, so the offset is the count skipped so far.
		keys, err := pageIDs(ctx, tx, `
			SELECT state_key FROM alert_states
			 WHERE state_key LIKE ?
			 ORDER BY state_key
			 LIMIT ? OFFSET ?`, "%"+from+"%", mergePageSize, skipped)
		if err != nil {
			return 0, fmt.Errorf("reading alert states: %w", err)
		}
		newKeys := make([]string, len(keys))
		for i, key := range keys {
			newKeys[i] = strings.ReplaceAll(key, from, to)
		}
		held, err := existingAlertStateKeys(ctx, tx, newKeys)
		if err != nil {
			return 0, err
		}
		// bounded by mergePageSize, the page just read.
		for i, key := range keys {
			newKey := newKeys[i]
			if held[newKey] {
				skipped++
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE alert_states SET state_key = ? WHERE state_key = ?`, newKey, key); err != nil {
				return 0, fmt.Errorf("merging alert state: %w", err)
			}
			held[newKey] = true
		}
		if len(keys) < mergePageSize {
			return skipped, nil
		}
	}
}

// existingAlertStateKeys reports which of keys already exist in alert_states,
// one query for the whole page. Callers add each key they write to the result,
// because two source keys can re-key to the same target key.
func existingAlertStateKeys(ctx context.Context, tx *sql.Tx, keys []string) (map[string]bool, error) {
	held := make(map[string]bool, len(keys))
	if len(keys) == 0 {
		return held, nil
	}
	args := make([]any, len(keys))
	for i, key := range keys {
		args[i] = key
	}
	found, err := pageIDs(ctx, tx, `SELECT state_key FROM alert_states WHERE state_key IN (`+placeholders(len(keys))+`) LIMIT 1000`, args...) // SQL identifier: placeholders(n) emits only "?" markers
	if err != nil {
		return nil, fmt.Errorf("checking alert states: %w", err)
	}
	for _, key := range found {
		held[key] = true
	}
	return held, nil
}

// mergeJSONIDList rewrites one environment id inside a JSON array column.
func mergeJSONIDList(ctx context.Context, tx *sql.Tx, driver db.Driver, table, idColumn, column, sourceID, targetID string) error {
	qTable, qID, qCol := db.QuoteIdent(driver, table), db.QuoteIdent(driver, idColumn), db.QuoteIdent(driver, column)
	pending, err := pendingJSONRows(ctx, tx, driver, table, idColumn, column, sourceID)
	if err != nil {
		return err
	}

	for _, row := range pending {
		if !row.value.Valid || strings.TrimSpace(row.value.String) == "" {
			continue
		}
		var ids []string
		if err := json.Unmarshal([]byte(row.value.String), &ids); err != nil {
			// Not a JSON array — the LIKE matched something else in the
			// column. Left alone rather than guessed at.
			continue
		}
		rewritten := make([]string, 0, len(ids))
		seen := map[string]bool{}
		changed := false
		for _, id := range ids {
			if id == sourceID {
				id = targetID
				changed = true
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			rewritten = append(rewritten, id)
		}
		if !changed {
			continue
		}
		encoded, err := json.Marshal(rewritten)
		if err != nil {
			return fmt.Errorf("encoding %s.%s: %w", table, column, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+qTable+` SET `+qCol+` = ? WHERE `+qID+` = ?`, // SQL identifier: qTable, qCol and qID come from db.QuoteIdent
			string(encoded), row.id); err != nil {
			return fmt.Errorf("writing %s.%s: %w", table, column, err)
		}
	}
	return nil
}

// envColumn is one discovered (table, column) pair holding an environment id,
// with the unique keys that include it and so decide what may collide.
type envColumn struct {
	table  string
	column string
	keys   [][]string
	hasID  bool
}

// mergeSchema is what a merge needs to know about the live schema, read once
// before any write.
type mergeSchema struct {
	driver  db.Driver
	tables  map[string]bool
	columns []envColumn
}

// discoverMergeSchema walks the live schema for columns holding an environment
// id and the unique keys that constrain each.
//
// The walk goes through core/db's catalog queries, so it runs on all four
// backends; only a driver those queries do not know is refused.
func discoverMergeSchema(ctx context.Context, database *sql.DB, driver db.Driver) (*mergeSchema, error) {
	switch driver {
	case db.DriverSQLite, db.DriverPostgres, db.DriverMySQL, db.DriverMSSQL:
	default:
		return nil, ErrMergeUnsupportedDriver
	}

	tables, err := db.ListTables(ctx, database, driver)
	if err != nil {
		return nil, err
	}

	schema := &mergeSchema{driver: driver, tables: map[string]bool{}}
	for _, table := range tables {
		schema.tables[strings.ToLower(table)] = true
		if strings.EqualFold(table, "environments") {
			continue
		}
		cols, err := db.TableColumns(ctx, database, driver, table)
		if err != nil {
			return nil, err
		}
		var envCols []string
		hasID := false
		for _, col := range cols {
			if envColumnNames[strings.ToLower(col)] {
				envCols = append(envCols, col)
			}
			if strings.EqualFold(col, "id") {
				hasID = true
			}
		}
		if len(envCols) == 0 {
			continue
		}
		keys, err := db.UniqueKeys(ctx, database, driver, table)
		if err != nil {
			return nil, err
		}
		for _, col := range envCols {
			schema.columns = append(schema.columns, envColumn{
				table: table, column: col, keys: keysContaining(keys, col), hasID: hasID,
			})
		}
	}
	schema.columns = sortColumns(schema.columns)
	return schema, nil
}

// keysContaining keeps the unique keys a change to column could make collide.
func keysContaining(keys [][]string, column string) [][]string {
	var out [][]string
	for _, key := range keys {
		for _, k := range key {
			if strings.EqualFold(k, column) {
				out = append(out, key)
				break
			}
		}
	}
	return out
}

// sortColumns puts the plan in a stable, readable order.
func sortColumns(in []envColumn) []envColumn {
	sort.Slice(in, func(i, j int) bool {
		if in[i].table == in[j].table {
			return in[i].column < in[j].column
		}
		return in[i].table < in[j].table
	})
	return in
}

// countRows counts what one column would move.
func (s *Service) countRows(ctx context.Context, driver db.Driver, col envColumn, envID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+db.QuoteIdent(driver, col.table)+
		` WHERE `+db.QuoteIdent(driver, col.column)+` = ?`, envID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting %s.%s: %w", col.table, col.column, err)
	}
	return count, nil
}

const (
	scopeKeysSelect = "SELECT scope_key, id FROM secrets WHERE scope_key IN ("
	scopeKeysTail   = ") LIMIT 1000"
)

// readScopeKeyOwners fills owners from one chunk query and closes its cursor, so the merge can write inside the same transaction afterwards.
func readScopeKeyOwners(ctx context.Context, tx *sql.Tx, query string, args []any, owners map[string]string) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("checking secret scope keys: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			return fmt.Errorf("scanning secret scope key: %w", err)
		}
		owners[key] = id
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading secret scope keys: %w", err)
	}
	return nil
}

type secretRow struct {
	id, name  string
	stackName sql.NullString
}

// pendingSecrets loads the source environment's secrets and closes the cursor before the merge rewrites them in the same transaction.
func pendingSecrets(ctx context.Context, tx *sql.Tx, sourceID string) ([]secretRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, stack_name
		  FROM secrets
		 WHERE environment_id = ?
		 LIMIT 1000`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("reading secrets to merge: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	var pending []secretRow
	for rows.Next() {
		var row secretRow
		if err := rows.Scan(&row.id, &row.name, &row.stackName); err != nil {
			return nil, fmt.Errorf("scanning secret to merge: %w", err)
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading secrets to merge: %w", err)
	}
	return pending, nil
}

type jsonRow struct {
	id    string
	value sql.NullString
}

// pendingJSONRows loads the rows whose JSON id-list column mentions sourceID and closes the cursor before the merge rewrites them.
func pendingJSONRows(ctx context.Context, tx *sql.Tx, driver db.Driver, table, idColumn, column, sourceID string) ([]jsonRow, error) {
	qTable, qID, qCol := db.QuoteIdent(driver, table), db.QuoteIdent(driver, idColumn), db.QuoteIdent(driver, column)
	rows, err := tx.QueryContext(ctx, `SELECT `+qID+`, `+qCol+` FROM `+qTable+` WHERE `+qCol+` LIKE ? LIMIT 1000`, // SQL identifier: qTable, qCol and qID come from db.QuoteIdent
		"%"+sourceID+"%")
	if err != nil {
		return nil, fmt.Errorf("reading %s.%s: %w", table, column, err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	var pending []jsonRow
	for rows.Next() {
		var row jsonRow
		if err := rows.Scan(&row.id, &row.value); err != nil {
			return nil, fmt.Errorf("scanning %s.%s: %w", table, column, err)
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading %s.%s: %w", table, column, err)
	}
	return pending, nil
}
