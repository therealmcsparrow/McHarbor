// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
	backupspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/backup"
)

// backupRunSummary is the spec's run shape, aliased so this file reads
// without the package qualifier on every line.
type backupRunSummary = backupspec.RunSummary

// backupRunQueryFor builds the query sent to every provider.
func backupRunQueryFor(envID string) backupspec.RunQuery {
	return backupspec.RunQuery{EnvironmentID: envID, Limit: providerRunLimit}
}

// providerRunLimit bounds what one provider may contribute to a page.
//
// The page is a monitoring view, not an archive: a provider with ten thousand
// runs must not be able to push every other source off the list, and nobody
// scrolls past the first few hundred rows looking for last night's failure.
const providerRunLimit = 200

// providerTimeout bounds one provider's answer.
//
// Short on purpose. The page aggregates several sources and renders as soon
// as it has them; a provider that has gone slow is named as unavailable
// rather than holding the whole list behind it.
const providerTimeout = 5 * time.Second

// SourceKindContainer is what the host's own backups report as their source.
//
// The host is a provider like any other from the page's point of view — it
// simply does not register, because it is the thing being registered into.
const SourceKindContainer = "container"

// ProviderRun is one row from a registered provider, flattened for the page.
//
// It deliberately mirrors BackupRun's JSON field names where they mean the
// same thing, so the table renders both from one shape rather than branching
// per source.
type ProviderRun struct {
	ID string `json:"id"`
	// SourceKind is what the page's source column shows: "container" for the
	// host's own, or whatever a provider declared — "vm" today.
	SourceKind  string `json:"sourceKind"`
	SourceSlug  string `json:"sourceSlug,omitempty"`
	SourceLabel string `json:"sourceLabel,omitempty"`

	SubjectID   string `json:"subjectId,omitempty"`
	SubjectName string `json:"subjectName,omitempty"`

	EnvironmentID string `json:"environmentId,omitempty"`
	Operation     string `json:"operation"`
	Status        string `json:"status"`
	Kind          string `json:"kind,omitempty"`

	SizeBytes    int64    `json:"sizeBytes"`
	Destinations []string `json:"destinations,omitempty"`

	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`

	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt,omitempty"`
	DurationMS  int64  `json:"durationMs"`
}

// BackupSourceInfo describes one source for the page's filter.
type BackupSourceInfo struct {
	Kind        string `json:"kind"`
	Slug        string `json:"slug,omitempty"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon,omitempty"`
	CanRestore  bool   `json:"canRestore"`
	CanDelete   bool   `json:"canDelete"`
	// Available is false when the provider is registered but did not answer.
	// The filter still lists it, so an operator can see that a source they
	// expect is silent rather than concluding it has no backups.
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

// backupProviders resolves the registered providers, or none.
func (s *Service) backupProviders() []host.BackupProviderEntry {
	if s.pluginStorageAdapter == nil {
		return nil
	}
	source, ok := s.pluginStorageAdapter.(host.BackupProviderSource)
	if !ok {
		return nil
	}
	return source.BackupProviders()
}

// ProviderRuns collects every registered provider's runs for a page.
//
// One provider failing never fails the page: its rows are absent and its
// entry in the returned source list says so. A monitoring view that goes
// blank because one source is unreachable is worse than one that is honest
// about which source is missing.
func (s *Service) ProviderRuns(ctx context.Context, envID string) ([]ProviderRun, []BackupSourceInfo) {
	providers := s.backupProviders()
	if len(providers) == 0 {
		return nil, nil
	}

	var runs []ProviderRun
	sources := make([]BackupSourceInfo, 0, len(providers))

	for _, provider := range providers {
		info := BackupSourceInfo{
			Kind:        provider.Spec.SubjectKind,
			Slug:        provider.Spec.Slug,
			DisplayName: provider.Spec.DisplayName,
			Icon:        provider.Spec.Icon,
			CanRestore:  provider.Spec.Capabilities.Restore,
			CanDelete:   provider.Spec.Capabilities.Delete,
			Available:   true,
		}

		callCtx, cancel := context.WithTimeout(ctx, providerTimeout)
		answered, err := provider.Client.Runs(callCtx, backupRunQueryFor(envID))
		cancel()

		if err != nil {
			info.Available = false
			// The provider's own words: it knows why it could not answer,
			// and "unavailable" alone sends the operator to a log.
			info.Error = err.Error()
			if s.logger != nil {
				s.logger.Warn("backups: a provider did not answer",
					"provider", provider.Spec.Slug, "error", err)
			}
			sources = append(sources, info)
			continue
		}

		for _, run := range answered {
			runs = append(runs, toProviderRun(provider, run))
		}
		sources = append(sources, info)
	}

	// Newest first across every source, which is the order the question
	// "what happened last night" is asked in.
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt > runs[j].StartedAt })
	return runs, sources
}

