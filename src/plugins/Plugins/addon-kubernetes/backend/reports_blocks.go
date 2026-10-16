// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Small constructors for the report document model, so the builders in
// reports_build.go read as content rather than as plumbing.
//
// They mirror the ones the Reporting plugin uses internally for its own
// 35 report types. Duplicated rather than shared for the reason the
// whole family exists: two Go plugins cannot import a package that
// lives inside one of them, and the neutral model in
// core/plugin/specs/reporting is a data contract, not a toolkit.

func rsec(level, title string, blocks ...plugin.ReportBlock) plugin.ReportSection {
	return plugin.ReportSection{Level: level, Title: title, Blocks: compactBlocks(blocks)}
}

// compactBlocks drops the zero blocks the builders emit when a table or
// a chart had nothing to show, so a section never carries a heading
// followed by nothing.
func compactBlocks(in []plugin.ReportBlock) []plugin.ReportBlock {
	out := make([]plugin.ReportBlock, 0, len(in))
	for _, b := range in {
		if b.Kind == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

func rhead(depth int, text string) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "heading", Depth: depth, Text: text}
}

func rlead(text string) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "paragraph", Text: text, Lead: true}
}

func rcallout(tone, title, text string) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "callout", Tone: tone, Title: title, Text: text}
}

func rkpis(items ...plugin.ReportKPI) plugin.ReportBlock {
	if len(items) == 0 {
		return plugin.ReportBlock{}
	}
	return plugin.ReportBlock{Kind: "kpis", KPIs: items}
}

func kpi(label, value, status, hint string) plugin.ReportKPI {
	return plugin.ReportKPI{Label: label, Value: value, Status: status, Hint: hint}
}

// rtable returns a zero block for an empty body. A table with a caption
// and no rows is a promise the document does not keep.
func rtable(cols []plugin.ReportColumn, rows [][]plugin.ReportCell, caption, note string) plugin.ReportBlock {
	if len(rows) == 0 {
		return plugin.ReportBlock{}
	}
	return plugin.ReportBlock{Kind: "table", Columns: cols, Rows: rows, Caption: caption, Note: note}
}

func col(label string, width float64) plugin.ReportColumn {
	return plugin.ReportColumn{Label: label, Width: width}
}

func colRight(label string, width float64) plugin.ReportColumn {
	return plugin.ReportColumn{Label: label, Width: width, Align: "right"}
}

func cell(text string) plugin.ReportCell { return plugin.ReportCell{Text: text} }

func cellStatus(text, status string) plugin.ReportCell {
	return plugin.ReportCell{Text: text, Status: status}
}

func pill(text, status string) plugin.ReportCell {
	return plugin.ReportCell{Text: text, Status: status, Pill: true}
}

func mono(text string) plugin.ReportCell { return plugin.ReportCell{Text: text, Mono: true} }

func rfindings(items ...plugin.ReportFinding) plugin.ReportBlock {
	if len(items) == 0 {
		return plugin.ReportBlock{}
	}
	return plugin.ReportBlock{Kind: "findings", Findings: items}
}

func finding(sev, title, detail, rec string) plugin.ReportFinding {
	return plugin.ReportFinding{Severity: sev, Title: title, Detail: detail, Recommendation: rec}
}

// rchart returns a zero block when every series is empty, so a report
// with nothing to plot prints prose instead of an empty axis.
func rchart(c plugin.ReportChart) plugin.ReportBlock {
	kept := make([]plugin.ReportChartSeries, 0, len(c.Series))
	for _, s := range c.Series {
		if len(s.Points) > 0 {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return plugin.ReportBlock{}
	}
	c.Series = kept
	return plugin.ReportBlock{Kind: "chart", Chart: &c}
}

func point(label string, value float64, status string) plugin.ReportChartPoint {
	return plugin.ReportChartPoint{Label: label, Value: value, Status: status}
}

// donut builds a single-series categorical chart, dropping the zero
// slices: a legend entry for a category with no members is noise, and
// on a donut it is an invisible wedge with a visible label.
func donut(title string, pts []plugin.ReportChartPoint) plugin.ReportBlock {
	kept := make([]plugin.ReportChartPoint, 0, len(pts))
	for _, p := range pts {
		if p.Value > 0 {
			kept = append(kept, p)
		}
	}
	return rchart(plugin.ReportChart{
		Kind: "donut", Title: title, ShowLegend: true, ShowValues: true,
		Series: []plugin.ReportChartSeries{{Points: kept}},
	})
}

// --- small numeric and string helpers --------------------------------

// readyCounts parses the "3/5" shape Kubernetes prints for readiness.
// The bool is false for anything else, and every caller checks it —
// treating an unparsable value as 0/0 would report a workload nobody
// can read as fully degraded.
func readyCounts(raw string) (ready, total int, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(raw), "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	t, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return r, t, true
}

// fullyReady reports whether a "ready" string says every replica is up.
// An unparsable value counts as ready: it is not evidence of a problem,
// and a report that invented degradation from a format it did not
// recognise would send someone to look at a healthy workload.
func fullyReady(raw string) bool {
	ready, total, ok := readyCounts(raw)
	if !ok {
		return true
	}
	return total > 0 && ready >= total
}

func pct(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

func fmtInt(v int) string { return strconv.Itoa(v) }

// ratioStatus tints a "how much of this is healthy" figure. The
// thresholds are the ones the Reporting plugin's own bands use, so a
// contributed report reads the same way as a built-in one.
func ratioStatus(part, whole int) string {
	if whole == 0 {
		return ""
	}
	switch p := pct(part, whole); {
	case p >= 99:
		return "good"
	case p >= 90:
		return "warn"
	default:
		return "bad"
	}
}

// countStatus tints a "how many problems" figure, where zero is the
// good answer.
func countStatus(n, warnAt int) string {
	switch {
	case n == 0:
		return "good"
	case n <= warnAt:
		return "warn"
	default:
		return "bad"
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// truncate keeps a cell readable. Kubernetes event messages routinely
// run to several hundred characters, and a table column that carries
// one of those unbroken is a column nothing else fits beside.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max-1]) + "…"
}

// topN sorts descending by the extracted count and keeps the head. Used
// wherever a report shows "the worst offenders" rather than everything.
func topN[T any](items []T, n int, weight func(T) int) []T {
	out := append([]T(nil), items...)
	sort.SliceStable(out, func(i, j int) bool { return weight(out[i]) > weight(out[j]) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
