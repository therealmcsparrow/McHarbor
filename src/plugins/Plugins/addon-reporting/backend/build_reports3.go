// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
	"time"
)

// --- 7. Capacity & Utilisation ---------------------------------------

func buildCapacity(b *buildCtx) []Section {
	f := b.facts
	var out []Section

	ta := section(Tactical, "Capacity position")
	if !f.Capacity.Assessed || f.Capacity.MemTotalBytes == 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Host metrics unavailable",
			"Host CPU and memory could not be read ("+b.snap.note(srcMetrics)+"). "+
				"Storage figures below are still accurate; compute headroom is not reported."))
	} else {
		ta.Blocks = append(ta.Blocks,
			para("Headroom is governed by whichever of CPU and memory is tighter. A host with "+
				"plenty of free memory and no spare CPU has no headroom, and averaging the two "+
				"would hide that. It is measured at the 95th percentile rather than at the "+
				"average, because headroom is a question about the busy hours: capacity that "+
				"exists only when the estate is quiet is not headroom."),
			kpis(
				KPI{Label: capacityKPILabel(f, "CPU"), Value: fmtPercent(f.Capacity.CPUPercent), Target: "≤ 75%",
					Status: invThresholdStatus(f.Capacity.CPUStat.grade(f.Capacity.CPUPercent), 75, 90)},
				KPI{Label: capacityKPILabel(f, "Memory"), Value: fmtPercent(f.Capacity.MemPercent), Target: "≤ 75%",
					Status: invThresholdStatus(f.Capacity.MemStat.grade(f.Capacity.MemPercent), 75, 90)},
				KPI{Label: "Headroom", Value: fmtPercent(f.Capacity.HeadroomPercent), Target: "≥ 25%",
					Status: thresholdStatus(f.Capacity.HeadroomPercent, 25, 15)},
				KPI{Label: "Run queue per core", Value: fmtDecimal(f.Capacity.Load1PerCore, 2), Target: "< 1.0",
					Status: invThresholdStatus(f.Capacity.Load1PerCore, 1, 2)},
			),
			chartBlock(Chart{
				Kind: ChartGauge, Title: "Capacity headroom", Max: 100, Height: 120,
				Series: []ChartSeries{{Points: []ChartPoint{{
					Label:  bandLabel(bandFor(int(headroomScore(f.Capacity.HeadroomPercent)))),
					Value:  f.Capacity.HeadroomPercent,
					Status: bandStatus(bandFor(int(headroomScore(f.Capacity.HeadroomPercent)))),
				}}}},
				Caption: "Free capacity on the tighter of CPU and memory.",
			}))
	}
	ta.Blocks = append(ta.Blocks, metricsCoverageCallouts(f)...)
	ta.Blocks = append(ta.Blocks, heading(2, "Storage"), resourceTable(b), reclaimCallout(f))
	ta.Blocks = append(ta.Blocks, capacityForecastBlocks(b)...)
	out = append(out, ta)

	op := section(Operational, "Where the space is going")
	if len(f.Images.Largest) > 0 {
		pts := make([]ChartPoint, 0, len(f.Images.Largest))
		for _, im := range f.Images.Largest {
			pts = append(pts, ChartPoint{Label: im.Label, Value: im.Value / (1 << 20)})
		}
		op.Blocks = append(op.Blocks, chartBlock(Chart{
			Kind: ChartHBar, Title: "Largest images", Unit: " MiB", ShowValues: true,
			Height: float64(28*len(pts)) + 10,
			Series: []ChartSeries{{Points: pts}},
		}))
	}
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, volumeTable(b.snap, 30), imageTable(b.snap, 30))
	}
	out = append(out, op)
	return out
}

