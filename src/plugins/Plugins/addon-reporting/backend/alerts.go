// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The conditions this plugin contributes to the host's alert engine.
const (
	alertScheduledReportFailed  = "scheduled_report_failed"
	alertScheduledReportOverdue = "scheduled_report_overdue"
	alertReportRunFailed        = "report_run_failed"
)

// alertPollInterval matches the platform source's cadence. A schedule
// fires on a cron measured in hours; nothing here changes on a
// 30-second timescale.
const alertPollInterval = 5 * time.Minute

// reportingAlertSpec declares what this plugin can alert on.
//
// A report nobody looks at is the point of a schedule — you find out it
// stopped working when you need the report, which is exactly too late.
// These three conditions are the ones that turn that into a message at
// the time it breaks.
//
// TargetNone, like every host-wide condition: the subject is a schedule
// or a report, not a container in one environment. Subjects are named
// after the schedule or report, so a rule's target pattern can single
// one out.
func reportingAlertSpec() plugin.AlertSourceAdapterSpec {
	return plugin.AlertSourceAdapterSpec{
		SourceType:  "reporting",
		DisplayName: "Reporting",
		Slug:        "addon-reporting",
		Icon:        "reporting.svg",
		Description: "Scheduled and on-demand report generation.",
		Conditions: []plugin.AlertConditionDecl{
			{
				Key:          alertScheduledReportFailed,
				DisplayName:  "Scheduled report failed",
				Description:  "A schedule's most recent run failed, or delivered to only some of its targets. Resolves when a later run succeeds in full.",
				Kind:         plugin.AlertKindEvent,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertScheduledReportOverdue,
				DisplayName:  "Scheduled report overdue",
				Description:  "An enabled schedule has not produced a report within the given period — the run is failing to start at all, rather than failing.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertReportRunFailed,
				DisplayName:  "Report generation failed",
				Description:  "The most recent generation of a report failed, whether it was scheduled or run by hand.",
				Kind:         plugin.AlertKindEvent,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
		},
	}
}

// alertSource is the runtime half of the contract.
//
// Every query reads this plugin's own `plugin_reporting_*` tables
// through the host's scoped DB handle, which is the only thing a
// plugin may read. The host owns the thresholds, the schedule, the
// firing state and the delivery; this answers "is it true, and for
// which schedules?" and nothing else.
type alertSource struct {
	plugin *ReportingPlugin
}

// Evaluate implements plugin.AlertSourceAdapterClient.
func (a *alertSource) Evaluate(ctx context.Context, req plugin.AlertEvaluateRequest) ([]plugin.AlertObservation, error) {
	if a.plugin == nil || a.plugin.host.DB == nil {
		return nil, fmt.Errorf("reporting: plugin not initialised")
	}

	switch req.ConditionKey {
	case alertScheduledReportFailed:
		return a.scheduledReportFailed(ctx)
	case alertScheduledReportOverdue:
		return a.scheduledReportOverdue(ctx)
	case alertReportRunFailed:
		return a.reportRunFailed(ctx)
	default:
		return nil, fmt.Errorf("reporting: unknown condition %q", req.ConditionKey)
	}
}

// scheduledReportFailed reports the latest run of each schedule.
//
// `plugin_reporting_schedule_runs` keeps one row per schedule — the
// most recent run — so the subject is naturally the schedule. A failed
// run fires once and the next full success resolves it, rather than
// producing a fresh alert for a schedule that has been broken all week.
//
// 'partial' counts as a failure: a report that reached two of three
// targets did not do what the operator asked for, and the one target
// it missed is usually the one that mattered.
func (a *alertSource) scheduledReportFailed(ctx context.Context) ([]plugin.AlertObservation, error) {
	rows, err := a.plugin.host.DB.Query(ctx, `
		SELECT s.id, s.name, COALESCE(r.status, ''), COALESCE(r.error, '')
		  FROM plugin_reporting_schedules s
		  LEFT JOIN plugin_reporting_schedule_runs r ON r.schedule_id = s.id
		 WHERE s.enabled = 1
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reporting: listing schedule runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]plugin.AlertObservation, 0, 16)
	for rows.Next() {
		var id, name, status, errCode string
		if err := rows.Scan(&id, &name, &status, &errCode); err != nil {
			return nil, fmt.Errorf("reporting: scanning schedule run: %w", err)
		}

		observation := plugin.AlertObservation{
			SubjectID:   id,
			SubjectName: name,
			Active:      status == "failed" || status == "partial",
			ValueLabel:  status,
			Context:     map[string]string{"schedule": name, "status": status},
		}
		// error is already an i18n code rather than a raw error string —
		// the schedule table stores it that way precisely so it can be
		// shown without leaking a path or a hostname — so it is safe to
		// put in a message.
		if errCode != "" {
			observation.Context["error"] = errCode
		}
		out = append(out, observation)
	}

	return out, rows.Err()
}

// scheduledReportOverdue reports how long each enabled schedule has gone
// without producing anything.
//
// Active is always true and Since carries the last run, because the host
// times the duration: "overdue by more than 48h" is then the operator's
// threshold rather than something baked into this query, and a fresh run
// moves Since forward and resolves the alert on the next evaluation.
//
// This catches the failure the condition above cannot: a scheduler that
// never fires writes no run row at all, so there is no failed status to
// find — only a silence.
func (a *alertSource) scheduledReportOverdue(ctx context.Context) ([]plugin.AlertObservation, error) {
	rows, err := a.plugin.host.DB.Query(ctx, `
		SELECT id, name, COALESCE(last_run_at, ''), created_at
		  FROM plugin_reporting_schedules
		 WHERE enabled = 1
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reporting: listing schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]plugin.AlertObservation, 0, 16)
	for rows.Next() {
		var id, name, lastRun, createdAt string
		if err := rows.Scan(&id, &name, &lastRun, &createdAt); err != nil {
			return nil, fmt.Errorf("reporting: scanning schedule: %w", err)
		}

		// A schedule that has never run counts from when it was
		// created. Counting from zero time would fire the instant a
		// schedule is saved, before its first cron slot came round.
		reference := lastRun
		if strings.TrimSpace(reference) == "" {
			reference = createdAt
		}

		observation := plugin.AlertObservation{
			SubjectID:   id,
			SubjectName: name,
			Active:      true,
			Context:     map[string]string{"schedule": name, "last_run": lastRun},
		}
		if since, parseErr := parseAlertTime(reference); parseErr == nil {
			observation.Since = since
		}
		out = append(out, observation)
	}

	return out, rows.Err()
}

