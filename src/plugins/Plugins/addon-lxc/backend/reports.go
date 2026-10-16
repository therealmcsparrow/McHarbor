// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// reportCTIsolation is the one report type this plugin keeps: what separates a
// container from its host is specific to system containers. Inventory, capacity,
// protection, health and cost reach the shared reports as facets.
const reportCTIsolation = "lxc-isolation"

// facetPlatform is the token every facet row carries; the printed name is the
// spec's DisplayName.
const facetPlatform = "lxc"

// facetPlatformLabelKey names the platform in this plugin's own bundle.
const facetPlatformLabelKey = "ct:reports.platform"

const reportBuildTimeout = 2 * time.Minute

// lxcReportSpec declares the isolation report type and the facets this plugin
// contributes to the shared reports. Exposure is absent on purpose: no port
// forward or published port is modelled here, so there is nothing to report.
func lxcReportSpec() plugin.ReportProviderSpec {
	facets := make([]plugin.ReportFacetDecl, 0, len(lxcFacets))
	for _, f := range lxcFacets {
		facets = append(facets, plugin.ReportFacetDecl{
			Facet: f, Platform: facetPlatform, PlatformLabelKey: facetPlatformLabelKey,
		})
	}
	return plugin.ReportProviderSpec{
		Slug:        "addon-lxc",
		DisplayName: "System Containers",
		Facets:      facets,
		Types: []plugin.ReportTypeDecl{
			{
				// The one a reviewer actually asks for: for a container the
				// interesting risk is "what boundary has been removed", because a
				// privileged container with a host bind mount is not isolated from
				// the host in any meaningful sense and nothing else in the UI
				// gathers that fact across an estate.
				Key:               reportCTIsolation,
				Category:          "assurance",
				Levels:            []string{"strategic", "tactical"},
				PointInTime:       true,
				TitleEN:           "System container isolation",
				SubtitleEN:        "Privilege, id mapping, host mounts and nesting",
				LabelKey:          "ct:reports.isolation.label",
				DescriptionKey:    "ct:reports.isolation.description",
				EnvironmentScoped: true,
				Subjects:          []string{"lxc-host"},
				Options: []plugin.ReportOptionDecl{
					{Key: "onlyExposed", Default: false},
					{Key: "aiSummary", Default: false},
				},
			},
		},
	}
}

// reportProvider builds this plugin's reports and answers its facets.
type reportProvider struct {
	plugin *LXCPlugin
	// source and perms are set by tests; nil means the live service and RBAC handle.
	source facetSource
	perms  facetPermissions
}

var _ plugin.ReportProviderClient = (*reportProvider)(nil)