// capacityForecastBlocks prints the projection table: a date per subject, or the
// reason there is not one.
//
// The refusals are printed, not filtered. A capacity page that is blank because
// eleven of twelve subjects had too little history is a broken report; one that
// lists them as "insufficient history" tells the reader to come back in a week.
//
// Every number here was computed by a deterministic fit over the retained series
// and is restated, never recalculated. There is no rate, no price and no currency
// anywhere in this report, and adding one would make it a bill.
func capacityForecastBlocks(b *buildCtx) []Block {
	cap := b.snap.Capacity
	if len(cap.Subjects) == 0 {
		return nil
	}
	rows := make([][]Cell, 0, len(cap.Subjects))
	dated := 0
	for _, c := range cap.Subjects {
		used := "—"
		if c.TotalBytes > 0 {
			used = fmtPercent(c.UsedPercent) + " of " + fmtBytes(c.TotalBytes)
		}
		when, rate, confidence := "—", "—", "—"
		st := StatusNeutral
		if c.Red != nil {
			dated++
			when = fmtDate(parseTS(c.Red.At)) + " (" + fmtDecimal(c.Red.Days, 1) + " days)"
			rate = fmtDecimal(c.PercentPerDay, 2) + "%/day"
			confidence = "R² " + fmtDecimal(c.R2, 2)
			st = StatusWarn
			if c.Red.Days <= 7 {
				st = StatusBad
			}
		} else {
			when = capacityRefusalText(c)
		}
		rows = append(rows, []Cell{
			cell(c.SubjectName), cell(capacitySubjectKindLabel(c.SubjectKind)),
			cell(used), pill(when, st), cell(rate), cell(confidence),
		})
	}
	note := "Projected by a least-squares fit over the retained hourly capacity series, " +
		"at the current rate, against the " + fmtPercent(cap.RedPercent) + " line. A subject needs " +
		fmtDecimal(cap.RequiredDays, 0) + " days of history before any date is given; the window is " +
		fmtInt(cap.WindowDays) + " days."
	blocks := []Block{
		heading(2, "Projected exhaustion"),
		table([]Column{
			{Label: "Subject", Width: 2.6},
			{Label: "Kind", Width: 1.4},
			{Label: "Used", Width: 1.6},
			{Label: "Full on", Width: 2.2},
			{Label: "Rate", Width: 1.2, Align: "right"},
			{Label: "Confidence", Width: 1.2, Align: "right"},
		}, rows, "Capacity projections", note),
	}
	if dated == 0 {
		blocks = append(blocks, callout(StatusInfo, "No projection yet",
			"Every subject was refused for want of history or a trend, so this report gives no dates. "+
				"That is the intended behaviour: a date extrapolated from three samples is worse than no date, "+
				"because somebody acts on it."))
	}
	return blocks
}

// capacityRefusalText says why a subject has no date, in the words an operator can
// act on rather than the machine key.
func capacityRefusalText(c capacitySubject) string {
	switch c.Refusal {
	case "short_series":
		return "Needs " + fmtDecimal(c.RequiredDays, 0) + " days, has " + fmtDecimal(c.ObservedDays, 1)
	case "noisy":
		return "Too variable to project"
	case "not_rising":
		return "Not growing"
	case "already_over":
		return "Already past the limit"
	case "beyond_horizon":
		return "More than 30 days away"
	}
	return "No projection"
}

func capacitySubjectKindLabel(kind string) string {
	if kind == "storage_location" {
		return "Storage destination"
	}
	return "Filesystem"
}

// capacityKPILabel names a KPI for what it actually is. A figure
// averaged over a month and a figure read once are different claims, and
// the label is the cheapest place to keep them apart.
func capacityKPILabel(f Facts, resource string) string {
	if f.Capacity.HistoryAvailable {
		return resource + " average"
	}
	return resource + " in use"
}

func reclaimCallout(f Facts) Block {
	total := f.Images.ReclaimableBytes
	if total <= 0 && f.Capacity.UnusedVolumes == 0 {
		return callout(StatusGood, "Nothing to reclaim",
			"Every image and volume on this host is referenced by a workload.")
	}
	parts := []string{}
	if f.Images.Unused > 0 {
		parts = append(parts, fmtBytes(f.Images.ReclaimableBytes)+" across "+
			plural(f.Images.Unused, "unused image", "unused images"))
	}
	if f.Images.Dangling > 0 {
		parts = append(parts, plural(f.Images.Dangling, "dangling image", "dangling images"))
	}
	if f.Capacity.UnusedVolumes > 0 {
		parts = append(parts, plural(f.Capacity.UnusedVolumes, "unreferenced volume", "unreferenced volumes"))
	}
	return callout(StatusInfo, "Reclaimable now",
		joinAnd(parts)+" can be removed without affecting a running workload. "+
			"Confirm before pruning: an image with no running container may still be the "+
			"rollback target for one.")
}

