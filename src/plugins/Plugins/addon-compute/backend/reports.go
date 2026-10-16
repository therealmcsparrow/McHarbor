// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The report types this plugin contributes to the Reporting plugin's Report
// Studio.
//
// It costs nothing when Reporting is not installed: the provider registers
// either way, and the types only become reachable once something is there to
// show them.
const (
	reportComputeReadiness = "compute-readiness"
	reportComputeHygiene   = "compute-image-lease-hygiene"
)

// reportBuildTimeout bounds one report. The studio runs Build inside the
// generation job's context and a provider that blocks forever hangs a job the
// operator is watching.
const reportBuildTimeout = 2 * time.Minute

// computeReportSpec declares the two report types and the facets this plugin
// contributes to the shared reports.
//
// Both types are point-in-time. Compute infrastructure has no meaningful
// period-over-period story in this plugin — pool history lives in the metrics
// module, and offering a date range this provider would then ignore is the one
// way to make the declaration dishonest.
func computeReportSpec() plugin.ReportProviderSpec {
	return plugin.ReportProviderSpec{
		Slug:        "addon-compute",
		DisplayName: "Compute",
		Facets: []plugin.ReportFacetDecl{
			{Facet: plugin.ReportFacetInventory, Platform: computePlatform, PlatformLabelKey: "compute:reports.platform"},
			{Facet: plugin.ReportFacetCapacity, Platform: computePlatform, PlatformLabelKey: "compute:reports.platform"},
			{Facet: plugin.ReportFacetHealth, Platform: computePlatform, PlatformLabelKey: "compute:reports.platform"},
		},
		Types: []plugin.ReportTypeDecl{
			{
				Key:               reportComputeReadiness,
				Category:          "assurance",
				Levels:            []string{"operational"},
				PointInTime:       true,
				TitleEN:           "Compute host readiness",
				SubtitleEN:        "What each host can run, and what is stopping the ones that cannot",
				LabelKey:          "compute:reports.readiness.label",
				DescriptionKey:    "compute:reports.readiness.description",
				EnvironmentScoped: true,
				Subjects:          []string{"compute-host"},
				Options: []plugin.ReportOptionDecl{
					{Key: "includeRemediation", Default: true},
					{Key: "aiSummary", Default: false},
				},
			},
			{
				Key:               reportComputeHygiene,
				Category:          "operations",
				Levels:            []string{"tactical", "operational"},
				PointInTime:       true,
				TitleEN:           "Compute image and lease hygiene",
				SubtitleEN:        "Image cache state per host, unused images, and stale or orphaned DHCP leases",
				LabelKey:          "compute:reports.hygiene.label",
				DescriptionKey:    "compute:reports.hygiene.description",
				EnvironmentScoped: true,
				Subjects:          []string{"compute-host"},
				Options: []plugin.ReportOptionDecl{
					{Key: "onlyIssues", Default: false},
				},
			},
		},
	}
}

// reportProvider builds this plugin's reports.
type reportProvider struct {
	plugin *ComputePlugin
}

var _ plugin.ReportProviderClient = (*reportProvider)(nil)

// Subjects lists the compute hosts a report can be narrowed to.
func (r *reportProvider) Subjects(
	ctx context.Context,
	kind string,
	_ plugin.ReportRequest,
) ([]plugin.ReportSubjectOption, error) {
	if kind != "compute-host" || r.plugin.svc == nil {
		return nil, nil
	}
	environments, err := r.plugin.svc.ListEnvironments(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]plugin.ReportSubjectOption, 0, len(environments))
	for _, env := range environments {
		if !env.ComputeEnabled {
			continue
		}
		name := env.Name
		if name == "" {
			name = env.EnvironmentID
		}
		hint := "ready"
		if !env.Available {
			hint = unreachableHint(env.UnavailableReason)
		}
		out = append(out, plugin.ReportSubjectOption{ID: env.EnvironmentID, Name: name, Hint: hint})
	}
	return out, nil
}

func unreachableHint(reason string) string {
	switch reason {
	case "not_agent_environment":
		return "not an agent host"
	case "agent_too_old":
		return "agent too old"
	default:
		return "agent offline"
	}
}

