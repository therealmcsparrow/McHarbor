// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package backup is the contract between the host's Backups page and any
// plugin that backs something up.
//
// The host owns container backups itself. Everything else that produces
// backups — virtual machines today, system containers or a future appliance
// tomorrow — owns its own plans, runs, retention and restore, and appears on
// the same page by registering here.
//
// The alternative, and the reason this family exists: the host could read the
// plugin's tables directly. That inverts the split the plugin architecture is
// built on, couples core to a schema it does not own, and goes quiet rather
// than failing when that schema changes. A provider that answers questions is
// a contract; a SELECT into someone else's table is a guess.
//
// What stays with the host: the page, the filters, the aggregation, the
// retention *display*, and the source column. What stays with the provider:
// what a backup is, where it went, and whether it worked.
package backup

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported is returned by a provider asked for something it does not do.
//
// Distinct from a failure: a provider with no plans is not broken, and the
// page shows its runs regardless rather than reporting an error the operator
// cannot act on.
var ErrUnsupported = errors.New("backup: this provider does not support that")

// BackupAdapterRegistry is the per-category sub-registry a plugin receives.
type BackupAdapterRegistry interface {
	Register(spec BackupProviderSpec, client BackupProviderClient) error
}

// BackupProviderSpec describes a provider before anything has been run.
//
// It is what lets the page render a source filter, and label a row, without
// calling the provider at all — which matters because the page loads before
// any provider has answered.
type BackupProviderSpec struct {
	// Slug is the owning plugin's slug ("addon-kvm"). It is the dispatch
	// key, the i18n namespace labels resolve from, and the origin tag the
	// loader prunes on uninstall. Required.
	Slug string

	// SubjectKind is what this provider backs up, lowercase and singular:
	// "container", "vm", "ct". It is the value the page's source column
	// shows and filters on, so it is a stable identifier rather than a
	// label — the translated name comes from the plugin's own bundle.
	// Required.
	SubjectKind string

	// DisplayName is the untranslated fallback shown when the plugin's i18n
	// bundle has no entry. Required: a source column with a blank cell tells
	// the operator nothing.
	DisplayName string

	// Icon is a Tabler icon name for the source column and the filter.
	Icon string

	Capabilities BackupProviderCapabilities
}

// BackupProviderCapabilities says which actions the page may offer.
//
// Declared rather than discovered, so a control is disabled with a reason
// instead of failing when it is used — the same discipline the permission
// gates follow.
type BackupProviderCapabilities struct {
	// Restore reports whether a run can be restored through this provider.
	Restore bool
	// Delete reports whether a run and its copies can be removed.
	Delete bool
	// Plans reports whether the provider has schedules to list. A provider
	// that only ever runs on demand answers false and the page shows its
	// runs without an empty schedules section.
	Plans bool
}

// BackupProviderClient is what the page asks.
//
// Both methods are reads. A provider is never asked to *do* anything from
// here: starting, restoring and deleting stay on the plugin's own routes,
// under the plugin's own permission keys, because those are the operations
// where getting authorization wrong actually costs something.
type BackupProviderClient interface {
	// Runs returns backups and restores matching the query, newest first.
	//
	// Honour the context deadline: the page aggregates several providers and
	// one that blocks would hold the whole list. A provider that cannot
	// answer returns an error, and the page shows the others with that one
	// named as unavailable rather than failing entirely.
	Runs(ctx context.Context, query RunQuery) ([]RunSummary, error)

	// Plans returns the provider's schedules for an environment, or
	// ErrUnsupported when it has none to show.
	Plans(ctx context.Context, environmentID string) ([]PlanSummary, error)
}

// RunQuery bounds what the page asks for.
type RunQuery struct {
	// EnvironmentID narrows to one environment, or is empty for all.
	EnvironmentID string
	// Limit caps the rows returned. The host always sets it; a provider
	// that ignores it makes the page's paging a lie.
	Limit int
	// Since, when set, asks only for runs started at or after it — how the
	// page answers "what happened last night" without reading everything.
	Since time.Time
}

// RunSummary is one backup or restore, as the page shows it.
//
// Deliberately flat and display-shaped. A provider's own model is richer;
// this is the subset the shared page can render for every provider without
// knowing what any of them is.
type RunSummary struct {
	ID string
	// SubjectID and SubjectName identify what was backed up — a machine, a
	// container. SubjectName is what the row shows; SubjectID is what a
	// link back into the provider's own page is built from.
	SubjectID   string
	SubjectName string

	EnvironmentID string

	// Operation is "backup" or "restore". Both appear in one list because
	// the question the page answers is "what has been happening", and a
	// restore is part of that.
	Operation string
	// Status is "running", "success", "failure" or "cancelled".
	Status string
	// Kind is free text the provider owns — "full", "incremental", or empty
	// where the distinction does not apply. Shown as given.
	Kind string

	// SizeBytes is what moved. Zero where the provider does not know.
	SizeBytes int64
	// Destinations names where copies landed, for the page's destination
	// column. Empty where the provider does not use storage locations.
	Destinations []string

	// Error is the provider's own message for a failed run, already fit to
	// show an operator. The page renders it behind a status-error control
	// rather than truncating it into a cell.
	Error string
	// Warning is a run that succeeded with something worth saying — an
	// incremental that fell back to a full copy, a backup taken without
	// quiescing. Distinct from Error because the run did work.
	Warning string

	StartedAt   time.Time
	CompletedAt *time.Time
	DurationMS  int64
}

// PlanSummary is one schedule, as the page shows it.
type PlanSummary struct {
	ID          string
	SubjectID   string
	SubjectName string

	EnvironmentID string

	Name string
	// Cron is the five-field expression. The page renders it readably
	// through the same helper every other schedule uses.
	Cron    string
	Enabled bool

	// RetentionCount and RetentionDays are shown as the provider's own
	// policy. Zero means "no limit" for both, matching how the host's own
	// container plans read.
	RetentionCount int
	RetentionDays  int

	LastRunAt *time.Time
	NextRunAt *time.Time
}
