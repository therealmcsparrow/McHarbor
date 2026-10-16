// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"sort"
	"strings"
)

// Two pseudo-levels bracket the content levels. Summary always leads —
// board practice is a one-page summary a reader can act on without
// turning a page — and Annex always closes, carrying the methodology and
// glossary that make the numbers challengeable.
const (
	SummaryLevel Level = "summary"
	AnnexLevel   Level = "annex"
)

// buildCtx is everything a builder needs. Builders are pure: same
// context in, same document out, which is what makes a report
// reproducible and a diff between two periods meaningful.
type buildCtx struct {
	rt     ReportType
	meta   Meta
	snap   *Snapshot
	facts  Facts
	score  *Scorecard
	opts   map[string]bool
	levels map[Level]bool
	nar    *narrator
}

func (b *buildCtx) wants(l Level) bool { return b.levels[l] }

func (b *buildCtx) opt(key string) bool { return b.opts[key] }

// sectionBuilder produces the content sections for one report type.
type sectionBuilder func(b *buildCtx) []Section

// builders is the dispatch table. A report type with no builder is a
// catalog entry that was never finished, and the service rejects it
// rather than emitting an empty document.
func builders() map[string]sectionBuilder {
	table := map[string]sectionBuilder{
		TypeExecutiveOverview:  buildExecutiveOverview,
		TypeSecurityCompliance: buildSecurityCompliance,
		TypeIncidentProblem:    buildIncidentProblem,
		TypeAvailability:       buildAvailability,
		TypeChangeDelivery:     buildChangeDelivery,
		TypeEnvironmentHealth:  buildEnvironmentHealth,
		TypeCapacity:           buildCapacity,
		TypeBackupRecovery:     buildBackupRecovery,
		TypeImageSupplyChain:   buildImageSupplyChain,
		TypeAccessReview:       buildAccessReview,
		TypeWorkloadInventory:  buildWorkloadInventory,
		TypePlatformInventory:  buildPlatformInventory,

		TypeUptimeAvailability:   buildUptimeAvailability,
		TypeContainerService:     buildContainerService,
		TypeHealthcheckQuality:   buildHealthcheckQuality,
		TypeStabilityFlapping:    buildStabilityFlapping,
		TypeStackPortfolio:       buildStackPortfolio,
		TypeOperationalReadiness: buildOperationalReadiness,

		TypeUserActivity:      buildUserActivity,
		TypePermissionMatrix:  buildPermissionMatrix,
		TypeCredentialHygiene: buildCredentialHygiene,
		TypeAuditAssurance:    buildAuditAssurance,

		TypePatchCurrency:       buildPatchCurrency,
		TypeAutomationSchedules: buildAutomationSchedules,
		TypeAlertingCoverage:    buildAlertingCoverage,
		TypeAgentFleet:          buildAgentFleet,
		TypeNetworkConnectivity: buildNetworkConnectivity,
		TypeVolumeDataEstate:    buildVolumeDataEstate,

		TypeResourceEfficiency: buildResourceEfficiency,
		TypeCostAllocation:     buildCostAllocation,

		TypeEnvironmentCompare: buildEnvironmentCompare,
		TypeDataProtection:     buildDataProtection,

		// Platform-specific types are absent on purpose: they arrive
		// through providers.go, which dispatches on the provider's own
		// key rather than through this table.
	}
	for _, fam := range reportFamilies() {
		for key, fn := range fam.builders {
			table[key] = fn
		}
	}
	return table
}

// buildDocument assembles the whole report: summary, the requested
// content levels, then the annexes.
func buildDocument(b *buildCtx) *Document {
	doc := &Document{Meta: b.meta, Sources: b.snap.Sources}
	if b.rt.hasLevel(Strategic) {
		doc.Scorecard = b.score
	}

	doc.Sections = append(doc.Sections, buildSummary(b))

	fn, ok := builders()[b.rt.Key]
	if ok {
		for _, s := range insertFacetSections(b, fn(b)) {
			if !b.wants(s.Level) || !s.hasContent() {
				continue
			}
			doc.Sections = append(doc.Sections, s)
		}
	}

	doc.Sections = append(doc.Sections, buildAnnex(b))
	return doc
}

