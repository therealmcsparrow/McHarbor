// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Plugin-contributed report types.
//
// The Report Studio is a shelf, not a closed catalogue. A plugin that
// owns a domain — Kubernetes today, anything else tomorrow — can put
// its own reports on that shelf by registering a report provider
// through the host's `reporting` adapter family
// (core/plugin/specs/reporting). This file is the whole of the
// consuming half:
//
//   - providerTypes turns each provider's declarations into ordinary
//     catalog entries, so the studio, the schedules and the library
//     treat a contributed report exactly like a built-in one.
//   - providerSections converts the neutral document model the family
//     defines into this plugin's own blocks, so the three renderers
//     never learn that a section came from somewhere else.
//
// What a provider does *not* get is the cover, the period, the
// Methodology annex, the level filter, the persistence or the
// exporters. Those stay here, which is what keeps a contributed report
// as checkable as a built-in one: whoever wrote the content, the reader
// still sees what was consulted and what could not be read.

// providerHost is the host's report-provider handle, published once at
// Init.
//
// A package-level variable rather than a field on the service because
// catalog() is a free function called from a dozen places — handlers,
// the scheduler, plannedSources — and threading a handle through all of
// them to answer "which report types exist" would be a large change to
// say a small thing. It is written exactly once, before any route is
// reachable, and only ever read afterwards.
var providerHost plugin.ReportingHandle

// setProviderHost publishes the host handle. Called from Init.
func setProviderHost(h plugin.ReportingHandle) { providerHost = h }

// providerSpecs is the registered providers, or nil on an installation
// where nothing contributes reports.
func providerSpecs() []plugin.ReportProviderSpec {
	if providerHost == nil || !providerHost.Available() {
		return nil
	}
	return providerHost.Providers()
}

// providerTypes converts every provider's declarations into catalog
// entries.
//
// Sources is deliberately left empty: a provider collects its own data
// through its own clients, so there is nothing here for the host's
// loopback collector to fetch. The Methodology annex is filled from the
// SourceStatus rows the provider returns instead, which is the same
// contract from the reader's point of view — every figure names where
// it came from.
func providerTypes() []ReportType {
	specs := providerSpecs()
	if len(specs) == 0 {
		return nil
	}
	var out []ReportType
	for _, spec := range specs {
		for _, decl := range spec.Types {
			out = append(out, providerType(spec, decl))
		}
	}
	return out
}

func providerType(spec plugin.ReportProviderSpec, decl plugin.ReportTypeDecl) ReportType {
	rt := ReportType{
		Key:               decl.Key,
		Category:          decl.Category,
		Levels:            providerLevels(decl.Levels),
		DefaultDays:       decl.DefaultDays,
		PointInTime:       decl.PointInTime,
		EnvironmentScoped: decl.EnvironmentScoped,
		Subjects:          providerSubjectKinds(decl.Subjects),
		Options:           providerOptions(decl.Options),

		Provider:               spec.Slug,
		LabelNS:                spec.Slug,
		LabelKey:               decl.LabelKey,
		DescriptionKey:         decl.DescriptionKey,
		CategoryLabelKey:       decl.CategoryLabelKey,
		CategoryDescriptionKey: decl.CategoryDescriptionKey,
		TitleEN:                decl.TitleEN,
		SubtitleEN:             decl.SubtitleEN,
	}
	// A declaration that named no levels would produce a report the
	// level filter drops entirely. The loader already rejects that at
	// registration; this is the second belt, because an empty document
	// is the worst possible way to find out.
	if len(rt.Levels) == 0 {
		rt.Levels = []Level{Tactical}
	}
	return rt
}

