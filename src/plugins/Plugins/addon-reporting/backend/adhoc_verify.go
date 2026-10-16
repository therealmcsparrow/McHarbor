// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"strconv"
	"strings"
)

// Numeric verification for the ad-hoc AI report.
//
// The model is handed a digest of Facts and a set of projected slices,
// and asked to write prose and tables around them. Nothing stops it from
// writing a number that was never in that payload — a fabricated growth
// rate, a made-up count. This file builds the allow-set of every figure
// the model actually saw, in every spelling a report could reasonably
// use, and diffs every numeric token the model wrote against it. A token
// that survives the diff was not traceable to the data it was handed and
// is disclosed to the reader rather than shipped silently.
//
// The allow-set is deliberately generous: over-flagging — a report where
// every second number is marked unverified — is the failure mode that
// makes the whole feature useless, because nobody reads a report that
// looks like it doubts itself constantly. A model handed 0.86 will
// legitimately write "86%"; one handed 1509949440 will legitimately
// write "1.4 GB"; one handed 1234 will legitimately write "1,234".
// Registering each value through every formatter, both percentage
// interpretations, comma grouping, and raw decimal rendering is what
// keeps those honest renderings from being flagged, while still catching
// a figure that came from nowhere.

// commaInt renders v with comma grouping. No formatter in format.go ever
// produces this spelling — fmtInt/fmtDecimal group with a thin space via
// groupInt — but a model writing prose is not bound to the host's own
// formatter and "1,234" is the ordinary way to spell a four-digit count
// in English. It exists only to widen the allow-set; it must never be
// used to render anything a reader sees.
func commaInt(v int64) string {
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
	joined := strings.Join(out, ",")
	if neg {
		return "-" + joined
	}
	return joined
}

// commaDecimal is commaInt with dp decimal places kept, e.g. "1,234.5".
func commaDecimal(v float64, dp int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	s := strconv.FormatFloat(v, 'f', dp, 64)
	parts := strings.SplitN(s, ".", 2)
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	out := commaInt(whole)
	if len(parts) == 2 {
		out += "." + parts[1]
	}
	return out
}

// buildAllowSet walks the digest (from factsDigest) and the served
// slices (from buildSlices) and registers every numeric value found,
// through every formatter a report might reasonably have used to render
// it. Both arguments may be nil or empty; the walk is a no-op on either.
func buildAllowSet(digest, slices map[string]any) map[string]bool {
	allow := make(map[string]bool)

	// register runs the value through numericTokens the same way
	// verifySections will run it over report prose, so the allow-set is
	// self-consistent with the extraction by construction — it does not
	// matter whether a formatter emits a thin space, a comma, a percent
	// sign or a unit suffix; whatever token numericTokens would pull out
	// of that formatted string is exactly what gets allowed.
	register := func(formatted string) {
		for _, tok := range numericTokens(formatted) {
			allow[tok] = true
		}
	}

	add := func(f float64) {
		register(fmtInt(int(f)))
		register(fmtDecimal(f, 0))
		register(fmtDecimal(f, 1))
		register(fmtDecimal(f, 2))
		register(fmtPercent(f))
		register(fmtBytes(int64(f)))
		register(fmtDays(f))
		register(fmtDuration(f))
		register(strconv.FormatFloat(f, 'f', -1, 64))
		// Comma grouping: no formatter emits it, but it is the ordinary
		// English spelling of a count over 1000 and a model will use it
		// unprompted. Route it through numericTokens exactly like every
		// other candidate so the self-consistency property holds.
		register(commaInt(int64(f)))
		register(commaDecimal(f, 1))
		register(commaDecimal(f, 2))

		// A percentage may arrive as a fraction (0.86) or already scaled
		// (86); register both interpretations so either spelling the
		// model might legitimately write resolves.
		register(fmtPercent(f * 100))
		register(fmtDecimal(f*100, 0))
		register(fmtDecimal(f*100, 1))
		register(fmtInt(int(f * 100)))
		register(commaInt(int64(f * 100)))
		register(fmtPercent(f / 100))
		register(fmtDecimal(f/100, 1))
		register(fmtDecimal(f/100, 2))
	}

	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case nil:
			return
		case map[string]any:
			for _, x := range t {
				walk(x)
			}
		case []map[string]any:
			for _, x := range t {
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		case []string:
			for _, x := range t {
				walk(x)
			}
		case float64:
			add(t)
		case int:
			add(float64(t))
		case int64:
			add(float64(t))
		case string:
			// Every run of digits actually present in a string leaf is
			// registered directly — not only when the whole string
			// parses as a float. A leaf carrying a timestamp ("at",
			// "created", "last_login", "scanned_at"), a version
			// ("2.0.8") or an image tag ("1.25.3-alpine") never parses
			// as a float, but a model quoting that exact figure back
			// (the year, the version, the tag) is not fabricating
			// anything; the ParseFloat path below still covers a source
			// that projects a genuinely numeric-looking string column.
			for _, tok := range numericTokens(t) {
				allow[tok] = true
			}
			if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
				add(f)
			}
		}
		// bool and every other leaf type (time.Time formatted upstream
		// into a string, etc.) carry no numeric claim and are ignored.
	}
	walk(digest)
	walk(slices)

	// Small integers are ordinals, list positions, heading depths and
	// counts of a handful of items far more often than they are a claim
	// worth checking. Flagging them buries the figures that matter.
	for i := 0; i <= 10; i++ {
		allow[strconv.Itoa(i)] = true
	}
	return allow
}