// buildAdHocDocument assembles a model-authored report.
//
// The model owns the content sections, including the summary. The host
// owns everything that makes the document checkable: meta, source
// status, scorecard and annexes. That split is the whole safety story —
// a reader can always see what was consulted and what could not be
// traced, whoever wrote the prose.
func buildAdHocDocument(b *buildCtx, secs []Section, unv []unverified) *Document {
	doc := &Document{Meta: b.meta, Sources: b.snap.Sources}
	// doc.Scorecard is deliberately never set on the ad-hoc path.
	//
	// buildScorecard runs on whatever sources the planner happened to
	// pick for one question, and unassessed domains are excluded from
	// the mean — so a question about, say, API keys can still print
	// "92 / 100 · Strong" on the cover as if it summed up estate-wide
	// posture. The 32 built-in types curate their Sources precisely
	// enough that the number means something; ad-hoc has no such
	// curation, and both PDF and DOCX print the cover score
	// unconditionally whenever Document.Scorecard is non-nil, regardless
	// of whether any section actually contains a BlockScorecard. A
	// posture score derived from an arbitrary subset, shown on a board
	// cover, is worse than no score at all. This is a deliberate product
	// decision, not an oversight — do not "fix" it back without first
	// solving the arbitrary-subset problem.

	// A model section labelled "summary" anywhere but the true lead
	// produces two "Executive summary" sections and two TOC entries.
	// Whichever section ends up leading — secs[0] if it already claims
	// summary, otherwise the deterministic one prepended below — is the
	// only one allowed to keep that level.
	leadIsModelSummary := len(secs) > 0 && secs[0].Level == SummaryLevel
	for i, s := range secs {
		if s.Level != SummaryLevel {
			continue
		}
		if leadIsModelSummary && i == 0 {
			continue
		}
		secs[i].Level = Tactical
	}

	// The summary must lead. A model that omitted it gets the
	// deterministic lead instead of the report opening mid-argument.
	if !leadIsModelSummary {
		summary := Section{Level: SummaryLevel, Title: "Executive summary"}
		summary.Blocks = append(summary.Blocks, lead(b.nar.summaryLead(b)))
		secs = append([]Section{summary}, secs...)
	}

	// The plan's Levels gate which altitudes the model was told it may
	// write at (see authorSystemPrompt), but sanitiseSections has no
	// access to the plan, so the same gate is enforced here — the way
	// buildDocument enforces it per-section via b.wants for the 32 fixed
	// types. The summary always survives regardless of b.levels, the
	// same way buildDocument always includes buildSummary().
	filtered := secs[:0]
	for _, s := range secs {
		if s.Level == SummaryLevel || b.wants(s.Level) {
			filtered = append(filtered, s)
		}
	}
	secs = filtered

	if len(unv) > 0 {
		note := callout(StatusWarn, "Unverified figures",
			plural(len(unv), "figure in this report could not be traced",
				"figures in this report could not be traced")+
				" to a collected source. They are listed in the methodology annex.")
		secs[0].Blocks = append([]Block{note}, secs[0].Blocks...)
	}

	doc.Sections = append(doc.Sections, secs...)
	doc.Sections = append(doc.Sections, buildAdHocAnnex(b, unv))
	return doc
}

// buildProviderDocument assembles a report a plugin built.
//
// Same split as the ad-hoc path, and for the same reason: the provider
// owns the content, this plugin owns everything that makes the document
// checkable — cover, level filter, source status, annex. A contributed
// report is therefore exactly as auditable as a built-in one, whoever
// wrote the sections.
//
// doc.Scorecard is never set. The posture score is computed by
// buildScorecard from this plugin's own collected snapshot, and a
// provider's report is built from data that snapshot never saw; printing
// a cover score derived from an empty collection would be a number about
// nothing. A provider that wants a score of its own renders it as KPIs
// or a gauge inside its own sections, where the reader can see what it
// was computed from.
func buildProviderDocument(b *buildCtx, secs []Section, notes []string) *Document {
	doc := &Document{Meta: b.meta, Sources: b.snap.Sources}

	// Only the lead section may claim the summary level. A provider that
	// labelled three sections "summary" would otherwise produce three
	// "Executive summary" entries in the contents, and a reader would
	// have no way to tell which one leads.
	leadIsSummary := len(secs) > 0 && secs[0].Level == SummaryLevel
	for i := range secs {
		if secs[i].Level == SummaryLevel && !(leadIsSummary && i == 0) {
			secs[i].Level = Tactical
		}
	}

	// A provider that wrote no lead gets a deterministic one rather than
	// a document that opens mid-argument. It is deliberately plain: this
	// plugin knows nothing about the provider's subject matter, and a
	// generated sentence that guessed at it would be worse than one that
	// simply states what the document is.
	if !leadIsSummary {
		summary := Section{Level: SummaryLevel, Title: "Executive summary", Intent: levelIntent(SummaryLevel)}
		summary.Blocks = append(summary.Blocks, lead(providerSummaryLead(b)))
		secs = append([]Section{summary}, secs...)
	}

	if gaps := b.missingSources(); len(gaps) > 0 {
		secs[0].Blocks = append(secs[0].Blocks, callout(StatusInfo, "Coverage limits",
			"This report could not read "+joinAnd(gaps)+". Those areas are reported as "+
				"unavailable rather than as zero. See Methodology for detail."))
	}

	for _, s := range secs {
		if s.Level != SummaryLevel && !b.wants(s.Level) {
			continue
		}
		if !s.hasContent() {
			continue
		}
		doc.Sections = append(doc.Sections, s)
	}

	doc.Sections = append(doc.Sections, buildProviderAnnex(b, notes))
	return doc
}

