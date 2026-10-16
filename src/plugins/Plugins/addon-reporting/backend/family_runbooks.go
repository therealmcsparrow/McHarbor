// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// TypeRunbooks is the runbook family's only type. Narrowed to one runbook it
// is that runbook as a printable procedure; unnarrowed it is the coverage
// picture across every runbook the instance holds.
const TypeRunbooks = "runbooks"

// Runbook family source keys, prefixed so they cannot collide with a core
// source or with a sibling family's.
const (
	srcRbRunbooks = "rb_runbooks"
	srcRbRuns     = "rb_runs"
	srcRbAlerts   = "rb_alerts"
)

// rbRunbookRow is one runbook as GET /api/runbooks returns it. Deliberately
// narrower than the host's model: action parameters can name secrets and
// container ids, and a document has no use for either.
type rbRunbookRow struct {
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	Description        string       `json:"description"`
	AttachKind         string       `json:"attachKind"`
	AttachConditionKey string       `json:"attachConditionKey"`
	AttachSubject      string       `json:"attachSubject"`
	EnvironmentID      string       `json:"environmentId"`
	Enabled            bool         `json:"enabled"`
	CreatedBy          string       `json:"createdBy"`
	UpdatedAt          string       `json:"updatedAt"`
	Steps              []rbStepRow  `json:"steps"`
	Automation         rbAutomation `json:"automation"`
}

