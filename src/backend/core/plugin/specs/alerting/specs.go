// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package alerting defines the alerting-family adapter spec types
// (sub-registry, Spec struct, condition declarations, Client
// interface, and the request / observation shapes exchanged with the
// host).
//
// Why split: see the package doc in `core/plugin/contract`. Each
// family lives in its own sub-package so the host's reach set of
// `specs/alerting` matches the plugin's reach set exactly.
//
// # Scope: an alert *source*, not an alert *channel*
//
// An alerting adapter answers one question — "is this condition true
// right now, and for which subjects?" — and nothing else. It does not
// decide severity, does not format a message, does not know which
// communication channel the operator picked, and never delivers
// anything. The host owns the rule (threshold, target, environments,
// destinations), the state machine (sustained-for, cooldown,
// recovery), and delivery. A source contributes facts.
//
// That split is what lets an operator point a Kubernetes
// CrashLoopBackOff condition at the same Telegram channel as a
// container CPU condition, with the same cooldown semantics, without
// either source knowing the other exists.
//
// # Why the host polls instead of the plugin pushing
//
// The opposite contract — a plugin calling a host "fire this alert"
// callback — was rejected. A push model puts the evaluation schedule,
// the deduplication, and the "has this already fired?" bookkeeping
// inside every plugin, so two sources would drift on the semantics
// operators care most about (how often does this re-notify?). The
// host already runs a leader-elected ticker for exactly this; a
// source that is asked returns facts and stays stateless.
//
// # Built-ins use this contract too
//
// The host's own container / host / platform conditions register
// through this same registry, in-process. There is one evaluation
// path, so a plugin-contributed condition is never a second-class
// citizen and the engine has no per-source special cases.
//
// Imported by:
//   - the host module that evaluates rules (alerts)
//   - alerting plugins (kubernetes, and any plugin that owns a
//     resource worth alerting on)
package alerting

