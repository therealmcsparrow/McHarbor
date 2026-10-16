// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// resExportRow is one instance export from GET /api/instance-backups. Only
// metadata is decoded; the artifact and its key are never requested.
type resExportRow struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	Trigger        string  `json:"trigger"`
	ScheduleID     *string `json:"scheduleId"`
	CreatedBy      string  `json:"createdBy"`
	SizeBytes      int64   `json:"sizeBytes"`
	KeyKind        string  `json:"keyKind"`
	IncludeMetrics bool    `json:"includeMetrics"`
	Error          string  `json:"error"`
	LocationID     *string `json:"locationId"`
	LocationStatus string  `json:"locationStatus"`
	LocationError  string  `json:"locationError"`
	VerifyStatus   string  `json:"verifyStatus"`
	VerifyError    string  `json:"verifyError"`
	VerifiedAt     *string `json:"verifiedAt"`
	CompletedAt    *string `json:"completedAt"`
	CreatedAt      string  `json:"createdAt"`
}

// resDRScheduleRow is one schedule from GET /api/instance-backups/schedules.
type resDRScheduleRow struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Cron           string  `json:"cron"`
	Enabled        bool    `json:"enabled"`
	KeyKind        string  `json:"keyKind"`
	LocationID     *string `json:"locationId"`
	RetentionCount int     `json:"retentionCount"`
	RetentionDays  int     `json:"retentionDays"`
	VerifyAfter    bool    `json:"verifyAfter"`
	LastRunAt      *string `json:"lastRunAt"`
	NextRunAt      *string `json:"nextRunAt"`
	LastStatus     string  `json:"lastStatus"`
	LastError      string  `json:"lastError"`
}

// resRestoreStatus is GET /api/instance-backups/restore/status; null when no
// restore has ever been started on this data directory.
type resRestoreStatus struct {
	ID         string   `json:"id"`
	State      string   `json:"state"`
	Actor      string   `json:"actor"`
	Mode       string   `json:"mode"`
	StartedAt  string   `json:"startedAt"`
	FinishedAt string   `json:"finishedAt"`
	Error      string   `json:"error"`
	Refusals   []string `json:"refusals"`
}

// resLedgerRow is one line of the restore ledger, the record that survives the
// audit log itself being replaced by a restore.
type resLedgerRow struct {
	At     string `json:"at"`
	Phase  string `json:"phase"`
	ID     string `json:"id"`
	Error  string `json:"error"`
	Intent *struct {
		Actor           string `json:"actor"`
		Via             string `json:"via"`
		Mode            string `json:"mode"`
		ExportCreatedAt string `json:"exportCreatedAt"`
	} `json:"intent"`
}

// drRestore is one restore reconstructed from its ledger lines.
type drRestore struct {
	ID, At, Actor, Mode, Outcome, Error, ExportCreatedAt string
}

// drFacts is the computed recovery position of McHarbor itself.
type drFacts struct {
	ExportsOK      bool
	Total          int
	Completed      []resExportRow
	LastGoodAge    float64
	LastVerified   float64
	LastOffsite    float64
	RecentOK       int
	RecentFailed   int
	SuccessRate    float64
	VerifyFailed   int
	UploadFailed   int
	SchedulesOK    bool
	Enabled        int
	Failing        []resDRScheduleRow
	Offsite        int
	Verifying      int
	LedgerOK       bool
	Restores       []drRestore
	RestoresDone   int
	RestoresFailed int
	RestoreOK      bool
}

// drSuccessWindowDays is the window the export success rate is taken over,
// so an old failure that has since been fixed does not colour the figure.
const drSuccessWindowDays = 30

func exportFinishedAt(e resExportRow) string {
	if c := resStrPtr(e.CompletedAt); c != "" {
		return c
	}
	return e.CreatedAt
}

