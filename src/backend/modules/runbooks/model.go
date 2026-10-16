// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package runbooks is the embedded runbook engine: an ordered set of steps an
// operator follows against an alert, a container, a stack or an environment,
// where every action step is authorised against that operator, per step, and
// the run records where they stopped.
//
// It is deliberately not a workflow node type. A workflow step has no
// principal — FlowContext.UserID is empty for every non-manual trigger and the
// Docker nodes never read it — so an engine that checks permissions once, at
// the route, cannot express the one thing a runbook is for.
package runbooks

import (
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

// The host attachment kinds. A runbook attached to an alert condition survives the
// rule being deleted and recreated, which is why it is the durable one. A plugin declares
// its own through the runbook-provider capability, so none is named here.
const (
	AttachAlertCondition = "alert_condition"
	AttachContainer      = "container"
	AttachStack          = "stack"
	AttachEnvironment    = "environment"
)

// The host's own target kinds. Every other kind is declared by a plugin's runbook-provider
// and is only as present as its plugin: the editor offers it while the plugin is loaded,
// and a step naming it fails with the plugin's name once it is gone.
const (
	TargetContainer   = "container"
	TargetEnvironment = plugincaps.TargetEnvironment
)

// The parameter types a catalog entry declares, which the editor renders as a
// selector, a number field or a toggle instead of free text.
const (
	ParamTarget = plugincaps.ParamTarget
	ParamEnum   = plugincaps.ParamEnum
	ParamInt    = plugincaps.ParamInt
	ParamString = plugincaps.ParamString
	ParamBool   = plugincaps.ParamBool
)

// The step kinds: a human instruction with a checkbox, a read-only diagnostic
// whose output is shown inline, or an action executed as the operator.
const (
	StepInstruction = "instruction"
	StepDiagnostic  = "diagnostic"
	StepAction      = "action"
)

// The states a step result can be recorded in. There is no state for "not yet
// reached" on a row — an unwritten row is that.
const (
	StateDone    = "done"
	StateSkipped = "skipped"
	StateFailed  = "failed"
)

// The outcomes a run can end in. Abandoned and escalated are first-class:
// an abandoned run with a reason is more informative than a completed one.
const (
	OutcomeInProgress = "in_progress"
	OutcomeCompleted  = "completed"
	OutcomeAbandoned  = "abandoned"
	OutcomeEscalated  = "escalated"
)

// MaxOutputBytes bounds one step's recorded diagnostic output, for the reason
// evidence.go bounds its own: an uncapped output column becomes the largest
// table in the database.
const MaxOutputBytes = 16 << 10

// MaxLogLines bounds a log diagnostic, matching the incident assembler.
const MaxLogLines = 150

// MaxFindingBytes bounds the operator's own free text on a step.
const MaxFindingBytes = 4 << 10

// Runbook is an ordered set of steps and what it is attached to.
type Runbook struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	AttachKind         string `json:"attachKind"`
	AttachSourceType   string `json:"attachSourceType,omitempty"`
	AttachConditionKey string `json:"attachConditionKey,omitempty"`
	AttachAlertID      string `json:"attachAlertId,omitempty"`
	AttachSubject      string `json:"attachSubject,omitempty"`
	EnvironmentID      string `json:"environmentId,omitempty"`
	Enabled            bool   `json:"enabled"`
	CreatedBy          string `json:"createdBy,omitempty"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
	Steps              []Step `json:"steps"`

	// Automation is how this runbook runs unattended, as a service principal.
	// Its zero value is the default: a runbook only a person can follow.
	Automation Automation `json:"automation"`
}

// Step is one instruction, diagnostic or action. Position is an explicit
// integer rather than array order because a runbook is reordered and a finished
// run must still say which step was which.
type Step struct {
	ID            string            `json:"id"`
	RunbookID     string            `json:"runbookId"`
	Position      int               `json:"position"`
	Kind          string            `json:"kind"`
	Title         string            `json:"title"`
	Body          string            `json:"body,omitempty"`
	DiagnosticKey string            `json:"diagnosticKey,omitempty"`
	ActionKey     string            `json:"actionKey,omitempty"`
	ActionParams  map[string]string `json:"actionParams,omitempty"`
	Required      bool              `json:"required"`

	// RequiredPermissions is what this step will be checked against at the
	// moment it is pressed, resolved from the action or diagnostic catalog.
	// It is sent to the client so a denied control can name the key rather
	// than waiting for a 403.
	RequiredPermissions []string `json:"requiredPermissions,omitempty"`
}

// Run is one operator following one runbook once: who, when, against what, and
// how it ended. A run is never rewritten — a skip with a reason is data.
type Run struct {
	ID            string       `json:"id"`
	RunbookID     string       `json:"runbookId"`
	RunbookName   string       `json:"runbookName"`
	EnvironmentID string       `json:"environmentId,omitempty"`
	SubjectKind   string       `json:"subjectKind,omitempty"`
	SubjectID     string       `json:"subjectId,omitempty"`
	SubjectName   string       `json:"subjectName,omitempty"`
	IncidentID    string       `json:"incidentId,omitempty"`
	AlertEventID  string       `json:"alertEventId,omitempty"`
	StartedBy     string       `json:"startedBy"`
	StartedByName string       `json:"startedByName"`
	StartedAt     string       `json:"startedAt"`
	FinishedAt    string       `json:"finishedAt,omitempty"`
	Outcome       string       `json:"outcome"`
	Note          string       `json:"note,omitempty"`
	Results       []StepResult `json:"results"`

	// Trigger says what started the run: a person, an alert or a schedule.
	// For the last two StartedBy is the service principal, never a label.
	Trigger string `json:"trigger"`
}

// StepResult is what the operator did with one step. The step's position and
// title are copied in so the record survives the runbook being reordered or
// rewritten afterwards.
type StepResult struct {
	ID           string `json:"id"`
	RunID        string `json:"runId"`
	StepID       string `json:"stepId"`
	StepPosition int    `json:"stepPosition"`
	StepTitle    string `json:"stepTitle"`
	StepKind     string `json:"stepKind"`
	State        string `json:"state"`
	SkipReason   string `json:"skipReason,omitempty"`
	Output       string `json:"output,omitempty"`
	Finding      string `json:"finding,omitempty"`
	ActedBy      string `json:"actedBy"`
	ActedByName  string `json:"actedByName"`
	At           string `json:"at"`
	LastError    string `json:"lastError,omitempty"`
}

// RunbookInput is an authoring create or replace. Steps are sent whole; the
// service renumbers their positions from the order given.
type RunbookInput struct {
	Name               string      `json:"name"`
	Description        string      `json:"description"`
	AttachKind         string      `json:"attachKind"`
	AttachSourceType   string      `json:"attachSourceType"`
	AttachConditionKey string      `json:"attachConditionKey"`
	AttachAlertID      string      `json:"attachAlertId"`
	AttachSubject      string      `json:"attachSubject"`
	EnvironmentID      string      `json:"environmentId"`
	Enabled            bool        `json:"enabled"`
	Steps              []StepInput `json:"steps"`
}

// StepInput is one step as the editor sends it. Position is not accepted from
// the client; the service assigns it from the array order.
type StepInput struct {
	Kind          string            `json:"kind"`
	Title         string            `json:"title"`
	Body          string            `json:"body"`
	DiagnosticKey string            `json:"diagnosticKey"`
	ActionKey     string            `json:"actionKey"`
	ActionParams  map[string]string `json:"actionParams"`
	Required      bool              `json:"required"`
}

// StartInput begins a run. StartedBy is never read from it — the handler fills
// the operator from the request, so a client cannot impersonate.
type StartInput struct {
	RunbookID     string `json:"runbookId"`
	EnvironmentID string `json:"environmentId"`
	SubjectKind   string `json:"subjectKind"`
	SubjectID     string `json:"subjectId"`
	SubjectName   string `json:"subjectName"`
	IncidentID    string `json:"incidentId"`
	AlertEventID  string `json:"alertEventId"`
}

// StepInputResult records one step. For a diagnostic or an action the service
// runs it and overwrites Output and LastError with what actually happened.
type StepInputResult struct {
	State      string `json:"state"`
	SkipReason string `json:"skipReason"`
	Finding    string `json:"finding"`
}

// FinishInput closes a run with an outcome and the operator's note.
type FinishInput struct {
	Outcome string `json:"outcome"`
	Note    string `json:"note"`
}

// ResolvedRunbooks is what the alert and the on-call inbox read: the runbooks
// that apply here, with their steps, plus any run still in progress so it can
// be offered back instead of started again.
type ResolvedRunbooks struct {
	Runbooks []Runbook `json:"runbooks"`
	OpenRuns []Run     `json:"openRuns"`
}

// CatalogView is the authoring picker's menu: every action and diagnostic a
// step may name, each with the permission it will be checked against.
type CatalogView struct {
	Actions     []CatalogEntryView `json:"actions"`
	Diagnostics []CatalogEntryView `json:"diagnostics"`

	// Targets names the plugin object kinds GET /runbooks/targets can list right
	// now, each with the label the owning plugin gave it, so the editor renders a
	// kind it has no code for.
	Targets []TargetKindView `json:"targets"`

	// AttachKinds names the attach kinds the loaded providers declare, each titled in the
	// plugin's own namespace and listing its subjects through the Selector target kind.
	AttachKinds []AttachKindView `json:"attachKinds"`
}

// AttachKindView is one attach kind a loaded plugin declares: LabelKey in the plugin's i18n
// namespace LabelNS, and Selector the target kind the editor lists to pick the subject.
type AttachKindView struct {
	Kind     string `json:"kind"`
	LabelKey string `json:"labelKey"`
	LabelNS  string `json:"labelNs"`
	Selector string `json:"selector"`
}

// TargetKindView is one selector kind a loaded plugin lists, and where its title is:
// LabelKey in the plugin's own i18n namespace LabelNS. EmptyLabelKey, in the same namespace,
// names what leaving the selector unpinned means when that is not the run's subject.
type TargetKindView struct {
	Kind          string `json:"kind"`
	LabelKey      string `json:"labelKey"`
	LabelNS       string `json:"labelNs"`
	EmptyLabelKey string `json:"emptyLabelKey,omitempty"`
}

// CatalogEntryView is one action or diagnostic as the picker shows it.
type CatalogEntryView struct {
	Key                 string      `json:"key"`
	Description         string      `json:"description"`
	DescriptionKey      string      `json:"descriptionKey,omitempty"`
	DescriptionNS       string      `json:"descriptionNs,omitempty"`
	Params              []string    `json:"params,omitempty"`
	ParamSpecs          []ParamSpec `json:"paramSpecs"`
	RequiredPermissions []string    `json:"requiredPermissions"`
	Destructive         bool        `json:"destructive"`
	Streams             bool        `json:"streams"`
	Plugin              string      `json:"plugin,omitempty"`
	Target              string      `json:"target"`
}

// ParamSpec declares one parameter a step may pin, typed so the editor can offer
// a selector and the server can refuse a value outside the declared set.
type ParamSpec struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Target   string   `json:"target,omitempty"`
	Options  []string `json:"options,omitempty"`
	Min      *int     `json:"min,omitempty"`
	Max      *int     `json:"max,omitempty"`
	Required bool     `json:"required"`
}

// TargetOption is one object a selector offers, read from the plugin that owns
// it as the operator, so the list shows only what their own view key reaches.
type TargetOption struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

// The triggers a run can be started by. Manual is a person in their own
// request; alert and schedule are the automation engine acting as a service
// principal.
const (
	TriggerManual   = "manual"
	TriggerAlert    = "alert"
	TriggerSchedule = "schedule"
)

// Automation is a runbook's unattended configuration. PrincipalID names a
// service principal; every step is authorised against that principal's roles,
// exactly as a person's run is authorised against theirs.
type Automation struct {
	PrincipalID   string `json:"principalId,omitempty"`
	PrincipalName string `json:"principalName,omitempty"`
	OnAlert       bool   `json:"onAlert"`
	Cron          string `json:"cron,omitempty"`
	Since         string `json:"since,omitempty"`
	NextRunAt     string `json:"nextRunAt,omitempty"`
	LastRunAt     string `json:"lastRunAt,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	UpdatedBy     string `json:"updatedBy,omitempty"`
}

