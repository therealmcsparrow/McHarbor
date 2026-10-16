// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "time"

// ReportType is one entry in the report catalog.
//
// The catalog is data, not code branches: the levels a type carries, the
// sources it needs and the options it offers are all declared here, and
// the studio UI, the collector and the builder all read the same entry.
// Adding a report type is a catalog entry plus a builder function.
type ReportType struct {
	Key      string   `json:"key"`
	Category string   `json:"category"`
	Levels   []Level  `json:"levels"`
	Sources  []string `json:"sources"`
	Icon     string   `json:"icon"`
	// DefaultDays is the period the studio pre-selects. A change report
	// wants a sprint; an inventory wants today.
	DefaultDays int `json:"default_days"`
	// PointInTime marks types that describe the present rather than a
	// window, so the studio can hide the period picker.
	PointInTime bool `json:"point_in_time"`
	// EnvironmentScoped reports whether any of this type's sources
	// actually vary by environment, so the studio only asks the question
	// when the answer changes the document. Derived from Sources by
	// catalog(); never hand-written on an entry.
	EnvironmentScoped bool `json:"environment_scoped"`
	// Subjects are the object kinds this report can be narrowed to, in
	// the order the studio offers them. Declared rather than derived from
	// Sources: reading the audit log does not make a board-level posture
	// report a per-user report, and offering a scope that cannot produce
	// a coherent document is worse than not offering one.
	Subjects []SubjectKind `json:"subjects,omitempty"`
	// Options are per-type switches the studio renders as checkboxes.
	Options []ReportOption `json:"options,omitempty"`
	// Facets are the cross-platform questions this report prints, answered
	// by Docker and by every platform plugin that declares them. Derived
	// from reportFacetUses by catalog(); never hand-written on an entry.
	Facets []string `json:"facets,omitempty"`

	// --- plugin-contributed types ------------------------------------
	//
	// Empty on all 35 built-in types. A non-empty Provider means this
	// entry came from another plugin through the host's `reporting`
	// adapter family; the studio then resolves every label below
	// against that plugin's own i18n namespace instead of this one's,
	// which is what lets a contributed report be translated by the team
	// that wrote it rather than by this plugin guessing.

	// Provider is the contributing plugin's slug, and the dispatch key
	// the generate path uses to reach it.
	Provider string `json:"provider,omitempty"`
	// LabelNS is the i18n namespace the four keys below resolve
	// against. Equal to Provider today; kept separate because a plugin
	// shipping several namespaces may not want its reports in the one
	// named after its slug.
	LabelNS string `json:"label_ns,omitempty"`
	// LabelKey and DescriptionKey label the studio card.
	LabelKey       string `json:"label_key,omitempty"`
	DescriptionKey string `json:"description_key,omitempty"`
	// CategoryLabelKey and CategoryDescriptionKey label a shelf the
	// provider minted. Empty for a provider reusing a built-in shelf,
	// which this plugin already labels.
	CategoryLabelKey       string `json:"category_label_key,omitempty"`
	CategoryDescriptionKey string `json:"category_description_key,omitempty"`
	// TitleEN and SubtitleEN are the document's own cover strings, and
	// the fallback the studio shows when the provider's bundle has no
	// entry for LabelKey.
	TitleEN    string `json:"title_en,omitempty"`
	SubtitleEN string `json:"subtitle_en,omitempty"`
}

// supportsSubject reports whether this type may be narrowed to a kind,
// so a request naming a scope the type does not offer is ignored rather
// than quietly emptying the document.
func (t ReportType) supportsSubject(kind SubjectKind) bool {
	for _, k := range t.Subjects {
		if k == kind {
			return true
		}
	}
	return false
}

// ReportOption is a per-report toggle.
type ReportOption struct {
	Key     string `json:"key"`
	Default bool   `json:"default"`
}

// Report type keys.
const (
	TypeAdHoc              = "ad-hoc"
	TypeExecutiveOverview  = "executive-overview"
	TypeEnvironmentHealth  = "environment-health"
	TypeWorkloadInventory  = "workload-inventory"
	TypeIncidentProblem    = "incident-problem"
	TypeSecurityCompliance = "security-compliance"
	TypeCapacity           = "capacity-utilisation"
	TypeChangeDelivery     = "change-delivery"
	TypeAvailability       = "availability-slo"
	TypeBackupRecovery     = "backup-recovery"
	TypeImageSupplyChain   = "image-supply-chain"
	TypeAccessReview       = "access-review"
	TypePlatformInventory  = "platform-inventory"
)

