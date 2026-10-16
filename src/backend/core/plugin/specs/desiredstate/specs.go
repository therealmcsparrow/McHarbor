// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package desiredstate is the optional contract a plugin implements so the
// reconciler can watch and correct what it manages; a new package, so no built
// plugin changes ABI.
package desiredstate

import "context"

// Field types a property can have; the form renders a control per type.
const (
	FieldSelect = "select"
	FieldText   = "text"
	FieldNumber = "number"
	FieldBool   = "bool"
)

// Option is one choice for a select field or a subject picker. Detail is a short
// secondary line ("running · 4 vCPU").
type Option struct {
	Value  string
	Label  string
	Detail string
}

// Field is one property a desired state can pin. Options are static choices;
// Dynamic means the host asks DesiredOptions for them per environment. An empty
// value in a saved spec means "not managed", so every field is optional to pin.
type Field struct {
	Key         string
	Label       string
	Description string
	Type        string
	Options     []Option
	Dynamic     bool
	Unit        string
	Min, Max    float64
}

// Kind is one kind of thing a plugin can hold to a desired state. SubjectLabel
// names what is picked ("Virtual machine", "Service"); Enforceable is false when
// the plugin only ever proposes manual steps.
type Kind struct {
	Kind         string
	Label        string
	Description  string
	SubjectLabel string
	Fields       []Field
	Enforceable  bool
}

// Difference is one departure from the spec. Class is configuration, state,
// existence or image, matching the reconciler's own report.
type Difference struct {
	Class    string
	Field    string
	Expected string
	Actual   string
}

// Step is one change a plan makes. Manual steps are shown but never applied;
// Reason is a short sentence for a person. Action and Fields are plugin-defined
// and handed back to ApplyStep unchanged.
type Step struct {
	Action      string
	Description string
	Fields      []string
	Manual      bool
	Reason      string
	Disruptive  bool
}

// Provider is implemented by a plugin whose kinds the reconciler can manage. The
// host calls DetectDrift about once a minute per desired state, with a deadline,
// and ApplyStep only for steps a person approved or enforce mode allows.
type Provider interface {
	DesiredKinds() []Kind
	DesiredSubjects(ctx context.Context, kind, environmentID string) ([]Option, error)
	DesiredOptions(ctx context.Context, kind, field, environmentID string) ([]Option, error)
	CurrentSpec(ctx context.Context, kind, environmentID, subjectID string) (map[string]string, error)
	DetectDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string) ([]Difference, error)
	PlanDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, diffs []Difference) ([]Step, error)
	ApplyStep(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, step Step) error
}

// ScopeProvider is implemented by a plugin whose subjects live outside McHarbor
// environments (a Kubernetes cluster); the form then lists these scopes instead of
// environments, and each Option.Value is passed as environmentID everywhere else.
type ScopeProvider interface {
	DesiredScopes(ctx context.Context, kind string) ([]Option, error)
}