// providerSummaryLead is the one sentence a provider-built report opens
// with when the provider wrote no lead of its own.
func providerSummaryLead(b *buildCtx) string {
	scope := b.meta.Environment
	if b.meta.SubjectLabel != "" {
		scope = b.meta.SubjectLabel
	}
	period := "as at " + b.meta.PeriodEnd.Format("2 January 2006")
	if !b.meta.PeriodStart.Equal(b.meta.PeriodEnd) {
		period = "for the period " + b.meta.PeriodStart.Format("2 January 2006") +
			" to " + b.meta.PeriodEnd.Format("2 January 2006")
	}
	return b.meta.Title + " covering " + orDash(scope, "the whole estate") + ", " + period +
		". Every figure below is derived from the sources listed in the methodology annex."
}

// buildProviderAnnex is buildAnnex plus whatever caveats the provider
// asked to have printed against the whole document.
func buildProviderAnnex(b *buildCtx, notes []string) Section {
	s := buildAnnex(b)
	items := compact(notes)
	if len(items) == 0 {
		return s
	}
	s.Blocks = append(s.Blocks, heading(2, "Notes from the reporting source"), bullets(items...))
	return s
}

// buildSummary is the one page a director reads.
//
// Board-reporting practice, and it holds up: lead with where we stand,
// name what moved, name what needs a decision, and keep it to bullets.
// Everything else in the document exists to support these few lines.
func buildSummary(b *buildCtx) Section {
	s := Section{Level: SummaryLevel, Title: "Executive summary"}

	s.Blocks = append(s.Blocks, lead(b.nar.summaryLead(b)))

	if b.score != nil && b.score.Overall >= 0 && b.rt.hasLevel(Strategic) {
		s.Blocks = append(s.Blocks, Block{Kind: BlockScorecard})
	}

	if k := b.headlineKPIs(); len(k) > 0 {
		s.Blocks = append(s.Blocks, kpis(k...))
	}

	if moves := b.nar.whatMoved(b); len(moves) > 0 {
		s.Blocks = append(s.Blocks,
			heading(3, "What changed this period"),
			bullets(moves...))
	}

	if f := b.topFindings(4); len(f) > 0 {
		s.Blocks = append(s.Blocks,
			heading(3, "Matters requiring attention"),
			findings(f...))
	}

	if recs := b.nar.recommendations(b); len(recs) > 0 {
		s.Blocks = append(s.Blocks,
			heading(3, "Recommended actions"),
			numbered(recs...))
	}

	if gaps := b.missingSources(); len(gaps) > 0 {
		s.Blocks = append(s.Blocks, callout(StatusInfo, "Coverage limits",
			"This report could not read "+joinAnd(gaps)+". Those areas are marked "+
				"“not assessed” rather than scored, and are excluded from the overall "+
				"figure. See Methodology for detail."))
	}
	return s
}

