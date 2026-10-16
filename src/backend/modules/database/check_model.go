// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import "time"

// Check families, in the order they run.
const (
	familyIntegrity = "integrity"
	familyOrphans   = "orphans"
	familySchema    = "schema"
	familyBloat     = "bloat"
)

// Finding severities. A check with no meaningful form on the active driver
// reports severitySkip with a stated reason rather than nothing at all — an
// operator must be able to tell "checked, clean" from "never checked".
const (
	severityOK   = "ok"
	severityInfo = "info"
	severityWarn = "warn"
	severityFail = "fail"
	severitySkip = "skip"
)

// Remedy identifiers. Empty on a finding means nothing can be done about it.
const (
	remedyVacuum          = "vacuum"
	remedyApplyMigrations = "apply_migrations"
	remedyCreateIndex     = "create_index"
	remedyDeleteOrphans   = "delete_orphans"
)

// Check phases, in the order they run.
const (
	checkPhaseQueued    = "queued"
	checkPhaseInventory = "inventory"
	checkPhaseIntegrity = "integrity"
	checkPhaseOrphans   = "orphans"
	checkPhaseSchema    = "schema"
	checkPhaseBloat     = "bloat"
	checkPhaseDone      = "done"
	checkPhaseFailed    = "failed"
	checkPhaseCancelled = "cancelled"
)

// Finding is one result line.
//
// It is data for the UI to translate, not prose: ID is a stable key and every
// human-facing string is rendered client-side from it.
type Finding struct {
	// ID is stable within a run and is what a repair request names.
	ID string `json:"id"`
	// Key is the i18n key suffix the UI renders, e.g. "orphan_rows".
	Key    string `json:"key"`
	Family string `json:"family"`
	// Severity is ok, info, warn, fail or skip.
	Severity string `json:"severity"`
	Table    string `json:"table,omitempty"`
	Column   string `json:"column,omitempty"`
	// RefTable and RefColumn name the parent of an orphan finding.
	RefTable  string `json:"refTable,omitempty"`
	RefColumn string `json:"refColumn,omitempty"`
	// Count is rows affected; Bytes is space involved.
	Count int64 `json:"count,omitempty"`
	Bytes int64 `json:"bytes,omitempty"`
	// Reason is the machine-readable cause of a skip, e.g.
	// "driver_no_native_integrity".
	Reason string `json:"reason,omitempty"`
	// Detail is the driver's own message, already redacted.
	Detail string `json:"detail,omitempty"`
	// Remedy is empty when the finding cannot be acted on.
	Remedy string `json:"remedy,omitempty"`
	// Destructive marks a remedy that deletes rows, so the UI can demand a
	// second confirmation for it.
	Destructive bool `json:"destructive,omitempty"`
	// Index names the index a create_index remedy would create.
	Index string `json:"index,omitempty"`
}

// CheckStatus is the whole run as the progress dialog reads it.
type CheckStatus struct {
	ID      string `json:"id"`
	Running bool   `json:"running"`
	// Phase is one of the check phase constants.
	Phase string `json:"phase"`
	// Driver is the backend the run executed against.
	Driver string `json:"driver"`
	// CurrentTable is the table being examined right now.
	CurrentTable string `json:"currentTable,omitempty"`
	// TableIndex is 1-based within TableTotal, for the progress bar.
	TableIndex int `json:"tableIndex"`
	TableTotal int `json:"tableTotal"`
	// Findings accumulates as the run proceeds.
	Findings []Finding `json:"findings"`
	// Counts is a severity histogram, so the card can summarise without
	// walking every finding.
	Counts map[string]int `json:"counts"`
	// Error is set on a failed run: an i18n code plus a redacted detail.
	Error       string     `json:"error,omitempty"`
	ErrorDetail string     `json:"errorDetail,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

// RepairRequest names the findings to act on. An ID the current report does
// not carry is refused: a repair only ever runs against a finding the server
// produced and the operator has seen.
type RepairRequest struct {
	FindingIDs []string `json:"findingIds"`
}

// RepairOutcome is what one remedy actually did.
type RepairOutcome struct {
	FindingID string `json:"findingId"`
	Remedy    string `json:"remedy"`
	// OK is false when the remedy failed or was refused.
	OK bool `json:"ok"`
	// Rows is how many rows the remedy affected.
	Rows int64 `json:"rows"`
	// Code is an i18n code on failure.
	Code string `json:"code,omitempty"`
	// Detail is a redacted driver message on failure.
	Detail string `json:"detail,omitempty"`
	// Skipped is true when re-verification found the finding no longer
	// applied — the orphans were already gone, the index already exists.
	Skipped bool `json:"skipped,omitempty"`
}

// RepairResponse is the whole repair pass.
type RepairResponse struct {
	Outcomes []RepairOutcome `json:"outcomes"`
	// Rows is the total across every remedy that ran.
	Rows int64 `json:"rows"`
}