// Active reports whether the engine should consider this runbook at all: a
// principal and at least one trigger.
func (a Automation) Active() bool {
	return a.PrincipalID != "" && (a.OnAlert || a.Cron != "")
}

// AutomationInput configures unattended runs. An empty PrincipalID turns them
// off; otherwise at least one of OnAlert and Cron must be set.
type AutomationInput struct {
	PrincipalID string `json:"principalId"`
	OnAlert     bool   `json:"onAlert"`
	Cron        string `json:"cron"`
}

// ServicePrincipal is a principal a runbook may be assigned to, as the picker
// and the engine read it. It carries no credential because it has none.
type ServicePrincipal struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName,omitempty"`
	Active      bool   `json:"active"`
}

// RunbookExport is the portable per-runbook export file format.
//
// It carries what a runbook *is* and deliberately not where it lived: ids,
// the environment, the author and the automation principal are all local to
// the instance that wrote the file, and an import that carried them would
// either collide or point at a principal that does not exist.
type RunbookExport struct {
	Kind       string              `json:"kind"`
	Version    int                 `json:"version"`
	ExportedAt string              `json:"exportedAt"`
	Runbook    RunbookExportRecord `json:"runbook"`
}

// RunbookExportRecord is the runbook payload inside an export file.
type RunbookExportRecord struct {
	// OriginalID is a breadcrumb for the operator reading the file, never a key
	// an import reuses.
	OriginalID         string           `json:"originalId,omitempty"`
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	AttachKind         string           `json:"attachKind"`
	AttachSourceType   string           `json:"attachSourceType,omitempty"`
	AttachConditionKey string           `json:"attachConditionKey,omitempty"`
	AttachAlertID      string           `json:"attachAlertId,omitempty"`
	AttachSubject      string           `json:"attachSubject,omitempty"`
	Enabled            bool             `json:"enabled"`
	Steps              []StepExport     `json:"steps"`
	Automation         AutomationExport `json:"automation"`
	OriginalCreatedAt  string           `json:"originalCreatedAt,omitempty"`
	OriginalUpdatedAt  string           `json:"originalUpdatedAt,omitempty"`
}