// headlineKPIs picks the four to six numbers that belong on page one.
// Which ones depends on what the report is about, but the shape does
// not: a value, what it is measured against, and whether that is good.
func (b *buildCtx) headlineKPIs() []KPI {
	f := b.facts
	var out []KPI
	if fam, ok := familyFor(b.rt.Key); ok && fam.kpis != nil {
		if k := fam.kpis(b); k != nil {
			if len(k) > 6 {
				k = k[:6]
			}
			return k
		}
	}

	add := func(k KPI) {
		if k.Value != "" && k.Value != "—" {
			out = append(out, k)
		}
	}

	// The measured-availability reports share one headline row, built from
	// the history rather than from the current state. It is listed first so a
	// type that appears in both this case and a later one gets the stronger
	// evidence.
	switch b.rt.Key {
	case TypeUptimeAvailability, TypeHealthcheckQuality, TypeStabilityFlapping,
		TypeContainerService, TypeOperationalReadiness, TypeStackPortfolio:
		if k := availabilityKPIs(f.Availability); len(k) > 0 {
			for _, item := range k {
				add(item)
			}
			if len(out) > 6 {
				out = out[:6]
			}
			return out
		}
	}

	switch b.rt.Key {
	case TypeSecurityCompliance, TypeImageSupplyChain:
		if f.Security.Assessed {
			add(KPI{Label: "Critical findings", Value: fmtInt(f.Security.Critical),
				Status: countStatus(f.Security.Critical, 0, 0), Hint: "Open, highest severity"})
			add(KPI{Label: "High findings", Value: fmtInt(f.Security.High),
				Status: countStatus(f.Security.High, 0, 5)})
			add(KPI{Label: "Scan coverage", Value: fmtPercent(f.Security.CoveragePercent),
				Target: "100%", Status: thresholdStatus(f.Security.CoveragePercent, 90, 60)})
			add(KPI{Label: "Last scan", Value: fmtDays(f.Security.LastScanAge),
				Status: freshnessStatus(f.Security.LastScanAge, 7, 30)})
		}
	case TypeAvailability, TypeIncidentProblem:
		// Measured uptime leads when history exists: it is the only line here
		// that describes the period rather than the moment.
		if a := f.Availability; a.Assessed {
			add(KPI{Label: "Measured uptime", Value: fmtUptime(a.UptimePercent),
				Target: fmtDecimal(a.TargetPercent, 1) + "%",
				Status: thresholdStatus(a.UptimePercent, a.TargetPercent, 99)})
			add(KPI{Label: "Observed downtime", Value: fmtDuration(a.DowntimeMinutes),
				Status: countStatus(a.WithDowntime, 0, 2)})
		}
		add(KPI{Label: "Workloads running", Value: fmtInt(f.Containers.Running),
			Target: fmtInt(f.Containers.Total) + " total"})
		add(KPI{Label: "Unhealthy", Value: fmtInt(f.Containers.Unhealthy),
			Status: countStatus(f.Containers.Unhealthy, 0, 0)})
		add(KPI{Label: "Unexpected stops", Value: fmtInt(f.Reliability.Crashes),
			Status: countStatus(f.Reliability.Crashes, 0, 5)})
		if f.Reliability.RecoverySamples > 0 {
			add(KPI{Label: "Mean recovery", Value: fmtDuration(f.Reliability.MeanRecoveryMinutes),
				Target: "< 60 min", Hint: fmt.Sprintf("from %d observed recoveries", f.Reliability.RecoverySamples),
				Status: invThresholdStatus(f.Reliability.MeanRecoveryMinutes, 60, 240)})
		}
	case TypeChangeDelivery:
		add(KPI{Label: "Deployments", Value: fmtInt(f.Delivery.Deployments)})
		add(KPI{Label: "Per week", Value: fmtDecimal(f.Delivery.DeploymentsPerWeek, 1),
			Target: "≥ 7 (elite)", Status: thresholdStatus(f.Delivery.DeploymentsPerWeek, 7, 1)})
		add(KPI{Label: "Change failure rate", Value: fmtPercent(f.Delivery.ChangeFailureRate),
			Target: "≤ 5%", Status: invThresholdStatus(f.Delivery.ChangeFailureRate, 5, 20)})
		if f.Delivery.PromotionsTotal > 0 {
			add(KPI{Label: "Promotions", Value: fmtInt(f.Delivery.PromotionsSucceeded) + " / " + fmtInt(f.Delivery.PromotionsTotal)})
		}
	case TypeBackupRecovery, TypeDataProtection:
		add(KPI{Label: "Backup coverage", Value: fmtPercent(f.Resilience.CoveragePercent),
			Target: "100%", Status: thresholdStatus(f.Resilience.CoveragePercent, 90, 60)})
		add(KPI{Label: "Run success rate", Value: fmtPercent(f.Resilience.SuccessRate),
			Target: "100%", Status: thresholdStatus(f.Resilience.SuccessRate, 98, 90)})
		add(KPI{Label: "Last good backup", Value: fmtDays(f.Resilience.LastSuccessAge),
			Status: freshnessStatus(f.Resilience.LastSuccessAge, 2, 7)})
		add(KPI{Label: "Archive volume", Value: fmtBytes(f.Resilience.TotalArchiveBytes)})
	case TypeCapacity, TypeResourceEfficiency, TypeCostAllocation:
		add(KPI{Label: "CPU in use", Value: fmtPercent(f.Capacity.CPUPercent),
			Status: invThresholdStatus(f.Capacity.CPUPercent, 75, 90)})
		add(KPI{Label: "Memory in use", Value: fmtPercent(f.Capacity.MemPercent),
			Status: invThresholdStatus(f.Capacity.MemPercent, 75, 90)})
		add(KPI{Label: "Headroom", Value: fmtPercent(f.Capacity.HeadroomPercent),
			Target: "≥ 25%", Status: thresholdStatus(f.Capacity.HeadroomPercent, 25, 15)})
		add(KPI{Label: "Reclaimable", Value: fmtBytes(f.Images.ReclaimableBytes)})
	case TypeAccessReview, TypePermissionMatrix, TypeCredentialHygiene, TypeUserActivity, TypeAuditAssurance:
		add(KPI{Label: "Active users", Value: fmtInt(f.Access.ActiveUsers),
			Target: fmtInt(f.Access.Users) + " total"})
		add(KPI{Label: "Administrators", Value: fmtInt(f.Access.Admins),
			Status: countStatus(f.Access.Admins, 3, 6), Hint: "Fewer is better"})
		add(KPI{Label: "Dormant 90 days", Value: fmtInt(f.Access.DormantUsers),
			Status: countStatus(f.Access.DormantUsers, 0, 3)})
		add(KPI{Label: "Stale API keys", Value: fmtInt(f.Access.APIKeysStale),
			Status: countStatus(f.Access.APIKeysStale, 0, 2)})
	default:
		add(KPI{Label: "Workloads", Value: fmtInt(f.Containers.Total),
			Hint: fmtInt(f.Containers.Running) + " running"})
		add(KPI{Label: "Unhealthy", Value: fmtInt(f.Containers.Unhealthy),
			Status: countStatus(f.Containers.Unhealthy, 0, 0)})
		if f.Security.Assessed {
			add(KPI{Label: "Critical findings", Value: fmtInt(f.Security.Critical),
				Status: countStatus(f.Security.Critical, 0, 0)})
		}
		if f.Capacity.Assessed && f.Capacity.MemTotalBytes > 0 {
			add(KPI{Label: "Headroom", Value: fmtPercent(f.Capacity.HeadroomPercent),
				Status: thresholdStatus(f.Capacity.HeadroomPercent, 25, 15)})
		}
		if f.Resilience.Assessed {
			add(KPI{Label: "Backup coverage", Value: fmtPercent(f.Resilience.CoveragePercent),
				Status: thresholdStatus(f.Resilience.CoveragePercent, 90, 60)})
		}
		add(KPI{Label: "Images", Value: fmtInt(f.Images.Total), Hint: fmtBytes(f.Images.TotalBytes)})
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// topFindings gathers the issues worth a director's attention, ordered
// by severity. The list is deliberately short: a summary that names ten
// problems names none.
func (b *buildCtx) topFindings(limit int) []Finding {
	f := b.facts
	var all []Finding
	if fam, ok := familyFor(b.rt.Key); ok && fam.findings != nil {
		all = append(all, fam.findings(b)...)
	}

	if f.Security.Critical > 0 {
		all = append(all, Finding{
			Severity: SevCritical,
			Title:    plural(f.Security.Critical, "critical vulnerability is open", "critical vulnerabilities are open"),
			Detail: "Critical findings are exploitable without privileges and are the first thing an " +
				"attacker looks for. They are concentrated in a small number of base images.",
			Recommendation: "Rebuild the affected images on a patched base and redeploy within the next change window.",
			Owner:          "Platform engineering",
		})
	}
	if len(f.Reliability.CrashLooping) > 0 {
		names := make([]string, 0, len(f.Reliability.CrashLooping))
		for _, c := range f.Reliability.CrashLooping {
			names = append(names, c.Label)
		}
		all = append(all, Finding{
			Severity:       SevHigh,
			Title:          plural(len(names), "workload is crash-looping", "workloads are crash-looping"),
			Detail:         "Affected: " + joinAnd(names) + ". A crash-looping workload consumes scheduling capacity and is not serving traffic.",
			Recommendation: "Inspect the container logs and the last image change; roll back if the loop began at a deployment.",
			Owner:          "Service owner",
		})
	}
	if f.Resilience.Assessed && f.Resilience.CoveragePercent < 60 {
		all = append(all, Finding{
			Severity: SevHigh,
			Title:    "Backup coverage is " + fmtPercent(f.Resilience.CoveragePercent) + " of running workloads",
			Detail: "Workloads without a backup plan cannot be restored from this platform. " +
				"Recovery would depend on rebuilding from source and on data held elsewhere.",
			Recommendation: "Agree which workloads are in scope for recovery, then create plans for the ones that are.",
			Owner:          "Platform engineering",
		})
	}
	if f.Resilience.Assessed && f.Resilience.LastSuccessAge > 7 {
		all = append(all, Finding{
			Severity:       SevHigh,
			Title:          "Last successful backup was " + fmtDays(f.Resilience.LastSuccessAge),
			Detail:         "The recovery point is older than the weekly cadence the schedule implies.",
			Recommendation: "Investigate the failing runs before the next reporting period.",
			Owner:          "Platform engineering",
		})
	}
	if f.Containers.Unhealthy > 0 {
		all = append(all, Finding{
			Severity:       SevMedium,
			Title:          plural(f.Containers.Unhealthy, "workload is failing its health check", "workloads are failing their health checks"),
			Detail:         "The container is running but reporting itself unhealthy, so it may be serving errors.",
			Recommendation: "Review the health-check command and the service's own logs.",
			Owner:          "Service owner",
		})
	}
	if f.Capacity.Assessed && f.Capacity.HeadroomPercent < 15 && f.Capacity.MemTotalBytes > 0 {
		all = append(all, Finding{
			Severity:       SevMedium,
			Title:          "Only " + fmtPercent(f.Capacity.HeadroomPercent) + " capacity headroom remains",
			Detail:         "At this level a single additional workload, or one traffic spike, can exhaust the host.",
			Recommendation: "Reclaim unused images and volumes, or plan additional capacity.",
			Owner:          "Platform engineering",
		})
	}
	if f.Access.Assessed && f.Access.DormantUsers > 0 {
		all = append(all, Finding{
			Severity:       SevLow,
			Title:          plural(f.Access.DormantUsers, "account has not signed in for 90 days", "accounts have not signed in for 90 days"),
			Detail:         "Dormant accounts keep their permissions and are rarely noticed if they are misused.",
			Recommendation: "Confirm each account is still needed; disable the rest.",
			Owner:          "Security",
		})
	}
	if f.Access.Assessed && f.Access.APIKeysStale > 0 {
		all = append(all, Finding{
			Severity:       SevLow,
			Title:          plural(f.Access.APIKeysStale, "API key is unused or has never been used", "API keys are unused or have never been used"),
			Detail:         "An unused credential is all risk and no benefit.",
			Recommendation: "Revoke keys with no recent use.",
			Owner:          "Security",
		})
	}
	if f.Security.Assessed && f.Security.CoveragePercent < 60 && f.Images.Total > 0 {
		all = append(all, Finding{
			Severity:       SevMedium,
			Title:          "Only " + fmtPercent(f.Security.CoveragePercent) + " of images have been scanned",
			Detail:         "Unscanned images are not known to be safe; they are simply unknown.",
			Recommendation: "Enable scheduled scanning so every image in use is assessed.",
			Owner:          "Platform engineering",
		})
	}

	sort.SliceStable(all, func(i, j int) bool {
		return severityRank(all[i].Severity) < severityRank(all[j].Severity)
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

func severityRank(s Severity) int {
	switch s {
	case SevCritical:
		return 0
	case SevHigh:
		return 1
	case SevMedium:
		return 2
	case SevLow:
		return 3
	default:
		return 4
	}
}

// missingSources lists the sources that did not answer, for the coverage
// callout on page one.
func (b *buildCtx) missingSources() []string {
	var out []string
	for _, s := range b.snap.Sources {
		if !s.OK {
			out = append(out, strings.ToLower(s.Label))
		}
	}
	sort.Strings(out)
	if len(out) > 5 {
		out = append(out[:5], fmt.Sprintf("%d more", len(out)-5))
	}
	return out
}

// buildAnnex closes the document with the material that makes it
// auditable: what was read, how the scores were computed, and what the
// terms mean.
func buildAnnex(b *buildCtx) Section {
	// No leading page break or repeated "Methodology" heading here: every
	// section already starts its own page and prints its own title, and
	// the pair produced a near-empty page followed by two contents
	// entries that said the same thing.
	s := Section{Level: AnnexLevel, Title: "Methodology and notes"}

	s.Blocks = append(s.Blocks, para(
		"This report was generated by McHarbor from the platform's own records at the time "+
			"shown on the cover. Every figure is derived from the sources listed below, read "+
			"through the same permission model as the McHarbor interface — it contains nothing "+
			"the requesting user could not already see."))

	// Sources table: what answered, what did not, and how long it took.
	rows := make([][]Cell, 0, len(b.snap.Sources))
	src := append([]SourceStatus(nil), b.snap.Sources...)
	sort.Slice(src, func(i, j int) bool { return src[i].Label < src[j].Label })
	for _, st := range src {
		status, note := pill("Read", StatusGood), st.Note
		if !st.OK {
			status = pill("Not available", StatusWarn)
			if note == "" {
				note = "no response"
			}
		}
		rows = append(rows, []Cell{cell(st.Label), status, cell(note), cell(fmtInt(int(st.DurationMS)) + " ms")})
	}
	if len(rows) > 0 {
		s.Blocks = append(s.Blocks, heading(2, "Data sources"), table(
			[]Column{
				{Label: "Source", Width: 3},
				{Label: "Outcome", Width: 1.6},
				{Label: "Note", Width: 3.4},
				{Label: "Elapsed", Width: 1.2, Align: "right"},
			}, rows, "",
			"A source marked “not available” is excluded from every figure in this report; it is never treated as a zero."))
	}

	s.Blocks = append(s.Blocks, facetCoverageBlocks(b.snap)...)

	if b.score != nil && b.rt.hasLevel(Strategic) {
		s.Blocks = append(s.Blocks, heading(2, "How the posture score is calculated"), para(
			"Each domain is scored from 0 to 100 by the rule shown against it, then combined "+
				"as a weighted mean. Domains that could not be assessed are excluded from the "+
				"mean rather than scored as satisfactory."))

		var mrows [][]Cell
		for _, d := range b.score.Domains {
			w := fmtInt(d.Weight) + "%"
			if d.Band == BandNotAssessed {
				w = "excluded"
			}
			mrows = append(mrows, []Cell{cell(d.Label), cell(w), cell(d.Basis)})
		}
		s.Blocks = append(s.Blocks, table(
			[]Column{{Label: "Domain", Width: 1.6}, {Label: "Weight", Width: 1}, {Label: "Computed from", Width: 6}},
			mrows, "", ""))

		s.Blocks = append(s.Blocks, heading(3, "Bands"), table(
			[]Column{{Label: "Band", Width: 1.4}, {Label: "Score", Width: 1}, {Label: "Reading", Width: 5}},
			[][]Cell{
				{pill("Strong", StatusGood), cell("85–100"), cell("Performing well; no action required this period.")},
				{pill("Stable", StatusGood), cell("70–84"), cell("Acceptable; watch the named items.")},
				{pill("Watch", StatusWarn), cell("55–69"), cell("Drifting; act before the next period.")},
				{pill("At risk", StatusBad), cell("35–54"), cell("Materially exposed; act now.")},
				{pill("Critical", StatusBad), cell("0–34"), cell("Immediate action and escalation.")},
			}, "", ""))
	}

	if b.opt(OptIncludeGlossary) {
		s.Blocks = append(s.Blocks, heading(2, "Glossary"), buildGlossary(b))
	}

	s.Blocks = append(s.Blocks, divider(), para(
		b.meta.Classification+" · Generated by "+b.meta.Product+" "+b.meta.Version+
			" on "+fmtDateTime(b.meta.GeneratedAt)+" by "+b.meta.GeneratedBy+". "+
			"Figures reflect the platform's state at that moment and will move as the estate changes."))
	return s
}

// buildAdHocAnnex is the standard annex plus the unverified-figure
// table. The table is unconditional whenever unv is non-empty: it is the
// record that makes a model-authored figure challengeable.
func buildAdHocAnnex(b *buildCtx, unv []unverified) Section {
	s := buildAnnex(b)
	if len(unv) == 0 {
		return s
	}
	// buildAnnex's lead paragraph (its first block, unconditionally
	// added there) asserts "Every figure is derived from the sources
	// listed below" — true for the other 32 types, where the host writes
	// every word, but not here: a model wrote some of this report's
	// prose, and the very next table lists figures that contradict that
	// claim. Corrected on this path only; buildAnnex itself is shared
	// with the other 32 types and must go on saying it unconditionally
	// for them.
	if len(s.Blocks) > 0 && s.Blocks[0].Kind == BlockParagraph {
		s.Blocks[0].Text = "This report was generated by McHarbor from the platform's own records at the time " +
			"shown on the cover, with its narrative sections drafted by an AI model from the data " +
			"listed below, read through the same permission model as the McHarbor interface — it " +
			"contains nothing the requesting user could not already see. Not every figure the model " +
			"wrote could be matched back to that data; those are listed under Unverified figures below."
	}
	rows := make([][]Cell, 0, len(unv))
	for _, u := range unv {
		block := "—"
		if u.Block >= 0 {
			block = fmtInt(u.Block + 1)
		}
		rows = append(rows, []Cell{cell(u.Section), cell(block), cell(u.Token)})
	}
	s.Blocks = append(s.Blocks,
		heading(2, "Unverified figures"),
		para("These figures appear in the report but could not be matched to any "+
			"value in the sources listed above. Treat them as the model's own "+
			"characterisation rather than as measurements. Block numbers count from 1 "+
			"within their section, in the order the model wrote them; “—” marks a "+
			"section's own caption rather than one of its blocks."),
		table(
			[]Column{{Label: "Section", Width: 3}, {Label: "Block", Width: 0.8}, {Label: "Figure", Width: 1}},
			rows, "", ""))
	return s
}

func buildGlossary(b *buildCtx) Block {
	terms := []Pair{
		{K: "Availability", V: "The share of workloads that are running and reporting healthy, weighted by how often they restart."},
		{K: "Change failure rate", V: "The proportion of deployments that needed a rollback, a hotfix or caused an incident. DORA's stability key; elite teams sit at or below 5%."},
		{K: "Deployment frequency", V: "How often change reaches the platform. DORA's throughput key; elite teams deploy on demand, several times a day."},
		{K: "Headroom", V: "Free capacity on whichever of CPU and memory is tighter. The looser resource is ignored because it cannot rescue the tighter one."},
		{K: "Mean recovery time", V: "Average interval between a workload stopping unexpectedly and starting again, measured from the platform's own event log."},
		{K: "Crash loop", V: "A workload that restarts repeatedly without staying up. It consumes capacity and serves nothing."},
		{K: "Scan coverage", V: "The share of images in the estate that have been scanned at least once. Unscanned is not the same as clean."},
		{K: "Severity weighting", V: "Findings are weighted critical ×10, high ×5, medium ×2, low ×0.5 so one critical is not offset by a hundred trivia."},
		{K: "Backup coverage", V: "The share of running workloads that have at least one backup plan."},
		{K: "Recovery point", V: "How far back the most recent usable backup reaches — the amount of work that would be lost in a recovery."},
		{K: "Dormant account", V: "A user account with no sign-in for 90 days that still holds its permissions."},
		{K: "Not assessed", V: "A domain whose data could not be read. Excluded from the overall score rather than counted as satisfactory."},
		{K: "Measured uptime", V: "The share of observed time a workload was running, from the platform's own 30-second sampling. Distinct from “healthy now”, which describes only the instant the report ran."},
		{K: "Observed time", V: "Samples taken × the sampling interval. The denominator for every availability percentage, and deliberately not “days × 24 hours”: a workload created part-way through the period was not down for the first half of it."},
		{K: "Error budget", V: "The downtime an availability target permits. At 99.9% that is roughly 43 minutes a month; “spent” means more than that has been used."},
		{K: "MTTR", V: "Mean time to recover — the average length of an outage, measured from run-state transitions rather than from incident tickets."},
		{K: "MTBF", V: "Mean time between failures — the average run of uninterrupted service between outages."},
		{K: "Flapping", V: "Repeated short transitions between running and stopped. It can coexist with good uptime, which is why transitions are counted separately from downtime."},
		{K: "Recovery point", V: "How far back the newest usable backup reaches — the amount of work a failure would lose."},
		{K: "Readiness", V: "Whether a workload has the operational basics: an observable health check, availability at target, a backup plan, an update policy and a scanned image."},
		{K: "Share of consumption", V: "A stack's proportion of the estate's running workloads and persistent storage, weighted equally. A proportion, not a price — the platform holds no rate card."},
	}
	// Only glossary the terms this report actually uses; a glossary of
	// words that do not appear is padding.
	return keyvalue(terms...)
}

// --- status helpers --------------------------------------------------
//
// One place decides whether a number is good news, so a KPI tile, a
// table cell and a chart slice cannot disagree.

// countStatus: good at or below `good`, bad above `warn`.
func countStatus(v, good, warn int) Status {
	switch {
	case v <= good:
		return StatusGood
	case v <= warn:
		return StatusWarn
	default:
		return StatusBad
	}
}

// thresholdStatus: higher is better.
func thresholdStatus(v, good, warn float64) Status {
	switch {
	case v >= good:
		return StatusGood
	case v >= warn:
		return StatusWarn
	default:
		return StatusBad
	}
}

// invThresholdStatus: lower is better.
func invThresholdStatus(v, good, warn float64) Status {
	switch {
	case v <= good:
		return StatusGood
	case v <= warn:
		return StatusWarn
	default:
		return StatusBad
	}
}

// freshnessStatus judges an age in days; a negative age means "never",
// which is always the worst case.
func freshnessStatus(days, good, warn float64) Status {
	switch {
	case days < 0:
		return StatusBad
	case days <= good:
		return StatusGood
	case days <= warn:
		return StatusWarn
	default:
		return StatusBad
	}
}

// joinAnd renders a list the way a sentence needs it.
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

// levelTitle and levelIntent give each altitude its standing header and
// the one line that tells a reader whether this section is for them.
func levelTitle(l Level) string {
	switch l {
	case Strategic:
		return "Strategic"
	case Tactical:
		return "Tactical"
	case Operational:
		return "Operational"
	case SummaryLevel:
		return "Summary"
	case AnnexLevel:
		return "Annex"
	}
	return string(l)
}

func levelIntent(l Level) string {
	switch l {
	case Strategic:
		return "Where the platform stands, what is changing, and what needs a decision."
	case Tactical:
		return "Performance against targets, and what is drifting away from them."
	case Operational:
		return "The detail behind the figures above, for the people who will act on it."
	}
	return ""
}

// metricsCoverageCallouts returns the blocks that qualify a report's
// resource figures, in the order they should be read.
//
// There are three cases and they are not interchangeable:
//
//   - The metrics source itself failed. The caller's own "unavailable"
//     callout stands; this returns nothing so it is not duplicated.
//   - Metrics read fine but no history exists yet. The figures are real
//     but they are single readings, and a report that prints a period on
//     its cover must not let them pass as period figures.
//   - History shallower than the requested period. This is the normal
//     case for as long as the rollup is younger than the periods people
//     ask for, and it is the one that has to be stated: a report that
//     averaged six days while printing thirty on its cover would be
//     worse than no report at all.
//   - Full coverage. One line saying what was measured and how, because
//     a reader comparing two months needs to know the p95 is taken over
//     ten-minute means rather than over raw samples.
func metricsCoverageCallouts(f Facts) []Block {
	c := f.Capacity
	if !c.Assessed || c.MemTotalBytes == 0 {
		return nil
	}

	if !c.HistoryAvailable {
		return []Block{callout(StatusWarn, "Resource figures are point-in-time",
			"No metric history is stored for this period, so CPU and memory are single "+
				"readings taken when this report was generated rather than figures for the "+
				"period on the cover. A momentary spike will read as pressure and a quiet "+
				"moment will read as headroom. Storage, image and volume figures below are "+
				"unaffected. The metric store fills as it runs, so later reports over this "+
				"same period will be measured rather than sampled.")}
	}

	measured := "Figures are measured over " + fmtDecimal(c.CoveredDays, 1) +
		" days from " + c.CoveredFrom.Format("2 January 2006") + " to " +
		c.CoveredTo.Format("2 January 2006") + ", from " + fmtInt(c.CPUStat.Buckets) +
		" ten-minute buckets. Peak is the highest bucket; thresholds are applied to " +
		"the 95th percentile of bucket means, so a single excursion does not colour " +
		"the period and sustained pressure cannot hide behind the average."

	if !c.CoverageShort() {
		return []Block{callout(StatusInfo, "How these figures were measured", measured)}
	}

	return []Block{callout(StatusWarn, "Metrics history is shorter than the period",
		"Metrics history covers "+fmtDecimal(c.CoveredDays, 1)+" of the "+
			fmtDecimal(c.RequestedDays, 0)+" requested days; the figures reflect the "+
			"covered period only and the rest of the period is not represented. "+
			"The metric store keeps what it has collected since it was switched on, "+
			"so this resolves itself with time rather than with configuration. "+
			measured)}
}