func analyzeDR(snap *Snapshot) drFacts {
	r := snap.Resilience
	f := drFacts{
		ExportsOK: snap.available(srcResExports), SchedulesOK: snap.available(srcResDRSchedules),
		RestoreOK: snap.available(srcResRestore), LedgerOK: snap.available(srcResRestoreLedger),
		LastGoodAge: -1, LastVerified: -1, LastOffsite: -1,
	}
	if f.ExportsOK {
		f.Total = len(r.Exports)
		for _, e := range r.Exports {
			switch e.Status {
			case "completed":
				f.Completed = append(f.Completed, e)
				if age := resAgeDays(snap, exportFinishedAt(e)); age >= 0 {
					if f.LastGoodAge < 0 || age < f.LastGoodAge {
						f.LastGoodAge = age
					}
					if e.VerifyStatus == "passed" && (f.LastVerified < 0 || age < f.LastVerified) {
						f.LastVerified = age
					}
					if e.LocationStatus == "uploaded" && (f.LastOffsite < 0 || age < f.LastOffsite) {
						f.LastOffsite = age
					}
				}
				if resAgeDays(snap, e.CreatedAt) <= drSuccessWindowDays {
					f.RecentOK++
				}
			case "failed":
				if resAgeDays(snap, e.CreatedAt) <= drSuccessWindowDays {
					f.RecentFailed++
				}
			}
			if e.VerifyStatus == "failed" {
				f.VerifyFailed++
			}
			if e.LocationStatus == "failed" {
				f.UploadFailed++
			}
		}
		f.SuccessRate = pct(f.RecentOK, f.RecentOK+f.RecentFailed)
		sort.Slice(f.Completed, func(i, j int) bool { return exportFinishedAt(f.Completed[i]) > exportFinishedAt(f.Completed[j]) })
	}
	if f.SchedulesOK {
		for _, s := range r.DRSchedules {
			if !s.Enabled {
				continue
			}
			f.Enabled++
			if s.LastStatus == "failed" {
				f.Failing = append(f.Failing, s)
			}
			if s.LocationID != nil && *s.LocationID != "" {
				f.Offsite++
			}
			if s.VerifyAfter {
				f.Verifying++
			}
		}
	}
	if f.LedgerOK {
		f.Restores = drRestoresFromLedger(r.RestoreLedger)
		for _, x := range f.Restores {
			switch x.Outcome {
			case "done":
				f.RestoresDone++
			case "failed", "rolled-back":
				f.RestoresFailed++
			}
		}
	}
	return f
}

// drRestoresFromLedger folds the append-only ledger into one row per restore,
// newest first; the last phase recorded for an id is its outcome.
func drRestoresFromLedger(ledger []resLedgerRow) []drRestore {
	byID := map[string]*drRestore{}
	var order []string
	for _, l := range ledger {
		id := l.ID
		if id == "" {
			continue
		}
		x, ok := byID[id]
		if !ok {
			x = &drRestore{ID: id, At: l.At, Outcome: "started"}
			byID[id] = x
			order = append(order, id)
		}
		if l.Intent != nil {
			x.Actor, x.Mode, x.ExportCreatedAt = l.Intent.Actor, l.Intent.Mode, l.Intent.ExportCreatedAt
			x.At = l.At
		}
		switch l.Phase {
		case "done", "failed", "rolled-back":
			x.Outcome = l.Phase
			if l.Error != "" {
				x.Error = l.Error
			}
		}
	}
	out := make([]drRestore, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At > out[j].At })
	return out
}

func buildInstanceDRReadiness(b *buildCtx) []Section {
	snap := b.snap
	f := analyzeDR(snap)
	var out []Section

	st := section(Strategic, "Recovery position")
	st.Blocks = append(st.Blocks, para(
		"Every other backup this platform takes is described by McHarbor's own database: its "+
			"environments, plans, users and keys. If that instance is lost, those backups are "+
			"unreadable until it is rebuilt. This report asks whether McHarbor itself can be rebuilt: "+
			"how recent its last good export is, whether that export was proven readable, whether a "+
			"copy lives off the host, and whether a restore has ever actually been carried out."),
		kpis(drKPIs(b)...))
	if !f.ExportsOK {
		st.Blocks = append(st.Blocks, resNotAssessed(snap, srcResExports, "The instance backup history"))
	} else {
		st.Blocks = append(st.Blocks, drPositionCallout(f))
	}
	if fs := drFindings(b); len(fs) > 0 {
		st.Blocks = append(st.Blocks, heading(2, "Findings"), findings(fs...))
	}
	out = append(out, st)

	ta := section(Tactical, "Schedules and restore evidence")
	if f.SchedulesOK {
		ta.Blocks = append(ta.Blocks, drScheduleTable(snap))
	} else {
		ta.Blocks = append(ta.Blocks, resNotAssessed(snap, srcResDRSchedules, "The backup schedules"))
	}
	if f.ExportsOK && len(f.Completed) > 1 {
		ta.Blocks = append(ta.Blocks, drSizeChart(f))
	}
	ta.Blocks = append(ta.Blocks, heading(2, "Restores"))
	ta.Blocks = append(ta.Blocks, drRestoreBlocks(snap, f)...)
	out = append(out, ta)

	op := section(Operational, "Export history")
	if f.ExportsOK {
		op.Blocks = append(op.Blocks, drExportTable(snap))
		op.Blocks = append(op.Blocks, drFailureDetail(snap)...)
	}
	out = append(out, op)
	return out
}

