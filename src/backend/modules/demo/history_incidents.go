// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// incidentPlan is one seeded incident. The incidents module knows only open
// and resolved; "acknowledged" is not a state it stores.
type incidentPlan struct {
	signal, summary, fix string
	open                 bool
}

var incidentPlans = []incidentPlan{
	{"crash_loop", "Crash loop after a config push; the new config referenced a missing secret.", "rolled back the stack", false},
	{"oom", "Killed out of memory during the nightly import; the memory limit was raised to 2 GiB.", "raised memory limit", false},
	{"unhealthy", "Health check failed while the database was failing over.", "restarted container", false},
	{"exit_nonzero", "Exited with code 1 on start: the upstream API returned 503.", "restarted container", false},
	{"crash_loop", "Crash loop on the new image tag; pinned back to the previous release.", "pinned image tag", false},
	{"alert", "CPU above 90% for 12 minutes during a traffic spike.", "scaled worker replicas", false},
	{"unhealthy", "TLS certificate expired on the internal endpoint.", "renewed certificate", false},
	{"oom", "Memory leak in the report generator.", "restarted container", false},
	{"exit_nonzero", "Migration job failed on a lock timeout.", "re-ran migration", false},
	{"alert", "Disk on the Docker root above 80%; old images pruned.", "pruned images", false},
	{"crash_loop", "", "", true},
	{"unhealthy", "", "", true},
	{"oom", "", "", true},
	{"exit_nonzero", "", "", true},
	{"alert", "", "", true},
}

// seedHistoryIncidents writes ~15 incidents on demo containers: the resolved
// ones with a timeline and summary, the open ones still collecting signals.
// Narration stays off: nothing here asks a model provider for anything.
func seedHistoryIncidents(ctx context.Context, conn *sql.DB, hc *historyCtx) error {
	ok, err := tableExists(ctx, conn, "incidents")
	if err != nil || !ok {
		return err
	}
	var rows [][]any
	for i, p := range incidentPlans {
		h := &hc.hosts[(i*5+1)%len(hc.hosts)]
		c, ok := hc.pickContainer(h)
		if !ok {
			continue
		}
		var first, last time.Time
		if p.open {
			first = hc.now.Add(-time.Duration(5+hc.rng.IntN(40)) * time.Minute)
			last = hc.now.Add(-time.Duration(hc.rng.IntN(4)) * time.Minute)
			if last.Before(first) {
				last = first
			}
		} else {
			first = hc.anyTime(hc.window - 24*time.Hour)
			last = first.Add(time.Duration(3+hc.rng.IntN(90)) * time.Minute)
		}
		first, last = first.Truncate(time.Second), last.Truncate(time.Second)
		count := 2 + hc.rng.IntN(14)
		status, timeline, summary, resolvedAt := "open", "", "", any(nil)
		if !p.open {
			resolved := last.Add(30 * time.Minute)
			who := hc.pickActor().username
			items := []map[string]string{
				{"at": rfc(first), "kind": "symptom", "detail": p.signal},
				{"at": rfc(first.Add(40 * time.Second)), "kind": "event", "detail": "die (exit 1)"},
				{"at": rfc(first.Add(2 * time.Minute)), "kind": "alert", "detail": "Health check failing: firing"},
				{"at": rfc(last.Add(-time.Minute)), "kind": "action", "who": who, "detail": "restart " + c.Name},
				{"at": rfc(last), "kind": "change", "who": who, "detail": p.fix},
				{"at": rfc(resolved), "kind": "resolved", "detail": "after " + resolved.Sub(first).Round(time.Minute).String()},
			}
			raw, err := json.Marshal(items)
			if err != nil {
				return fmt.Errorf("encoding incident timeline: %w", err)
			}
			status, timeline, summary, resolvedAt = "resolved", string(raw), p.summary, rfc(resolved)
		}
		rows = append(rows, []any{stableID("incident", fmt.Sprint(i)), h.envID, c.Name, stackOf(c), p.signal, status, count,
			rfc(first), rfc(last), timeline, summary, resolvedAt, rfc(first), rfc(last)})
	}
	return withTx(ctx, conn, func(tx *sql.Tx) error {
		return insertAll(ctx, tx, `
			INSERT OR IGNORE INTO incidents (id, env_id, subject_type, subject_name, stack_name, signal_kind, status, signal_count,
			                                 first_signal_at, last_signal_at, timeline, summary, resolved_at, narrative_status,
			                                 created_at, updated_at)
			VALUES (?, ?, 'container', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'none', ?, ?)`, rows)
	})
}