// Subjects lists the hosts a report can be narrowed to.
func (r *reportProvider) Subjects(
	ctx context.Context,
	kind string,
	_ plugin.ReportRequest,
) ([]plugin.ReportSubjectOption, error) {
	if kind != "lxc-host" || r.plugin.svc == nil {
		return nil, nil
	}
	containers, err := r.plugin.svc.List(ctx, "", false)
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	for _, ct := range containers {
		counts[ct.EnvironmentID]++
	}

	out := make([]plugin.ReportSubjectOption, 0, len(counts))
	for envID, count := range counts {
		out = append(out, plugin.ReportSubjectOption{
			ID: envID, Name: envID,
			Hint: fmt.Sprintf("%d containers", count),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Build composes one report.
func (r *reportProvider) Build(
	ctx context.Context,
	req plugin.ReportRequest,
) (plugin.ReportResult, error) {
	if r.plugin.svc == nil {
		return plugin.ReportResult{}, fmt.Errorf("lxc: the plugin is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()

	started := time.Now()
	envID := req.EnvironmentID
	if req.SubjectKind == "lxc-host" && req.SubjectID != "" {
		envID = req.SubjectID
	}

	containers, err := r.plugin.svc.List(ctx, envID, false)
	if err != nil {
		return plugin.ReportResult{}, fmt.Errorf("lxc: read containers: %w", err)
	}

	sources := []plugin.ReportSourceStatus{{
		Key: "lxc_containers", Label: "System containers", OK: true,
		DurationMS: time.Since(started).Milliseconds(),
	}}

	// A container whose host is unreachable is named and excluded, rather than
	// counted with whatever its last known figures were.
	var unreachable []string
	reachable := containers[:0]
	for _, ct := range containers {
		if !ct.HostAvailable {
			unreachable = append(unreachable, ct.Name)
			continue
		}
		reachable = append(reachable, ct)
	}

	var notes []string
	if len(unreachable) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d containers could not be read because their host is unreachable and are excluded from the figures: %s.",
			len(unreachable), strings.Join(unreachable, ", ")))
	}

	switch req.TypeKey {
	case reportCTIsolation:
		return r.buildIsolation(ctx, req, reachable, sources, notes)
	default:
		return plugin.ReportResult{}, fmt.Errorf("lxc: unknown report type %q", req.TypeKey)
	}
}

// buildIsolation reports what separates each container from its host.
//
// The container analogue of a backup-coverage report, and for the same reason:
// the fact that matters is spread across several fields nobody reviews
// together. A privileged container, a container with a bind mount of a host
// path, a container with nesting on, and a container whose mounts are not
// id-shifted are all normal individually and alarming as a set.
func (r *reportProvider) buildIsolation(
	ctx context.Context,
	req plugin.ReportRequest,
	containers []Container,
	sources []plugin.ReportSourceStatus,
	notes []string,
) (plugin.ReportResult, error) {
	type entry struct {
		ct         Container
		hostMounts []string
		unshifted  int
		// exposure is a small ordinal used only for sorting: it is a triage
		// order, not a score anybody should quote.
		exposure int
	}

	var entries []entry
	privileged, nested, withHostMounts := 0, 0, 0

	for _, ct := range containers {
		mounts, err := r.plugin.svc.Mounts(ctx, ct.ID)
		if err != nil {
			continue
		}

		item := entry{ct: ct}
		for _, mount := range mounts {
			if mount.Kind != MountVolume {
				item.hostMounts = append(item.hostMounts, mount.Destination)
			}
			if !mount.ShiftIDs && !ct.Privileged {
				item.unshifted++
			}
		}

		if ct.Privileged {
			privileged++
			item.exposure += 4
		}
		if ct.Nesting {
			nested++
			item.exposure += 2
		}
		if len(item.hostMounts) > 0 {
			withHostMounts++
			item.exposure += 3
		}
		if ct.IDMapMechanism == "chown" {
			// Not a hole, but worth surfacing: a chowned rootfs cannot be
			// shared with another container and cannot be moved to a host with
			// a different id range without being rewritten again.
			item.exposure++
		}

		if req.Options["onlyExposed"] && item.exposure == 0 {
			continue
		}
		entries = append(entries, item)
	}

	// Most exposed first: that is the order somebody reviewing this reads in.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].exposure != entries[j].exposure {
			return entries[i].exposure > entries[j].exposure
		}
		return entries[i].ct.Name < entries[j].ct.Name
	})

	rows := make([][]plugin.ReportCell, 0, len(entries))
	for _, item := range entries {
		hostMounts := "—"
		hostMountStatus := ""
		if len(item.hostMounts) > 0 {
			hostMounts = strings.Join(item.hostMounts, ", ")
			hostMountStatus = "warning"
		}

		idMapping := "—"
		if !item.ct.Privileged {
			idMapping = fmt.Sprintf("%d..%d (%s)",
				item.ct.UIDMapStart, item.ct.UIDMapStart+item.ct.UIDMapCount,
				item.ct.IDMapMechanism)
		}

		unshifted := "—"
		unshiftedStatus := ""
		if item.unshifted > 0 {
			unshifted = fmt.Sprintf("%d", item.unshifted)
			unshiftedStatus = "warning"
		}

		rows = append(rows, []plugin.ReportCell{
			{Text: item.ct.Name},
			{Text: item.ct.EnvironmentID},
			privilegeCell(item.ct),
			boolCell(item.ct.Nesting, "on", "off", item.ct.Nesting),
			{Text: hostMounts, Status: hostMountStatus},
			{Text: unshifted, Status: unshiftedStatus},
			{Text: idMapping, Mono: true},
		})
	}

	sections := []plugin.ReportSection{
		{
			Level: "summary",
			Title: "Summary",
			Blocks: []plugin.ReportBlock{{
				Kind: "kpis",
				KPIs: []plugin.ReportKPI{
					{Label: "Containers", Value: fmt.Sprintf("%d", len(containers))},
					{Label: "Privileged", Value: fmt.Sprintf("%d", privileged),
						Status: boolStatus(privileged == 0),
						Hint:   "Root inside a privileged container is root on the host in every way that matters."},
					{Label: "With host mounts", Value: fmt.Sprintf("%d", withHostMounts),
						Status: boolStatus(withHostMounts == 0)},
					{Label: "Nesting enabled", Value: fmt.Sprintf("%d", nested),
						Status: boolStatus(nested == 0)},
				},
			}},
		},
		{
			Level: "tactical",
			Title: "Isolation",
			Blocks: []plugin.ReportBlock{{
				Kind: "table",
				Columns: []plugin.ReportColumn{
					{Label: "Container"}, {Label: "Host"},
					{Label: "Privilege"}, {Label: "Nesting"},
					{Label: "Host mounts"}, {Label: "Unshifted mounts"},
					{Label: "Id mapping", Mono: true},
				},
				Rows:    rows,
				Caption: "Most exposed first.",
				Note:    "An unprivileged container with only pool volumes and shifted ids is isolated from its host. Every other row on this table is a deliberate exception somebody made.",
			}},
		},
	}

	if req.Options["aiSummary"] {
		if summary := r.aiSummary(ctx, rows, privileged, withHostMounts); summary != "" {
			sections = append([]plugin.ReportSection{{
				Level:  "summary",
				Title:  "Assessment",
				Blocks: []plugin.ReportBlock{{Kind: "paragraph", Text: summary, Lead: true}},
			}}, sections...)
			sources = append(sources, plugin.ReportSourceStatus{
				Key: "lxc_ai_summary", Label: "AI assessment", OK: true,
			})
		} else {
			sources = append(sources, plugin.ReportSourceStatus{
				Key: "lxc_ai_summary", Label: "AI assessment", OK: false,
				Note: "No AI provider is configured, so the written assessment was skipped.",
			})
		}
	}

	return plugin.ReportResult{Sections: sections, Sources: sources, Notes: notes}, nil
}

