// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package compliancepacks runs the mechanical half of a compliance pack: a named
// control framework's checks against a live Docker daemon, persisted so "was this
// check passing last month" has an answer.
//
// A pack is evidence for a framework, never a claim of compliance with one. A check
// that passes says the check passed; whether the control is met is the operator's
// judgement and the auditor's, and McHarbor is not entitled to make it.
package compliancepacks

import (
	"github.com/therealmcsparrow/mcharbor/modules/compliance"
)

// The status and fact vocabulary is compliance's, deliberately. A second verdict
// vocabulary would mean the posture dashboard and a pack disagree about what
// "warn" means, and the reporting plugin already renders this one.
type (
	// Status is a control's verdict, reusing the posture vocabulary.
	Status = compliance.Status
	// Kind is a fact's format hint, reusing the posture vocabulary.
	Kind = compliance.Kind
	// Fact is one measured value behind a control's verdict.
	Fact = compliance.Fact
	// Result is one control's verdict, its facts and its reason codes. Section is
	// the posture's name for the same shape; ID carries a control ID here.
	Result = compliance.Section
)

// Verdicts, re-exported so a caller in this package never reaches for a literal.
const (
	StatusPass    = compliance.StatusPass
	StatusWarn    = compliance.StatusWarn
	StatusFail    = compliance.StatusFail
	StatusInfo    = compliance.StatusInfo
	StatusUnknown = compliance.StatusUnknown
)

// Fact kinds, re-exported for the same reason.
const (
	KindBool   = compliance.KindBool
	KindNumber = compliance.KindNumber
	KindDate   = compliance.KindDate
	KindText   = compliance.KindText
	KindCode   = compliance.KindCode
)

// Run statuses. A run is `running` until the runner finishes it, so a crashed
// run is visibly unfinished rather than silently absent.
const (
	RunRunning   = "running"
	RunCompleted = "completed"
	RunFailed    = "failed"
)

// Trigger kinds. Scheduled runs are leader-gated; manual runs carry an actor.
const (
	TriggerManual    = "manual"
	TriggerScheduled = "scheduled"
)

// Run is one execution of a pack against one environment.
type Run struct {
	ID            string             `json:"id"`
	Framework     string             `json:"framework"`
	EnvironmentID string             `json:"environmentId"`
	Trigger       string             `json:"trigger"`
	StartedBy     string             `json:"startedBy"`
	StartedAt     string             `json:"startedAt"`
	FinishedAt    string             `json:"finishedAt,omitempty"`
	Status        string             `json:"status"`
	Summary       compliance.Summary `json:"summary"`
	LastError     string             `json:"lastError,omitempty"`
}

// RunDetail is a run with the control results it recorded, newest-first in the
// order the framework declares its controls.
type RunDetail struct {
	Run     Run      `json:"run"`
	Results []Result `json:"results"`
}

// rank mirrors the posture's worst-status ordering (modules/compliance
// evaluate.go raise): a control that could not be assessed outranks an
// informational one, because unknown is not a pass.
func rank(s Status) int {
	switch s {
	case StatusFail:
		return 4
	case StatusWarn:
		return 3
	case StatusUnknown:
		return 2
	case StatusInfo:
		return 1
	default:
		return 0
	}
}

// Worst returns the verdict a whole run reports: the worst of its controls, so a
// run with one unknown and nine passes never reads as passing.
func Worst(results []Result) Status {
	worst := StatusPass
	for _, r := range results {
		if rank(r.Status) > rank(worst) {
			worst = r.Status
		}
	}
	return worst
}

// builder accumulates one control's facts and reasons and keeps the verdict at the
// worst raised so far.
type builder struct {
	r Result
}

func newResult(controlID string) *builder {
	return &builder{r: Result{ID: controlID, Status: StatusPass, Facts: []Fact{}, Reasons: []string{}}}
}

func (b *builder) fact(key string, kind Kind, value string) *builder {
	b.r.Facts = append(b.r.Facts, Fact{Key: key, Kind: kind, Value: value})
	return b
}

// raise records a reason code and moves the verdict to the worse of the two.
// Reasons are codes, never prose: a code is translatable at read time and a
// formatted English sentence stored in the database is not.
func (b *builder) raise(st Status, reason string) *builder {
	b.r.Reasons = append(b.r.Reasons, reason)
	if rank(st) > rank(b.r.Status) {
		b.r.Status = st
	}
	return b
}

// unassessed is what a check returns when its evidence could not be gathered. It
// is unknown with a reason, never pass — the one rule that decides whether the
// pack is worth trusting.
func (b *builder) unassessed(reason string) Result {
	b.r.Status = StatusUnknown
	b.r.Reasons = append(b.r.Reasons, reason)
	return b.r
}

func (b *builder) done() Result { return b.r }