func drPositionCallout(f drFacts) Block {
	switch {
	case len(f.Completed) == 0:
		return callout(StatusBad, "No recovery point",
			"There is no completed export of this instance. A loss of the McHarbor host today could not be recovered from McHarbor's own backups.")
	case f.LastGoodAge <= 1 && f.LastVerified >= 0 && f.LastOffsite >= 0:
		return callout(StatusGood, "Recoverable",
			"A completed export from the last day exists, an export has been verified readable, and a copy is held off the host.")
	default:
		return callout(StatusWarn, "Recoverable with gaps",
			"A completed export exists from "+fmtDays(f.LastGoodAge)+". The findings below name what stands between it and a dependable recovery.")
	}
}

func drScheduleTable(snap *Snapshot) Block {
	list := append([]resDRScheduleRow(nil), snap.Resilience.DRSchedules...)
	if len(list) == 0 {
		return callout(StatusWarn, "No schedules",
			"Instance exports are taken only when someone remembers to take one.")
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	rows := make([][]Cell, 0, len(list))
	for _, s := range list {
		state := pill("Enabled", StatusGood)
		if !s.Enabled {
			state = pill("Disabled", StatusMuted)
		}
		last := cell("Never run")
		if lr := resStrPtr(s.LastRunAt); lr != "" {
			age := resAgeDays(snap, lr)
			last = cellStatus(fmtDays(age), freshnessStatus(age, 1, 7))
		}
		outcome := cell(orDash(titleCase(s.LastStatus)))
		if s.LastStatus == "failed" {
			outcome = pill("Failed", StatusBad)
		}
		offsite := "Local only"
		if s.LocationID != nil && *s.LocationID != "" {
			offsite = "Off-site"
		}
		retention := "—"
		switch {
		case s.RetentionCount > 0 && s.RetentionDays > 0:
			retention = fmtInt(s.RetentionCount) + " / " + fmtInt(s.RetentionDays) + " d"
		case s.RetentionCount > 0:
			retention = fmtInt(s.RetentionCount) + " kept"
		case s.RetentionDays > 0:
			retention = fmtInt(s.RetentionDays) + " days"
		}
		verify := "No"
		if s.VerifyAfter {
			verify = "Yes"
		}
		rows = append(rows, []Cell{
			cell(s.Name), mono(s.Cron), state, last, outcome, cell(offsite), cell(verify), cell(retention),
			cell(fmtDate(parseTS(resStrPtr(s.NextRunAt)))),
		})
	}
	return table([]Column{
		{Label: "Schedule", Width: 1.8}, {Label: "Cron", Width: 1.4}, {Label: "State", Width: 1},
		{Label: "Last run", Width: 1.2}, {Label: "Outcome", Width: 1}, {Label: "Copy", Width: 1},
		{Label: "Verify", Width: 0.8}, {Label: "Retention", Width: 1.1}, {Label: "Next run", Width: 1.2},
	}, rows, "Instance backup schedules", "")
}

// drSizeChartPoints caps the export-size series so a daily schedule does not
// crowd the axis; the most recent exports are the ones that matter.
const drSizeChartPoints = 30

func drSizeChart(f drFacts) Block {
	n := len(f.Completed)
	if n > drSizeChartPoints {
		n = drSizeChartPoints
	}
	pts := make([]ChartPoint, 0, n)
	for i := n - 1; i >= 0; i-- {
		e := f.Completed[i]
		pts = append(pts, ChartPoint{Label: parseTS(exportFinishedAt(e)).Format("2 Jan"),
			Value: float64(e.SizeBytes) / (1 << 20)})
	}
	return chartBlock(Chart{Kind: ChartLine, Title: "Export size", Unit: " MiB",
		Series:  []ChartSeries{{Name: "Export size", Points: pts}},
		Caption: "A sudden drop in size is worth a look: it can mean data was excluded rather than removed."})
}

func drRestoreBlocks(snap *Snapshot, f drFacts) []Block {
	var blocks []Block
	if f.RestoreOK && snap.Resilience.Restore != nil && snap.Resilience.Restore.State != "" {
		rs := snap.Resilience.Restore
		pairs := []Pair{
			{K: "Most recent restore", V: titleCase(rs.State)},
			{K: "Started", V: fmtDateTime(parseTS(rs.StartedAt))},
			{K: "Finished", V: fmtDateTime(parseTS(rs.FinishedAt))},
			{K: "Mode", V: orDash(rs.Mode)},
			{K: "Requested by", V: orDash(rs.Actor)},
		}
		if rs.Error != "" {
			pairs = append(pairs, Pair{K: "Error", V: rs.Error})
		}
		blocks = append(blocks, keyvalue(pairs...))
		if len(rs.Refusals) > 0 {
			blocks = append(blocks, para("The restore was refused for these reasons:"), bullets(rs.Refusals...))
		}
	} else if !f.RestoreOK {
		blocks = append(blocks, resNotAssessed(snap, srcResRestore, "The restore status"))
	}
	if !f.LedgerOK {
		return append(blocks, resNotAssessed(snap, srcResRestoreLedger, "The restore ledger"))
	}
	if len(f.Restores) == 0 {
		return append(blocks, callout(StatusWarn, "No restore has been carried out",
			"The ledger records no restore on this instance. Until one has been done, the time a recovery takes and whether it works are both assumptions."))
	}
	rows := make([][]Cell, 0, len(f.Restores))
	for _, x := range f.Restores {
		st := StatusWarn
		switch x.Outcome {
		case "done":
			st = StatusGood
		case "failed", "rolled-back":
			st = StatusBad
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(x.At))), cell(orDash(x.Actor)), cell(orDash(x.Mode)),
			cell(fmtDate(parseTS(x.ExportCreatedAt))), pill(resHumanise(x.Outcome), st),
		})
	}
	blocks = append(blocks, table([]Column{
		{Label: "Started", Width: 1.8}, {Label: "By", Width: 1.4}, {Label: "Mode", Width: 1.1},
		{Label: "Export from", Width: 1.3}, {Label: "Outcome", Width: 1.2},
	}, rows, "Restores recorded in the ledger", ""))
	var errs []string
	for _, x := range f.Restores {
		if x.Error != "" {
			errs = append(errs, fmtDateTime(parseTS(x.At))+" — "+x.Error)
		}
	}
	if len(errs) > 0 {
		blocks = append(blocks, heading(3, "Why restores failed"), bullets(errs...))
	}
	return blocks
}