func volumeTable(snap *Snapshot, limit int) Block {
	list := append([]volumeRow(nil), snap.Volumes...)
	sort.Slice(list, func(i, j int) bool { return volumeSize(list[i]) > volumeSize(list[j]) })
	rows := make([][]Cell, 0, limit)
	truncated := false
	for i, v := range list {
		if i >= limit {
			truncated = true
			break
		}
		st := StatusNeutral
		use := "In use"
		if v.RefCount == 0 {
			st, use = StatusWarn, "Unreferenced"
		}
		size := "—"
		if s := volumeSize(v); s > 0 {
			size = fmtBytes(s)
		}
		rows = append(rows, []Cell{cell(v.Name), cell(v.Driver), cell(size), pill(use, st), cell(fmtDate(parseTS(v.CreatedAt)))})
	}
	note := ""
	if truncated {
		note = "Showing the " + fmtInt(limit) + " largest of " + fmtInt(len(list)) + " volumes."
	}
	return table(
		[]Column{
			{Label: "Volume", Width: 3.4},
			{Label: "Driver", Width: 1.2},
			{Label: "Size", Width: 1.2, Align: "right"},
			{Label: "Use", Width: 1.4},
			{Label: "Created", Width: 1.4},
		}, rows, "Volumes", note)
}

func volumeSize(v volumeRow) int64 {
	if v.UsageData == nil {
		return 0
	}
	return v.UsageData.Size
}

func imageTable(snap *Snapshot, limit int) Block {
	list := append([]imageRow(nil), snap.Images...)
	sort.Slice(list, func(i, j int) bool { return list[i].Size > list[j].Size })
	rows := make([][]Cell, 0, limit)
	truncated := false
	for i, im := range list {
		if i >= limit {
			truncated = true
			break
		}
		st, use := StatusNeutral, "In use"
		if im.Containers <= 0 {
			st, use = StatusWarn, "Unused"
		}
		rows = append(rows, []Cell{
			cell(imageName(im)), mono(shortID(im.ID)), cell(fmtBytes(im.Size)),
			pill(use, st), cell(fmtDays(daysSinceUnix(im.Created))),
		})
	}
	note := ""
	if truncated {
		note = "Showing the " + fmtInt(limit) + " largest of " + fmtInt(len(list)) + " images."
	}
	return table(
		[]Column{
			{Label: "Image", Width: 3.4},
			{Label: "ID", Width: 1.4, Mono: true},
			{Label: "Size", Width: 1.2, Align: "right"},
			{Label: "Use", Width: 1.2},
			{Label: "Built", Width: 1.4},
		}, rows, "Images", note)
}

// --- 8. Backup & Recovery --------------------------------------------

