// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package reporting is the report-provider adapter family.
//
// It is the one family whose consumer is another plugin rather than the
// host: the Reporting plugin owns the Report Studio, the library, the
// PDF/DOCX writers and the schedules, and this family is how a plugin
// that owns a *domain* — Kubernetes, GitOps, an incident tracker — puts
// its own reports on that shelf without either side importing the other.
//
// The split is deliberate:
//
//   - The provider declares its report **types** (what questions it can
//     answer) and builds the **sections** (the answer). It knows its own
//     data, its own clients and its own permissions.
//   - The Reporting plugin owns everything that makes a document a
//     document: the cover, the period, the source-status annex, the
//     level filtering, the persistence, the exporters. A provider never
//     renders and never writes a file.
//
// That is why the document model below is duplicated here rather than
// imported from the Reporting plugin: two Go plugins cannot share a
// package that lives inside one of them, and a JSON blob across the
// boundary would give a provider no compiler help at all. The types are
// a narrow mirror of the Reporting plugin's own model — anything a
// built-in report can say, a provider can say.
package reporting

import (
	"context"
	"time"
)

// ReportAdapterRegistry is the sub-registry a plugin calls to put its
// reports in the Report Studio.
//
// One Register call per provider, not per report type: a provider is
// one client with one set of credentials answering several questions,
// and registering per type would make a plugin build the same client
// five times. Registering the same provider slug twice, or two
// providers claiming the same report type key, is an error — the
// Reporting plugin dispatches on the type key and a silent second
// winner would make which report ran depend on map iteration order.
type ReportAdapterRegistry interface {
	Register(spec ReportProviderSpec, client ReportProviderClient) error
}

// ReportProviderSpec describes everything the Report Studio needs to
// render a plugin's shelf before any report has been run.
type ReportProviderSpec struct {
	// Slug is the owning plugin's slug ("kubernetes"). It is the
	// dispatch key for the provider, the i18n namespace the studio
	// resolves labels from, and the origin tag the loader prunes on
	// uninstall. Required.
	Slug string

	// DisplayName is the untranslated fallback shown when the plugin's
	// i18n bundle has no entry for a label key. Required — a shelf with
	// no name is worse than an English one.
	DisplayName string

	// Types are the report types this provider offers, in the order the
	// studio should show them. A provider needs at least one type or one
	// facet; with neither it registered for nothing.
	Types []ReportTypeDecl

	// Facets are the cross-platform questions this provider answers inside
	// the Reporting plugin's shared reports (see facets.go). Declaring one
	// requires the client to implement ReportFacetContributor.
	Facets []FacetDecl
}

// ReportTypeDecl is one report type, in the same shape the Reporting
// plugin's own catalog entries take.
//
// The declaration is data, not code branches: the studio reads it to
// decide which questions to ask (period? environment? scope?) and the
// provider reads the resulting request back in Build. Declaring a
// control the provider then ignores is the one way to make this
// dishonest, so declare only what changes the document.
type ReportTypeDecl struct {
	// Key is the globally unique report type key. Prefix it with the
	// plugin slug ("kubernetes-cluster-health") — the Reporting
	// plugin's library, schedules and stored documents all key on this
	// string, and a collision with a built-in type would silently
	// re-point every stored report of that type.
	Key string

	// Category is the studio shelf this type sits on. A provider may
	// reuse a built-in category ("platform", "workload", "assurance",
	// "operations", "service", "finance", "board") or mint its own; an
	// unknown category is appended after the built-in shelves rather
	// than dropped.
	Category string

	// Levels are the management altitudes this type writes at, in
	// reading order. "strategic", "tactical", "operational".
	Levels []string

	// Options are per-type switches the studio renders as checkboxes
	// and hands back in ReportRequest.Options.
	Options []ReportOptionDecl

	// Subjects are the object kinds this report can be narrowed to.
	// Prefix them with the plugin slug ("kubernetes-cluster") for the
	// same reason Key is prefixed. The provider supplies the candidate
	// list through Subjects() and does its own narrowing in Build() —
	// the Reporting plugin's snapshot filter knows nothing about a
	// provider's objects.
	Subjects []string

	// DefaultDays is the period the studio pre-selects. Ignored when
	// PointInTime is true.
	DefaultDays int

	// PointInTime marks a type that describes the present rather than a
	// window, so the studio hides the period picker.
	PointInTime bool

	// EnvironmentScoped declares whether the host's environment picker
	// changes this document. Declared rather than derived: a provider
	// with its own connection registry (a Kubernetes cluster, a Git
	// remote) is usually *not* scoped by the host's Docker environment,
	// and offering a control that cannot change the answer is worse
	// than offering none.
	EnvironmentScoped bool

	// TitleEN and SubtitleEN are the document's own cover strings.
	// English by design: a stored report's identity must not change
	// when it is forwarded to a reader with another locale, which is
	// exactly the rule the built-in types follow.
	TitleEN    string
	SubtitleEN string

	// LabelKey and DescriptionKey are i18n keys resolved against the
	// provider's own namespace (ReportProviderSpec.Slug) for the
	// studio's card. Empty falls back to TitleEN / SubtitleEN.
	LabelKey       string
	DescriptionKey string

	// CategoryLabelKey and CategoryDescriptionKey label a category the
	// provider minted. Ignored for built-in categories, which the
	// Reporting plugin already labels.
	CategoryLabelKey       string
	CategoryDescriptionKey string
}