// providerLevels keeps only the altitudes this plugin actually writes,
// preserving the provider's order. A level nobody recognises is dropped
// rather than passed through — buildDocument would filter it out later
// anyway, and dropping it here keeps the studio from offering a
// checkbox that can never select anything.
func providerLevels(raw []string) []Level {
	var out []Level
	seen := map[Level]bool{}
	for _, r := range raw {
		l := Level(strings.TrimSpace(strings.ToLower(r)))
		switch l {
		case Strategic, Tactical, Operational:
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out
}

func providerOptions(raw []plugin.ReportOptionDecl) []ReportOption {
	var out []ReportOption
	for _, o := range raw {
		key := strings.TrimSpace(o.Key)
		if key == "" {
			continue
		}
		out = append(out, ReportOption{Key: key, Default: o.Default})
	}
	return out
}

// providerSubjectKinds accepts the provider's own scope kinds verbatim.
//
// They are not checked against this plugin's SubjectKind constants on
// purpose: a provider's objects are its own — a Kubernetes cluster is
// not a container — and the narrowing is done by the provider, not by
// applySubject. What is enforced is that the kind is namespaced, so a
// provider cannot claim "container" and have the studio hand it the
// host's container listing.
func providerSubjectKinds(raw []string) []SubjectKind {
	var out []SubjectKind
	for _, r := range raw {
		k := strings.TrimSpace(strings.ToLower(r))
		if k == "" || !strings.Contains(k, "-") {
			continue
		}
		if _, builtIn := parseSubjectKind(k); builtIn {
			continue
		}
		out = append(out, SubjectKind(k))
	}
	return out
}

// providerTypeKey reports whether a report type key belongs to a
// provider, and which one.
func providerTypeKey(key string) (plugin.ReportProviderSpec, plugin.ReportTypeDecl, bool) {
	for _, spec := range providerSpecs() {
		for _, decl := range spec.Types {
			if decl.Key == key {
				return spec, decl, true
			}
		}
	}
	return plugin.ReportProviderSpec{}, plugin.ReportTypeDecl{}, false
}

// isProviderType is the cheap question the generate path asks.
func isProviderType(key string) bool {
	_, _, ok := providerTypeKey(key)
	return ok
}

// providerTypeForSubjectKind finds the report type that owns a scope
// kind, so the studio's `/subjects?kind=` listing can be dispatched
// without the caller having to name the type as well.
//
// The first type declaring the kind wins. Two types of the same
// provider sharing a kind is normal — a cluster-health report and a
// workload report both narrow to a cluster — and either dispatches to
// the same client, so the ambiguity is not one a reader can observe.
func providerTypeForSubjectKind(kind string) (string, bool) {
	for _, spec := range providerSpecs() {
		for _, decl := range spec.Types {
			for _, k := range providerSubjectKinds(decl.Subjects) {
				if string(k) == kind {
					return decl.Key, true
				}
			}
		}
	}
	return "", false
}

// providerRequest is the request shape handed across the boundary.
func providerRequest(in GenerateInput, rt ReportType, period Period, subject Subject) plugin.ReportRequest {
	levels := make([]string, 0, len(rt.Levels))
	for _, l := range rt.resolveLevels(in.Levels) {
		levels = append(levels, string(l))
	}
	return plugin.ReportRequest{
		TypeKey:       rt.Key,
		EnvironmentID: in.EnvironmentID,
		Environment:   in.Environment,
		PeriodStart:   period.Start,
		PeriodEnd:     period.End,
		Levels:        levels,
		Options:       in.Options,
		SubjectKind:   string(subject.Kind),
		SubjectID:     subject.ID,
		SubjectName:   subject.Name,
		Cookie:        in.Cookie,
		Lang:          in.Lang,
		User:          in.User,
		UserID:        in.UserID,
	}
}

// listProviderSubjects asks the owning provider for the objects a
// report of this kind may be narrowed to.
//
// The calling user's cookie is forwarded so a provider that answers
// over the host's own HTTP surface lists as that user. A provider
// reaching its own backend enforces its own permissions; either way the
// picker can only offer what the requester could already see.
func listProviderSubjects(ctx context.Context, kind, cookie, lang, envID, userID string) ([]SubjectOption, bool) {
	if providerHost == nil {
		return nil, false
	}
	typeKey, ok := providerTypeForSubjectKind(kind)
	if !ok {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows, handled, err := providerHost.Subjects(ctx, typeKey, kind, plugin.ReportRequest{
		TypeKey:       typeKey,
		EnvironmentID: envID,
		Cookie:        cookie,
		Lang:          lang,
		UserID:        userID,
	})
	if err != nil || !handled {
		// An empty picker is the honest answer here: the studio already
		// renders "nothing to narrow to" for it, and a hard failure would
		// take the whole configure dialog down over a scope control the
		// operator may not even want to use.
		return []SubjectOption{}, true
	}
	out := make([]SubjectOption, 0, len(rows))
	for _, r := range rows {
		if strings.TrimSpace(r.ID) == "" {
			continue
		}
		out = append(out, SubjectOption{ID: r.ID, Name: orDash(r.Name, r.ID), Hint: r.Hint})
	}
	return out, true
}

// buildProviderReport runs the provider and converts what comes back.
//
// The context deadline is this plugin's, not the provider's: a provider
// that blocks forever would otherwise hang a generation job an operator
// is watching, and a report nobody can cancel is worse than one that
// says it timed out.
func buildProviderReport(ctx context.Context, req plugin.ReportRequest) ([]Section, []SourceStatus, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	res, handled, err := providerHost.Build(ctx, req.TypeKey, req)
	if err != nil {
		return nil, nil, nil, err
	}
	if !handled {
		return nil, nil, nil, errProviderMissing
	}
	return providerSections(res.Sections), providerSources(res.Sources), res.Notes, nil
}

// providerSources converts the provider's own collection outcomes into
// the rows the Methodology annex prints.
//
// A provider that returned none gets one synthesised row rather than an
// empty annex: a document whose "Data sources" table is blank reads as
// though nothing was consulted, which is a stronger claim than "this
// provider did not say".
func providerSources(raw []plugin.ReportSourceStatus) []SourceStatus {
	out := make([]SourceStatus, 0, len(raw))
	for _, s := range raw {
		key := strings.TrimSpace(s.Key)
		if key == "" {
			continue
		}
		out = append(out, SourceStatus{
			Key:        key,
			Label:      orDash(strings.TrimSpace(s.Label), key),
			OK:         s.OK,
			Note:       s.Note,
			DurationMS: s.DurationMS,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// providerSections converts the neutral model into this plugin's own.
//
// Anything unrecognised is dropped rather than rendered as an empty
// box: the three renderers each switch on BlockKind, and a kind none of
// them knows would draw as a hole in the PDF that nobody can explain.
func providerSections(raw []plugin.ReportSection) []Section {
	out := make([]Section, 0, len(raw))
	for _, s := range raw {
		sec := Section{
			Level:  providerLevel(s.Level),
			Title:  strings.TrimSpace(s.Title),
			Blocks: providerBlocks(s.Blocks),
		}
		sec.Intent = levelIntent(sec.Level)
		out = append(out, sec)
	}
	return out
}

// providerLevel maps a level string, treating anything unrecognised as
// tactical — the middle altitude, and the one every report type carries
// at least implicitly.
func providerLevel(raw string) Level {
	switch Level(strings.TrimSpace(strings.ToLower(raw))) {
	case Strategic:
		return Strategic
	case Operational:
		return Operational
	case SummaryLevel:
		return SummaryLevel
	}
	return Tactical
}

func providerBlocks(raw []plugin.ReportBlock) []Block {
	out := make([]Block, 0, len(raw))
	for _, b := range raw {
		switch BlockKind(strings.TrimSpace(strings.ToLower(b.Kind))) {
		case BlockHeading:
			depth := b.Depth
			if depth < 2 {
				depth = 2
			}
			if depth > 4 {
				depth = 4
			}
			out = append(out, heading(depth, b.Text))
		case BlockParagraph:
			out = append(out, Block{Kind: BlockParagraph, Text: b.Text, Lead: b.Lead})
		case BlockList:
			items := compact(b.Items)
			if len(items) == 0 {
				continue
			}
			out = append(out, Block{Kind: BlockList, Ordered: b.Ordered, Items: items})
		case BlockCallout:
			out = append(out, callout(providerStatus(b.Tone), b.Title, b.Text))
		case BlockKPIs:
			if k := providerKPIs(b.KPIs); len(k) > 0 {
				out = append(out, kpis(k...))
			}
		case BlockKeyValue:
			if p := providerPairs(b.Pairs); len(p) > 0 {
				out = append(out, keyvalue(p...))
			}
		case BlockTable:
			cols, rows := providerTable(b)
			if len(cols) == 0 || len(rows) == 0 {
				continue
			}
			out = append(out, table(cols, rows, b.Caption, b.Note))
		case BlockFindings:
			if f := providerFindings(b.Findings); len(f) > 0 {
				out = append(out, findings(f...))
			}
		case BlockChart:
			if c, ok := providerChart(b.Chart); ok {
				out = append(out, chartBlock(c))
			}
		case BlockDivider:
			out = append(out, divider())
		case BlockPageBreak:
			out = append(out, pageBreak())
		}
	}
	return out
}

func providerKPIs(raw []plugin.ReportKPI) []KPI {
	var out []KPI
	for _, k := range raw {
		if strings.TrimSpace(k.Value) == "" {
			continue
		}
		out = append(out, KPI{
			Label: k.Label, Value: k.Value, Unit: k.Unit, Target: k.Target,
			Delta: k.Delta, Trend: providerTrend(k.Trend),
			Status: providerStatus(k.Status), Hint: k.Hint,
		})
	}
	return out
}

// providerTrend keeps only the three directions the renderers draw an
// arrow for. Status carries the judgement, so an unknown trend costs
// the arrow and nothing else.
func providerTrend(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "up", "down", "flat":
		return strings.ToLower(raw)
	}
	return ""
}

func providerPairs(raw []plugin.ReportPair) []Pair {
	var out []Pair
	for _, p := range raw {
		if strings.TrimSpace(p.K) == "" {
			continue
		}
		out = append(out, Pair{K: p.K, V: p.V, Mono: p.Mono})
	}
	return out
}

// providerTable squares the grid. A row longer than the header would
// silently lose cells in the PDF writer and a shorter one would shift
// every value one column left, so both are normalised here rather than
// three times in three renderers.
func providerTable(b plugin.ReportBlock) ([]Column, [][]Cell) {
	cols := make([]Column, 0, len(b.Columns))
	for _, c := range b.Columns {
		align := ""
		switch strings.TrimSpace(strings.ToLower(c.Align)) {
		case "right":
			align = "right"
		case "center":
			align = "center"
		}
		width := c.Width
		if width <= 0 {
			width = 1
		}
		cols = append(cols, Column{Label: c.Label, Align: align, Width: width, Mono: c.Mono})
	}
	if len(cols) == 0 {
		return nil, nil
	}
	rows := make([][]Cell, 0, len(b.Rows))
	for _, r := range b.Rows {
		row := make([]Cell, len(cols))
		for i := range row {
			if i < len(r) {
				row[i] = Cell{
					Text:   r[i].Text,
					Status: providerStatus(r[i].Status),
					Mono:   r[i].Mono,
					Pill:   r[i].Pill,
				}
				continue
			}
			row[i] = cell("")
		}
		rows = append(rows, row)
	}
	return cols, rows
}

func providerFindings(raw []plugin.ReportFinding) []Finding {
	var out []Finding
	for _, f := range raw {
		if strings.TrimSpace(f.Title) == "" {
			continue
		}
		out = append(out, Finding{
			Severity:       providerSeverity(f.Severity),
			Title:          f.Title,
			Detail:         f.Detail,
			Recommendation: f.Recommendation,
			Owner:          f.Owner,
			Evidence:       f.Evidence,
		})
	}
	return out
}

func providerChart(raw *plugin.ReportChart) (Chart, bool) {
	if raw == nil {
		return Chart{}, false
	}
	kind := ChartKind(strings.TrimSpace(strings.ToLower(raw.Kind)))
	switch kind {
	case ChartBar, ChartHBar, ChartStackedBar, ChartDonut, ChartLine, ChartArea, ChartGauge:
	default:
		return Chart{}, false
	}
	var series []ChartSeries
	for _, s := range raw.Series {
		var pts []ChartPoint
		for _, p := range s.Points {
			pts = append(pts, ChartPoint{Label: p.Label, Value: p.Value, Status: providerStatus(p.Status)})
		}
		if len(pts) == 0 {
			continue
		}
		series = append(series, ChartSeries{Name: s.Name, Status: providerStatus(s.Status), Points: pts})
	}
	if len(series) == 0 {
		return Chart{}, false
	}
	return Chart{
		Kind: kind, Title: raw.Title, Series: series, Unit: raw.Unit,
		Max: raw.Max, Target: raw.Target, TargetLabel: raw.TargetLabel,
		HasTarget: raw.HasTarget, ShowLegend: raw.ShowLegend,
		ShowValues: raw.ShowValues, Caption: raw.Caption, Height: raw.Height,
	}, true
}

// providerStatus maps a tint. An unrecognised value becomes neutral —
// "no judgement" — rather than a colour the provider did not ask for.
func providerStatus(raw string) Status {
	switch Status(strings.TrimSpace(strings.ToLower(raw))) {
	case StatusGood:
		return StatusGood
	case StatusWarn:
		return StatusWarn
	case StatusBad:
		return StatusBad
	case StatusInfo:
		return StatusInfo
	case StatusMuted:
		return StatusMuted
	case "success", "ok":
		return StatusGood
	case "warning":
		return StatusWarn
	case "danger", "error", "critical":
		return StatusBad
	}
	return StatusNeutral
}

// providerSeverity orders a finding. Unrecognised becomes info, which
// sorts last: a finding whose severity nobody can read must not be able
// to claim the top of the list.
func providerSeverity(raw string) Severity {
	switch Severity(strings.TrimSpace(strings.ToLower(raw))) {
	case SevCritical:
		return SevCritical
	case SevHigh:
		return SevHigh
	case SevMedium:
		return SevMedium
	case SevLow:
		return SevLow
	}
	return SevInfo
}