// rbStepRow is one step. Position is carried rather than inferred from array
// order, because a runbook is reordered and a finished run still has to say
// which step was which.
type rbStepRow struct {
	Position      int    `json:"position"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	DiagnosticKey string `json:"diagnosticKey"`
	ActionKey     string `json:"actionKey"`
	Required      bool   `json:"required"`
}

// rbAutomation is how a runbook runs unattended. The principal's name is
// carried; its id is not, because a document that names a service account's id
// helps nobody and travels further than the instance.
type rbAutomation struct {
	PrincipalName string `json:"principalName"`
	OnAlert       bool   `json:"onAlert"`
	Cron          string `json:"cron"`
	NextRunAt     string `json:"nextRunAt"`
	LastRunAt     string `json:"lastRunAt"`
	LastError     string `json:"lastError"`
}

// active mirrors the host's rule: a principal and at least one trigger.
func (a rbAutomation) active() bool {
	return strings.TrimSpace(a.PrincipalName) != "" && (a.OnAlert || strings.TrimSpace(a.Cron) != "")
}

// runbooksData is the snapshot slice this family fills.
type runbooksData struct {
	Runbooks []rbRunbookRow
	Runs     []runbookRunRow
	Alerts   []alertRow
}

// runbooksFamily declares the runbook report family.
//
// It exists as a family rather than a plain catalog entry because the document
// is produced by the same PDF and DOCX writers every other report uses. Putting
// a second writer in the host to print one runbook would be the largest
// duplication in the tree for the smallest reason.
func runbooksFamily() reportFamily {
	return reportFamily{
		types: rbTypes,
		builders: map[string]sectionBuilder{
			TypeRunbooks: buildRunbooks,
		},
		titles: map[string][2]string{
			TypeRunbooks: {"Runbooks",
				"The procedures this instance holds, and what they are attached to"},
		},
		envScoped: map[string]bool{srcRbRunbooks: true, srcRbAlerts: true},
		jobs:      rbJobs,
		kpis:      rbKPIs,
		findings:  rbFindings,
		lead:      rbLead,
	}
}

func rbTypes(common []ReportOption) []ReportType {
	return []ReportType{
		{
			Key: TypeRunbooks, Category: "assurance", Icon: "book",
			Levels:  []Level{Tactical, Operational},
			Sources: []string{srcRbRunbooks, srcRbRuns, srcRbAlerts},
			// A runbook is a procedure, not a measurement over a window, so
			// the studio hides the period picker. The run history it also
			// prints is bounded by the collector's own page size instead.
			PointInTime: true,
			Subjects:    []SubjectKind{SubjectRunbook},
			Options:     common,
		},
	}
}

func rbJobs(ctx context.Context, api *hostAPI, snap *Snapshot, _ Period) map[string]func() {
	r := &snap.Runbooks
	return map[string]func(){
		srcRbRunbooks: func() {
			api.getList(ctx, srcRbRunbooks, "Runbooks", "/api/runbooks",
				map[string]string{"per_page": "500"}, &r.Runbooks)
		},
		srcRbRuns: func() {
			api.getList(ctx, srcRbRuns, "Runbook runs", "/api/runbooks/runs",
				map[string]string{"per_page": "500"}, &r.Runs)
		},
		srcRbAlerts: func() {
			api.getList(ctx, srcRbAlerts, "Alert rules", "/api/alerts",
				map[string]string{"per_page": "500"}, &r.Alerts)
		},
	}
}

// rbFacts is what both the subject document and the aggregate are derived from.
type rbFacts struct {
	OK        bool
	Runbooks  []rbRunbookRow
	Enabled   int
	Automated int
	Steps     int
	ByAttach  map[string]int
	// UncoveredConditions are alert conditions no runbook attaches to. That
	// gap is the single most useful thing an aggregate runbook report says:
	// it names the alerts that will wake somebody with no written procedure.
	UncoveredConditions []string
	Runs                []runbookRunRow
	RunOutcomes         map[string]int
}

func analyzeRunbooks(snap *Snapshot) rbFacts {
	f := rbFacts{
		OK:          snap.available(srcRbRunbooks),
		Runbooks:    append([]rbRunbookRow(nil), snap.Runbooks.Runbooks...),
		ByAttach:    map[string]int{},
		Runs:        snap.Runbooks.Runs,
		RunOutcomes: map[string]int{},
	}
	sort.SliceStable(f.Runbooks, func(i, j int) bool {
		return strings.ToLower(f.Runbooks[i].Name) < strings.ToLower(f.Runbooks[j].Name)
	})

	covered := map[string]bool{}
	for _, rb := range f.Runbooks {
		if rb.Enabled {
			f.Enabled++
		}
		if rb.Automation.active() {
			f.Automated++
		}
		f.Steps += len(rb.Steps)
		f.ByAttach[rb.AttachKind]++
		if key := strings.TrimSpace(rb.AttachConditionKey); key != "" {
			covered[strings.ToLower(key)] = true
		}
	}

	if snap.available(srcRbAlerts) {
		seen := map[string]bool{}
		for _, a := range snap.Runbooks.Alerts {
			key := strings.TrimSpace(a.Condition)
			if key == "" || seen[strings.ToLower(key)] || covered[strings.ToLower(key)] {
				continue
			}
			seen[strings.ToLower(key)] = true
			f.UncoveredConditions = append(f.UncoveredConditions, key)
		}
		sort.Strings(f.UncoveredConditions)
	}

	for _, run := range f.Runs {
		f.RunOutcomes[run.Outcome]++
	}
	return f
}

func rbKPIs(b *buildCtx) []KPI {
	f := analyzeRunbooks(b.snap)
	if !f.OK {
		return nil
	}
	if !b.snap.Subject.empty() {
		if len(f.Runbooks) != 1 {
			return nil
		}
		rb := f.Runbooks[0]
		return []KPI{
			{Label: "Steps", Value: strconv.Itoa(len(rb.Steps))},
			{Label: "Required steps", Value: strconv.Itoa(rbRequired(rb))},
			{Label: "Status", Value: rbEnabledLabel(rb.Enabled), Status: rbEnabledStatus(rb.Enabled)},
			{Label: "Unattended", Value: rbAutomationLabel(rb.Automation)},
		}
	}
	out := []KPI{
		{Label: "Runbooks", Value: strconv.Itoa(len(f.Runbooks))},
		{Label: "Enabled", Value: strconv.Itoa(f.Enabled)},
		{Label: "Run unattended", Value: strconv.Itoa(f.Automated)},
	}
	if b.snap.available(srcRbAlerts) {
		out = append(out, KPI{
			Label:  "Alert conditions with no runbook",
			Value:  strconv.Itoa(len(f.UncoveredConditions)),
			Status: rbGapStatus(len(f.UncoveredConditions)),
		})
	}
	return out
}

func rbFindings(b *buildCtx) []Finding {
	f := analyzeRunbooks(b.snap)
	if !f.OK {
		return nil
	}
	var out []Finding

	if !b.snap.Subject.empty() && len(f.Runbooks) == 1 {
		rb := f.Runbooks[0]
		if len(rb.Steps) == 0 {
			out = append(out, Finding{
				Severity: SevHigh, Title: "This runbook has no steps",
				Detail:         rb.Name + " is attached but carries no procedure, so following it does nothing.",
				Recommendation: "Add the steps somebody would actually take, or remove the runbook.",
			})
		}
		if !rb.Enabled {
			out = append(out, Finding{
				Severity: SevMedium, Title: "This runbook is disabled",
				Detail:         rb.Name + " will not be offered when its attachment fires.",
				Recommendation: "Enable it, or say in its description why it is kept.",
			})
		}
		if err := strings.TrimSpace(rb.Automation.LastError); err != "" {
			out = append(out, Finding{
				Severity: SevHigh, Title: "The last unattended run failed",
				Detail:         err,
				Recommendation: "Fix the cause, then run it by hand once to confirm.",
				Evidence:       rb.Name,
			})
		}
		return out
	}

	if len(f.UncoveredConditions) > 0 {
		out = append(out, Finding{
			Severity: rbGapSeverity(len(f.UncoveredConditions)),
			Title:    "Alert conditions with no runbook",
			Detail: strconv.Itoa(len(f.UncoveredConditions)) + " alert conditions can fire with no written " +
				"procedure behind them: " + rbJoin(f.UncoveredConditions, 8) + ".",
			Recommendation: "Attach a runbook to the conditions that wake somebody out of hours first.",
		})
	}
	if empty := rbEmptyRunbooks(f.Runbooks); len(empty) > 0 {
		out = append(out, Finding{
			Severity: SevMedium, Title: "Runbooks with no steps",
			Detail:         rbJoin(empty, 8) + " carry no procedure, so following them does nothing.",
			Recommendation: "Add the steps, or remove the runbook so the list stays honest.",
		})
	}
	if failing := rbFailingAutomation(f.Runbooks); len(failing) > 0 {
		out = append(out, Finding{
			Severity: SevHigh, Title: "Unattended runs are failing",
			Detail:         rbJoin(failing, 8) + " last failed when run unattended.",
			Recommendation: "An automation that fails silently is worse than none — fix or disable it.",
		})
	}
	return out
}

func rbLead(b *buildCtx) string {
	f := analyzeRunbooks(b.snap)
	if !f.OK {
		return ""
	}
	if !b.snap.Subject.empty() && len(f.Runbooks) == 1 {
		rb := f.Runbooks[0]
		return rb.Name + " has " + strconv.Itoa(len(rb.Steps)) + " steps and " +
			strings.ToLower(rbAutomationLabel(rb.Automation)) + "."
	}
	if len(f.UncoveredConditions) == 0 {
		return "Every alert condition has a runbook behind it."
	}
	return strconv.Itoa(len(f.UncoveredConditions)) + " alert conditions can fire with no runbook behind them."
}

func buildRunbooks(b *buildCtx) []Section {
	snap := b.snap
	f := analyzeRunbooks(snap)

	if !f.OK {
		return []Section{section(Tactical, "Runbooks",
			resNotAssessed(snap, srcRbRunbooks, "The runbook list"))}
	}
	if !snap.Subject.empty() {
		return rbSubjectSections(b, f)
	}
	return rbEstateSections(b, f)
}

// rbSubjectSections is the per-runbook document: the thing somebody prints and
// puts in a binder, so it leads with what it is for and then the steps in order.
func rbSubjectSections(b *buildCtx, f rbFacts) []Section {
	if len(f.Runbooks) != 1 {
		return []Section{section(Tactical, "Runbooks",
			callout(StatusWarn, "Not found",
				"No runbook matched "+b.snap.Subject.label()+". It may have been deleted since the report was asked for."))}
	}
	rb := f.Runbooks[0]

	ta := section(Tactical, "What this runbook is for")
	if d := strings.TrimSpace(rb.Description); d != "" {
		ta.Blocks = append(ta.Blocks, lead(d))
	}
	ta.Blocks = append(ta.Blocks,
		kpis(rbKPIs(b)...),
		keyvalue(
			Pair{K: "Attached to", V: rbAttachLabel(rb)},
			Pair{K: "Status", V: rbEnabledLabel(rb.Enabled)},
			Pair{K: "Unattended", V: rbAutomationLabel(rb.Automation)},
			Pair{K: "Next unattended run", V: orDash(rb.Automation.NextRunAt, "")},
			Pair{K: "Last changed", V: orDash(rb.UpdatedAt, "")},
			Pair{K: "Author", V: orDash(rb.CreatedBy, "")},
		))
	if err := strings.TrimSpace(rb.Automation.LastError); err != "" {
		ta.Blocks = append(ta.Blocks, callout(StatusBad, "The last unattended run failed", err))
	}
	if fs := rbFindings(b); len(fs) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Findings"), findings(fs...))
	}

	op := section(Operational, "The procedure")
	if len(rb.Steps) == 0 {
		op.Blocks = append(op.Blocks, callout(StatusWarn, "No steps",
			"This runbook carries no procedure. Following it does nothing."))
	} else {
		op.Blocks = append(op.Blocks, rbStepTable(rb), pageBreak(), heading(2, "Step detail"))
		op.Blocks = append(op.Blocks, rbStepDetail(rb)...)
	}
	if runs := rbRunsFor(f, rb); len(runs) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Recent runs"), rbRunTable(runs))
	}
	return []Section{ta, op}
}

// rbEstateSections is the aggregate: coverage, automation and the gap.
func rbEstateSections(b *buildCtx, f rbFacts) []Section {
	snap := b.snap

	ta := section(Tactical, "Runbook coverage")
	ta.Blocks = append(ta.Blocks, para(
		"A runbook is what somebody follows at three in the morning. This report says how many exist, "+
			"what they are attached to, which of them run unattended, and — the part worth reading first — "+
			"which alert conditions can fire with nothing written down behind them."),
		kpis(rbKPIs(b)...))
	if len(f.ByAttach) > 0 {
		ta.Blocks = append(ta.Blocks, rbAttachTable(f))
	}
	if !snap.available(srcRbAlerts) {
		ta.Blocks = append(ta.Blocks, resNotAssessed(snap, srcRbAlerts, "The alert rule list"))
	} else if len(f.UncoveredConditions) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Alert conditions with no runbook"),
			bullets(f.UncoveredConditions...))
	}
	if fs := rbFindings(b); len(fs) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Findings"), findings(fs...))
	}

	op := section(Operational, "Every runbook")
	if len(f.Runbooks) == 0 {
		op.Blocks = append(op.Blocks, callout(StatusWarn, "No runbooks",
			"This instance holds no runbooks, so every incident is handled from memory."))
	} else {
		op.Blocks = append(op.Blocks, rbInventoryTable(f))
	}
	if len(f.Runs) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Recent runs"), rbRunTable(f.Runs))
	}
	return []Section{ta, op}
}

func rbStepTable(rb rbRunbookRow) Block {
	cols := []Column{
		{Label: "#", Align: "right", Width: 0.6},
		{Label: "Kind", Width: 1.4},
		{Label: "Step", Width: 5},
		{Label: "Required", Width: 1.2},
	}
	steps := rbOrderedSteps(rb)
	rows := make([][]Cell, 0, len(steps))
	for _, s := range steps {
		rows = append(rows, []Cell{
			cell(strconv.Itoa(s.Position)),
			pill(rbStepKindLabel(s.Kind), rbStepKindStatus(s.Kind)),
			cell(s.Title),
			cell(rbYesNo(s.Required)),
		})
	}
	return table(cols, rows, "Steps in order", "")
}

// rbStepDetail prints each step's body and what it invokes, because the table
// above is a contents page and the body is the instruction.
func rbStepDetail(rb rbRunbookRow) []Block {
	var out []Block
	for _, s := range rbOrderedSteps(rb) {
		out = append(out, heading(3, strconv.Itoa(s.Position)+". "+s.Title))
		var facts []string
		facts = append(facts, "Kind: "+rbStepKindLabel(s.Kind))
		if k := strings.TrimSpace(s.DiagnosticKey); k != "" {
			facts = append(facts, "Runs the diagnostic "+k)
		}
		if k := strings.TrimSpace(s.ActionKey); k != "" {
			facts = append(facts, "Performs the action "+k)
		}
		if s.Required {
			facts = append(facts, "Must be completed before the run can finish")
		}
		if body := strings.TrimSpace(s.Body); body != "" {
			out = append(out, para(body))
		}
		out = append(out, bullets(facts...))
	}
	return out
}

func rbAttachTable(f rbFacts) Block {
	cols := []Column{{Label: "Attached to", Width: 3}, {Label: "Runbooks", Align: "right", Width: 1}}
	kinds := make([]string, 0, len(f.ByAttach))
	for k := range f.ByAttach {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	rows := make([][]Cell, 0, len(kinds))
	for _, k := range kinds {
		rows = append(rows, []Cell{cell(rbAttachKindLabel(k)), cell(strconv.Itoa(f.ByAttach[k]))})
	}
	return table(cols, rows, "Runbooks by what they attach to", "")
}

func rbInventoryTable(f rbFacts) Block {
	cols := []Column{
		{Label: "Runbook", Width: 3},
		{Label: "Attached to", Width: 2.4},
		{Label: "Steps", Align: "right", Width: 0.8},
		{Label: "Status", Width: 1},
		{Label: "Unattended", Width: 2},
	}
	rows := make([][]Cell, 0, len(f.Runbooks))
	for _, rb := range f.Runbooks {
		rows = append(rows, []Cell{
			cell(rb.Name),
			cell(rbAttachLabel(rb)),
			cell(strconv.Itoa(len(rb.Steps))),
			pill(rbEnabledLabel(rb.Enabled), rbEnabledStatus(rb.Enabled)),
			cell(rbAutomationLabel(rb.Automation)),
		})
	}
	return table(cols, rows, "Every runbook on this instance", "")
}

func rbRunTable(runs []runbookRunRow) Block {
	cols := []Column{
		{Label: "Started", Width: 1.8},
		{Label: "Runbook", Width: 2.4},
		{Label: "Against", Width: 2},
		{Label: "By", Width: 1.6},
		{Label: "Outcome", Width: 1.2},
	}
	sorted := append([]runbookRunRow(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartedAt > sorted[j].StartedAt })
	if len(sorted) > 25 {
		sorted = sorted[:25]
	}
	rows := make([][]Cell, 0, len(sorted))
	for _, r := range sorted {
		rows = append(rows, []Cell{
			cell(r.StartedAt),
			cell(r.RunbookName),
			cell(orDash(r.SubjectName, "")),
			cell(orDash(r.StartedBy, "")),
			pill(rbOutcomeLabel(r.Outcome), rbOutcomeStatus(r.Outcome)),
		})
	}
	return table(cols, rows, "The most recent runs", "At most 25 are shown.")
}

// --- small helpers -----------------------------------------------------

func rbOrderedSteps(rb rbRunbookRow) []rbStepRow {
	steps := append([]rbStepRow(nil), rb.Steps...)
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Position < steps[j].Position })
	return steps
}

func rbRunsFor(f rbFacts, rb rbRunbookRow) []runbookRunRow {
	var out []runbookRunRow
	for _, r := range f.Runs {
		if strings.EqualFold(strings.TrimSpace(r.RunbookName), strings.TrimSpace(rb.Name)) {
			out = append(out, r)
		}
	}
	return out
}

func rbRequired(rb rbRunbookRow) int {
	n := 0
	for _, s := range rb.Steps {
		if s.Required {
			n++
		}
	}
	return n
}

func rbEmptyRunbooks(rows []rbRunbookRow) []string {
	var out []string
	for _, rb := range rows {
		if len(rb.Steps) == 0 {
			out = append(out, rb.Name)
		}
	}
	return out
}

func rbFailingAutomation(rows []rbRunbookRow) []string {
	var out []string
	for _, rb := range rows {
		if strings.TrimSpace(rb.Automation.LastError) != "" {
			out = append(out, rb.Name)
		}
	}
	return out
}

func rbJoin(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + " and " + strconv.Itoa(len(items)-max) + " more"
}

func rbYesNo(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func rbEnabledLabel(v bool) string {
	if v {
		return "Enabled"
	}
	return "Disabled"
}

func rbEnabledStatus(v bool) Status {
	if v {
		return StatusGood
	}
	return StatusWarn
}

func rbAutomationLabel(a rbAutomation) string {
	if !a.active() {
		return "Followed by a person"
	}
	var triggers []string
	if a.OnAlert {
		triggers = append(triggers, "when its alert fires")
	}
	if c := strings.TrimSpace(a.Cron); c != "" {
		triggers = append(triggers, "on the schedule "+c)
	}
	return "Runs as " + a.PrincipalName + " " + strings.Join(triggers, " and ")
}

func rbAttachLabel(rb rbRunbookRow) string {
	label := rbAttachKindLabel(rb.AttachKind)
	switch {
	case strings.TrimSpace(rb.AttachConditionKey) != "":
		return label + ": " + rb.AttachConditionKey
	case strings.TrimSpace(rb.AttachSubject) != "":
		return label + ": " + rb.AttachSubject
	}
	return label
}

func rbAttachKindLabel(kind string) string {
	switch kind {
	case "alert_condition":
		return "Alert condition"
	case "container":
		return "Container"
	case "stack":
		return "Stack"
	case "environment":
		return "Environment"
	case "":
		return "Nothing"
	}
	return rbHumaniseKind(kind)
}

// rbHumaniseKind titles an attach kind a plugin declared. The kinds are the
// plugins' own, so this report names none of them; short tokens are acronyms.
func rbHumaniseKind(kind string) string {
	words := strings.Fields(strings.ReplaceAll(kind, "_", " "))
	for i, w := range words {
		if len(w) <= 3 {
			words[i] = strings.ToUpper(w)
		} else if i == 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func rbStepKindLabel(kind string) string {
	switch kind {
	case "instruction":
		return "Instruction"
	case "diagnostic":
		return "Diagnostic"
	case "action":
		return "Action"
	}
	return kind
}

func rbStepKindStatus(kind string) Status {
	switch kind {
	case "diagnostic":
		return StatusInfo
	case "action":
		return StatusWarn
	}
	return StatusNeutral
}

func rbOutcomeLabel(outcome string) string {
	switch outcome {
	case "completed":
		return "Completed"
	case "abandoned":
		return "Abandoned"
	case "running":
		return "Running"
	}
	return orDash(outcome, "")
}

func rbOutcomeStatus(outcome string) Status {
	switch outcome {
	case "completed":
		return StatusGood
	case "abandoned":
		return StatusBad
	case "running":
		return StatusInfo
	}
	return StatusNeutral
}

func rbGapStatus(n int) Status {
	switch {
	case n == 0:
		return StatusGood
	case n <= 3:
		return StatusWarn
	}
	return StatusBad
}

func rbGapSeverity(n int) Severity {
	if n > 5 {
		return SevHigh
	}
	return SevMedium
}
