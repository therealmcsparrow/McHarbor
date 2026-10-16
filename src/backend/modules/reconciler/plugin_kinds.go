// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/desiredstatesources"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

// pluginCallTimeout bounds one plugin call, so a plugin waiting on an unreachable
// cluster or host cannot hold the loop that every other watch shares.
const pluginCallTimeout = 20 * time.Second

// maxSpecValue bounds a pinned value; nothing a plugin compares needs more.
const maxSpecValue = 256

// ErrPluginUnavailable means the plugin that owns a kind is not loaded, so the
// state is unknown rather than drifted.
var ErrPluginUnavailable = errors.New("reconciler: the plugin for this kind is not loaded")

// KindField is one property of a kind as the form renders it.
type KindField struct {
	Key         string       `json:"key"`
	Label       string       `json:"label"`
	Description string       `json:"description,omitempty"`
	Type        string       `json:"type"`
	Options     []KindOption `json:"options,omitempty"`
	Dynamic     bool         `json:"dynamic,omitempty"`
	Unit        string       `json:"unit,omitempty"`
	Min         float64      `json:"min,omitempty"`
	Max         float64      `json:"max,omitempty"`
}

// KindOption is one choice in a selector.
type KindOption struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

// PluginKind is one kind a loaded plugin offers the reconciler.
type PluginKind struct {
	Source       string      `json:"source"`
	Kind         string      `json:"kind"`
	Label        string      `json:"label"`
	Description  string      `json:"description,omitempty"`
	SubjectLabel string      `json:"subjectLabel"`
	Enforceable  bool        `json:"enforceable"`
	Fields       []KindField `json:"fields"`
	// OwnScopes is true when the plugin's subjects live outside McHarbor
	// environments; the form then asks the plugin for its scopes.
	OwnScopes bool `json:"ownScopes"`
}

func options(in []desiredstate.Option) []KindOption {
	out := make([]KindOption, 0, len(in))
	for _, o := range in {
		out = append(out, KindOption{Value: o.Value, Label: o.Label, Detail: o.Detail})
	}
	return out
}

