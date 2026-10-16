// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"sort"

	"github.com/therealmcsparrow/mcharbor/modules/compliance"
)

// Evaluate produces one result per control in f, in control order. Every control
// in the framework gets a row, including the ones McHarbor cannot check: a pack
// that lists only what it checked reads as a clean bill of health, which is the
// misrepresentation this whole feature exists to avoid.
//
// The two "not assessed" shapes are deliberately different verdicts.
// A mechanical check whose evidence was unreachable is `unknown` — it should have
// had an answer and does not. A control McHarbor structurally cannot see is
// `info`, because it describes rather than judges and would otherwise hold every
// run below passing forever.
func Evaluate(f Framework, in Inputs) []Result {
	out := make([]Result, 0, len(f.Controls))
	for _, c := range f.Controls {
		switch {
		case c.daemonCheck != nil:
			out = append(out, c.daemonCheck(in))
		case c.containerCheck != nil:
			out = append(out, c.containerCheck(in))
		case c.Coverage == CoverageEvidenced:
			out = append(out, newResult(c.ID).raise(StatusInfo, "evidencedNotChecked").done())
		default:
			out = append(out, newResult(c.ID).raise(StatusInfo, "notCoveredByMcHarbor").done())
		}
	}
	order := ControlOrder(f)
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].ID] < order[out[j].ID] })
	return out
}

// Summarize counts the verdicts of a run, reusing the posture's counter so the
// dashboard and a pack agree on what a summary is.
func Summarize(results []Result) compliance.Summary {
	return compliance.Summarize(results)
}