// drExportRows caps the history table; the schedules above explain cadence
// and the size chart covers trend, so older rows add length, not insight.
const drExportRows = 40

func drExportTable(snap *Snapshot) Block {
	list := append([]resExportRow(nil), snap.Resilience.Exports...)
	if len(list) == 0 {
		return callout(StatusBad, "No exports", "This instance has never been exported.")
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt > list[j].CreatedAt })
	if len(list) > drExportRows {
		list = list[:drExportRows]
	}
	rows := make([][]Cell, 0, len(list))
	for _, e := range list {
		status := pill(titleCase(e.Status), StatusWarn)
		switch e.Status {
		case "completed":
			status = pill("Completed", StatusGood)
		case "failed":
			status = pill("Failed", StatusBad)
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(e.CreatedAt))), cell(titleCase(orDash(e.Trigger))), status,
			cell(fmtBytes(e.SizeBytes)), drVerifyCell(e.VerifyStatus), drLocationCell(e.LocationStatus),
		})
	}
	return table([]Column{
		{Label: "Created", Width: 1.9}, {Label: "Trigger", Width: 1.1}, {Label: "Status", Width: 1.1},
		{Label: "Size", Width: 1, Align: "right"}, {Label: "Verified", Width: 1.1}, {Label: "Off-site copy", Width: 1.2},
	}, rows, "Most recent exports", "")
}