func buildBackupRecovery(b *buildCtx) []Section {
	f := b.facts.Resilience
	var out []Section

	ta := section(Tactical, "Recovery readiness")
	if !f.Assessed {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Not assessed",
			"Backup data could not be read ("+b.snap.note(srcBackups)+"). Recovery readiness "+
				"is not reported for this period."))
		out = append(out, ta)
		return out
	}
	if f.Plans == 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusBad, "No backup plans exist",
			"Nothing on this environment is being backed up by McHarbor. A host or storage "+
				"failure would mean rebuilding every workload from source, and losing any state "+
				"held in volumes."))
	}
	ta.Blocks = append(ta.Blocks,
		para("Recovery readiness is three questions: is enough covered, do the runs succeed, and "+
			"how old is the newest usable copy. A perfect success rate over a plan that covers "+
			"one workload is not readiness."),
		resilienceKPIs(b.facts),
		chartBlock(Chart{
			Kind: ChartDonut, Title: "Backup outcomes in the period", Unit: "runs", Height: 130,
			Series: []ChartSeries{{Points: []ChartPoint{
				{Label: "Succeeded", Value: float64(f.RunsSucceeded), Status: StatusGood},
				{Label: "Failed", Value: float64(f.RunsFailed), Status: StatusBad},
			}}},
		}),
		table(
			[]Column{
				{Label: "Measure", Width: 3},
				{Label: "Value", Width: 1.4, Align: "right"},
				{Label: "Target", Width: 1.4, Align: "right"},
				{Label: "Verdict", Width: 1.4},
			},
			[][]Cell{
				{cell("Coverage of running workloads"), cellStatus(fmtPercent(f.CoveragePercent), thresholdStatus(f.CoveragePercent, 90, 60)), cell("100%"), verdictCell(thresholdStatus(f.CoveragePercent, 90, 60))},
				{cell("Run success rate"), cellStatus(fmtPercent(f.SuccessRate), thresholdStatus(f.SuccessRate, 98, 90)), cell("100%"), verdictCell(thresholdStatus(f.SuccessRate, 98, 90))},
				{cell("Age of newest good backup"), cellStatus(fmtDays(f.LastSuccessAge), freshnessStatus(f.LastSuccessAge, 2, 7)), cell("≤ 24 h"), verdictCell(freshnessStatus(f.LastSuccessAge, 2, 7))},
				{cell("Plans enabled"), cell(fmtInt(f.PlansEnabled) + " / " + fmtInt(f.Plans)), cell("all"), verdictCell(thresholdStatus(pct(f.PlansEnabled, maxIntOf(f.Plans, 1)), 100, 80))},
				{cell("Storage destinations"), cell(fmtInt(f.Destinations)), cell("≥ 2"), verdictCell(thresholdStatus(float64(f.Destinations), 2, 1))},
			}, "Recovery measures", "A single destination means one failure can take the backups with the data."),
	)
	out = append(out, ta)

	op := section(Operational, "Plans and runs")
	if len(b.snap.BackupPlans) > 0 {
		op.Blocks = append(op.Blocks, backupPlanTable(b.snap))
	}
	if b.opt(OptIncludeDetail) && len(b.snap.BackupRuns) > 0 {
		op.Blocks = append(op.Blocks, backupRunTable(b.snap, 30))
	}
	if b.opt(OptIncludeDetail) && len(b.snap.BackupLogs) > 0 {
		op.Blocks = append(op.Blocks, backupLogTable(b.snap, 25))
	}
	out = append(out, op)
	return out
}

func verdictCell(s Status) Cell {
	switch s {
	case StatusGood:
		return pill("On target", StatusGood)
	case StatusWarn:
		return pill("Drifting", StatusWarn)
	default:
		return pill("Off target", StatusBad)
	}
}

func backupPlanTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.BackupPlans))
	for _, p := range snap.BackupPlans {
		st, label := StatusGood, "Enabled"
		if !p.Enabled {
			st, label = StatusWarn, "Disabled"
		}
		retention := "—"
		switch {
		case p.RetentionCount > 0 && p.RetentionDays > 0:
			retention = fmtInt(p.RetentionCount) + " copies / " + fmtInt(p.RetentionDays) + " days"
		case p.RetentionCount > 0:
			retention = fmtInt(p.RetentionCount) + " copies"
		case p.RetentionDays > 0:
			retention = fmtInt(p.RetentionDays) + " days"
		}
		rows = append(rows, []Cell{
			cell(p.Name), cell(p.ContainerName), mono(orDash(p.Cron)), cell(retention), pill(label, st),
		})
	}
	return table(
		[]Column{
			{Label: "Plan", Width: 2.4},
			{Label: "Workload", Width: 2.4},
			{Label: "Schedule", Width: 1.6, Mono: true},
			{Label: "Retention", Width: 2},
			{Label: "State", Width: 1.2},
		}, rows, "Backup plans", "")
}

