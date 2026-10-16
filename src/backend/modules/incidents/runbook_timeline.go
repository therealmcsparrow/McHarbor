// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"fmt"
	"strings"
)

// maxRunbookRuns bounds how many runs one incident's timeline folds in. More
// than a handful of runbooks followed for one incident is itself the finding,
// and the summary is not the place to enumerate them.
const maxRunbookRuns = 10

// RunbookTrace is one runbook run as the timeline reads it: who followed which
// runbook, how it ended, and — the thing these timelines have never recorded —
// where it stopped.
type RunbookTrace struct {
	At           string
	Who          string
	Runbook      string
	Outcome      string
	StoppedAt    int
	StoppedTitle string
	StoppedState string
	Finding      string
	Note         string
}

// runbookRuns is the timeline's fourth source, beside events, audit rows and
// alert events.
//
// The timeline itself is a JSON blob written once by Store.Resolve, so there is
// no append path a run could write into as it happens. This is the append path:
// the run is its own rows with its own audit trail, and it is read back here at
// resolve time.
func (a *Assembler) runbookRuns(ctx context.Context, in *Incident) ([]RunbookTrace, error) {
	if in.ID == "" && in.AlertEventID == "" {
		return nil, nil
	}
	return NewStore(a.DB).runbookRuns(ctx, in.ID, in.AlertEventID, maxRunbookRuns)
}

// describeRunbookTrace is the timeline line. "Runbook X was followed; it stopped
// at step 4, 'restart did not clear it'" replaces a heuristic with a fact, which
// is the one thing the post-incident summary was missing.
func describeRunbookTrace(tr RunbookTrace) string {
	parts := []string{"runbook " + quoteName(tr.Runbook) + " " + outcomeVerb(tr.Outcome)}
	if tr.StoppedAt > 0 {
		stop := fmt.Sprintf("stopped at step %d", tr.StoppedAt)
		if tr.StoppedTitle != "" {
			stop += " (" + tr.StoppedTitle + ")"
		}
		if tr.StoppedState != "" {
			stop += ", recorded as " + tr.StoppedState
		}
		parts = append(parts, stop)
	}
	if tr.Finding != "" {
		parts = append(parts, "finding: "+tr.Finding)
	}
	if tr.Note != "" {
		parts = append(parts, "note: "+tr.Note)
	}
	return strings.Join(parts, "; ")
}

func quoteName(name string) string {
	if name == "" {
		return "(unnamed)"
	}
	return "\"" + name + "\""
}

func outcomeVerb(outcome string) string {
	switch outcome {
	case "completed":
		return "was completed"
	case "abandoned":
		return "was abandoned"
	case "escalated":
		return "was escalated"
	default:
		return "was still in progress"
	}
}