// Build composes one report.
//
// A host that could not be read is recorded in Sources with OK=false and left
// out of every figure, rather than counted as a zero. A report that quietly
// treats an unreachable host as an empty one understates the estate, which is
// the failure mode that matters here.
func (r *reportProvider) Build(
	ctx context.Context,
	req plugin.ReportRequest,
) (plugin.ReportResult, error) {
	if r.plugin.svc == nil {
		return plugin.ReportResult{}, fmt.Errorf("compute: the plugin is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()

	started := time.Now()
	environments, err := r.plugin.svc.ListEnvironments(ctx)
	if err != nil {
		return plugin.ReportResult{}, fmt.Errorf("compute: read hosts: %w", err)
	}

	// Narrow to the chosen environment. An empty EnvironmentID is the studio's
	// "All environments", and covering the estate is the right answer for it —
	// ignoring the field entirely, which is what this did before, produced a
	// report about every host under a cover that named one.
	if req.EnvironmentID != "" {
		scoped := make([]ComputeEnvironment, 0, len(environments))
		for _, env := range environments {
			if env.EnvironmentID == req.EnvironmentID {
				scoped = append(scoped, env)
			}
		}
		environments = scoped
	}

	// Narrow to the chosen host, when one was chosen.
	if req.SubjectKind == "compute-host" && req.SubjectID != "" {
		filtered := environments[:0]
		for _, env := range environments {
			if env.EnvironmentID == req.SubjectID {
				filtered = append(filtered, env)
			}
		}
		environments = filtered
	}

	sources := []plugin.ReportSourceStatus{{
		Key:        "compute_hosts",
		Label:      "Compute hosts",
		OK:         true,
		DurationMS: time.Since(started).Milliseconds(),
	}}

	switch req.TypeKey {
	case reportComputeReadiness:
		return r.buildReadiness(ctx, req, environments, sources)
	case reportComputeHygiene:
		return r.buildHygiene(ctx, req, environments, sources)
	default:
		return plugin.ReportResult{}, fmt.Errorf("compute: unknown report type %q", req.TypeKey)
	}
}

func (r *reportProvider) buildReadiness(
	ctx context.Context,
	req plugin.ReportRequest,
	environments []ComputeEnvironment,
	sources []plugin.ReportSourceStatus,
) (plugin.ReportResult, error) {
	var hostRows [][]plugin.ReportCell
	var blockerRows [][]plugin.ReportCell
	var notes []string
	ready := 0

	for _, env := range environments {
		if !env.ComputeEnabled {
			continue
		}
		name := hostName(env)

		if env.Capability == nil {
			hostRows = append(hostRows, []plugin.ReportCell{
				{Text: name},
				{Text: "never checked", Status: "warning", Pill: true},
				{Text: "—"}, {Text: "—"},
			})
			notes = append(notes, fmt.Sprintf("%s has never been checked.", name))
			continue
		}

		blockers := 0
		for _, item := range env.Capability.Items {
			if !item.Critical || item.Present {
				continue
			}
			blockers++
			if req.Options["includeRemediation"] {
				blockerRows = append(blockerRows, []plugin.ReportCell{
					{Text: name},
					{Text: item.Key, Mono: true},
					{Text: item.Detail},
					{Text: item.Remediation, Mono: true},
				})
			}
		}
		if blockers == 0 {
			ready++
		}

		hostRows = append(hostRows, []plugin.ReportCell{
			{Text: name},
			readinessCell(blockers),
			{Text: yesNo(env.Capability.CanRunVMs)},
			{Text: yesNo(env.Capability.CanRunCTs)},
		})

		// The seccomp posture is a note rather than a blocker: containers run
		// either way, but an operator who assumed a syscall filter was there
		// should read it here rather than discover it in an incident.
		if !env.Capability.SeccompBuiltIn {
			notes = append(notes, fmt.Sprintf(
				"%s runs an agent built without seccomp, so system containers there have no syscall filter. User namespace isolation still applies.",
				name))
		}
	}

	blocked := len(hostRows) - ready
	summaryBlocks := []plugin.ReportBlock{
		leadBlock(readinessLead(len(hostRows), ready, blocked, len(blockerRows))),
		{
			Kind: "kpis",
			KPIs: []plugin.ReportKPI{
				{Label: "Hosts checked", Value: fmt.Sprintf("%d", len(hostRows))},
				{Label: "Ready", Value: fmt.Sprintf("%d", ready), Status: statusFor(ready, len(hostRows))},
				{Label: "With blockers", Value: fmt.Sprintf("%d", blocked),
					Status: kpiStatus(blocked == 0)},
				{Label: "Checks failing", Value: fmt.Sprintf("%d", len(blockerRows)),
					Hint: "Critical capability checks, across every host."},
			},
		},
	}

	if len(hostRows) > 0 {
		points := []plugin.ReportChartPoint{}
		if ready > 0 {
			points = append(points, chartPoint("Ready", float64(ready), "good"))
		}
		if blocked > 0 {
			points = append(points, chartPoint("Blocked", float64(blocked), "bad"))
		}
		summaryBlocks = append(summaryBlocks, donutChart("Host readiness", points))
	}

	if blocked > 0 {
		summaryBlocks = append(summaryBlocks, findingsBlock([]plugin.ReportFinding{
			finding("high", plural(blocked, "host cannot run guests", "hosts cannot run guests"),
				fmt.Sprintf("%s failing across them. A host in this state accepts a guest definition and fails at start, which reads as a broken guest rather than a missing host package.",
					plural(len(blockerRows), "critical check is", "critical checks are")),
				"Run the fix listed for each host in \"What is missing\", then re-probe the host from Compute so its capability is refreshed."),
		}))
	}

	sections := []plugin.ReportSection{
		{
			Level:  "summary",
			Title:  "Summary",
			Blocks: summaryBlocks,
		},
		{
			Level: "operational",
			Title: "Host readiness",
			Blocks: []plugin.ReportBlock{{
				Kind: "table",
				Columns: []plugin.ReportColumn{
					{Label: "Host"}, {Label: "State"},
					{Label: "VMs"}, {Label: "Containers"},
				},
				Rows: hostRows,
			}},
		},
	}

	if len(blockerRows) > 0 {
		sections = append(sections, plugin.ReportSection{
			Level: "operational",
			Title: "What is missing",
			Blocks: []plugin.ReportBlock{{
				Kind: "table",
				Columns: []plugin.ReportColumn{
					{Label: "Host"}, {Label: "Check", Mono: true},
					{Label: "Observed"}, {Label: "Fix", Mono: true},
				},
				Rows:    blockerRows,
				Caption: "Each fix is the command for that host's own distribution.",
			}},
		})
	}

	// The AI narrative is opt-in and additive. A report without it is complete;
	// this only adds a written reading of the same table, and it is skipped
	// silently when no AI provider is configured rather than failing the run.
	if req.Options["aiSummary"] {
		if summary := r.aiSummary(ctx, hostRows, blockerRows); summary != "" {
			sections = append([]plugin.ReportSection{{
				Level: "summary",
				Title: "Assessment",
				Blocks: []plugin.ReportBlock{
					{Kind: "paragraph", Text: summary, Lead: true},
				},
			}}, sections...)
			sources = append(sources, plugin.ReportSourceStatus{
				Key: "compute_ai_summary", Label: "AI assessment", OK: true,
			})
		} else {
			sources = append(sources, plugin.ReportSourceStatus{
				Key:   "compute_ai_summary",
				Label: "AI assessment",
				OK:    false,
				Note:  "No AI provider is configured, so the written assessment was skipped.",
			})
		}
	}

	return plugin.ReportResult{Sections: sections, Sources: sources, Notes: notes}, nil
}

// aiSummary asks the host's AI router to read the readiness tables.
//
// Only the tables go across — host names, check keys and remediation commands.
// No credentials, no addresses, no guest data. It returns empty on any failure,
// including no configured provider, because a report is complete without it.
func (r *reportProvider) aiSummary(
	ctx context.Context,
	hosts [][]plugin.ReportCell,
	blockers [][]plugin.ReportCell,
) string {
	ai := r.plugin.host.AI
	if ai == nil || !ai.Available() {
		return ""
	}

	var facts strings.Builder
	facts.WriteString("Compute host readiness:\n")
	for _, row := range hosts {
		if len(row) >= 4 {
			fmt.Fprintf(&facts, "- %s: %s (VMs: %s, containers: %s)\n",
				row[0].Text, row[1].Text, row[2].Text, row[3].Text)
		}
	}
	if len(blockers) > 0 {
		facts.WriteString("\nBlocking items:\n")
		for _, row := range blockers {
			if len(row) >= 3 {
				fmt.Fprintf(&facts, "- %s: %s — %s\n",
					row[0].Text, row[1].Text, row[2].Text)
			}
		}
	}

	// A tight ceiling on a pointer field: the host distinguishes "unset" from
	// "deliberately small", and this is a paragraph in a report, not an essay.
	maxTokens := 400
	resp, err := ai.Complete(ctx, plugin.AIRequest{
		MaxTokens: &maxTokens,
		Messages: []plugin.AIMessage{
			{
				Role: "system",
				Content: "You summarise infrastructure readiness for an operations report. " +
					"Write two or three sentences of plain prose. State what is ready and what " +
					"is blocking, name the hosts, and do not invent anything that is not in " +
					"the data. No bullet points, no headings, no preamble.",
			},
			{Role: "user", Content: facts.String()},
		},
	})
	if err != nil {
		r.plugin.logger.Debug("compute: AI report summary unavailable", "error", err)
		return ""
	}
	return strings.TrimSpace(resp.Content)
}

// --- small helpers --------------------------------------------------------

func hostName(env ComputeEnvironment) string {
	if env.Name != "" {
		return env.Name
	}
	return env.EnvironmentID
}

func readinessCell(blockers int) plugin.ReportCell {
	if blockers == 0 {
		return plugin.ReportCell{Text: "ready", Status: "success", Pill: true}
	}
	return plugin.ReportCell{
		Text:   fmt.Sprintf("%d blocking", blockers),
		Status: "danger",
		Pill:   true,
	}
}

func statusFor(ready, total int) string {
	switch {
	case total == 0:
		return ""
	case ready == total:
		return "success"
	case ready == 0:
		return "danger"
	default:
		return "warning"
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func orDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

// humanBytes renders binary units, matching what `zfs list` and `df` print so
// the report and the shell agree.
func humanBytes(bytes int64) string {
	if bytes <= 0 {
		return "—"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
