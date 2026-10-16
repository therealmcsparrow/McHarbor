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

// calloutBlock draws an aside the reader should not skip.
func calloutBlock(tone, title, text string) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "callout", Tone: tone, Title: title, Text: text}
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

// hbarChart draws a horizontal bar per labelled value, which is the shape that
// survives long host and pool names.
func hbarChart(title, unit string, points []plugin.ReportChartPoint, max float64) plugin.ReportBlock {
	return plugin.ReportBlock{
		Kind: "chart",
		Chart: &plugin.ReportChart{
			Kind:       "hbar",
			Title:      title,
			Unit:       unit,
			Max:        max,
			ShowValues: true,
			Series:     []plugin.ReportChartSeries{{Name: title, Points: points}},
		},
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

// barChart draws a vertical bar per bucket.
func barChart(title, unit string, points []plugin.ReportChartPoint) plugin.ReportBlock {
	return plugin.ReportBlock{
		Kind: "chart",
		Chart: &plugin.ReportChart{
			Kind:       "bar",
			Title:      title,
			Unit:       unit,
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

// ageBucket groups a snapshot age into the buckets the coverage chart uses.
func ageBucket(days int) string {
	switch {
	case days <= 1:
		return "Today"
	case days <= 7:
		return "This week"
	case days <= 30:
		return "This month"
	case days <= 90:
		return "1-3 months"
	default:
		return "Over 3 months"
	}
}

// ageBucketOrder is the order the buckets appear in, oldest last, because a
// chart sorted alphabetically puts "Today" in the middle.
var ageBucketOrder = []string{"Today", "This week", "This month", "1-3 months", "Over 3 months"}

// ageBucketTone colours a bucket by how much staleness it represents.
func ageBucketTone(bucket string) string {
	switch bucket {
	case "Today", "This week":
		return "good"
	case "This month":
		return "warn"
	default:
		return "bad"
	}
}