// ReportOptionDecl is one per-report toggle.
type ReportOptionDecl struct {
	// Key is the option key handed back in ReportRequest.Options. The
	// Reporting plugin's shared keys — "include_detail",
	// "include_glossary", "include_narrative" — are already translated
	// in its bundle; a provider-specific key needs a label in the
	// provider's own namespace under `option.<key>`.
	Key     string
	Default bool
}

// ReportRequest is what the Reporting plugin hands a provider for one
// generation.
//
// It carries no credential of its own. Cookie is the calling user's
// session, forwarded only so a provider that answers over the host's
// own HTTP surface collects as that user and can never assemble a
// document containing something the requester could not already see.
// A provider talking to its own backend (client-go, a Git remote)
// ignores it.
type ReportRequest struct {
	// TypeKey is the report type being generated. A provider that
	// registered several types dispatches on it.
	TypeKey string

	// EnvironmentID and Environment are the host environment the
	// operator selected. Both empty when the type declared
	// EnvironmentScoped=false.
	EnvironmentID string
	Environment   string

	// PeriodStart / PeriodEnd is the window on the cover. Equal for a
	// point-in-time type.
	PeriodStart time.Time
	PeriodEnd   time.Time

	// Levels are the altitudes the operator asked for, already
	// intersected with the ones the type declares.
	Levels []string

	// Options are the resolved per-type switches, defaults applied.
	Options map[string]bool

	// SubjectKind / SubjectID / SubjectName is the object the operator
	// narrowed to, empty when the report covers everything the provider
	// can see. The provider does the narrowing.
	SubjectKind string
	SubjectID   string
	SubjectName string

	// Cookie and Lang come from the calling request. Never log either,
	// never store them, never render them.
	Cookie string
	Lang   string

	// User is the display name of whoever asked, for a provider that
	// wants to attribute something. Never a credential.
	User string

	// UserID is the requesting account's id, and it is the *only*
	// thing that lets a provider enforce its own permissions.
	//
	// This matters more than it looks. The Reporting plugin gates
	// generation on `reporting.generate`; it knows nothing about what a
	// provider's data is worth. A provider whose own pages are gated on
	// (say) `kubernetes.view` MUST re-check that key here with
	// HostContext.RBAC.HasPermission before reading anything, or a
	// report becomes a way to read data the requester was refused in
	// the interface. Empty only when a report runs outside any user
	// context, which a provider should treat as "deny".
	UserID string
}

// ReportResult is what a provider returns.
//
// Sections are the content; the Reporting plugin filters them to the
// requested levels, wraps them in the cover and the annex, and hands
// the whole thing to the three renderers unchanged. Sources is what
// the Methodology annex prints — a provider that skips it produces a
// document a reader cannot challenge, which is the one thing the
// reporting contract will not do quietly.
type ReportResult struct {
	// Title and Subtitle override the type's declared cover strings for
	// this one run. Empty keeps the declaration.
	Title    string
	Subtitle string

	// Sections are the content, in reading order. A section whose Level
	// is "summary" is only honoured as the first element; anywhere else
	// it is demoted, because two executive summaries in one document is
	// a bug the reader pays for.
	Sections []Section

	// Sources is what was consulted and whether it answered. Anything
	// the provider read belongs here, including the reads that failed —
	// a source marked not-OK is excluded from every figure rather than
	// being treated as a zero.
	Sources []SourceStatus

	// Notes are provider-level caveats appended to the annex, one line
	// each. Use them for the things that qualify the whole document
	// ("metrics-server is not installed, so no utilisation is shown"),
	// not for per-section commentary.
	Notes []string
}

// SubjectOption is one row in the studio's scope picker.
type SubjectOption struct {
	ID   string
	Name string
	// Hint is secondary text — a namespace, a state, a cluster name —
	// shown beside the name so two similarly named objects can be told
	// apart.
	Hint string
}

// SourceStatus is one data source's outcome during a provider's own
// collection.
type SourceStatus struct {
	// Key is the stable identifier ("kubernetes_nodes"). Label is what
	// the annex prints ("Cluster nodes").
	Key   string
	Label string
	// OK false means the source did not answer. Note says why, in one
	// line, with no hostname, token or path in it.
	OK   bool
	Note string
	// DurationMS is how long the read took. 0 is acceptable.
	DurationMS int64
}