// reportRunFailed reports the latest generation attempt per report.
//
// Covers the on-demand path as well as the scheduled one: a report an
// operator asked for by hand and never came back to look at fails just
// as silently.
func (a *alertSource) reportRunFailed(ctx context.Context) ([]plugin.AlertObservation, error) {
	rows, err := a.plugin.host.DB.Query(ctx, `
		SELECT r.report_id, COALESCE(p.title, r.report_id), r.ok, COALESCE(r.note, '')
		  FROM plugin_reporting_runs r
		  JOIN (SELECT report_id, MAX(created_at) AS latest
		          FROM plugin_reporting_runs
		         GROUP BY report_id) newest
		    ON newest.report_id = r.report_id
		   AND newest.latest = r.created_at
		  LEFT JOIN plugin_reporting_reports p ON p.id = r.report_id
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reporting: listing report runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]plugin.AlertObservation, 0, 32)
	for rows.Next() {
		var reportID, title, note string
		var ok int
		if err := rows.Scan(&reportID, &title, &ok, &note); err != nil {
			return nil, fmt.Errorf("reporting: scanning report run: %w", err)
		}

		out = append(out, plugin.AlertObservation{
			SubjectID:   reportID,
			SubjectName: title,
			Active:      ok == 0,
			Context:     map[string]string{"report": title},
		})
	}

	return out, rows.Err()
}

// parseAlertTime reads the two timestamp shapes this database holds:
// Go's RFC3339 (what this plugin writes) and SQLite's CURRENT_TIMESTAMP
// (what a column default writes).
func parseAlertTime(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", trimmed)
}
