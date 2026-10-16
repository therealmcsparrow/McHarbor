// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// service is the plugin's business logic: collect, analyse, build,
// persist, export. Handlers do none of this; they parse, authorise,
// call in here, audit and respond.
type service struct {
	store  *store
	host   plugin.HostContext
	assets *assets
	// onGenerated, when set, is told about every report Generate
	// produces so the report-generated workflow trigger can fire.
	onGenerated func(generatedEvent)
}

// GenerateInput is a validated generation request.
type GenerateInput struct {
	ReportType    string
	EnvironmentID string
	Environment   string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	Levels        []Level
	Options       map[string]bool
	Persist       bool
	// Subject narrows the report to one object. Empty means the whole
	// estate, which is what every report did before scopes existed.
	Subject Subject
	// Progress, when non-nil, receives stage and per-source updates so a
	// caller running this as a job can show something honest.
	Progress *progress

	// Cookie and Lang come from the calling request so collection runs
	// as that user. Never logged, never stored, never rendered.
	Cookie string
	// Authorization is the caller's Authorization header, forwarded to the
	// loopback collector so an API-key request collects as that key's user.
	// It never reaches plugin.ReportRequest, which stays cookie-only: a
	// provider re-checks its own permissions from UserID.
	Authorization string
	Lang          string
	User          string
	// Source records who asked: "manual" (studio or API), "schedule" or
	// "workflow". It only feeds the report-generated trigger's filter.
	Source string
	// UserID is the requesting account's id. Unused by every built-in
	// report — the host already gated the route on reporting.generate —
	// and forwarded to plugin-contributed types so a provider can
	// re-check its own permission key. A provider's data is worth
	// whatever its own pages are gated on, and this plugin cannot know
	// that.
	UserID string

	// Prompt is the user's question. Set only for TypeAdHoc.
	Prompt string
	// Plan, when non-nil, is a pinned pass-1 result being replayed. Set
	// only when regenerating a stored ad-hoc report.
	Plan *AdHocPlan

	// AIConnectionID optionally pins generation to one configured AI
	// connection (see plugin.AIConnection / host.AI.Connections()).
	// Empty means the host's default/favourite connection. Threaded into
	// both aiComplete (ad-hoc planning and authoring) and aiWriter
	// (narrative polish, gated separately by Settings.AINarrative) so a
	// single request always uses one AI, never a silent mix.
	AIConnectionID string
}

// errAIReportsOff, errAIUnavailable and errAIConnectionUnavailable are
// three genuinely different conditions that a single errAIDisabled used
// to collapse into one message — including telling a user whose
// AIReports setting was already ON to go enable it in Reporting
// settings, when the real cause was that no AI provider was reachable
// at all. Kept distinct so the client can tell them apart:
//   - errAIReportsOff: the operator has not switched Settings.AIReports on.
//   - errAIUnavailable: no AI provider is configured/available on the
//     host at all (host.AI is nil or host.AI.Available() is false).
//   - errAIConnectionUnavailable: the caller named a specific connection
//     id that is missing or disabled on the host.
var (
	errAIReportsOff            = errors.New("reporting: ai reports setting is off")
	errAIUnavailable           = errors.New("reporting: no ai provider available")
	errAIConnectionUnavailable = errors.New("reporting: requested ai connection unavailable")
	// errProviderMissing means the catalog offered a plugin-contributed
	// type whose provider is no longer registered — the usual cause is
	// the contributing plugin having been disabled between the studio
	// loading its catalog and the operator pressing Generate. It is a
	// distinct condition from "the provider failed": nothing ran.
	errProviderMissing = errors.New("reporting: report provider is not installed")
)

// connectionIDSet turns the host's connection listing into a lookup set,
// so validate() and the Generate gate can check an id in O(1) without
// either of them needing to know how the host represents a connection.
func connectionIDSet(conns []plugin.AIConnection) map[string]bool {
	set := make(map[string]bool, len(conns))
	for _, c := range conns {
		set[c.ID] = true
	}
	return set
}

