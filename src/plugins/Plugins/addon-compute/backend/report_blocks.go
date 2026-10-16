// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// leadBlock is the opening paragraph of a report.
//
// Without one the reporting plugin synthesises a deliberately plain lead that
// says nothing about the subject, because it knows nothing about it. A provider
// that writes its own is the difference between a document that opens with an
// argument and one that opens with a disclaimer.
func leadBlock(text string) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "paragraph", Text: text, Lead: true}
}

// paraBlock is an ordinary paragraph.
func paraBlock(text string) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "paragraph", Text: text}
}

// findingsBlock renders issues with their recommendations.
func findingsBlock(findings []plugin.ReportFinding) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "findings", Findings: findings}
}

// finding builds one issue. A finding without a recommendation wastes the
// reader's time, so every caller supplies one.
func finding(severity, title, detail, recommendation string) plugin.ReportFinding {
	return plugin.ReportFinding{
		Severity:       severity,
		Title:          title,
		Detail:         detail,
		Recommendation: recommendation,
	}
}

// donutChart draws a share-of-total breakdown.
func donutChart(title string, points []plugin.ReportChartPoint) plugin.ReportBlock {
	return plugin.ReportBlock{
		Kind: "chart",
		Chart: &plugin.ReportChart{
			Kind:       "donut",
			Title:      title,
			ShowLegend: true,
			ShowValues: true,
			Series:     []plugin.ReportChartSeries{{Name: title, Points: points}},
		},
	}
}

// chartPoint is one datum, with an optional status colour.
func chartPoint(label string, value float64, status string) plugin.ReportChartPoint {
	return plugin.ReportChartPoint{Label: label, Value: value, Status: status}
}

// plural renders "1 pool" / "3 pools" without a second format string at every
// call site.
func plural(count int, one, many string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, one)
	}
	return fmt.Sprintf("%d %s", count, many)
}

// kpiStatus maps a boolean judgement onto the KPI status vocabulary.
func kpiStatus(good bool) string {
	if good {
		return "good"
	}
	return "warn"
}

// readinessLead writes the opening paragraph of the readiness report.
func readinessLead(hosts, ready, blocked, checks int) string {
	if hosts == 0 {
		return "No host in this scope reported a capability probe, so there is nothing to assess. " +
			"Run the compute setup on a host to produce one."
	}
	if blocked == 0 {
		return fmt.Sprintf(
			"All %s passed every critical capability check. Guests can be created and started anywhere in this scope.",
			plural(hosts, "host", "hosts"))
	}
	return fmt.Sprintf(
		"%d of %s are ready to run guests. %s cannot, with %s failing between them. "+
			"A host in that state accepts a guest definition and then fails at start, so the problem surfaces as a broken guest rather than as a missing package on the host.",
		ready, plural(hosts, "host", "hosts"),
		plural(blocked, "host", "hosts"),
		plural(checks, "critical check", "critical checks"))
}