// Report type keys added with the health and uptime history.
//
// The first group exists because the platform can now answer questions about
// a period rather than only about the present. The rest fill gaps a reader
// kept having to fill by hand: per-workload depth, who did what, what the
// estate costs, and whether the operational basics are actually in place.
const (
	// Workload — one question per object, answered in depth.
	TypeContainerService     = "container-service"
	TypeUptimeAvailability   = "uptime-availability"
	TypeHealthcheckQuality   = "healthcheck-quality"
	TypeStabilityFlapping    = "stability-flapping"
	TypeStackPortfolio       = "stack-portfolio"
	TypeOperationalReadiness = "operational-readiness"

	// Assurance — who holds what, and who did what.
	TypeUserActivity      = "user-activity"
	TypePermissionMatrix  = "permission-matrix"
	TypeCredentialHygiene = "credential-hygiene"
	TypeAuditAssurance    = "audit-assurance"

	// Operations — the machinery that keeps the estate running.
	TypeAgentFleet          = "agent-fleet"
	TypeAutomationSchedules = "automation-schedules"
	TypeAlertingCoverage    = "alerting-coverage"
	TypePatchCurrency       = "patch-currency"
	TypeNetworkConnectivity = "network-connectivity"
	TypeVolumeDataEstate    = "volume-data-estate"

	// Finance — what the estate consumes, and who consumes it.
	TypeResourceEfficiency = "resource-efficiency"
	TypeCostAllocation     = "cost-allocation"

	// Service — cross-environment and recovery targets.
	TypeEnvironmentCompare = "environment-comparison"
	TypeDataProtection     = "data-protection-rpo"
)

// Report types about one platform only live in that platform's plugin and
// reach the studio through providers.go like any other contributed type.
// A question every platform can answer is a facet instead (facets.go), so
// the estate is reported once rather than once per platform.

// Option keys shared across types.
const (
	OptIncludeDetail    = "include_detail"
	OptIncludeGlossary  = "include_glossary"
	OptIncludeNarrative = "include_narrative"
)