// reportedAIConnectionID decides what actually gets persisted in
// StoredReport.AIConnectionID — the connection that wrote the report,
// never merely one that was offered to it.
//
// For TypeAdHoc, authorAdHoc's completion call IS the report's content:
// Generate has already returned early on any planning or authoring
// failure (errPlanFailed / errAuthorFailed), so by the time this runs
// connID (plan.AIConnectionID, already resolved by validate()) genuinely
// wrote the document. Recording it unconditionally is correct here.
//
// For every other type, aiWriter's narrative polish is the only AI path,
// and it is optional and best-effort: narrator.summaryLead falls back to
// deterministic text when the writer is nil (AINarrative off, or no AI
// provider available), on a transport error, on an empty reply, or on a
// reply that fails the number-preservation check. usedAI is set by the
// narrator only at the exact moment it adopts a model's text (see
// narrative.go) — it is the one signal that distinguishes "an AI wrote
// part of this report" from "an AI connection was merely offered to
// it". Recording connID when usedAI is false would let the library
// claim a report was AI-written when no model ever touched it — worse
// than an empty column, which at least reads as "unknown" rather than
// as a false fact.
func reportedAIConnectionID(rtKey string, connID string, usedAI bool) string {
	if rtKey == TypeAdHoc {
		return connID
	}
	if !usedAI {
		return ""
	}
	return connID
}