// PluginKinds lists every kind the loaded plugins offer, for the desired-state form.
func PluginKinds() []PluginKind {
	out := []PluginKind{}
	for _, slug := range desiredstatesources.Slugs() {
		p, ok := desiredstatesources.Get(slug)
		if !ok {
			continue
		}
		_, ownScopes := p.(desiredstate.ScopeProvider)
		for _, k := range p.DesiredKinds() {
			pk := PluginKind{Source: slug, Kind: k.Kind, Label: k.Label, Description: k.Description,
				SubjectLabel: k.SubjectLabel, Enforceable: k.Enforceable, Fields: []KindField{}, OwnScopes: ownScopes}
			for _, f := range k.Fields {
				pk.Fields = append(pk.Fields, KindField{Key: f.Key, Label: f.Label, Description: f.Description,
					Type: f.Type, Options: options(f.Options), Dynamic: f.Dynamic, Unit: f.Unit, Min: f.Min, Max: f.Max})
			}
			out = append(out, pk)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

func providerFor(source string) (desiredstate.Provider, error) {
	p, ok := desiredstatesources.Get(source)
	if !ok {
		return nil, ErrPluginUnavailable
	}
	return p, nil
}

func kindOf(p desiredstate.Provider, kind string) (desiredstate.Kind, bool) {
	for _, k := range p.DesiredKinds() {
		if k.Kind == kind {
			return k, true
		}
	}
	return desiredstate.Kind{}, false
}

// pinned drops unpinned (empty) keys, because an empty value means "not managed".
func pinned(spec map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range spec {
		if strings.TrimSpace(v) != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// ValidatePluginInput checks a plugin desired state before it is stored: the
// plugin is loaded, the kind and subject exist, and only declared fields are pinned.
func ValidatePluginInput(source, kind, subjectID string, spec map[string]string) error {
	p, err := providerFor(source)
	if err != nil {
		return err
	}
	k, ok := kindOf(p, kind)
	if !ok {
		return fmt.Errorf("reconciler: %s has no kind %q", source, kind)
	}
	if strings.TrimSpace(subjectID) == "" {
		return errors.New("reconciler: a subject is required")
	}
	declared := map[string]desiredstate.Field{}
	for _, f := range k.Fields {
		declared[f.Key] = f
	}
	set := pinned(spec)
	if len(set) == 0 {
		return errors.New("reconciler: pin at least one property")
	}
	for key, v := range set {
		f, ok := declared[key]
		if !ok {
			return fmt.Errorf("reconciler: %q is not a property of %s", key, kind)
		}
		if len(v) > maxSpecValue {
			return fmt.Errorf("reconciler: %q is too long", key)
		}
		if f.Type == desiredstate.FieldSelect && !f.Dynamic && len(f.Options) > 0 {
			found := false
			for _, o := range f.Options {
				found = found || o.Value == v
			}
			if !found {
				return fmt.Errorf("reconciler: %q is not a choice for %q", v, key)
			}
		}
	}
	return nil
}

// plugin asks the owning plugin for drift. An unreachable plugin or subject is an
// error, never drift, so a flapping cluster cannot trigger enforcement.
func (d *Detector) plugin(ctx context.Context, ds *DesiredState, report *DriftReport) error {
	p, err := providerFor(ds.Source)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	diffs, err := p.DetectDrift(cctx, ds.Kind, ds.EnvID, ds.SubjectID, pinned(ds.Spec))
	if err != nil {
		return fmt.Errorf("reconciler: %s drift check: %w", ds.Source, err)
	}
	for _, diff := range diffs {
		class := diff.Class
		switch class {
		case ClassConfiguration, ClassState, ClassExistence, ClassImage:
		default:
			class = ClassConfiguration
		}
		report.add(class, diff.Field, diff.Expected, diff.Actual)
	}
	return nil
}

// planPlugin turns a plugin's plan into reconciler steps; a plugin step that
// is manual keeps its sentence in Detail so the reviewer knows what to do.
func planPlugin(ctx context.Context, ds *DesiredState, report *DriftReport) ([]Step, error) {
	if report == nil || !report.HasDrift {
		return []Step{}, nil
	}
	p, err := providerFor(ds.Source)
	if err != nil {
		return nil, err
	}
	diffs := make([]desiredstate.Difference, 0, len(report.Diffs))
	for _, d := range report.Diffs {
		diffs = append(diffs, desiredstate.Difference{Class: d.Class, Field: d.Field, Expected: d.Expected, Actual: d.Actual})
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	planned, err := p.PlanDrift(cctx, ds.Kind, ds.EnvID, ds.SubjectID, pinned(ds.Spec), diffs)
	if err != nil {
		return nil, fmt.Errorf("reconciler: %s plan: %w", ds.Source, err)
	}
	enforceable := true
	if k, ok := kindOf(p, ds.Kind); ok {
		enforceable = k.Enforceable
	}
	steps := make([]Step, 0, len(planned))
	for _, ps := range planned {
		st := Step{Action: ActionPlugin, PluginAction: ps.Action, Target: ds.Name, Fields: ps.Fields,
			Detail: ps.Description, Disruptive: ps.Disruptive}
		if ps.Manual || !enforceable {
			st.Action = ActionManual
			if ps.Reason != "" {
				st.Detail = ps.Reason
			}
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// ApplyFor runs a plan against whatever owns the desired state: the Docker
// actuator for built-in kinds, the plugin for a plugin kind.
func ApplyFor(ctx context.Context, act Actuator, ds *DesiredState, steps []Step) ([]StepResult, error) {
	if ds.Source == "" {
		return Apply(ctx, act, ds.EnvID, steps)
	}
	p, err := providerFor(ds.Source)
	if err != nil {
		return nil, err
	}
	results := make([]StepResult, 0, len(steps))
	var failure error
	for _, s := range steps {
		r := StepResult{Action: s.Action, Target: s.Target}
		if s.PluginAction != "" {
			r.Action = s.PluginAction
		}
		switch {
		case !s.Applicable():
			r.Status = StepManual
		case failure != nil:
			r.Status = StepSkipped
		default:
			cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			err := p.ApplyStep(cctx, ds.Kind, ds.EnvID, ds.SubjectID, pinned(ds.Spec), desiredstate.Step{
				Action: s.PluginAction, Description: s.Detail, Fields: s.Fields, Disruptive: s.Disruptive,
			})
			cancel()
			if err != nil {
				r.Status, r.Error = StepFailed, err.Error()
				failure = fmt.Errorf("%s %s: %w", r.Action, s.Target, err)
			} else {
				r.Status = StepDone
			}
		}
		results = append(results, r)
	}
	return results, failure
}

// pluginSubjects, pluginOptions and pluginCurrent back the form's selectors and
// its "use current as desired", each under the same deadline as a drift check.
func pluginSubjects(ctx context.Context, source, kind, envID string) ([]KindOption, error) {
	p, err := providerFor(source)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	got, err := p.DesiredSubjects(cctx, kind, envID)
	if err != nil {
		return nil, err
	}
	return options(got), nil
}

func pluginOptions(ctx context.Context, source, kind, field, envID string) ([]KindOption, error) {
	p, err := providerFor(source)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	got, err := p.DesiredOptions(cctx, kind, field, envID)
	if err != nil {
		return nil, err
	}
	return options(got), nil
}

func pluginCurrent(ctx context.Context, source, kind, envID, subjectID string) (map[string]string, error) {
	p, err := providerFor(source)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	cur, err := p.CurrentSpec(cctx, kind, envID, subjectID)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		cur = map[string]string{}
	}
	return cur, nil
}

// pluginScopes lists a plugin's own scopes (its clusters) for the form; a plugin
// that works within McHarbor environments has none and the form uses environments.
func pluginScopes(ctx context.Context, source, kind string) ([]KindOption, error) {
	p, err := providerFor(source)
	if err != nil {
		return nil, err
	}
	sp, ok := p.(desiredstate.ScopeProvider)
	if !ok {
		return []KindOption{}, nil
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	got, err := sp.DesiredScopes(cctx, kind)
	if err != nil {
		return nil, err
	}
	return options(got), nil
}
