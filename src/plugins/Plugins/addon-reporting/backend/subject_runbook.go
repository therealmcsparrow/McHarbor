// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "strings"

// narrowToRunbook keeps the one runbook a report was scoped to, and the runs
// recorded against it.
//
// The rest of the snapshot is left whole on purpose. A runbook attached to a
// container does not make every other container irrelevant to the document, and
// emptying a collection reads to a builder as "there are none" - a lie told
// with a straight face, which is the rule applySubject already states.
func narrowToRunbook(snap *Snapshot, sub Subject) {
	want := strings.TrimSpace(sub.ID)
	name := strings.TrimSpace(sub.Name)

	kept := snap.Runbooks.Runbooks[:0:0]
	for _, rb := range snap.Runbooks.Runbooks {
		if (want != "" && rb.ID == want) || (name != "" && strings.EqualFold(rb.Name, name)) {
			kept = append(kept, rb)
		}
	}
	snap.Runbooks.Runbooks = kept

	if len(kept) != 1 {
		return
	}
	runs := snap.Runbooks.Runs[:0:0]
	for _, r := range snap.Runbooks.Runs {
		if strings.EqualFold(strings.TrimSpace(r.RunbookName), strings.TrimSpace(kept[0].Name)) {
			runs = append(runs, r)
		}
	}
	snap.Runbooks.Runs = runs
}
