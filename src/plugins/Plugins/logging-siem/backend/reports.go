// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// reportDelivery is the one report type this plugin contributes. It is
// prefixed with the slug because the Reporting plugin's library and
// schedules key on it globally.
const reportDelivery = "logging-siem-delivery"

// subjectForwarder narrows the report to one forwarder instance.
const subjectForwarder = "siem-forwarder"

const (
	reportDefaultDays  = 7
	reportBuildTimeout = time.Minute
)

// errReportForbidden is returned when the requester may not read the
// forwarder history. It names no forwarder: an error is not the place to
// tell someone what they were refused.
var errReportForbidden = errors.New("logging-siem: caller lacks audit.view and settings.manage")

// siemReportSpec declares the delivery report. It is not environment
// scoped: forwarders are global, and the host's environment picker could
// not change the answer.
func siemReportSpec() plugin.ReportProviderSpec {
	return plugin.ReportProviderSpec{
		Slug:        "logging-siem",
		DisplayName: "SIEM",
		Types: []plugin.ReportTypeDecl{{
			Key:            reportDelivery,
			Category:       "operations",
			Levels:         []string{"strategic", "tactical", "operational"},
			DefaultDays:    reportDefaultDays,
			TitleEN:        "SIEM log forwarding delivery",
			SubtitleEN:     "Delivered and failed audit events per forwarder, gaps and recurring errors",
			LabelKey:       "logging-siem:reports.delivery.label",
			DescriptionKey: "logging-siem:reports.delivery.description",
			Subjects:       []string{subjectForwarder},
		}},
	}
}

// reportProvider builds the delivery report from this plugin's own
// delivery ledger. It holds the plugin rather than the client because
// it is registered before Init hands the plugin its host context.
type reportProvider struct {
	plugin *LoggingSIEMPlugin
}

var _ plugin.ReportProviderClient = (*reportProvider)(nil)

// Subjects lists the SIEM forwarders a report can be narrowed to, and
// nothing for a caller who may not read forwarder configuration.
func (r *reportProvider) Subjects(ctx context.Context, kind string, req plugin.ReportRequest) ([]plugin.ReportSubjectOption, error) {
	if kind != subjectForwarder || !r.mayRead(req) || r.plugin.client.host.DB == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()
	forwarders, err := r.plugin.client.reportForwarders(ctx, time.Now(), "")
	if err != nil {
		return nil, err
	}
	out := make([]plugin.ReportSubjectOption, 0, len(forwarders))
	for _, f := range forwarders {
		hint := "disabled"
		if f.Enabled {
			hint = "last delivery " + formatTime(f.LastSuccess)
		}
		out = append(out, plugin.ReportSubjectOption{ID: f.ID, Name: forwarderName(f.ID, f.Name, true), Hint: hint})
	}
	return out, nil
}