func drVerifyCell(s string) Cell {
	switch s {
	case "passed":
		return pill("Passed", StatusGood)
	case "failed":
		return pill("Failed", StatusBad)
	case "running":
		return pill("Running", StatusInfo)
	}
	return cell("Not verified")
}

func drLocationCell(s string) Cell {
	switch s {
	case "uploaded":
		return pill("Uploaded", StatusGood)
	case "failed":
		return pill("Failed", StatusBad)
	}
	return cell("—")
}

// drFailureDetail prints every export, verification and upload error in full.
func drFailureDetail(snap *Snapshot) []Block {
	var items []string
	for _, e := range snap.Resilience.Exports {
		when := fmtDateTime(parseTS(e.CreatedAt))
		if e.Status == "failed" && e.Error != "" {
			items = append(items, when+" — export failed: "+e.Error)
		}
		if e.VerifyStatus == "failed" && e.VerifyError != "" {
			items = append(items, when+" — verification failed: "+e.VerifyError)
		}
		if e.LocationStatus == "failed" && e.LocationError != "" {
			items = append(items, when+" — off-site copy failed: "+e.LocationError)
		}
	}
	for _, s := range snap.Resilience.DRSchedules {
		if s.LastStatus == "failed" && strings.TrimSpace(s.LastError) != "" {
			items = append(items, "Schedule "+s.Name+" — last run failed: "+s.LastError)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return []Block{heading(2, "Errors recorded"), bullets(items...)}
}

func drKPIs(b *buildCtx) []KPI {
	f := analyzeDR(b.snap)
	var out []KPI
	if f.ExportsOK {
		out = append(out,
			KPI{Label: "Last good backup", Value: fmtDays(f.LastGoodAge), Target: "< 24 h",
				Status: freshnessStatus(f.LastGoodAge, 1, 7), Hint: plural(len(f.Completed), "completed export", "completed exports")},
			KPI{Label: "Last verified", Value: fmtDays(f.LastVerified), Status: freshnessStatus(f.LastVerified, 7, 30),
				Hint: "proven readable"},
			KPI{Label: "Off-site copy", Value: fmtDays(f.LastOffsite), Status: freshnessStatus(f.LastOffsite, 1, 7)})
		if f.RecentOK+f.RecentFailed > 0 {
			out = append(out, KPI{Label: "Export success", Value: fmtPercent(f.SuccessRate), Target: "100%",
				Status: thresholdStatus(f.SuccessRate, 98, 90), Hint: "last " + fmtInt(drSuccessWindowDays) + " days"})
		}
	} else {
		out = append(out, resNAKPI("Last good backup"), resNAKPI("Last verified"), resNAKPI("Off-site copy"))
	}
	if f.SchedulesOK {
		out = append(out, KPI{Label: "Schedules enabled", Value: fmtInt(f.Enabled),
			Status: countStatus(len(f.Failing), 0, 0), Hint: fmtInt(len(f.Failing)) + " failing"})
		if f.Enabled == 0 {
			out[len(out)-1].Status = StatusBad
		}
	}
	if f.LedgerOK {
		st := StatusGood
		if f.RestoresDone == 0 {
			st = StatusWarn
		}
		out = append(out, KPI{Label: "Restores completed", Value: fmtInt(f.RestoresDone), Status: st,
			Hint: fmtInt(f.RestoresFailed) + " failed or rolled back"})
	}
	return out
}

func drFindings(b *buildCtx) []Finding {
	f := analyzeDR(b.snap)
	var out []Finding
	if f.ExportsOK {
		switch {
		case len(f.Completed) == 0:
			out = append(out, Finding{
				Severity: SevCritical, Title: "McHarbor has no completed backup of itself",
				Detail:         "If the instance's database or data directory is lost, every environment, plan, key and audit record goes with it.",
				Recommendation: "Take an instance export now, then create a daily schedule with an off-site location.",
				Owner:          "Platform engineering",
			})
		case f.LastGoodAge > 7:
			out = append(out, Finding{
				Severity: SevHigh, Title: "Last good instance backup was " + fmtDays(f.LastGoodAge),
				Detail:         "A recovery today would lose every configuration change made since then.",
				Recommendation: "Find why scheduled exports stopped completing and take a manual export in the meantime.",
				Owner:          "Platform engineering",
			})
		case f.LastGoodAge > 1:
			out = append(out, Finding{
				Severity: SevMedium, Title: "Last good instance backup was " + fmtDays(f.LastGoodAge),
				Detail:         "The recovery point is older than a day.",
				Recommendation: "Move the instance export schedule to at least daily.",
				Owner:          "Platform engineering",
			})
		}
		if len(f.Completed) > 0 && f.LastVerified < 0 {
			out = append(out, Finding{
				Severity: SevMedium, Title: "No instance backup has been verified",
				Detail:         "An export that has never been opened and checked is a file, not a recovery point.",
				Recommendation: "Verify the latest export, and switch on verification after each scheduled run.",
				Owner:          "Platform engineering",
			})
		}
		if len(f.Completed) > 0 && f.LastOffsite < 0 {
			out = append(out, Finding{
				Severity: SevMedium, Title: "Every instance backup is on the McHarbor host",
				Detail:         "A backup that lives beside what it protects is lost with it.",
				Recommendation: "Give the export schedule a storage location so each export is copied off the host.",
				Owner:          "Platform engineering",
			})
		}
		if f.UploadFailed > 0 {
			out = append(out, Finding{
				Severity: SevLow, Title: plural(f.UploadFailed, "off-site copy failed", "off-site copies failed"),
				Detail:         "The export completed but did not reach its storage location.",
				Recommendation: "Check the storage location's credentials and capacity.",
				Owner:          "Platform engineering",
			})
		}
	}
	if f.SchedulesOK && f.Enabled == 0 {
		out = append(out, Finding{
			Severity: SevHigh, Title: "No instance backup schedule is enabled",
			Detail:         "Instance exports depend on someone remembering to take one.",
			Recommendation: "Create a daily schedule under Settings → System → Instance backup.",
			Owner:          "Platform engineering",
		})
	}
	if len(f.Failing) > 0 {
		names := make([]string, 0, len(f.Failing))
		for _, s := range f.Failing {
			names = append(names, s.Name)
		}
		out = append(out, Finding{
			Severity: SevHigh, Title: plural(len(f.Failing), "backup schedule is failing", "backup schedules are failing"),
			Detail:         "Last run failed: " + joinAnd(names) + ".",
			Recommendation: "Read the recorded error in the operational section and correct the schedule's key or location.",
			Owner:          "Platform engineering",
		})
	}
	if f.RestoreOK && b.snap.Resilience.Restore != nil && b.snap.Resilience.Restore.State == "failed" {
		out = append(out, Finding{
			Severity: SevHigh, Title: "The most recent restore failed",
			Detail:         orDash(b.snap.Resilience.Restore.Error),
			Recommendation: "Establish why before relying on these exports for a real recovery.",
			Owner:          "Platform engineering",
		})
	}
	if f.LedgerOK && f.RestoresDone == 0 {
		out = append(out, Finding{
			Severity: SevLow, Title: "No restore of this instance has ever been completed",
			Detail:         "The recovery time is unknown until a restore has been rehearsed.",
			Recommendation: "Rehearse a restore onto a spare instance and record how long it took.",
			Owner:          "Platform engineering",
		})
	}
	return out
}

func drLead(b *buildCtx) string {
	f := analyzeDR(b.snap)
	if !f.ExportsOK {
		return "The instance backup history could not be read, so recoverability is not assessed."
	}
	if len(f.Completed) == 0 {
		return "McHarbor has never completed an export of itself, so the instance cannot be rebuilt from its own backups."
	}
	s := "The newest usable export of McHarbor itself is from " + fmtDays(f.LastGoodAge)
	if f.LedgerOK {
		s += ", and " + plural(f.RestoresDone, "restore has", "restores have") + " been completed"
	}
	return s + "."
}
