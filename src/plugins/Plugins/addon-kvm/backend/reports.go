// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The report types this plugin contributes to the Report Studio. Inventory,
// protection and allocation are facet contributions (reports_facets.go).
const (
	reportVMBackupAssurance   = "kvm-backup-assurance"
	reportVMGuestStability    = "kvm-guest-stability"
	reportVMSnapshotSchedules = "kvm-snapshot-schedules"
)

const reportBuildTimeout = 2 * time.Minute

// kvmReportSpec declares the report types.
func kvmReportSpec() plugin.ReportProviderSpec {
	return plugin.ReportProviderSpec{
		Slug:        "addon-kvm",
		DisplayName: "Virtual Machines",
		Facets:      kvmFacetDecls(),
		Types: []plugin.ReportTypeDecl{
			{
				// A snapshot shares the pool with the disk it protects; this is
				// the report that says whether a copy exists anywhere else.
				Key:               reportVMBackupAssurance,
				Category:          "assurance",
				Levels:            []string{"strategic", "tactical", "operational"},
				DefaultDays:       30,
				TitleEN:           "Virtual machine backup assurance",
				SubtitleEN:        "Plan coverage, run success, last good backup and delivery to every destination",
				LabelKey:          "vm:reports.backupAssurance.label",
				DescriptionKey:    "vm:reports.backupAssurance.description",
				EnvironmentScoped: true,
				Subjects:          []string{"kvm-host"},
			},
			{
				Key:               reportVMGuestStability,
				Category:          "operations",
				Levels:            []string{"tactical", "operational"},
				DefaultDays:       30,
				TitleEN:           "Virtual machine guest stability",
				SubtitleEN:        "Crashes, unexpected stops, flapping guests, guest agent health and failed lifecycle actions",
				LabelKey:          "vm:reports.guestStability.label",
				DescriptionKey:    "vm:reports.guestStability.description",
				EnvironmentScoped: true,
				Subjects:          []string{"kvm-host"},
			},
			{
				Key:               reportVMSnapshotSchedules,
				Category:          "assurance",
				Levels:            []string{"tactical", "operational"},
				DefaultDays:       30,
				TitleEN:           "Virtual machine snapshot schedules",
				SubtitleEN:        "Runs due against runs taken, missed and failed firings, and retention",
				LabelKey:          "vm:reports.snapshotSchedules.label",
				DescriptionKey:    "vm:reports.snapshotSchedules.description",
				EnvironmentScoped: true,
				Subjects:          []string{"kvm-host"},
			},
		},
	}
}

// reportProvider builds this plugin's reports.
type reportProvider struct {
	plugin *KVMPlugin
}

var _ plugin.ReportProviderClient = (*reportProvider)(nil)

// Subjects lists the hosts a report can be narrowed to.
func (r *reportProvider) Subjects(
	ctx context.Context,
	kind string,
	_ plugin.ReportRequest,
) ([]plugin.ReportSubjectOption, error) {
	if kind != "kvm-host" || r.plugin.svc == nil {
		return nil, nil
	}
	guests, err := r.plugin.svc.List(ctx, "", false)
	if err != nil {
		return nil, err
	}

	// One entry per host, counted from the guests on it.
	counts := map[string]int{}
	for _, vm := range guests {
		counts[vm.EnvironmentID]++
	}

	out := make([]plugin.ReportSubjectOption, 0, len(counts))
	for envID, count := range counts {
		out = append(out, plugin.ReportSubjectOption{
			ID: envID, Name: envID,
			Hint: fmt.Sprintf("%d guests", count),
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
		return plugin.ReportResult{}, fmt.Errorf("kvm: the plugin is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()

	started := time.Now()
	envID := req.EnvironmentID
	if req.SubjectKind == "kvm-host" && req.SubjectID != "" {
		envID = req.SubjectID
	}

	guests, err := r.plugin.svc.List(ctx, envID, false)
	if err != nil {
		return plugin.ReportResult{}, fmt.Errorf("kvm: read guests: %w", err)
	}
	sources := []plugin.ReportSourceStatus{{
		Key: "kvm_guests", Label: "Virtual machines", OK: true,
		DurationMS: time.Since(started).Milliseconds(),
	}}

	switch req.TypeKey {
	case reportVMBackupAssurance:
		return r.buildBackupAssurance(ctx, req, guests, sources, nil)
	case reportVMGuestStability:
		return r.buildGuestStability(ctx, req, guests, sources, nil)
	case reportVMSnapshotSchedules:
		return r.buildSnapshotSchedules(ctx, req, guests, sources, nil)
	default:
		return plugin.ReportResult{}, fmt.Errorf("kvm: unknown report type %q", req.TypeKey)
	}
}

// --- Cell helpers ---------------------------------------------------------

func stateCell(state string) plugin.ReportCell {
	switch state {
	case StateRunning:
		return plugin.ReportCell{Text: state, Status: "success", Pill: true}
	case StateCrashed:
		return plugin.ReportCell{Text: state, Status: "danger", Pill: true}
	case StateUnknown:
		return plugin.ReportCell{Text: state, Status: "warning", Pill: true}
	default:
		return plugin.ReportCell{Text: state, Pill: true}
	}
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
