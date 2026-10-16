// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "context"

// reportFamily is a group of built-in report types that owns its sources,
// snapshot data, builders and headline in one file, so a family is added
// without editing the shared switch statements every other type lives in.
type reportFamily struct {
	// types returns the catalog entries; common is the shared option set.
	types func(common []ReportOption) []ReportType
	// builders is merged into the dispatch table.
	builders map[string]sectionBuilder
	// titles maps a type key to its English cover title and subtitle.
	titles map[string][2]string
	// envScoped names the family's sources whose contents vary by `?env=`.
	envScoped map[string]bool
	// jobs is the family's slice of the source table, keyed like collectJobs.
	jobs func(ctx context.Context, api *hostAPI, snap *Snapshot, period Period) map[string]func()
	// kpis returns the page-one numbers for a family type, or nil to defer.
	kpis func(b *buildCtx) []KPI
	// findings returns matters requiring attention for a family type.
	findings func(b *buildCtx) []Finding
	// lead returns the summary's closing sentence for a family type, or "".
	lead func(b *buildCtx) string
}

// reportFamilies is every family, in the order their types join the catalog.
func reportFamilies() []reportFamily {
	return []reportFamily{resilienceFamily(), governanceFamily(), runbooksFamily()}
}

// familyFor returns the family that owns a type key.
func familyFor(key string) (reportFamily, bool) {
	for _, f := range reportFamilies() {
		if _, ok := f.builders[key]; ok {
			return f, true
		}
	}
	return reportFamily{}, false
}