import (
	"context"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// AlertSourceAdapterRegistry is the sub-registry for alert condition
// providers. A plugin registers one source per family of conditions
// it owns; that source declares every condition it can evaluate.
//
// Registering the same SourceType twice (from the same plugin or from
// two different plugins) is an error — the host uses SourceType as
// the lookup key when it resolves a stored rule back to the code that
// evaluates it, and a silent overwrite would point an operator's rule
// at a different implementation than the one they configured.
type AlertSourceAdapterRegistry interface {
	// Register adds a source. The host evaluates rules whose
	// source_type == spec.SourceType by calling client.Evaluate once
	// per in-scope environment per poll interval. The client must be
	// safe for concurrent use — the host evaluates environments in
	// parallel.
	Register(spec AlertSourceAdapterSpec, client AlertSourceAdapterClient) error
}

// AlertSourceAdapterSpec describes one family of alert conditions a
// plugin can evaluate. The host uses the spec to render the rule
// builder and to route evaluation at runtime.
type AlertSourceAdapterSpec struct {
	// SourceType is the unique key the host stores on a rule and
	// uses to dispatch evaluation. Lowercase, kebab-friendly
	// identifiers ("kubernetes", "proxmox").
	SourceType string

	// DisplayName is the user-facing group label in the condition
	// picker ("Kubernetes").
	DisplayName string

	// Slug is the owning plugin's slug. Empty for the host's own
	// built-in sources, which is how the frontend tells whether to
	// resolve an icon through /api/plugins/loader/icon. Plugins
	// populate this in RegisterAdapters from their own manifest.Slug.
	Slug string

	// Icon is the asset filename inside the plugin's icons/
	// directory. Empty is valid — renderers fall back to a default.
	Icon string

	// Description is the operator-facing subtitle under the group
	// label in the picker. Empty is valid.
	Description string

	// Conditions is everything this source can alert on. A source
	// that registers zero conditions is accepted but useless, and
	// the host logs it.
	Conditions []AlertConditionDecl
}

// AlertConditionKind tells the host how to interpret an observation
// and which controls to render in the rule builder. It is the one
// piece of vocabulary shared between a source and the host's state
// machine, so the set is deliberately small.
type AlertConditionKind string

const (
	// KindThreshold compares AlertObservation.Value against the
	// rule's operator + threshold. Builder renders operator,
	// threshold, and unit.
	KindThreshold AlertConditionKind = "threshold"

	// KindDuration fires when AlertObservation.Active has held for
	// the rule's threshold duration. The source reports Active and
	// (where it can) Since; the host does the timing.
	KindDuration AlertConditionKind = "duration"

	// KindEvent fires on AlertObservation.Active becoming true, with
	// no threshold of its own.
	KindEvent AlertConditionKind = "event"

	// KindCountInWindow compares AlertObservation.Value — a count the
	// source measured over the rule's window — against the operator
	// + threshold. Builder renders a window duration alongside.
	KindCountInWindow AlertConditionKind = "count_in_window"
)

// AlertTargetKind tells the host (and the builder) what a rule's
// target pattern is matched against, so the UI can label the field
// honestly instead of always saying "container".
type AlertTargetKind string

const (
	// TargetContainer matches observation subjects that are
	// containers — by name or id, with wildcards.
	TargetContainer AlertTargetKind = "container"

	// TargetEnvironment matches whole environments.
	TargetEnvironment AlertTargetKind = "environment"

	// TargetNone means the condition is host-wide and has no target
	// field. The host evaluates it once per tick rather than once
	// per environment.
	TargetNone AlertTargetKind = "none"
)

// AlertConditionDecl declares one thing a source can alert on. The
// host renders a form from it, validates a stored rule against it,
// and schedules evaluation with it.
type AlertConditionDecl struct {
	// Key is unique within the source. The host stores
	// (SourceType, Key) on the rule, so renaming a key orphans
	// existing rules — treat it as a stable identifier.
	Key string

	// DisplayName and Description are operator-facing. Plugins
	// localise these themselves; the host does not hold i18n keys
	// for strings it does not own.
	DisplayName string
	Description string

	// Kind drives both the builder's controls and the host's state
	// machine. Required.
	Kind AlertConditionKind

	// Unit is appended to threshold inputs and formatted values
	// ("%", "MiB", "count"). Empty for unitless conditions.
	Unit string

	// Operators is the subset of > >= < <= == that makes sense for
	// this condition. Empty means the host's default set for the
	// kind. A rule naming an operator outside this list is rejected
	// at save time rather than silently coerced — the old free-text
	// condition field's habit of falling back to a default is
	// exactly what this replaces.
	Operators []string

	// DefaultOperator and DefaultThreshold prefill the builder.
	DefaultOperator  string
	DefaultThreshold float64

	// SupportsFor declares that a "sustained for" duration applies.
	// Ignored for KindDuration, where the duration *is* the
	// threshold.
	SupportsFor bool

	// TargetKind tells the host what the rule's target matches.
	TargetKind AlertTargetKind

	// PollInterval is how often this condition is worth evaluating.
	// Zero means the engine default. Set it high for anything
	// expensive: a registry round-trip or a full inspect sweep does
	// not belong on a 30-second tick.
	PollInterval time.Duration

	// Fields are extra per-rule parameters beyond operator /
	// threshold / duration — a namespace, a plan id, a severity
	// filter. The host renders them in the builder and hands the
	// values back in EvaluateRequest.Params.
	Fields []types.ConfigFieldDecl
}

// EvaluateRequest is one question the host asks a source: is this
// condition true, in this environment, right now?
//
// The host does not pass the rule's operator or threshold. A source
// reports measurements and states; comparing them against what the
// operator asked for is the host's job, so two sources cannot
// disagree about what ">= 80" means. The exception is
// KindCountInWindow, where the window is part of *measuring* rather
// than comparing, and so is passed in WindowSeconds.
type EvaluateRequest struct {
	// ConditionKey names which of the source's declared conditions
	// to evaluate.
	ConditionKey string

	// EnvironmentID and EnvironmentName identify the environment to
	// evaluate against. Both are empty for TargetNone conditions,
	// which are host-wide.
	EnvironmentID   string
	EnvironmentName string

	// WindowSeconds is the measurement window for KindCountInWindow.
	// Zero for every other kind.
	WindowSeconds int

	// Unit is the unit the operator chose, for the rare condition
	// that can honestly be measured more than one way — a disk in
	// percent or in gigabytes. It is passed because it changes the
	// measurement, not the comparison; a source that measures only
	// one way ignores it. Empty means the declaration's own Unit.
	Unit string

	// Params carries the values for the condition's declared Fields,
	// keyed by ConfigFieldDecl.Key.
	Params map[string]string
}

// AlertObservation is one subject's state for one condition.
//
// A source returns one observation per subject it looked at — not
// only the ones that look bad. The host needs the healthy ones too:
// a subject that stops appearing is treated as recovered, so
// reporting only failures would leave every resolved condition stuck
// firing until its subject disappeared entirely.
type AlertObservation struct {
	// SubjectID is the stable identity the host keys firing state
	// on: a container id, an environment id, a backup plan id.
	// Required. It must not change between ticks for the same
	// subject, or the host will read a rename as a recovery
	// followed by a fresh incident.
	SubjectID string

	// SubjectName is what a human should see in the message.
	SubjectName string

	// Value is the measurement for KindThreshold and
	// KindCountInWindow. Ignored for the other kinds.
	Value float64

	// ValueLabel is the preformatted value for messages ("87.3%",
	// "12 restarts"). Empty lets the host format Value with the
	// declaration's Unit.
	ValueLabel string

	// Active is the state for KindDuration and KindEvent: true when
	// the condition holds for this subject. Ignored for the
	// numeric kinds, where the host derives it from the comparison.
	Active bool

	// Since is when the state began, where the source can know it
	// (a container's FinishedAt, a run's started_at). Zero when
	// unknown — the host then starts timing from the first tick it
	// saw the state, which is later but never wrong in the
	// direction that fires early.
	Since time.Time

	// Context carries extra variables for message templates, exposed
	// to operators as {{ctx.<key>}}. Keep keys stable and
	// lowercase; they are user-facing.
	Context map[string]string
}

// AlertSourceAdapterClient is the runtime contract for alert condition
// providers.
//
// Implementations MUST be safe for concurrent use: the host evaluates
// environments in parallel, and a single tick can call Evaluate
// several times for different conditions of the same source.
//
// Evaluate should be cheap and side-effect free. It runs on a timer
// the operator did not choose and cannot see; anything expensive
// belongs behind a PollInterval on the declaration, or behind the
// source's own cache.
type AlertSourceAdapterClient interface {
	// Evaluate returns one observation per subject in scope for the
	// requested condition.
	//
	// Returning an error skips this (condition, environment) pair for
	// this tick and is logged — it does not clear firing state, and
	// it does not disable the rule. A source that cannot reach its
	// backend should return the error rather than an empty slice:
	// an empty slice means "I looked and there is nothing", which
	// the host reads as every subject having recovered.
	Evaluate(ctx context.Context, req EvaluateRequest) ([]AlertObservation, error)
}
