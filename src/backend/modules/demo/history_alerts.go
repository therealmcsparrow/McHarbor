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

// alertTemplate is one seeded alert rule. None of them is given a destination
// and none sends in-app, so a rule that fires records history and nothing else.
// Image-update rules are left out: evaluating one asks a registry.
type alertTemplate struct {
	name, severity, source, condition string
	operator                          string
	threshold                         float64
	unit                              string
	forSeconds                        int
	windowSeconds                     int
	scope                             int
}

var alertTemplates = []alertTemplate{
	{"Container CPU above 90%", "warning", "container", "cpu", ">", 90, "%", 300, 0, 0},
	{"Container memory above 85%", "warning", "container", "memory", ">", 85, "%", 300, 0, 0},
	{"Production container down", "critical", "container", "down", "", 0, "", 120, 0, 3},
	{"Health check failing", "critical", "container", "unhealthy", "", 0, "", 60, 0, 0},
	{"Restart loop", "critical", "container", "restart_loop", ">=", 3, "restarts", 0, 900, 0},
	{"OOM killed", "critical", "container", "oom_killed", "", 0, "", 0, 0, 0},
	{"Exited with an error", "warning", "container", "exit_nonzero", "", 0, "", 0, 0, 0},
	{"Host CPU above 85%", "warning", "host", "cpu", ">", 85, "%", 600, 0, 0},
	{"Host memory above 90%", "critical", "host", "memory", ">", 90, "%", 600, 0, 0},
	{"Load average high", "warning", "host", "load5", ">", 24, "", 900, 0, 0},
	{"Docker disk above 80%", "warning", "host", "docker_disk", ">", 80, "%", 0, 0, 0},
	{"Environment unreachable", "critical", "host", "unreachable", "", 0, "", 180, 0, 0},
	{"Edge host unreachable", "critical", "host", "unreachable", "", 0, "", 600, 0, 4},
	{"Staging CPU above 95%", "info", "container", "cpu", ">", 95, "%", 900, 0, 2},
	{"Database memory above 80%", "warning", "container", "memory", ">", 80, "%", 600, 0, 3},
	{"Kafka restart loop", "critical", "container", "restart_loop", ">=", 2, "restarts", 0, 600, 3},
	{"Store host memory pressure", "warning", "host", "memory", ">", 85, "%", 900, 0, 4},
	{"Production host load", "warning", "host", "load15", ">", 32, "", 1800, 0, 3},
	{"Legacy disk check", "info", "host", "docker_disk", ">", 70, "%", 0, 0, 0},
	{"Batch runner exit code", "info", "container", "exit_nonzero", "", 0, "", 0, 0, 1},
}

// scopeEnvIDs narrows a rule to a slice of the estate; scope 0 means every
// environment, the way the alerts page stores it (empty environment_ids).
func (hc *historyCtx) scopeEnvIDs(scope int) []string {
	if scope == 0 {
		return nil
	}
	var ids []string
	for i, h := range hc.hosts {
		if i%scope == 0 {
			ids = append(ids, h.envID)
		}
	}
	return ids
}

// seedHistoryAlerts writes ~20 alert rules with no destinations, and a
// history of firing and resolved events against the demo environments.
func seedHistoryAlerts(ctx context.Context, conn *sql.DB, hc *historyCtx) error {
	ok, err := tableExists(ctx, conn, "alerts")
	if err != nil || !ok {
		return err
	}
	eventsOK, err := tableExists(ctx, conn, "alert_events")
	if err != nil {
		return err
	}
	var ruleRows, eventRows [][]any
	for i, t := range alertTemplates {
		id := stableID("alert", t.name)
		cond, err := json.Marshal(map[string]any{"operator": t.operator, "threshold": t.threshold, "unit": t.unit, "forSeconds": t.forSeconds, "windowSeconds": t.windowSeconds})
		if err != nil {
			return fmt.Errorf("encoding alert condition: %w", err)
		}
		envs := ""
		if ids := hc.scopeEnvIDs(t.scope); len(ids) > 0 {
			raw, err := json.Marshal(ids)
			if err != nil {
				return fmt.Errorf("encoding alert scope: %w", err)
			}
			envs = string(raw)
		}
		created := hc.anyTime(hc.window)
		enabled := i%5 != 4
		ruleRows = append(ruleRows, []any{id, t.name, t.severity, t.condition, t.source, t.condition, string(cond), envs, 300, enabled, rfc(created), rfc(created)})
		for k := 0; k < 6+hc.rng.IntN(20); k++ {
			eventRows = append(eventRows, hc.alertEvents(id, t, k, created)...)
		}
	}
	return withTx(ctx, conn, func(tx *sql.Tx) error {
		if err := insertAll(ctx, tx, `
			INSERT OR IGNORE INTO alerts (id, name, severity, type, condition, target, channel_id, source_type, condition_key,
			                              condition_config, environment_ids, cooldown_seconds, notify_on_recovery,
			                              message_template, send_in_app, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, '', '', NULL, ?, ?, ?, ?, ?, 1, '', 0, ?, ?, ?)`, ruleRows); err != nil {
			return err
		}
		if !eventsOK {
			return nil
		}
		return insertAll(ctx, tx, `
			INSERT OR IGNORE INTO alert_events (id, alert_id, alert_name, source_type, condition_key, environment_id, environment_name,
			                                    subject_id, subject_name, state, severity, value, value_label, message, deliveries, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', ?)`, eventRows)
	})
}

// alertEvents returns one firing event and, usually, the resolution after it.
func (hc *historyCtx) alertEvents(alertID string, t alertTemplate, k int, created time.Time) [][]any {
	h := hc.pickHost()
	subjectID, subjectName := h.envID, h.host.Name
	if t.source == "container" {
		if c, ok := hc.pickContainer(h); ok {
			subjectID, subjectName = c.ID, c.Name
		}
	}
	fired := hc.anyTime(hc.now.Sub(created))
	value := t.threshold + 1 + hc.rng.Float64()*8
	label := fmt.Sprintf("%.1f%s", value, t.unit)
	if t.threshold == 0 {
		value, label = 1, ""
	}
	message := fmt.Sprintf("%s on %s", t.name, subjectName)
	base := fmt.Sprintf("%s/%d", alertID, k)
	out := [][]any{{stableID("alert_event", base+"/f"), alertID, t.name, t.source, t.condition, h.envID, h.host.Name,
		subjectID, subjectName, "firing", t.severity, value, label, message, rfc(fired)}}
	if hc.rng.Float64() < 0.9 {
		resolved := fired.Add(time.Duration(2+hc.rng.IntN(180)) * time.Minute)
		if resolved.Before(hc.now) {
			out = append(out, []any{stableID("alert_event", base+"/r"), alertID, t.name, t.source, t.condition, h.envID, h.host.Name,
				subjectID, subjectName, "resolved", t.severity, 0.0, "", message + " recovered", rfc(resolved)})
		}
	}
	return out
}
