// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// schedulePlan is one seeded schedule shape. Only start, stop and restart are
// ever enabled: each is a Docker call on a simulated environment and nothing
// else, so running it can never reach outside the demo.
type schedulePlan struct {
	name, cron, action, description string
	enabled                         bool
}

var schedulePlans = []schedulePlan{
	{"Nightly restart", "0 3 * * *", "restart", "Clears the slow memory growth in the worker", true},
	{"Business-hours start", "30 7 * * 1-5", "start", "Brings the reporting container up for the working day", true},
	{"Evening stop", "0 20 * * 1-5", "stop", "Stops the reporting container outside office hours", true},
	{"Weekend stop", "0 22 * * 5", "stop", "Parks the batch runner over the weekend", false},
	{"Six-hourly restart", "0 */6 * * *", "restart", "Works around a connection-pool leak until 2.32 ships", true},
	{"Monthly cache flush", "15 4 1 * *", "exec", "redis-cli FLUSHDB on the first of the month", false},
	{"Log rotation", "45 0 * * *", "exec", "logrotate -f /etc/logrotate.conf", false},
	{"Sunday maintenance", "0 5 * * 0", "restart", "Weekly restart during the maintenance window", true},
}

// seedHistorySchedules writes ~60 schedules across the demo environments with
// their next run computed, and a short execution history.
func seedHistorySchedules(ctx context.Context, conn *sql.DB, hc *historyCtx) error {
	ok, err := tableExists(ctx, conn, "schedules")
	if err != nil || !ok {
		return err
	}
	execOK, err := tableExists(ctx, conn, "schedule_executions")
	if err != nil {
		return err
	}
	var scheduleRows, execRows [][]any
	n := 0
	for hi := range hc.hosts {
		h := &hc.hosts[hi]
		for k := 0; k < 4 && n < 60; k++ {
			c, ok := hc.pickContainer(h)
			if !ok {
				break
			}
			plan := schedulePlans[(hi*4+k)%len(schedulePlans)]
			name := plan.name + " — " + c.Name
			id := stableID("schedule", h.host.Key+"/"+name)
			next, err := cronNext(plan.cron, hc.now)
			if err != nil {
				return err
			}
			created := hc.anyTime(hc.window)
			lastRun := ""
			for r := 0; r < 3+hc.rng.IntN(4); r++ {
				at := hc.anyTime(hc.now.Sub(created))
				status, output := "success", fmt.Sprintf("%s %s: ok", plan.action, c.Name)
				if hc.rng.Float64() < 0.12 {
					status = "failure"
					output = pick(hc, []string{
						"Error response from daemon: container " + c.Name + " is marked as protected",
						"agent for " + h.host.Name + " did not answer within 30s",
						"exec failed: executable file not found in $PATH",
					})
				}
				execRows = append(execRows, []any{stableID("schedule_exec", fmt.Sprintf("%s/%d", id, r)), id, status, output, 200 + hc.rng.IntN(9000), rfc(at), rfc(at), rfc(at)})
				if s := rfc(at); s > lastRun {
					lastRun = s
				}
			}
			scheduleRows = append(scheduleRows, []any{id, name, plan.description, plan.cron, plan.action, c.Name, h.envID,
				plan.enabled && isDockerOnlyAction(plan.action), nullIfBlank(lastRun), rfc(next), rfc(created), rfc(created)})
			n++
		}
	}
	return withTx(ctx, conn, func(tx *sql.Tx) error {
		if err := insertAll(ctx, tx, `
			INSERT OR IGNORE INTO schedules (id, name, description, cron, action, target, env_id, enabled, last_run_at, next_run_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, scheduleRows); err != nil {
			return err
		}
		if !execOK {
			return nil
		}
		return insertAll(ctx, tx, `
			INSERT OR IGNORE INTO schedule_executions (id, schedule_id, status, output, duration, executed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, execRows)
	})
}

// isDockerOnlyAction reports whether a schedule action is a plain lifecycle
// call on a container — the only kind a seeded schedule may leave enabled.
func isDockerOnlyAction(action string) bool {
	return action == "start" || action == "stop" || action == "restart"
}

// cronNext returns the first minute after from that spec matches. It handles
// the five-field subset the seeded plans use: *, */n, a-b, lists and numbers.
func cronNext(spec string, from time.Time) (time.Time, error) {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron %q: want 5 fields", spec)
	}
	limits := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	var sets [5]map[int]bool
	for i, f := range fields {
		set, err := cronField(f, limits[i][0], limits[i][1])
		if err != nil {
			return time.Time{}, fmt.Errorf("cron %q: %w", spec, err)
		}
		sets[i] = set
	}
	t := from.Truncate(time.Minute).Add(time.Minute)
	for end := t.Add(366 * 24 * time.Hour); t.Before(end); t = t.Add(time.Minute) {
		if sets[0][t.Minute()] && sets[1][t.Hour()] && sets[2][t.Day()] && sets[3][int(t.Month())] && sets[4][int(t.Weekday())] {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cron %q: no run within a year", spec)
}

func cronField(f string, lo, hi int) (map[int]bool, error) {
	set := make(map[int]bool)
	for _, part := range strings.Split(f, ",") {
		step := 1
		if base, s, ok := strings.Cut(part, "/"); ok {
			n, err := strconv.Atoi(s)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("bad step %q", part)
			}
			part, step = base, n
		}
		from, to := lo, hi
		switch {
		case part == "*":
		case strings.Contains(part, "-"):
			a, b, _ := strings.Cut(part, "-")
			x, err1 := strconv.Atoi(a)
			y, err2 := strconv.Atoi(b)
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("bad range %q", part)
			}
			from, to = x, y
		default:
			x, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("bad value %q", part)
			}
			from, to = x, x
		}
		if from < lo || to > hi || from > to {
			return nil, fmt.Errorf("value out of range in %q", f)
		}
		for v := from; v <= to; v += step {
			set[v] = true
		}
	}
	return set, nil
}