// ReportProviderClient is the runtime contract.
//
// Implementations MUST be safe for concurrent use: a scheduled report
// and an operator's report can run at the same moment, and the studio's
// scope picker calls Subjects while a generation is in flight.
type ReportProviderClient interface {
	// Build collects and composes one report. It is the whole of the
	// provider's work: there is no separate collect phase, because a
	// provider's data does not go through the Reporting plugin's
	// loopback collector and a two-call contract would only invent a
	// snapshot type neither side needs.
	//
	// Honour the context deadline — the Reporting plugin runs Build
	// inside the generation job's context and a provider that blocks
	// forever hangs a job the operator is watching.
	//
	// Returning an error fails the generation with a translated code;
	// returning a result whose Sources record the failure produces an
	// honest document that says what it could not read. Prefer the
	// second wherever a partial answer is still worth having.
	Build(ctx context.Context, req ReportRequest) (ReportResult, error)

	// Subjects lists the objects of one provider-declared kind the
	// calling user may narrow a report to. Return nil for a kind the
	// provider does not own. Filter by what the caller may see; the
	// studio offers whatever comes back.
	Subjects(ctx context.Context, kind string, req ReportRequest) ([]SubjectOption, error)
}

// ReportingHandle is the consumer half, handed to the Reporting plugin
// on HostContext so it can see and drive whatever providers are
// installed without knowing any of them exist at build time.
//
// nil when the host wired no reporting bridge; a consumer must treat
// that as "no providers" and carry on with its own catalog.
type ReportingHandle interface {
	// Providers lists every registered provider, in registration order.
	Providers() []ReportProviderSpec

	// Build dispatches to whichever provider registered typeKey.
	// The boolean is false when no provider owns the key.
	Build(ctx context.Context, typeKey string, req ReportRequest) (ReportResult, bool, error)

	// Subjects dispatches a scope listing the same way.
	Subjects(ctx context.Context, typeKey, kind string, req ReportRequest) ([]SubjectOption, bool, error)

	// Available reports whether at least one provider is registered, so
	// a consumer can skip the merge entirely on a plain install.
	Available() bool
}

// --- the neutral document model --------------------------------------
//
// A narrow mirror of the Reporting plugin's own model. Kept in step
// with it by hand rather than generated: the set of things a report can
// say changes rarely, and a provider's document must never be able to
// express something the three renderers cannot draw.

// Section is one altitude's worth of content.
type Section struct {
	// Level is "strategic", "tactical", "operational" — or "summary"
	// for a lead section, which is only honoured first.
	Level  string
	Title  string
	Blocks []Block
}

// Block is a tagged union. Only the fields relevant to Kind are read.
//
// Kind is one of: "heading", "paragraph", "kpis", "table", "chart",
// "callout", "list", "keyvalue", "findings", "divider", "pagebreak".
type Block struct {
	Kind string

	// heading / paragraph / callout
	Text  string
	Depth int
	Lead  bool

	// kpis
	KPIs []KPI

	// table
	Columns []Column
	Rows    [][]Cell
	Caption string
	Note    string

	// chart
	Chart *Chart

	// callout
	Tone  string
	Title string

	// list
	Ordered bool
	Items   []string

	// keyvalue
	Pairs []Pair

	// findings
	Findings []Finding
}

// KPI is one headline number. Status carries the judgement ("good",
// "warn", "bad", "info", "muted", or empty for none); Trend carries
// only the direction ("up", "down", "flat").
type KPI struct {
	Label  string
	Value  string
	Unit   string
	Target string
	Delta  string
	Trend  string
	Status string
	Hint   string
}

// Column describes a table column. Width is a relative weight, not an
// absolute size, so the same table works at PDF and screen widths.
type Column struct {
	Label string
	Align string // "", "right", "center"
	Width float64
	Mono  bool
}

// Cell is one table cell. Pill draws the text as a filled status chip
// rather than tinted text — used for states, bands and severities.
type Cell struct {
	Text   string
	Status string
	Mono   bool
	Pill   bool
}

// Pair is one key/value row.
type Pair struct {
	K    string
	V    string
	Mono bool
}

// Finding is an issue with a recommendation. A report that names a
// problem without saying what to do about it wastes the reader's time,
// so treat Recommendation as required in practice.
type Finding struct {
	// Severity is "critical", "high", "medium", "low" or "info".
	Severity       string
	Title          string
	Detail         string
	Recommendation string
	Owner          string
	Evidence       string
}

// Chart is a chart block's payload. Kind is one of "bar", "hbar",
// "stackedbar", "donut", "line", "area", "gauge".
type Chart struct {
	Kind        string
	Title       string
	Series      []ChartSeries
	Unit        string
	Max         float64
	Target      float64
	TargetLabel string
	HasTarget   bool
	ShowLegend  bool
	ShowValues  bool
	Caption     string
	// Height is in points; 0 lets each renderer pick its default.
	Height float64
}

// ChartSeries is one named series.
type ChartSeries struct {
	Name   string
	Status string
	Points []ChartPoint
}

// ChartPoint is one datum; Status overrides the categorical colour when
// the datum means something (a severity, a health state).
type ChartPoint struct {
	Label  string
	Value  float64
	Status string
}