// Generate builds one report.
func (s *service) Generate(ctx context.Context, in GenerateInput) (StoredReport, error) {
	rt, ok := reportTypeByKey(in.ReportType)
	if !ok {
		return StoredReport{}, fmt.Errorf("reporting: unknown report type %q", in.ReportType)
	}
	var plan AdHocPlan
	var availableAIConnections map[string]bool
	if rt.Key == TypeAdHoc {
		settings := s.store.settings(ctx, s.host.DB.KV())
		switch {
		case !settings.AIReports:
			return StoredReport{}, errAIReportsOff
		case s.host.AI == nil || !s.host.AI.Available():
			return StoredReport{}, errAIUnavailable
		}
		// availableAIConnections backs both the explicit-choice check
		// just below and AdHocPlan.validate's stale-pin drop, so a
		// connection id is judged against the exact same listing
		// everywhere in this request.
		availableAIConnections = connectionIDSet(s.host.AI.Connections())
		if in.AIConnectionID != "" && !availableAIConnections[in.AIConnectionID] {
			// A fresh, explicit choice naming a connection that is gone
			// or disabled is a request error, not a silent fallback: a
			// report that names its model must not quietly use another.
			// (A *pinned* plan replaying a stale choice is a different
			// case, handled by validate() below, which degrades instead.)
			return StoredReport{}, errAIConnectionUnavailable
		}
		// Reuse the same source the collector uses for srcEnvironments
		// (collect.go) rather than adding a second path to this data.
		// Needed whether the plan is fresh or pinned: validate() below
		// checks a plan's EnvironmentID against exactly this list, and a
		// plan pinned months ago may name an environment that no longer
		// exists. The bool is checked rather than ignored: an empty envs
		// on a failed fetch would make validate() silently drop every
		// plan.EnvironmentID, which is a data-loss failure mode dressed
		// up as a normal empty result.
		planAPI := newHostAPI(in.Cookie, in.Lang, in.EnvironmentID).withAuthorization(in.Authorization)
		var envs []envRow
		if listed := planAPI.getList(ctx, srcEnvironments, "Environments", "/api/environments", nil, &envs); !listed {
			return StoredReport{}, fmt.Errorf("reporting: list environments for ad-hoc planning failed")
		}
		if in.Plan != nil {
			// A pinned plan is replayed rather than re-planned, so a
			// scheduled ad-hoc report keeps the same shape month to
			// month. It is still re-validated here: a plan pinned months
			// ago may name a source or an environment that no longer
			// exists.
			validated, err := in.Plan.validate(envs, availableAIConnections, s.host.Logger)
			if err != nil {
				return StoredReport{}, err
			}
			plan = validated
		} else {
			var err error
			plan, err = s.planAdHoc(ctx, in.Prompt, envs, nil, in.AIConnectionID)
			if err != nil {
				return StoredReport{}, err
			}
		}
		rt = plan.reportType()
		if plan.EnvironmentID != "" {
			// The planner can choose a different environment than the one
			// the caller selected. Adopting the id without also updating
			// the display name would leave Meta.Environment — the PDF
			// cover, the library row, rec.Environment and the audit event
			// — naming the caller's original pick while the report
			// actually collected a different environment. Resolve the
			// real name from the same list validate() just checked the id
			// against; if it somehow does not resolve (should not happen
			// post-validate, but this must never print a name-less id),
			// keep the caller's environment instead.
			if name := environmentNameFromRows(envs, plan.EnvironmentID); name != "" {
				in.EnvironmentID = plan.EnvironmentID
				in.Environment = name
			}
		}
		// A subject the model named would be applied exactly the way an
		// explicitly-requested one is: through the shared gate and
		// applySubject below. It is unreachable today, deliberately:
		// planSystem never offers "subject" in the planner's JSON schema,
		// because the planner is given no list of candidate containers,
		// stacks, users or groups to choose from — without one it would
		// have to invent an identifier, which matches nothing and
		// silently narrows the report to empty. Enabling this requires
		// giving the planner real candidates first; until then
		// plan.Subject is always empty and this branch never runs.
		if !plan.Subject.empty() {
			in.Subject = plan.Subject
		}
		// The ad-hoc dialog has no period control, so there is no user
		// intent to honour here — resolvePeriod (handler.go) always
		// returns a non-zero window regardless of what the caller sent,
		// which made an IsZero() guard here permanently dead. plan.Days
		// is the only reporting-window decision that exists for this
		// type and owns the window unconditionally.
		in.PeriodEnd = time.Now()
		in.PeriodStart = in.PeriodEnd.AddDate(0, 0, -plan.Days)
	}
	// Neither ad-hoc nor plugin-contributed reports go through the
	// builder dispatch table: the first is assembled by
	// buildAdHocDocument from the model's own sections, the second by
	// buildProviderDocument from the provider's. This existence check
	// therefore only applies to the fixed catalog types.
	fromProvider := isProviderType(rt.Key)
	if rt.Key != TypeAdHoc && !fromProvider {
		if _, ok := builders()[rt.Key]; !ok {
			return StoredReport{}, fmt.Errorf("reporting: report type %q has no builder", rt.Key)
		}
	}

	settings := s.store.settings(ctx, s.host.DB.KV())

	period := Period{Start: in.PeriodStart, End: in.PeriodEnd, PointInTime: rt.PointInTime}
	if rt.PointInTime {
		period.Start, period.End = in.PeriodEnd, in.PeriodEnd
	}
	period.Days = period.End.Sub(period.Start).Hours() / 24
	if period.Days < 0 {
		period.Days = 0
	}

	api := newHostAPI(in.Cookie, in.Lang, in.EnvironmentID).withAuthorization(in.Authorization)

	// A plugin-contributed type collects through its own clients, so the
	// host's loopback collector has nothing to fetch for it. Running it
	// anyway would put a table of sources in the annex that no figure in
	// the document came from — the exact dishonesty the annex exists to
	// prevent. The provider returns its own source rows instead.
	subject := in.Subject
	if !subject.empty() && !rt.supportsSubject(subject.Kind) {
		subject = Subject{}
	}

	var (
		snap  *Snapshot
		facts Facts
		score *Scorecard
	)
	if fromProvider {
		in.Progress.plan(1)
		in.Progress.set(JobRunning, StageCollect, 5)
		snap = &Snapshot{Period: period, Subject: subject}
	} else {
		in.Progress.plan(plannedSources(rt))
		in.Progress.set(JobRunning, StageCollect, 5)
		snap = collect(ctx, api, rt, period, func(string) { in.Progress.sourceDone() })

		// Narrowing happens between collection and analysis, so the
		// scorecard, the findings and every table are computed from the
		// scoped estate rather than being filtered afterwards one builder
		// at a time.
		applySubject(ctx, api, snap, subject)

		// Facets run after narrowing so Docker's rows describe the same
		// scoped estate as every other table in the document.
		if facets := reportFacets(rt); len(facets) > 0 {
			collectFacets(ctx, snap, facets, facetRequest(in, period), !subject.empty())
		}

		in.Progress.set(JobRunning, StageAnalyze, 72)
		facts = analyze(snap)
		score = buildScorecard(facts, snap)
	}

	levels := rt.resolveLevels(in.Levels)
	levelSet := map[Level]bool{SummaryLevel: true, AnnexLevel: true}
	for _, l := range levels {
		levelSet[l] = true
	}

	title := reportTitle(rt)
	subtitle := reportSubtitle(rt)
	if fromProvider {
		// A contributed type's cover strings come from its declaration,
		// not from this plugin's switch — titleCase(rt.Key) would print
		// "Kubernetes Cluster Health" on a report whose author already
		// said what it is called.
		title = orDash(rt.TitleEN, titleCase(rt.Key))
		subtitle = rt.SubtitleEN
	}
	if rt.Key == TypeAdHoc {
		// The planner's title is the point of an ad-hoc report: without
		// it every one in a library reads the same generic name.
		title = orDash(plan.Title, reportTitle(rt))
	}
	meta := Meta{
		Title:          title,
		Subtitle:       subtitle,
		ReportType:     rt.Key,
		Organisation:   settings.Organisation,
		Environment:    orDash(in.Environment, environmentName(snap, in.EnvironmentID), "All environments"),
		EnvironmentID:  in.EnvironmentID,
		Subject:        subject.Kind,
		SubjectLabel:   subject.label(),
		PeriodStart:    period.Start,
		PeriodEnd:      period.End,
		GeneratedAt:    time.Now(),
		GeneratedBy:    orDash(in.User, "unknown"),
		Classification: settings.Classification,
		Product:        "McHarbor",
		Version:        platformVersion(snap),
		Levels:         levels,
	}

	in.Progress.set(JobRunning, StageBuild, 80)
	// connID is the connection narrative polish (aiWriter, gated on
	// Settings.AINarrative — a different setting from AIReports) uses.
	// For an ad-hoc report it is plan.AIConnectionID rather than
	// in.AIConnectionID directly, so a replayed pinned plan's narrative
	// polish stays on the same AI as its authoring pass even though the
	// pin may have degraded (see AdHocPlan.validate) to something other
	// than whatever the caller sent this time.
	connID := in.AIConnectionID
	if rt.Key == TypeAdHoc {
		connID = plan.AIConnectionID
	}
	nar := newNarrator(ctx, s.aiWriter(settings, connID))
	b := &buildCtx{
		rt: rt, meta: meta, snap: snap, facts: facts, score: score,
		opts: in.Options, levels: levelSet, nar: nar,
	}
	var doc *Document
	if fromProvider {
		in.Progress.sourceDone()
		secs, sources, notes, err := buildProviderReport(ctx, providerRequest(in, rt, period, subject))
		if err != nil {
			return StoredReport{}, err
		}
		// The provider's own source rows are what the Methodology annex
		// prints, so they are installed on the snapshot before the annex
		// is built rather than appended to the document afterwards —
		// b.missingSources() reads the same slice for the summary's
		// coverage callout, and two lists that could disagree is one
		// list too many.
		snap.Sources = sources
		doc = buildProviderDocument(b, secs, notes)
	} else if rt.Key == TypeAdHoc {
		digest := factsDigest(facts)
		slices := buildSlices(snap, plan, sliceByteBudget)
		secs, err := s.authorAdHoc(ctx, in.Prompt, digest, slices, plan.Levels, plan.AIConnectionID)
		if err != nil {
			return StoredReport{}, err
		}
		unv := verifySections(secs, buildAllowSet(digest, slices))
		doc = buildAdHocDocument(b, secs, unv)
	} else {
		doc = buildDocument(b)
	}

	rec := StoredReport{
		ID:            newID(),
		ReportType:    rt.Key,
		Title:         meta.Title,
		EnvironmentID: in.EnvironmentID,
		Environment:   meta.Environment,
		PeriodStart:   period.Start,
		PeriodEnd:     period.End,
		Levels:        levels,
		Status:        "ready",
		OverallScore:  -1,
		OverallBand:   BandNotAssessed,
		Headline:      headline(doc),
		GeneratedBy:   meta.GeneratedBy,
		CreatedAt:     time.Now(),
		Document:      doc,
		// AIConnectionID records which connection actually wrote (ad-hoc)
		// or polished (narrative, other types) this report — never merely
		// which one was offered. See reportedAIConnectionID's doc comment.
		AIConnectionID: reportedAIConnectionID(rt.Key, connID, nar.usedAI),
	}
	if doc.Scorecard != nil {
		rec.OverallScore = doc.Scorecard.Overall
		rec.OverallBand = doc.Scorecard.Band
	}
	if rt.Key == TypeAdHoc {
		// The prompt and the resolved plan are what let a regenerate
		// replay the same sources instead of re-planning, which is what
		// keeps a scheduled ad-hoc report comparable period to period.
		rec.Prompt = in.Prompt
		if raw, err := json.Marshal(plan); err == nil {
			rec.Plan = raw
		} else if s.host.Logger != nil {
			// A missing plan costs a re-plan on regenerate; it must not
			// cost the report the user is waiting for.
			s.host.Logger.Error("reporting: encode ad-hoc plan", "error", err)
		}
	}

	if in.Persist {
		in.Progress.set(JobRunning, StageStore, 95)
		if err := s.store.insert(ctx, rec, doc); err != nil {
			return StoredReport{}, fmt.Errorf("reporting: persist report: %w", err)
		}
		// Pruning is best-effort: failing to trim history must not fail
		// a report the user is waiting for.
		_ = s.store.prune(ctx, settings.RetainReports)
	}
	if s.onGenerated != nil {
		source := strings.TrimSpace(in.Source)
		if source == "" {
			source = "manual"
		}
		s.onGenerated(generatedEvent{report: rec, source: source, persisted: in.Persist})
	}
	return rec, nil
}