// catalog is the full set of report types, in the order the studio
// shows them: the board-facing report first, then the working ones.
func catalog() []ReportType {
	common := []ReportOption{
		{Key: OptIncludeDetail, Default: true},
		{Key: OptIncludeGlossary, Default: true},
		{Key: OptIncludeNarrative, Default: true},
	}
	// The two scopes most types share. A workload report answers about a
	// container, and a stack is the unit a team owns, so both are offered
	// wherever either makes sense; an identity report answers about a
	// person, and a group is how a person is usually granted access.
	workload := []SubjectKind{SubjectContainer, SubjectStack}
	identity := []SubjectKind{SubjectUser, SubjectGroup}
	types := []ReportType{
		{
			Key: TypeAdHoc, Category: "custom", Icon: "sparkles",
			Levels: []Level{Strategic, Tactical, Operational},
			// Sources are resolved per run by the pass-1 planner, so this
			// stays empty. EnvironmentScoped is special-cased below because
			// an empty list would otherwise derive "not environment
			// scoped", and an ad-hoc report can reach any source there is.
			Sources:     nil,
			DefaultDays: 30,
			Options: []ReportOption{
				{Key: OptIncludeDetail, Default: true},
				{Key: OptIncludeGlossary, Default: true},
			},
		},
		{
			Key: TypeExecutiveOverview, Category: "board", Icon: "presentation",
			Levels: []Level{Strategic, Tactical, Operational},
			Sources: []string{srcEnvironments, srcDockerInfo, srcContainers, srcImages, srcScans,
				srcEvents, srcAudit, srcUpdates, srcBackups, srcSystem, srcVolumes, srcUptime},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeSecurityCompliance, Category: "assurance", Icon: "shield",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcScans, srcImages, srcUsers, srcRoles, srcGroups, srcAPIKeys, srcAudit, srcIdentity, srcSystem},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeIncidentProblem, Category: "assurance", Icon: "alert",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcEvents, srcAlerts, srcContainers, srcActivity, srcAudit, srcAutoheal, srcUptime, srcRunbookRuns},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeAvailability, Category: "service", Icon: "activity",
			Levels:      []Level{Strategic, Tactical},
			Sources:     []string{srcContainers, srcEvents, srcEnvironments, srcDockerInfo, srcUptime},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeChangeDelivery, Category: "service", Icon: "rocket",
			Levels:      []Level{Strategic, Tactical},
			Sources:     []string{srcActivity, srcAudit, srcStacks, srcUpdates, srcGitops, srcImages},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeEnvironmentHealth, Category: "platform", Icon: "world",
			Levels: []Level{Tactical, Operational},
			Sources: []string{srcEnvironments, srcDockerInfo, srcContainers, srcAgents, srcVolumes,
				srcNetworks, srcUptime},
			DefaultDays: 7, Options: common,
		},
		{
			Key: TypeCapacity, Category: "platform", Icon: "gauge",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcDockerInfo, srcContainers, srcVolumes, srcImages, srcMetrics, srcCapacity},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeBackupRecovery, Category: "assurance", Icon: "archive",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcBackups, srcBackupLogs, srcStorage, srcSchedules, srcContainers},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeImageSupplyChain, Category: "platform", Icon: "package",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcImages, srcRegistries, srcScans, srcUpdates, srcContainers},
			Subjects:    []SubjectKind{SubjectImage, SubjectContainer},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeAccessReview, Category: "assurance", Icon: "users",
			Levels:      []Level{Tactical},
			Sources:     []string{srcUsers, srcRoles, srcGroups, srcAPIKeys, srcIdentity, srcAudit},
			Subjects:    identity,
			DefaultDays: 90, Options: common,
		},
		{
			Key: TypeWorkloadInventory, Category: "platform", Icon: "box",
			Levels:      []Level{Operational},
			Sources:     []string{srcContainers, srcImages, srcVolumes, srcNetworks, srcStacks},
			Subjects:    workload,
			PointInTime: true, Options: common,
		},
		{
			Key: TypePlatformInventory, Category: "platform", Icon: "server",
			Levels: []Level{Operational},
			Sources: []string{srcSystem, srcVersions, srcEnvironments, srcAgents, srcPlugins,
				srcSettings},
			PointInTime: true, Options: common,
		},

		// --- workload: one object, in depth -------------------------

		{
			Key: TypeUptimeAvailability, Category: "workload", Icon: "activity",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcUptime, srcContainers, srcEvents, srcActivity, srcEnvironments},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeContainerService, Category: "workload", Icon: "box",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcContainers, srcUptime, srcImages, srcVolumes, srcNetworks, srcBackups, srcUpdates, srcActivity},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeHealthcheckQuality, Category: "workload", Icon: "stethoscope",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcUptime, srcContainers},
			Subjects:    workload,
			DefaultDays: 7, Options: common,
		},
		{
			Key: TypeStabilityFlapping, Category: "workload", Icon: "wave",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcUptime, srcActivity, srcContainers, srcAutoheal, srcEvents},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeStackPortfolio, Category: "workload", Icon: "stack",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcStacks, srcContainers, srcUptime, srcImages, srcBackups},
			Subjects:    []SubjectKind{SubjectStack},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeOperationalReadiness, Category: "workload", Icon: "checklist",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcContainers, srcUptime, srcBackups, srcUpdates, srcScans, srcAlerts},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},

		// --- assurance: who holds what, and who did what -------------

		{
			Key: TypeUserActivity, Category: "assurance", Icon: "user",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcAudit, srcUsers, srcActivity, srcRoles},
			Subjects:    identity,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypePermissionMatrix, Category: "assurance", Icon: "grid",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcRoles, srcGroups, srcUsers, srcAPIKeys},
			Subjects:    identity,
			PointInTime: true, Options: common,
		},
		{
			Key: TypeCredentialHygiene, Category: "assurance", Icon: "key",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcAPIKeys, srcUsers, srcIdentity, srcAudit},
			Subjects:    []SubjectKind{SubjectUser},
			DefaultDays: 90, Options: common,
		},
		{
			Key: TypeAuditAssurance, Category: "assurance", Icon: "certificate",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcAudit, srcUsers, srcSettings},
			Subjects:    []SubjectKind{SubjectUser},
			DefaultDays: 30, Options: common,
		},

		// --- operations: the machinery -------------------------------

		{
			Key: TypePatchCurrency, Category: "operations", Icon: "refresh",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcImages, srcUpdates, srcContainers, srcRegistries, srcScans},
			Subjects:    []SubjectKind{SubjectImage, SubjectContainer},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeAutomationSchedules, Category: "operations", Icon: "clock",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcSchedules, srcUpdates, srcBackups, srcAutoheal, srcAlerts},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeAlertingCoverage, Category: "operations", Icon: "bell",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcAlerts, srcContainers, srcUptime, srcStacks},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeAgentFleet, Category: "operations", Icon: "plug",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcAgents, srcEnvironments, srcVersions},
			PointInTime: true, Options: common,
		},
		{
			Key: TypeNetworkConnectivity, Category: "operations", Icon: "network",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcNetworks, srcContainers, srcStacks, srcEnvironments},
			Subjects:    []SubjectKind{SubjectNetwork, SubjectContainer},
			PointInTime: true, Options: common,
		},
		{
			Key: TypeVolumeDataEstate, Category: "operations", Icon: "database",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcVolumes, srcContainers, srcBackups, srcStorage},
			Subjects:    []SubjectKind{SubjectVolume, SubjectContainer},
			PointInTime: true, Options: common,
		},

		// --- finance: what it consumes, and for whom ------------------

		{
			Key: TypeResourceEfficiency, Category: "finance", Icon: "gauge",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcMetrics, srcContainerMetrics, srcContainers, srcDockerInfo, srcImages, srcVolumes},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeCostAllocation, Category: "finance", Icon: "coin",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcContainers, srcStacks, srcVolumes, srcImages, srcMetrics, srcContainerMetrics},
			Subjects:    []SubjectKind{SubjectStack, SubjectContainer},
			DefaultDays: 30, Options: common,
		},

		// --- service: across environments, and against recovery targets

		{
			Key: TypeEnvironmentCompare, Category: "service", Icon: "columns",
			Levels:      []Level{Strategic, Tactical},
			Sources:     []string{srcEnvironments, srcAgents, srcContainers, srcDockerInfo, srcUptime},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeDataProtection, Category: "service", Icon: "lifebuoy",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcBackups, srcBackupLogs, srcContainers, srcStorage, srcSchedules},
			Subjects:    workload,
			DefaultDays: 30, Options: common,
		},
	}
	for _, fam := range reportFamilies() {
		types = append(types, fam.types(common)...)
	}
	// A report that prints a facet also collects the Docker sources the
	// facet's Docker rows are projected from.
	for i := range types {
		types[i].Facets = facetsForType(types[i].Key)
		types[i].Sources = withFacetSources(types[i].Sources, types[i].Facets)
	}
	// Derived rather than declared, so a source added to an entry cannot
	// leave the studio asking — or not asking — the wrong question.
	for i := range types {
		// An ad-hoc report's sources are not known until the planner has
		// run, so it must always be offered the environment question.
		types[i].EnvironmentScoped = types[i].Key == TypeAdHoc || environmentScoped(types[i].Sources)
	}
	// Plugin-contributed types are appended *after* the derivation
	// above, never through it: a provider collects its own data, so it
	// has no entry in envScopedSources and the derivation would read
	// every contributed type as not environment scoped. The provider
	// declares that answer itself (see providers.go).
	return append(types, providerTypes()...)
}