func backupRunTable(snap *Snapshot, limit int) Block {
	list := append([]backupRunRow(nil), snap.BackupRuns...)
	sort.Slice(list, func(i, j int) bool {
		return firstTS(list[i].CompletedAt, list[i].CreatedAt).After(firstTS(list[j].CompletedAt, list[j].CreatedAt))
	})
	rows := make([][]Cell, 0, limit)
	truncated := false
	for i, r := range list {
		if i >= limit {
			truncated = true
			break
		}
		st := StatusNeutral
		switch strings.ToLower(r.Status) {
		case "success", "succeeded", "completed", "ok":
			st = StatusGood
		case "failed", "error", "cancelled", "canceled":
			st = StatusBad
		case "running", "pending":
			st = StatusInfo
		}
		size := "—"
		if r.ArchiveSize > 0 {
			size = fmtBytes(r.ArchiveSize)
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(firstTS(r.CompletedAt, r.CreatedAt))),
			cell(orDash(r.ContainerName)),
			pill(titleCase(r.Status), st),
			cell(size),
			cell(truncateText(r.Error, 60)),
		})
	}
	note := ""
	if truncated {
		note = "Showing the most recent " + fmtInt(limit) + " of " + fmtInt(len(list)) + " runs."
	}
	return table(
		[]Column{
			{Label: "When", Width: 2.2},
			{Label: "Workload", Width: 2.2},
			{Label: "Outcome", Width: 1.3},
			{Label: "Size", Width: 1.2, Align: "right"},
			{Label: "Error", Width: 3},
		}, rows, "Backup runs", note)
}

func backupLogTable(snap *Snapshot, limit int) Block {
	list := append([]backupLogRow(nil), snap.BackupLogs...)
	sort.Slice(list, func(i, j int) bool { return parseTS(list[i].CreatedAt).After(parseTS(list[j].CreatedAt)) })
	rows := make([][]Cell, 0, limit)
	for i, l := range list {
		if i >= limit {
			break
		}
		st := StatusNeutral
		switch strings.ToLower(l.Severity) {
		case "error", "critical":
			st = StatusBad
		case "warning", "warn":
			st = StatusWarn
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(l.CreatedAt))),
			cell(derefOr(l.PlanName, "—")),
			cell(titleCase(l.Phase)),
			pill(titleCase(l.Severity), st),
			cell(truncateText(l.Message, 70)),
		})
	}
	return table(
		[]Column{
			{Label: "When", Width: 2.2},
			{Label: "Plan", Width: 2},
			{Label: "Phase", Width: 1.4},
			{Label: "Severity", Width: 1.2},
			{Label: "Message", Width: 3.4},
		}, rows, "Backup log", "")
}

// --- 9. Image & Supply Chain -----------------------------------------

func buildImageSupplyChain(b *buildCtx) []Section {
	f := b.facts
	var out []Section

	ta := section(Tactical, "Image estate")
	ta.Blocks = append(ta.Blocks,
		para("Every workload inherits the contents of its image, including everything the image "+
			"inherited from its base. Age is the single best proxy for accumulated unpatched "+
			"risk: an image built six months ago carries six months of published vulnerabilities "+
			"whether or not anyone has scanned it."),
		kpis(
			KPI{Label: "Images", Value: fmtInt(f.Images.Total), Hint: fmtBytes(f.Images.TotalBytes)},
			KPI{Label: "In use", Value: fmtInt(f.Images.InUse), Target: fmtInt(f.Images.Total) + " total"},
			KPI{Label: "Median age", Value: fmtDecimal(f.Images.MedianAgeDays, 0) + " days",
				Status: invThresholdStatus(f.Images.MedianAgeDays, 30, 90)},
			KPI{Label: "Older than 90 days", Value: fmtInt(f.Images.StaleOver90Days),
				Status: countStatus(f.Images.StaleOver90Days, 0, 5)},
		))
	if f.Security.Assessed {
		ta.Blocks = append(ta.Blocks, heading(2, "Assessed exposure"), severityChart(f), severityTable(f))
	} else {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Images are not being scanned",
			"No scan results were available ("+b.snap.note(srcScans)+"). The estate's "+
				"vulnerability exposure is unknown rather than zero."))
	}
	if len(b.snap.Registries) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Sources"), registryTable(b.snap))
	}
	if len(b.snap.Updates) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Update automation"), updatePolicyTable(b.snap))
	}
	out = append(out, ta)

	op := section(Operational, "Image detail")
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, imageTable(b.snap, 40))
		if len(b.snap.Scans) > 0 {
			op.Blocks = append(op.Blocks, scanTable(b.snap, 30))
		}
	}
	out = append(out, op)
	return out
}

func registryTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Registries))
	for _, r := range snap.Registries {
		def := "—"
		if r.IsDefault {
			def = "Default"
		}
		rows = append(rows, []Cell{cell(r.Name), mono(r.URL), cell(def)})
	}
	return table(
		[]Column{{Label: "Registry", Width: 2}, {Label: "Endpoint", Width: 4.5, Mono: true}, {Label: "Role", Width: 1.2}},
		rows, "Configured registries",
		"Registry credentials are never included in a report.")
}

func updatePolicyTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Updates))
	for _, p := range snap.Updates {
		st, label := StatusGood, "Enabled"
		if !p.Enabled {
			st, label = StatusWarn, "Disabled"
		}
		last := "never"
		if p.LastRunAt != "" {
			last = fmtDate(parseTS(p.LastRunAt))
		}
		rows = append(rows, []Cell{
			cell(p.Name), mono(orDash(p.ImageMatch, p.ContainerName)), mono(orDash(p.Schedule)),
			cell(titleCase(p.Strategy)), cell(last), pill(label, st),
		})
	}
	return table(
		[]Column{
			{Label: "Policy", Width: 2},
			{Label: "Matches", Width: 2.2, Mono: true},
			{Label: "Schedule", Width: 1.4, Mono: true},
			{Label: "Strategy", Width: 1.4},
			{Label: "Last run", Width: 1.3},
			{Label: "State", Width: 1.1},
		}, rows, "Update policies", "")
}

// --- 10. Access & Identity Review ------------------------------------

func buildAccessReview(b *buildCtx) []Section {
	f := b.facts.Access
	ta := section(Tactical, "Access review")
	if !f.Assessed {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Not assessed",
			"User data could not be read ("+b.snap.note(srcUsers)+")."))
		return []Section{ta}
	}
	ta.Blocks = append(ta.Blocks,
		para("This section supports a periodic access recertification: confirm that every account, "+
			"role and key below is still required by someone who is still here, and revoke the "+
			"rest. Standing access that nobody reviews is the most common way a small incident "+
			"becomes a large one."),
		kpis(
			KPI{Label: "Accounts", Value: fmtInt(f.Users), Hint: fmtInt(f.ActiveUsers) + " enabled"},
			KPI{Label: "Administrators", Value: fmtInt(f.Admins), Target: "≤ 3",
				Status: countStatus(f.Admins, 3, 6)},
			KPI{Label: "Dormant 90 days", Value: fmtInt(f.DormantUsers), Target: "0",
				Status: countStatus(f.DormantUsers, 0, 3)},
			KPI{Label: "Stale API keys", Value: fmtInt(f.APIKeysStale), Target: "0",
				Status: countStatus(f.APIKeysStale, 0, 2)},
		),
		accessControlTable(b),
		heading(2, "Accounts"), userTable(b.snap),
		heading(2, "Roles"), roleTable(b.snap))
	if len(b.snap.APIKeys) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "API keys"), apiKeyTable(b.snap))
	}
	return []Section{ta}
}

func userTable(snap *Snapshot) Block {
	list := append([]userRow(nil), snap.Users...)
	sort.Slice(list, func(i, j int) bool { return list[i].Username < list[j].Username })
	rows := make([][]Cell, 0, len(list))
	for _, u := range list {
		st, label := StatusGood, "Enabled"
		if !u.IsActive {
			st, label = StatusNeutral, "Disabled"
		}
		last := "never"
		lastStatus := StatusWarn
		if u.LastLogin != nil && *u.LastLogin != "" {
			d := daysSince(*u.LastLogin)
			last = fmtDays(d)
			lastStatus = freshnessStatus(d, 30, 90)
		}
		rows = append(rows, []Cell{
			cell(u.Username), cell(derefOr(u.Email, "—")), cell(titleCase(u.Role)),
			cellStatus(last, lastStatus), pill(label, st),
		})
	}
	return table(
		[]Column{
			{Label: "Account", Width: 2},
			{Label: "Email", Width: 2.6},
			{Label: "Role", Width: 1.6},
			{Label: "Last sign-in", Width: 1.6},
			{Label: "State", Width: 1.2},
		}, rows, "User accounts",
		"Recertify each account: still employed, still needs this role, still needs access at all.")
}

func roleTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Roles))
	for _, r := range snap.Roles {
		kind := "Custom"
		if r.IsSystem {
			kind = "Built-in"
		}
		rows = append(rows, []Cell{
			cell(r.Name), cell(kind), cell(fmtInt(len(r.Permissions))), cell(truncateText(r.Description, 70)),
		})
	}
	return table(
		[]Column{
			{Label: "Role", Width: 2},
			{Label: "Origin", Width: 1.2},
			{Label: "Permissions", Width: 1.2, Align: "right"},
			{Label: "Description", Width: 4},
		}, rows, "Roles", "")
}

func apiKeyTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.APIKeys))
	for _, k := range snap.APIKeys {
		st, label := StatusGood, "Active"
		if !k.IsActive {
			st, label = StatusNeutral, "Revoked"
		}
		used := "never"
		usedStatus := StatusWarn
		if k.LastUsedAt != nil && *k.LastUsedAt != "" {
			d := daysSince(*k.LastUsedAt)
			used = fmtDays(d)
			usedStatus = freshnessStatus(d, 30, 90)
		}
		expires := "no expiry"
		expStatus := StatusWarn
		if k.ExpiresAt != nil && *k.ExpiresAt != "" {
			t := parseTS(*k.ExpiresAt)
			expires = fmtDate(t)
			expStatus = StatusGood
			if !t.IsZero() && daysSince(*k.ExpiresAt) > 0 {
				expires = "expired"
				expStatus = StatusBad
			}
		}
		rows = append(rows, []Cell{
			cell(k.Name), cell(k.Username), cell(fmtInt(len(k.Scopes))),
			cellStatus(used, usedStatus), cellStatus(expires, expStatus), pill(label, st),
		})
	}
	return table(
		[]Column{
			{Label: "Key", Width: 2},
			{Label: "Owner", Width: 1.6},
			{Label: "Scopes", Width: 1, Align: "right"},
			{Label: "Last used", Width: 1.5},
			{Label: "Expires", Width: 1.5},
			{Label: "State", Width: 1.1},
		}, rows, "API keys",
		"Key values and prefixes are deliberately omitted. A key with no expiry and no recent use should be revoked.")
}

// --- 11. Workload Inventory ------------------------------------------

// buildWorkloadInventory is the Docker detail; the cross-platform table
// of VMs, containers and pods is the inventory facet section.
func buildWorkloadInventory(b *buildCtx) []Section {
	f := b.facts
	op := section(Operational, "Inventory")
	op.Blocks = append(op.Blocks,
		para("A point-in-time record of what exists on this environment, for change control, "+
			"audit evidence and handover."),
		kpis(
			KPI{Label: "Workloads", Value: fmtInt(f.Containers.Total), Hint: fmtInt(f.Containers.Running) + " running"},
			KPI{Label: "Stacks", Value: fmtInt(f.Containers.Stacks)},
			KPI{Label: "Standalone", Value: fmtInt(f.Containers.Unmanaged),
				Hint: "Not part of a stack", Status: countStatus(f.Containers.Unmanaged, 0, 5)},
			KPI{Label: "Images", Value: fmtInt(f.Images.Total), Hint: fmtBytes(f.Images.TotalBytes)},
			KPI{Label: "Volumes", Value: fmtInt(f.Capacity.VolumeCount), Hint: fmtBytes(f.Capacity.VolumeBytes)},
			KPI{Label: "Networks", Value: fmtInt(len(b.snap.Networks))},
		),
		workloadStateChart(f),
		containerTable(b.snap, 0))

	if len(f.Containers.ByStack) > 0 {
		pts := make([]ChartPoint, 0, len(f.Containers.ByStack))
		for _, s := range f.Containers.ByStack {
			pts = append(pts, ChartPoint{Label: s.Label, Value: float64(s.Count)})
		}
		op.Blocks = append(op.Blocks, heading(2, "Workloads by stack"), chartBlock(Chart{
			Kind: ChartHBar, Title: "Workloads per stack", ShowValues: true,
			Height: float64(28*len(pts)) + 10, Series: []ChartSeries{{Points: pts}},
		}))
	}
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, heading(2, "Images"), imageTable(b.snap, 0),
			heading(2, "Volumes"), volumeTable(b.snap, 0),
			heading(2, "Networks"), networkTable(b.snap))
	}
	return []Section{op}
}

func networkTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Networks))
	for _, n := range snap.Networks {
		kind := "User-defined"
		if n.BuiltIn {
			kind = "Built-in"
		}
		scope := n.Scope
		if n.Internal {
			scope += " · internal"
		}
		rows = append(rows, []Cell{cell(n.Name), cell(n.Driver), cell(scope), cell(fmtInt(n.Containers)), cell(kind)})
	}
	return table(
		[]Column{
			{Label: "Network", Width: 2.4},
			{Label: "Driver", Width: 1.4},
			{Label: "Scope", Width: 1.8},
			{Label: "Attached", Width: 1.2, Align: "right"},
			{Label: "Origin", Width: 1.4},
		}, rows, "Networks", "")
}

// --- 12. Platform Inventory & Configuration --------------------------

func buildPlatformInventory(b *buildCtx) []Section {
	op := section(Operational, "Platform inventory")
	v := b.snap.Versions
	op.Blocks = append(op.Blocks,
		para("The platform's own configuration, for change control and support. Credentials, "+
			"connection strings and tokens are deliberately excluded."),
		keyvalue(
			Pair{K: "Product", V: b.meta.Product},
			Pair{K: "Version", V: orDash(v.McHarbor.Version, b.meta.Version)},
			Pair{K: "Build", V: orDash(v.McHarbor.Commit), Mono: true},
			Pair{K: "Built", V: orDash(v.McHarbor.Built)},
			Pair{K: "Environments", V: fmtInt(len(b.snap.Environments))},
			Pair{K: "Remote agents", V: fmtInt(len(b.snap.Agents))},
			Pair{K: "Installed plugins", V: fmtInt(len(b.snap.Plugins))},
			Pair{K: "Report generated", V: fmtDateTime(b.meta.GeneratedAt)},
		),
		heading(2, "Environments"), environmentTable(b.snap))

	if len(b.snap.Agents) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Remote agents"), agentTable(b.snap))
	}
	if len(b.snap.Plugins) > 0 {
		rows := make([][]Cell, 0, len(b.snap.Plugins))
		sorted := append([]pluginRow(nil), b.snap.Plugins...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })
		for _, p := range sorted {
			st, label := StatusGood, "Loaded"
			switch {
			case !p.Enabled:
				st, label = StatusNeutral, "Disabled"
			case !p.Loaded:
				st, label = StatusWarn, "Not loaded"
			}
			rows = append(rows, []Cell{cell(orDash(p.Name, p.Slug)), mono(p.Slug), cell(p.Version), pill(label, st)})
		}
		op.Blocks = append(op.Blocks, heading(2, "Plugins"), table(
			[]Column{
				{Label: "Plugin", Width: 2.4},
				{Label: "Slug", Width: 2.2, Mono: true},
				{Label: "Version", Width: 1.2},
				{Label: "State", Width: 1.3},
			}, rows, "Installed plugins", ""))
	}
	if len(b.snap.Schedules) > 0 {
		rows := make([][]Cell, 0, len(b.snap.Schedules))
		for _, s := range b.snap.Schedules {
			st, label := StatusGood, "Enabled"
			if !s.Enabled {
				st, label = StatusNeutral, "Disabled"
			}
			rows = append(rows, []Cell{
				cell(s.Name), mono(s.Cron), cell(titleCase(s.Action)), cell(s.Target),
				cell(fmtDate(parseTS(s.NextRunAt))), pill(label, st),
			})
		}
		op.Blocks = append(op.Blocks, heading(2, "Scheduled tasks"), table(
			[]Column{
				{Label: "Task", Width: 2},
				{Label: "Schedule", Width: 1.4, Mono: true},
				{Label: "Action", Width: 1.6},
				{Label: "Target", Width: 1.8},
				{Label: "Next run", Width: 1.4},
				{Label: "State", Width: 1.1},
			}, rows, "Scheduled tasks", ""))
	}
	op.Blocks = append(op.Blocks, heading(2, "Container engine"), engineTable(b))
	return []Section{op}
}

// --- small helpers ---------------------------------------------------

func derefOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

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

// daysSinceUnix ages a Unix timestamp; Docker reports image and
// container creation times that way rather than as strings.
func daysSinceUnix(sec int64) float64 {
	if sec <= 0 {
		return -1
	}
	return time.Since(time.Unix(sec, 0)).Hours() / 24
}
