// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Number and date formatting for report copy.
//
// Everything a reader sees goes through here so the same quantity is
// never written two ways in one document. Thousands are separated with a
// thin space rather than a comma or a period, because a board pack gets
// read in several locales and "1,234" is ambiguous across them.

const thinSpace = " "

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
	case v == math.Trunc(v):
		return fmtDecimal(v, 0) + "%"
	case v >= 10:
		return fmtDecimal(v, 0) + "%"
	default:
		return fmtDecimal(v, 1) + "%"
	}
}

// fmtUptime renders an availability figure.
//
// fmtPercent is wrong here: it drops to whole numbers above 10%, and the
// entire argument about availability happens in the decimals — 99.9% and
// 99.0% are a factor of ten apart in permitted downtime and would both print
// as "99%". Two decimals below 100, and an exact 100 prints as "100%" rather
// than "100.00%" because that is a different and stronger claim.
func fmtUptime(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	if v >= 100 {
		return "100%"
	}
	if v <= 0 {
		return "0%"
	}
	return fmtDecimal(v, 2) + "%"
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

// fmtDuration renders an elapsed time the way a report reads it: the
// largest sensible unit, one decimal at most.
func fmtDuration(mins float64) string {
	switch {
	case mins <= 0:
		return "—"
	case mins < 1:
		return fmtDecimal(mins*60, 0) + " s"
	case mins < 90:
		return fmtDecimal(mins, 0) + " min"
	case mins < 60*48:
		return fmtDecimal(mins/60, 1) + " h"
	default:
		return fmtDecimal(mins/1440, 1) + " days"
	}
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

// fmtPeriod renders a window compactly, collapsing the month and year
// when both ends share them.
func fmtPeriod(p Period) string {
	if p.PointInTime {
		return "as at " + fmtDateTime(p.End)
	}
	if p.Start.Year() == p.End.Year() {
		if p.Start.Month() == p.End.Month() {
			return fmt.Sprintf("%d–%s", p.Start.Day(), p.End.Format("2 Jan 2006"))
		}
		return fmt.Sprintf("%s – %s", p.Start.Format("2 Jan"), p.End.Format("2 Jan 2006"))
	}
	return fmt.Sprintf("%s – %s", fmtDate(p.Start), fmtDate(p.End))
}

// fmtDelta renders a signed change with a true minus sign.
func fmtDelta(v float64, dp int) string {
	switch {
	case v > 0:
		return "+" + fmtDecimal(v, dp)
	case v < 0:
		return "−" + fmtDecimal(-v, dp)
	default:
		return "0"
	}
}

// plural renders "1 workload is" / "3 workloads are" — the count and the
// agreeing verb together, so callers cannot mismatch them.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmtInt(n) + " " + many
}

// --- parsing ---------------------------------------------------------

// parseTS accepts the timestamp shapes the host emits across modules:
// RFC3339 with and without zone, SQLite's "YYYY-MM-DD HH:MM:SS", and
// date-only values.
func parseTS(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func firstTS(candidates ...string) time.Time {
	for _, c := range candidates {
		if t := parseTS(c); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

// daysSince returns age in days, or -1 when the timestamp is unusable.
func daysSince(s string) float64 {
	t := parseTS(s)
	if t.IsZero() {
		return -1
	}
	return time.Since(t).Hours() / 24
}

// --- small numeric helpers -------------------------------------------

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxIntOf(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func pct(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}

// topCounted ranks a tally, breaking ties by label so the same data
// always produces the same table.
func topCounted(m map[string]int, limit int) []Counted {
	out := make([]Counted, 0, len(m))
	for k, v := range m {
		out = append(out, Counted{Label: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// seriesByDay turns a per-day tally into a dense series across the
// period, so a chart shows quiet days as zero rather than closing the
// gap and implying continuity that is not there.
//
// Long periods are bucketed by week; a 90-day daily axis is unreadable
// at report width.
func seriesByDay(m map[string]int, p Period) []Counted {
	if p.Start.IsZero() || p.End.IsZero() || !p.End.After(p.Start) {
		return topCounted(m, 14)
	}
	days := int(p.End.Sub(p.Start).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	if days > 400 {
		days = 400
	}

	step := 1
	format := "2 Jan"
	if days > 45 {
		step = 7
		format = "2 Jan"
	}

	var out []Counted
	for i := 0; i < days; i += step {
		bucket := 0
		for j := 0; j < step && i+j < days; j++ {
			d := p.Start.AddDate(0, 0, i+j)
			bucket += m[d.Format("2006-01-02")]
		}
		out = append(out, Counted{Label: p.Start.AddDate(0, 0, i).Format(format), Count: bucket})
	}
	return out
}

// --- naming ----------------------------------------------------------

func containerName(c containerRow) string {
	if len(c.Names) > 0 && c.Names[0] != "" {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return shortID(c.ID)
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

func imageName(im imageRow) string {
	for _, t := range im.RepoTags {
		if t != "" && t != "<none>:<none>" {
			return t
		}
	}
	return shortID(im.ID)
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

// titleCase renders an identifier as a heading: "container.create"
// becomes "Container create".
func titleCase(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), ".", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