// StepExport is one step, without the ids that bind it to this instance.
type StepExport struct {
	Position      int               `json:"position"`
	Kind          string            `json:"kind"`
	Title         string            `json:"title"`
	Body          string            `json:"body,omitempty"`
	DiagnosticKey string            `json:"diagnosticKey,omitempty"`
	ActionKey     string            `json:"actionKey,omitempty"`
	ActionParams  map[string]string `json:"actionParams,omitempty"`
	Required      bool              `json:"required"`
}

// AutomationExport carries the schedule but not the principal that ran it: a
// service principal is an account on one instance and means nothing on another.
type AutomationExport struct {
	OnAlert bool   `json:"onAlert"`
	Cron    string `json:"cron,omitempty"`
}

// ExportFile builds the portable export for rb.
func ExportFile(rb *Runbook, now time.Time) *RunbookExport {
	if rb == nil {
		return nil
	}
	steps := make([]StepExport, 0, len(rb.Steps))
	for _, s := range rb.Steps {
		steps = append(steps, StepExport{
			Position:      s.Position,
			Kind:          s.Kind,
			Title:         s.Title,
			Body:          s.Body,
			DiagnosticKey: s.DiagnosticKey,
			ActionKey:     s.ActionKey,
			ActionParams:  s.ActionParams,
			Required:      s.Required,
		})
	}
	return &RunbookExport{
		Kind:       "mcharbor.runbook",
		Version:    1,
		ExportedAt: now.UTC().Format(time.RFC3339),
		Runbook: RunbookExportRecord{
			OriginalID:         rb.ID,
			Name:               rb.Name,
			Description:        rb.Description,
			AttachKind:         rb.AttachKind,
			AttachSourceType:   rb.AttachSourceType,
			AttachConditionKey: rb.AttachConditionKey,
			AttachAlertID:      rb.AttachAlertID,
			AttachSubject:      rb.AttachSubject,
			Enabled:            rb.Enabled,
			Steps:              steps,
			Automation: AutomationExport{
				OnAlert: rb.Automation.OnAlert,
				Cron:    rb.Automation.Cron,
			},
			OriginalCreatedAt: rb.CreatedAt,
			OriginalUpdatedAt: rb.UpdatedAt,
		},
	}
}