// aiSummary asks the host's AI router to read the isolation table.
//
// Only container names, hosts and the isolation columns cross the boundary — no
// mount sources, no addresses, no credentials. Returns empty on any failure,
// because the report is complete without it.
func (r *reportProvider) aiSummary(
	ctx context.Context,
	rows [][]plugin.ReportCell,
	privileged, withHostMounts int,
) string {
	ai := r.plugin.host.AI
	if ai == nil || !ai.Available() {
		return ""
	}

	var facts strings.Builder
	fmt.Fprintf(&facts,
		"System container isolation. %d containers are privileged; %d have mounts of host paths outside their storage pool.\n\n",
		privileged, withHostMounts)
	// A ceiling on rows as well as tokens: a hundred-container estate would
	// otherwise send a hundred lines to be summarised into three.
	for i, row := range rows {
		if i >= 40 {
			fmt.Fprintf(&facts, "...and %d more containers.\n", len(rows)-i)
			break
		}
		if len(row) >= 6 {
			fmt.Fprintf(&facts,
				"- %s on %s: %s, nesting %s, host mounts at %s, %s unshifted mounts\n",
				row[0].Text, row[1].Text, row[2].Text, row[3].Text, row[4].Text, row[5].Text)
		}
	}

	maxTokens := 400
	resp, err := ai.Complete(ctx, plugin.AIRequest{
		MaxTokens: &maxTokens,
		Messages: []plugin.AIMessage{
			{
				Role: "system",
				Content: "You summarise system container isolation for an operations report. " +
					"Write two or three sentences of plain prose. Name the containers that " +
					"most weaken the boundary with their host and say why. Do not invent " +
					"anything that is not in the data. No bullet points, no headings, no " +
					"preamble.",
			},
			{Role: "user", Content: facts.String()},
		},
	})
	if err != nil {
		r.plugin.logger.Debug("lxc: AI report summary unavailable", "error", err)
		return ""
	}
	return strings.TrimSpace(resp.Content)
}

// --- Cell helpers ---------------------------------------------------------

// privilegeCell is the one column a reviewer scans for.
func privilegeCell(ct Container) plugin.ReportCell {
	if ct.Privileged {
		return plugin.ReportCell{Text: "privileged", Status: "danger", Pill: true}
	}
	return plugin.ReportCell{Text: "unprivileged", Status: "success", Pill: true}
}

func boolCell(value bool, whenTrue, whenFalse string, warnWhenTrue bool) plugin.ReportCell {
	cell := plugin.ReportCell{Text: whenFalse}
	if value {
		cell.Text = whenTrue
		if warnWhenTrue {
			cell.Status = "warning"
		}
	}
	return cell
}

func boolStatus(good bool) string {
	if good {
		return "success"
	}
	return "warning"
}
