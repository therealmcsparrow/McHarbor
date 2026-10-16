// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// probeSet is one binding of the fixture's placeholder values (env-03, key-1,
// u1, …) to values that exist in a real target, so a probe measures rows that
// are there instead of an empty index range.
type probeSet struct {
	label string
	subst map[string]any
}

// fixtureValues are the literal probe arguments the synthetic fixture seeds;
// realProbeSets replaces each with a value picked from the target.
const (
	fixtureEnv          = "env-03"
	fixtureKey          = "key-1"
	fixtureUser         = "u1"
	fixtureWorkflow     = "workflows-7"
	fixtureContainerKey = "compose:s/3"
	fixtureContainerID  = "c003"
)

// realProbeSets returns two bindings for a real target: its busiest and its
// quietest non-empty environment. Skew is only visible when both ends of the
// distribution are probed, since a plan can be right for one and wrong for the other.
func realProbeSets(t *testing.T, tgt *target) []probeSet {
	t.Helper()
	envs := rankedValues(tgt, "container_events", "environment_id")
	if len(envs) == 0 {
		envs = rankedValues(tgt, "host_metrics", "environment_id")
	}
	if len(envs) == 0 {
		t.Log("target has no environment-scoped rows; probing with fixture values")
		return []probeSet{{label: "fixture values", subst: nil}}
	}
	picks := []struct {
		label string
		env   string
	}{{"busiest env", envs[0]}}
	if last := envs[len(envs)-1]; last != envs[0] {
		picks = append(picks, struct {
			label string
			env   string
		}{"quietest env", last})
	}

	common := map[string]any{
		fixtureKey:      firstOr(rankedValues(tgt, "api_key_usage_events", "api_key_id"), fixtureKey),
		fixtureUser:     firstOr(rankedValues(tgt, "in_app_notification_reads", "user_id"), fixtureUser),
		fixtureWorkflow: firstOr(rankedValues(tgt, "workflow_runs", "workflow_id"), fixtureWorkflow),
	}
	var sets []probeSet
	for _, p := range picks {
		subst := map[string]any{fixtureEnv: p.env}
		for k, v := range common {
			subst[k] = v
		}
		key, id := busiestContainer(tgt, p.env)
		if key != "" {
			subst[fixtureContainerKey] = key
			subst[fixtureContainerID] = id
		}
		sets = append(sets, probeSet{label: p.label + " " + p.env, subst: subst})
	}
	for _, s := range sets {
		t.Logf("probe binding %q: %v", s.label, s.subst)
	}
	return sets
}

// rankedValues lists the non-empty values of column in table, most frequent
// first, or nil when the table or column is absent.
func rankedValues(tgt *target, table, column string) []string {
	if !hasColumn(tgt, table, column) {
		return nil
	}
	c := coredb.QuoteIdent(tgt.driver, column)
	q := fmt.Sprintf("SELECT %s, COUNT(*) AS n FROM %s WHERE %s IS NOT NULL AND %s <> '' GROUP BY 1 ORDER BY n DESC",
		c, coredb.QuoteIdent(tgt.driver, table), c, c)
	rows, err := tgt.db.Query(q)
	if err != nil {
		return nil
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []string
	for rows.Next() {
		var v string
		var n int64
		if rows.Scan(&v, &n) == nil {
			out = append(out, v)
		}
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}

// busiestContainer returns the container_key and container_id with the most
// samples on env, so the history probes read a real series.
func busiestContainer(tgt *target, env string) (string, string) {
	if !hasColumn(tgt, "container_metrics", "container_key") {
		return "", ""
	}
	q := rebind(tgt.driver, `SELECT container_key, container_id, COUNT(*) AS n FROM container_metrics WHERE environment_id = ? GROUP BY 1, 2 ORDER BY n DESC LIMIT 1`)
	var key, id string
	var n int64
	if err := tgt.db.QueryRow(q, env).Scan(&key, &id, &n); err != nil {
		return "", ""
	}
	return key, id
}

func firstOr(vals []string, fallback string) string {
	if len(vals) == 0 {
		return fallback
	}
	return vals[0]
}

// hasColumn guards the shape queries: SQLite reads an unknown double-quoted
// identifier as a string literal, so a missing column would otherwise be
// reported as one giant "environment" instead of being skipped.
func hasColumn(tgt *target, table, column string) bool {
	if !targetTableCache[table] {
		return false
	}
	cols, err := coredb.TableColumns(context.Background(), tgt.db, tgt.driver, table)
	if err != nil {
		return false
	}
	for _, c := range cols {
		if strings.EqualFold(c, column) {
			return true
		}
	}
	return false
}

// bindArgs applies a probe set's substitutions to one probe's arguments.
func bindArgs(args []any, subst map[string]any) []any {
	if len(subst) == 0 {
		return args
	}
	out := make([]any, len(args))
	for i, a := range args {
		if s, ok := a.(string); ok {
			if v, ok := subst[s]; ok {
				out[i] = v
				continue
			}
		}
		out[i] = a
	}
	return out
}

// reportTableSizes logs the largest tables by row count and whether planner
// statistics exist, because a plan read without statistics is not the plan
// the running instance gets.
func reportTableSizes(t *testing.T, tgt *target) {
	t.Helper()
	ctx := context.Background()
	names, err := coredb.ListTables(ctx, tgt.db, tgt.driver)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	type sized struct {
		name string
		rows int64
	}
	var all []sized
	for _, n := range names {
		c, err := coredb.RowCount(ctx, tgt.db, tgt.driver, n)
		if err != nil {
			continue
		}
		all = append(all, sized{n, c})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].rows > all[j].rows })
	var lines []string
	for i, s := range all {
		if i == 20 {
			break
		}
		lines = append(lines, fmt.Sprintf("%-40s %10d", s.name, s.rows))
	}
	t.Log("largest tables\n" + strings.Join(lines, "\n"))
	t.Log("planner statistics: " + statsState(tgt))
}

// statsState reports whether the engine has statistics to plan with: SQLite's
// sqlite_stat1 row count, or how many user tables Postgres has analyzed.
func statsState(tgt *target) string {
	if tgt.driver == coredb.DriverPostgres {
		var analyzed, total int
		err := tgt.db.QueryRow(`SELECT count(*) FILTER (WHERE last_analyze IS NOT NULL OR last_autoanalyze IS NOT NULL), count(*) FROM pg_stat_user_tables`).Scan(&analyzed, &total)
		if err != nil {
			return "unknown: " + err.Error()
		}
		return fmt.Sprintf("%d of %d tables analyzed", analyzed, total)
	}
	var n sql.NullInt64
	if err := tgt.db.QueryRow(`SELECT count(*) FROM sqlite_stat1`).Scan(&n); err != nil {
		return "no sqlite_stat1 (ANALYZE has never run on this database)"
	}
	return fmt.Sprintf("sqlite_stat1 has %d rows", n.Int64)
}