// Build reads the delivery ledger for the requested period and composes
// the report. A failed read is recorded in the sources annex and the
// document says what it could not read rather than showing zeros.
func (r *reportProvider) Build(ctx context.Context, req plugin.ReportRequest) (plugin.ReportResult, error) {
	if req.TypeKey != reportDelivery {
		return plugin.ReportResult{}, fmt.Errorf("logging-siem: unknown report type %q", req.TypeKey)
	}
	// Reporting gates generation on its own key. The history and the
	// forwarder names are behind audit.view plus settings.manage in the
	// Logging tab, so a report must not be a way around that pair.
	if !r.mayRead(req) {
		return plugin.ReportResult{}, errReportForbidden
	}
	if r.plugin.client.host.DB == nil {
		return plugin.ReportResult{}, errors.New("logging-siem: the plugin is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()

	start, end := req.PeriodStart, req.PeriodEnd
	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() || !start.Before(end) {
		start = end.AddDate(0, 0, -reportDefaultDays)
	}
	forwarderID := ""
	if req.SubjectKind == subjectForwarder {
		forwarderID = strings.TrimSpace(req.SubjectID)
	}

	var sources []plugin.ReportSourceStatus
	var notes []string
	client := r.plugin.client

	began := time.Now()
	forwarders, err := client.reportForwarders(ctx, end, forwarderID)
	sources = append(sources, r.source("siem_forwarders", "SIEM forwarder configuration", began, err))
	if err != nil {
		notes = append(notes, "The forwarder list could not be read, so forwarders are named by id and none is judged silent.")
	}

	began = time.Now()
	totals, err := client.reportTotals(ctx, start, end, forwarderID)
	sources = append(sources, r.source("siem_delivery_totals", "SIEM delivery history", began, err))
	if err != nil {
		return plugin.ReportResult{
			Sections: []plugin.ReportSection{{
				Level: "summary",
				Title: "Summary",
				Blocks: []plugin.ReportBlock{{
					Kind:  "callout",
					Tone:  "bad",
					Title: "Delivery history unavailable",
					Text:  "The SIEM delivery history could not be read, so no delivery figure is shown.",
				}},
			}},
			Sources: sources,
		}, nil
	}

	began = time.Now()
	gaps, err := client.reportGaps(ctx, start, end, forwarderID)
	sources = append(sources, r.source("siem_delivery_gaps", "SIEM delivery gaps", began, err))
	if err != nil {
		notes = append(notes, "Gaps between successful deliveries could not be computed; longest gaps only reflect the edges of the period.")
	}

	began = time.Now()
	errs, err := client.reportErrors(ctx, start, end, forwarderID)
	sources = append(sources, r.source("siem_delivery_errors", "SIEM delivery failure reasons", began, err))
	if err != nil {
		notes = append(notes, "Failure reasons could not be read, so no recurring error is reported.")
	}

	th := defaultThresholds()
	analysis := analyseDelivery(deliveryInput{
		PeriodStart: start,
		PeriodEnd:   end,
		Forwarders:  forwarders,
		Totals:      totals,
		Gaps:        gaps,
		Errors:      errs,
		Thresholds:  th,
	})
	notes = append(notes, fmt.Sprintf(
		"Thresholds: a forwarder below %s success is a finding (below %s is high), as is one with no successful delivery for over %s, and any failure reason seen %d or more times.",
		formatRate(th.WarnRate), formatRate(th.BadRate), formatDuration(th.Silent), th.RecurringErrors))

	result := plugin.ReportResult{
		Sections: buildDeliverySections(analysis, th, start, end),
		Sources:  sources,
		Notes:    notes,
	}
	if forwarderID != "" && req.SubjectName != "" {
		result.Subtitle = "Forwarder: " + req.SubjectName
	}
	return result, nil
}

// source records one read for the Methodology annex and logs the real
// error server-side, since the annex line must not carry a path or host.
func (r *reportProvider) source(key, label string, began time.Time, err error) plugin.ReportSourceStatus {
	s := plugin.ReportSourceStatus{Key: key, Label: label, OK: err == nil, DurationMS: time.Since(began).Milliseconds()}
	if err != nil {
		s.Note = "The read failed; see the McHarbor log."
		if r.plugin.host.Logger != nil {
			r.plugin.host.Logger.Error("logging-siem: report read failed", "source", key, "error", err)
		}
	}
	return s
}

// mayRead mirrors the gate on the Logging tab's history route: audit.view
// and a global settings.manage. A missing RBAC handle or user id denies.
func (r *reportProvider) mayRead(req plugin.ReportRequest) bool {
	host := r.plugin.host
	if host.RBAC == nil || strings.TrimSpace(req.UserID) == "" {
		return false
	}
	for _, perm := range []string{"audit.view", "settings.manage"} {
		allowed, err := host.RBAC.HasPermission(req.UserID, "", perm)
		if err != nil {
			if host.Logger != nil {
				host.Logger.Error("logging-siem: report permission check failed", "permission", perm, "error", err)
			}
			return false
		}
		if !allowed {
			return false
		}
	}
	return true
}