// envScopedSources are the sources whose contents change when the
// collector passes a different `?env=`.
//
// The exclusions matter as much as the entries. `environments`, `agents`
// and `plugins` each list every row on the installation regardless of
// the env parameter, and `system` / `versions` / `settings` / `users` /
// `roles` / `groups` / `api_keys` / `identity` / `audit` /
// `storage_locations` / `schedules` / `registries` / `gitops` are
// platform-wide by nature. A type built only from those — access-review,
// platform-inventory — reads identically whichever environment is
// chosen, so the studio must not ask a question that cannot change the
// answer.
var envScopedSources = map[string]bool{
	srcDockerInfo: true, srcContainers: true, srcImages: true,
	srcVolumes: true, srcNetworks: true, srcStacks: true,
	srcScans: true, srcActivity: true, srcEvents: true,
	srcAutoheal: true, srcMetrics: true, srcBackups: true,
	srcContainerMetrics: true,
	srcBackupLogs:       true, srcUpdates: true, srcAlerts: true,
	srcUptime:   true,
	srcCapacity: true,
}

func environmentScoped(sources []string) bool {
	for _, s := range sources {
		if envScopedSources[s] {
			return true
		}
		for _, fam := range reportFamilies() {
			if fam.envScoped[s] {
				return true
			}
		}
	}
	return false
}

func reportTypeByKey(key string) (ReportType, bool) {
	for _, t := range catalog() {
		if t.Key == key {
			return t, true
		}
	}
	return ReportType{}, false
}

// resolveLevels intersects the levels a caller asked for with the ones
// the type actually produces, preserving catalog order. An empty or
// fully-filtered request falls back to every level the type has, so a
// bad parameter degrades to "the whole report" rather than an empty one.
func (t ReportType) resolveLevels(requested []Level) []Level {
	if len(requested) == 0 {
		return t.Levels
	}
	want := make(map[Level]bool, len(requested))
	for _, l := range requested {
		want[l] = true
	}
	var out []Level
	for _, l := range t.Levels {
		if want[l] {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return t.Levels
	}
	return out
}

func (t ReportType) hasLevel(l Level) bool {
	for _, x := range t.Levels {
		if x == l {
			return true
		}
	}
	return false
}

// defaultPeriod is the window the studio pre-selects for a type.
func (t ReportType) defaultPeriod(now time.Time) (time.Time, time.Time) {
	if t.PointInTime || t.DefaultDays <= 0 {
		return now, now
	}
	return now.AddDate(0, 0, -t.DefaultDays), now
}