// aiWriter returns the narrator's polish hook, or nil when AI narrative
// is switched off or no provider is configured. connectionID pins which
// configured AI writes the polish — empty means the host's
// default/favourite connection, preserving prior behaviour for a caller
// that never sends one.
func (s *service) aiWriter(settings Settings, connectionID string) func(context.Context, string, string) (string, error) {
	if !settings.AINarrative || s.host.AI == nil || !s.host.AI.Available() {
		return nil
	}
	return func(ctx context.Context, system, user string) (string, error) {
		resp, err := s.host.AI.Complete(ctx, plugin.AIRequest{
			Messages: []plugin.AIMessage{
				{Role: "system", Content: system},
				{Role: "user", Content: user},
			},
			ConnectionID: connectionID,
			Metadata:     map[string]string{"source": "plugin:reporting"},
		})
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
}

// aiComplete is the raw call both ad-hoc passes use. aiWriter stays as
// it is: it carries the narrative-polish gate, which is a different
// setting from the one gating ad-hoc reports. connectionID pins which
// configured AI answers this call; empty means the host's
// default/favourite connection.
func (s *service) aiComplete(ctx context.Context, system, user string, maxTokens int, connectionID string) (string, error) {
	if s.host.AI == nil || !s.host.AI.Available() {
		return "", errAIUnavailable
	}
	temp := float32(0)
	resp, err := s.host.AI.Complete(ctx, plugin.AIRequest{
		Messages: []plugin.AIMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature:  &temp,
		MaxTokens:    &maxTokens,
		ConnectionID: connectionID,
		Metadata:     map[string]string{"source": "plugin:reporting", "stage": "adhoc"},
	})
	if err != nil {
		return "", fmt.Errorf("reporting: ai complete: %w", err)
	}
	return resp.Content, nil
}

// Export renders a stored report to a file.
func (s *service) Export(ctx context.Context, id, format string) (data []byte, filename, contentType string, err error) {
	rec, err := s.store.get(ctx, id)
	if err != nil {
		return nil, "", "", err
	}
	if rec.Document == nil {
		return nil, "", "", errNotFound
	}
	return s.render(ctx, rec, format, s.store.settings(ctx, s.host.DB.KV()))
}

func (s *service) render(ctx context.Context, rec StoredReport, format string, set Settings) ([]byte, string, string, error) {
	base := exportFilename(rec)
	a := s.brandedAssets(ctx, set)
	// `sanitised` guarantees the stored key resolves, so the fallback
	// here only covers a settings row written before this field existed.
	ps, ok := pageSizeByKey(set.PageSize)
	if !ok {
		ps = pageA4
	}
	switch strings.ToLower(format) {
	case "docx":
		data, err := renderDOCX(rec.Document, a, ps)
		if err != nil {
			return nil, "", "", err
		}
		return data, base + ".docx",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
	case "pdf", "":
		data, err := renderPDF(rec.Document, a, ps)
		if err != nil {
			return nil, "", "", err
		}
		return data, base + ".pdf", "application/pdf", nil
	default:
		return nil, "", "", errors.New("reporting: unsupported export format")
	}
}

// brandedAssets overlays the marks a cover page should carry on the
// compiled-in ones.
//
// Three layers, most specific first:
//
//  1. this plugin's own settings, set in Report settings → Branding;
//  2. the install's logo from Settings → Customization, light variant;
//  3. the McHarbor mark compiled into this plugin.
//
// Layer 2 is what makes a rebranded install produce rebranded reports with
// no configuration at all, while layer 1 keeps the override for estates
// whose reports carry a different mark from their console. See
// hostbranding.go for why it is the light PNG rendition specifically.
//
// The base assets are a process-wide singleton holding parsed font faces,
// so this copies the struct rather than mutating it: parsing Inter Tight
// on every export to change a logo would be a poor trade. A mark that
// fails to decode leaves the layer below it in place — an export with the
// default logo beats an export that failed.
func (s *service) brandedAssets(ctx context.Context, set Settings) *assets {
	logo, icon := set.LogoPNG, set.IconPNG
	if logo == "" || icon == "" {
		host := fetchHostBranding(ctx)
		if logo == "" {
			logo = host.LogoWidePNG
		}
		if icon == "" {
			icon = host.LogoIconPNG
		}
	}
	if logo == "" && icon == "" {
		return s.assets
	}
	branded := *s.assets
	if data, w, h, ok := markPNG(logo); ok {
		branded.logoWordmark, branded.wordmarkW, branded.wordmarkH = data, w, h
	}
	if data, _, _, ok := markPNG(icon); ok {
		branded.logoMark = data
	}
	return &branded
}

// exportFilename builds a name that sorts chronologically and survives
// every filesystem a recipient might save it to.
func exportFilename(rec StoredReport) string {
	parts := []string{
		"McHarbor",
		slugify(rec.ReportType),
		slugify(rec.Environment),
		rec.PeriodEnd.Format("2006-01-02"),
	}
	return strings.Join(compact(parts), "_")
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// headline is the one-line summary the library shows per row.
func headline(d *Document) string {
	for _, sec := range d.Sections {
		if sec.Level != SummaryLevel {
			continue
		}
		for _, b := range sec.Blocks {
			if b.Kind == BlockParagraph && b.Text != "" {
				return truncateText(b.Text, 220)
			}
		}
	}
	return ""
}

// environmentNameFromRows looks up an environment's display name by id
// from the rows the ad-hoc planner was given (fetched pre-collection, so
// snap.Environments is not built yet at the point this is needed). Used
// to keep Meta.Environment honest when the planner adopts a different
// environment than the caller selected.
func environmentNameFromRows(envs []envRow, id string) string {
	for _, e := range envs {
		if e.ID == id {
			return e.Name
		}
	}
	return ""
}

func environmentName(snap *Snapshot, id string) string {
	for _, e := range snap.Environments {
		if e.ID == id {
			return e.Name
		}
	}
	if len(snap.Environments) == 1 {
		return snap.Environments[0].Name
	}
	return ""
}

func platformVersion(snap *Snapshot) string {
	if v := strings.TrimSpace(snap.Versions.McHarbor.Version); v != "" {
		return v
	}
	return ""
}

// reportTitle and reportSubtitle are the English source strings. The UI
// shows the localised catalog labels; the document itself is generated
// in the requesting user's language for its data and in English for
// these structural titles, which keeps an exported file's identity
// stable when it is forwarded.
func reportTitle(rt ReportType) string {
	if fam, ok := familyFor(rt.Key); ok {
		if t, ok := fam.titles[rt.Key]; ok {
			return t[0]
		}
	}
	switch rt.Key {
	case TypeExecutiveOverview:
		return "Executive Platform Overview"
	case TypeSecurityCompliance:
		return "Security and Compliance Report"
	case TypeIncidentProblem:
		return "Incident and Problem Review"
	case TypeAvailability:
		return "Availability and Service Levels"
	case TypeChangeDelivery:
		return "Change and Delivery Report"
	case TypeEnvironmentHealth:
		return "Environment Health Report"
	case TypeCapacity:
		return "Capacity and Utilisation Report"
	case TypeBackupRecovery:
		return "Backup and Recovery Report"
	case TypeImageSupplyChain:
		return "Image and Supply Chain Report"
	case TypeAccessReview:
		return "Access and Identity Review"
	case TypeWorkloadInventory:
		return "Workload Inventory"
	case TypePlatformInventory:
		return "Platform Inventory and Configuration"

	case TypeUptimeAvailability:
		return "Uptime and Availability History"
	case TypeContainerService:
		return "Container Service Report"
	case TypeHealthcheckQuality:
		return "Health Check Quality and Coverage"
	case TypeStabilityFlapping:
		return "Stability and Flapping Analysis"
	case TypeStackPortfolio:
		return "Stack and Application Portfolio"
	case TypeOperationalReadiness:
		return "Operational Readiness Scorecard"

	case TypeUserActivity:
		return "User Activity and Operational Footprint"
	case TypePermissionMatrix:
		return "Role and Permission Matrix"
	case TypeCredentialHygiene:
		return "Credential and API Key Hygiene"
	case TypeAuditAssurance:
		return "Audit Trail Assurance"

	case TypePatchCurrency:
		return "Patch and Update Currency"
	case TypeAutomationSchedules:
		return "Scheduled Work and Automation"
	case TypeAlertingCoverage:
		return "Alerting and Monitoring Coverage"
	case TypeAgentFleet:
		return "Remote Agent Fleet Report"
	case TypeNetworkConnectivity:
		return "Network and Connectivity Report"
	case TypeVolumeDataEstate:
		return "Volume and Data Estate Report"

	case TypeResourceEfficiency:
		return "Resource Efficiency and Rightsizing"
	case TypeCostAllocation:
		return "Cost Allocation and Showback"

	case TypeEnvironmentCompare:
		return "Environment Comparison"
	case TypeDataProtection:
		return "Data Protection and Recovery Objectives"

	}
	return titleCase(rt.Key)
}

func reportSubtitle(rt ReportType) string {
	if fam, ok := familyFor(rt.Key); ok {
		if t, ok := fam.titles[rt.Key]; ok {
			return t[1]
		}
	}
	switch rt.Key {
	case TypeExecutiveOverview:
		return "Container platform posture, delivery and risk"
	case TypeSecurityCompliance:
		return "Vulnerability exposure, access controls and audit assurance"
	case TypeIncidentProblem:
		return "What failed, how quickly it recovered, and what keeps recurring"
	case TypeAvailability:
		return "Service levels measured against target"
	case TypeChangeDelivery:
		return "Deployment throughput and stability against the DORA benchmarks"
	case TypeEnvironmentHealth:
		return "Environment, engine and resource condition"
	case TypeCapacity:
		return "Compute and storage headroom, and what can be reclaimed"
	case TypeBackupRecovery:
		return "Coverage, success rate and recovery point"
	case TypeImageSupplyChain:
		return "Image provenance, currency and assessed exposure"
	case TypeAccessReview:
		return "Accounts, roles and credentials for periodic recertification"
	case TypeWorkloadInventory:
		return "A point-in-time record of everything running, on every platform"
	case TypePlatformInventory:
		return "Platform configuration for change control and support"

	case TypeUptimeAvailability:
		return "Measured uptime, outages and error budget, from the platform's own history"
	case TypeContainerService:
		return "One page per workload: availability, image, storage, backup and change"
	case TypeHealthcheckQuality:
		return "Whether the health checks are present, passing, and telling the truth"
	case TypeStabilityFlapping:
		return "Workloads that keep moving between states, and what it costs"
	case TypeStackPortfolio:
		return "Each application stack, its workloads, and how it behaved"
	case TypeOperationalReadiness:
		return "Whether each workload has the basics: health check, backup, updates, monitoring"

	case TypeUserActivity:
		return "Who did what, how often, and outside which hours"
	case TypePermissionMatrix:
		return "Roles against permissions, and who holds each one"
	case TypeCredentialHygiene:
		return "Key age, expiry, breadth of scope, and credentials nobody uses"
	case TypeAuditAssurance:
		return "Coverage, integrity and retention of the audit trail itself"

	case TypePatchCurrency:
		return "How far behind the estate has drifted, and what automation is catching"
	case TypeAutomationSchedules:
		return "Every scheduled job, when it last ran, and what it did"
	case TypeAlertingCoverage:
		return "Which workloads are watched, and which failures nobody would hear about"
	case TypeAgentFleet:
		return "Remote agents, their versions, and their connection health"
	case TypeNetworkConnectivity:
		return "Networks, their drivers and what is attached to each"
	case TypeVolumeDataEstate:
		return "Persistent data, what owns it, and whether it is protected"

	case TypeResourceEfficiency:
		return "Where the compute and storage actually go, and what could be smaller"
	case TypeCostAllocation:
		return "Consumption attributed to the stacks and owners that caused it"

	case TypeEnvironmentCompare:
		return "The same measures across every connected environment, side by side"
	case TypeDataProtection:
		return "Recovery point and recovery time against the objectives set for them"

	}
	return ""
}

// newID mints an identifier. crypto/rand rather than a counter or a
// timestamp: report ids appear in URLs, and a guessable id in a URL is
// an invitation.
func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A failing entropy source is a serious condition, but falling
		// back to a timestamp keeps the feature working rather than
		// failing a report the user is waiting for.
		return fmt.Sprintf("rp%d", time.Now().UnixNano())
	}
	return "rp" + hex.EncodeToString(b[:])
}
