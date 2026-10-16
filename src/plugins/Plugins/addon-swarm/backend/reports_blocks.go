// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Small constructors and formatters for the report document model, so
// the builders in reports_build.go and reports_tables.go read as content
// rather than as plumbing.
//
// They mirror the ones the Reporting plugin uses internally for its own
// built-in types — these three reports lived there until they were moved
// here, and the copy they were ported from is the reason a Swarm report
// still reads identically to a container one. Duplicated rather than
// shared for the reason the whole adapter family exists: two Go plugins
// cannot import a package that lives inside one of them, and the neutral
// model in core/plugin/specs/reporting is a data contract, not a toolkit.

// --- status and severity vocabulary ----------------------------------

// The neutral model carries these as plain strings. Naming them keeps a
// typo from silently becoming StatusNeutral at the far end, which is
// what providerStatus does with anything it does not recognise.
const (
	statusNeutral = ""
	statusGood    = "good"
	statusWarn    = "warn"
	statusBad     = "bad"
	statusInfo    = "info"
	statusMuted   = "muted"
)

const (
	sevCritical = "critical"
	sevHigh     = "high"
	sevMedium   = "medium"
	sevLow      = "low"
)

// Levels, as the studio's altitude filter names them.
const (
	levelStrategic   = "strategic"
	levelTactical    = "tactical"
	levelOperational = "operational"
)

// --- block constructors ----------------------------------------------

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

func rpara(text string) plugin.ReportBlock {
	return plugin.ReportBlock{Kind: "paragraph", Text: text}
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

func rpairs(pairs ...plugin.ReportPair) plugin.ReportBlock {
	if len(pairs) == 0 {
		return plugin.ReportBlock{}
	}
	return plugin.ReportBlock{Kind: "keyvalue", Pairs: pairs}
}

func rpair(k, v string) plugin.ReportPair { return plugin.ReportPair{K: k, V: v} }

func rpairMono(k, v string) plugin.ReportPair {
	return plugin.ReportPair{K: k, V: v, Mono: true}
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

func colMono(label string, width float64) plugin.ReportColumn {
	return plugin.ReportColumn{Label: label, Width: width, Mono: true}
}

func colRightMono(label string, width float64) plugin.ReportColumn {
	return plugin.ReportColumn{Label: label, Width: width, Align: "right", Mono: true}
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

// --- judgement helpers -----------------------------------------------

func boolStatus(ok bool) string {
	if ok {
		return statusGood
	}
	return statusBad
}

// countStatus: good at or below `good`, bad above `warn`.
func countStatus(v, good, warn int) string {
	switch {
	case v <= good:
		return statusGood
	case v <= warn:
		return statusWarn
	default:
		return statusBad
	}
}

// invThresholdStatus: lower is better.
func invThresholdStatus(v, good, warn float64) string {
	switch {
	case v <= good:
		return statusGood
	case v <= warn:
		return statusWarn
	default:
		return statusBad
	}
}

// --- number and date formatting --------------------------------------
//
// Everything a reader sees goes through here so the same quantity is
// never written two ways in one document. Thousands are separated with a
// thin space rather than a comma or a period, because a board pack gets
// read in several locales and "1,234" is ambiguous across them.

const thinSpace = " "

func fmtInt(v int) string { return groupInt(int64(v)) }

func groupInt(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	out = append([]string{s}, out...)
	joined := strings.Join(out, thinSpace)
	if neg {
		return "−" + joined
	}
	return joined
}

func fmtDecimal(v float64, dp int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	s := strconv.FormatFloat(v, 'f', dp, 64)
	parts := strings.SplitN(s, ".", 2)
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	out := groupInt(whole)
	if len(parts) == 2 {
		out += "." + parts[1]
	}
	return out
}

func fmtPercent(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	switch {
	case v == math.Trunc(v), v >= 10:
		return fmtDecimal(v, 0) + "%"
	default:
		return fmtDecimal(v, 1) + "%"
	}
}

// fmtBytes uses binary units, which is what Docker reports and what an
// operator expects to reconcile against `docker system df`.
func fmtBytes(b int64) string {
	if b <= 0 {
		return "0" + thinSpace + "B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	v := float64(b)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	dp := 0
	if v < 10 && i > 0 {
		dp = 1
	}
	return fmtDecimal(v, dp) + thinSpace + units[i]
}

func fmtDays(days float64) string {
	switch {
	case days < 0:
		return "never"
	case days < 1:
		return "today"
	case days < 2:
		return "yesterday"
	case days < 60:
		return fmtDecimal(days, 0) + " days ago"
	default:
		return fmtDecimal(days/30.44, 0) + " months ago"
	}
}

// fmtDate is the report's date format: unambiguous in every locale that
// matters, and short enough for a table cell.
func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2 Jan 2006")
}

func fmtDateTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2 Jan 2006, 15:04 MST")
}

// daysSince ages a timestamp. A zero time means "never", which every
// caller renders as the worst case rather than as "today".
func daysSince(t time.Time) float64 {
	if t.IsZero() {
		return -1
	}
	return time.Since(t).Hours() / 24
}

// plural renders "1 node is" / "3 nodes are" — the count and the
// agreeing verb together, so callers cannot mismatch them.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmtInt(n) + " " + many
}

func pct(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}

// --- string helpers ---------------------------------------------------

// orDash returns the first non-empty candidate, or an em dash. An em
// dash rather than an empty cell: a blank in a table reads as a
// rendering fault, and "we have no value for this" is information.
func orDash(candidates ...string) string {
	for _, c := range candidates {
		if c = strings.TrimSpace(c); c != "" {
			return c
		}
	}
	return "—"
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	if id == "" {
		return "—"
	}
	return id
}

// shortImage drops the registry host from a reference so a table column
// carries the part that identifies the image rather than the part every
// row shares.
func shortImage(ref string) string {
	if ref == "" {
		return "—"
	}
	if i := strings.Index(ref, "@"); i > 0 {
		ref = ref[:i]
	}
	parts := strings.Split(ref, "/")
	if len(parts) > 1 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		ref = strings.Join(parts[1:], "/")
	}
	return ref
}

// truncateText keeps a cell readable. A SwarmKit rejection message runs
// to several hundred characters, and a column that carries one of those
// unbroken is a column nothing else fits beside.
func truncateText(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return "—"
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// joinAnd renders a list the way a sentence needs it.
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}