// unverified is one figure the model wrote that could not be traced back
// to the digest or the slices it was given.
type unverified struct {
	Section string
	// Block is the index of the block within the section that carried
	// the figure, so a reader can find it among several tables or
	// paragraphs rather than just being told which section to search.
	// -1 means the figure was in the section's own Intent caption, which
	// is not part of any block.
	Block int
	Token string
}

// verifySections extracts every numeric token from the sanitised report
// blocks — and from each section's own Intent caption, and each KPI's
// Label alongside its Value — and reports any that is absent from the
// allow-set. Results are deduplicated by (section, block, token) so a
// figure repeated within one block — a KPI restated in its own label —
// is reported once, not once per occurrence, while the same figure
// appearing in two different blocks of the same section still gets a
// row each, so a reader can locate every occurrence.
func verifySections(secs []Section, allow map[string]bool) []unverified {
	var out []unverified
	seen := make(map[string]bool)

	record := func(section string, block int, text string) {
		if text == "" {
			return
		}
		for _, tok := range numericTokens(text) {
			if allow[tok] {
				continue
			}
			key := section + "\x00" + strconv.Itoa(block) + "\x00" + tok
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, unverified{Section: section, Block: block, Token: tok})
		}
	}

	for _, s := range secs {
		// Intent is section-level prose, not part of any block — figures
		// hidden there (e.g. a fabricated count in a section's own
		// caption) were previously never checked at all.
		record(s.Title, -1, s.Intent)
		for bi, b := range s.Blocks {
			record(s.Title, bi, b.Text)
			record(s.Title, bi, b.Title)
			record(s.Title, bi, b.Caption)
			if len(b.Items) > 0 {
				record(s.Title, bi, strings.Join(b.Items, " "))
			}
			for _, col := range b.Columns {
				record(s.Title, bi, col.Label)
			}
			for _, k := range b.KPIs {
				// Label is checked alongside Value: a KPI titled
				// "Containers over 30 days" hides a figure in the label
				// itself, and that figure was previously never checked.
				record(s.Title, bi, k.Label)
				record(s.Title, bi, k.Value)
			}
			for _, row := range b.Rows {
				for _, c := range row {
					record(s.Title, bi, c.Text)
				}
			}
			for _, p := range b.Pairs {
				record(s.Title, bi, p.K)
				record(s.Title, bi, p.V)
			}
		}
	}
	return out
}