// toProviderRun flattens one provider row, tagged with where it came from.
func toProviderRun(provider host.BackupProviderEntry, run backupRunSummary) ProviderRun {
	out := ProviderRun{
		ID:            run.ID,
		SourceKind:    provider.Spec.SubjectKind,
		SourceSlug:    provider.Spec.Slug,
		SourceLabel:   provider.Spec.DisplayName,
		SubjectID:     run.SubjectID,
		SubjectName:   run.SubjectName,
		EnvironmentID: run.EnvironmentID,
		Operation:     run.Operation,
		Status:        run.Status,
		Kind:          run.Kind,
		SizeBytes:     run.SizeBytes,
		Destinations:  run.Destinations,
		Error:         run.Error,
		Warning:       run.Warning,
		DurationMS:    run.DurationMS,
	}
	if !run.StartedAt.IsZero() {
		out.StartedAt = run.StartedAt.UTC().Format(time.RFC3339)
	}
	if run.CompletedAt != nil && !run.CompletedAt.IsZero() {
		out.CompletedAt = run.CompletedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ProviderPlan is one schedule from a registered provider, flattened for the
// page. It mirrors ContainerBackupPlan's JSON field names where they mean the
// same thing, so the table renders both from one shape.
type ProviderPlan struct {
	ID          string `json:"id"`
	SourceKind  string `json:"sourceKind"`
	SourceSlug  string `json:"sourceSlug,omitempty"`
	SourceLabel string `json:"sourceLabel,omitempty"`

	SubjectID   string `json:"subjectId,omitempty"`
	SubjectName string `json:"subjectName,omitempty"`

	EnvironmentID string `json:"environmentId,omitempty"`
	Name          string `json:"name"`
	Cron          string `json:"cron,omitempty"`
	Enabled       bool   `json:"enabled"`

	RetentionCount int `json:"retentionCount"`
	RetentionDays  int `json:"retentionDays"`

	LastRunAt string `json:"lastRunAt,omitempty"`
	NextRunAt string `json:"nextRunAt,omitempty"`
}

// BackupPlansPage is what every non-container source contributes to the
// Backups page's schedules tab.
type BackupPlansPage struct {
	ProviderPlans []ProviderPlan     `json:"providerPlans"`
	Sources       []BackupSourceInfo `json:"sources"`
}

// ProviderPlans collects every registered provider's schedules.
//
// A provider with no schedules to show answers ErrUnsupported, which is not a
// failure: it contributes no rows and is still listed as available, because
// "runs on demand only" is a valid answer rather than a broken provider.
func (s *Service) ProviderPlans(ctx context.Context, envID string) ([]ProviderPlan, []BackupSourceInfo) {
	providers := s.backupProviders()
	if len(providers) == 0 {
		return nil, nil
	}

	var plans []ProviderPlan
	sources := make([]BackupSourceInfo, 0, len(providers))

	for _, provider := range providers {
		info := BackupSourceInfo{
			Kind:        provider.Spec.SubjectKind,
			Slug:        provider.Spec.Slug,
			DisplayName: provider.Spec.DisplayName,
			Icon:        provider.Spec.Icon,
			CanRestore:  provider.Spec.Capabilities.Restore,
			CanDelete:   provider.Spec.Capabilities.Delete,
			Available:   true,
		}
		if !provider.Spec.Capabilities.Plans {
			sources = append(sources, info)
			continue
		}

		callCtx, cancel := context.WithTimeout(ctx, providerTimeout)
		answered, err := provider.Client.Plans(callCtx, envID)
		cancel()

		if err != nil && !errors.Is(err, backupspec.ErrUnsupported) {
			info.Available = false
			info.Error = err.Error()
			if s.logger != nil {
				s.logger.Warn("backups: a provider did not answer for schedules",
					"provider", provider.Spec.Slug, "error", err)
			}
			sources = append(sources, info)
			continue
		}

		for _, plan := range answered {
			plans = append(plans, toProviderPlan(provider, plan))
		}
		sources = append(sources, info)
	}

	sort.Slice(plans, func(i, j int) bool { return plans[i].Name < plans[j].Name })
	return plans, sources
}

// toProviderPlan flattens one schedule, tagged with where it came from.
func toProviderPlan(provider host.BackupProviderEntry, plan backupspec.PlanSummary) ProviderPlan {
	out := ProviderPlan{
		ID:             plan.ID,
		SourceKind:     provider.Spec.SubjectKind,
		SourceSlug:     provider.Spec.Slug,
		SourceLabel:    provider.Spec.DisplayName,
		SubjectID:      plan.SubjectID,
		SubjectName:    plan.SubjectName,
		EnvironmentID:  plan.EnvironmentID,
		Name:           plan.Name,
		Cron:           plan.Cron,
		Enabled:        plan.Enabled,
		RetentionCount: plan.RetentionCount,
		RetentionDays:  plan.RetentionDays,
	}
	if plan.LastRunAt != nil && !plan.LastRunAt.IsZero() {
		out.LastRunAt = plan.LastRunAt.UTC().Format(time.RFC3339)
	}
	if plan.NextRunAt != nil && !plan.NextRunAt.IsZero() {
		out.NextRunAt = plan.NextRunAt.UTC().Format(time.RFC3339)
	}
	return out
}

// BackupRunsPage is what every non-container source contributes to the
// Backups page.
//
// Container runs are not in it. They stay on their own route in their own
// shape, because they carry per-destination detail, restore options and live
// progress that no provider contract exposes — and because half a dozen
// callers already read that route for container work alone.
type BackupRunsPage struct {
	ProviderRuns []ProviderRun      `json:"providerRuns"`
	Sources      []BackupSourceInfo `json:"sources"`
}

// hostBackupSource is the host's own entry in the source filter.
//
// Always present and always available: it is the module answering about
// itself, so there is no call that could fail.
func hostBackupSource() []BackupSourceInfo {
	return []BackupSourceInfo{{
		Kind:        SourceKindContainer,
		DisplayName: "Containers",
		Icon:        "IconBox",
		CanRestore:  true,
		CanDelete:   true,
		Available:   true,
	}}
}
