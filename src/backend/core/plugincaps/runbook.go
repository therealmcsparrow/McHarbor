// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// The parameter types a runbook step may declare. The set is closed: the editor has a
// control for each and the host validates a pinned value against it.
const (
	ParamTarget = "target"
	ParamEnum   = "enum"
	ParamInt    = "int"
	ParamString = "string"
	ParamBool   = "bool"
)

// The two kinds of runbook step a provider may declare.
const (
	StepKindAction     = "action"
	StepKindDiagnostic = "diagnostic"
)

// TargetEnvironment is the reserved target of a step that acts on the run's
// environment rather than on a listed object.
const TargetEnvironment = "environment"

// RunbookTarget is the plugin list route behind one selector kind, and where in each
// item the id, the label and a detail line are read from. IDJoin builds a composite id;
// LabelKey names the kind itself, in the provider's i18n_ns, so the editor can title its selector;
// EmptyLabelKey names what an unpinned selector means, where that is not the run's subject.
type RunbookTarget struct {
	Kind          string            `yaml:"kind"`
	LabelKey      string            `yaml:"label_key"`
	EmptyLabelKey string            `yaml:"empty_label_key,omitempty"`
	Path          string            `yaml:"path"`
	Query         map[string]string `yaml:"query,omitempty"`
	Items         string            `yaml:"items,omitempty"`
	ID            string            `yaml:"id,omitempty"`
	IDJoin        []string          `yaml:"id_join,omitempty"`
	Name          []string          `yaml:"name"`
	Detail        []string          `yaml:"detail,omitempty"`
	Attach        *RunbookAttach    `yaml:"attach,omitempty"`
}

// RunbookAttach lets a runbook be attached to the objects a target lists. Kind is the
// stored attach kind, LabelKey titles it in the provider's i18n_ns, and SubjectTargets names
// the target kinds whose steps a run for that subject may execute (the carrying target when empty).
type RunbookAttach struct {
	Kind           string   `yaml:"kind"`
	LabelKey       string   `yaml:"label_key"`
	SubjectTargets []string `yaml:"subject_targets,omitempty"`
}

// HostAttachKinds are the attach kinds the host owns; a plugin may not declare one of them.
var HostAttachKinds = []string{"alert_condition", "container", "stack", "environment"}

// RunbookParam is one typed parameter a step may pin.
type RunbookParam struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Target   string   `yaml:"target,omitempty"`
	Options  []string `yaml:"options,omitempty"`
	Min      *int     `yaml:"min,omitempty"`
	Max      *int     `yaml:"max,omitempty"`
	Required bool     `yaml:"required,omitempty"`
}

// RunbookBodyField is one JSON body field and the typed step parameter it comes from.
type RunbookBodyField struct {
	JSON  string `yaml:"json"`
	Param string `yaml:"param"`
	Type  string `yaml:"type"`
}

// RunbookCall is the plugin route a step invokes, with its placeholders, query and
// body named after the step's parameters.
type RunbookCall struct {
	Method string             `yaml:"method"`
	Path   string             `yaml:"path"`
	Query  map[string]string  `yaml:"query,omitempty"`
	Body   []RunbookBodyField `yaml:"body,omitempty"`
	PickID string             `yaml:"pick_id,omitempty"`
	Text   string             `yaml:"text,omitempty"`
}

// RunbookStep is one action or diagnostic a provider offers to runbooks.
type RunbookStep struct {
	Key            string         `yaml:"key"`
	DescriptionKey string         `yaml:"description_key"`
	Kind           string         `yaml:"kind"`
	Target         string         `yaml:"target"`
	Destructive    bool           `yaml:"destructive,omitempty"`
	Permissions    []string       `yaml:"permissions"`
	Params         []RunbookParam `yaml:"params,omitempty"`
	Call           RunbookCall    `yaml:"call"`
}

var (
	reStepKey  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)
	reI18nKey  = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`)
	reNS       = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	paramTypes = []string{ParamTarget, ParamEnum, ParamInt, ParamString, ParamBool}
	bodyTypes  = []string{ParamInt, ParamString, ParamBool}
	callVerbs  = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
)

func validateRunbookProvider(doc *manifestDoc, c Capability) error {
	if err := token("domain", c.Domain); err != nil {
		return err
	}
	if len(c.Steps) > 0 || len(c.Targets) > 0 {
		if !reNS.MatchString(c.I18nNS) {
			return fmt.Errorf("i18n_ns %q is required with steps or targets and must match %s", c.I18nNS, reNS)
		}
	} else if c.I18nNS != "" {
		return errors.New("i18n_ns needs steps or targets to describe")
	}
	kinds := map[string]bool{}
	for i, t := range c.Targets {
		if err := validateRunbookTarget(doc, c.Domain, t); err != nil {
			return fmt.Errorf("targets[%d] (%s): %w", i, t.Kind, err)
		}
		if kinds[t.Kind] {
			return fmt.Errorf("targets[%d]: kind %s declared twice", i, t.Kind)
		}
		kinds[t.Kind] = true
	}
	if err := validateRunbookAttaches(c.Domain, c.Targets, kinds); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, s := range c.Steps {
		if err := validateRunbookStep(doc, c.Domain, kinds, s); err != nil {
			return fmt.Errorf("steps[%d] (%s): %w", i, s.Key, err)
		}
		if seen[s.Key] {
			return fmt.Errorf("steps[%d]: key %s declared twice", i, s.Key)
		}
		seen[s.Key] = true
	}
	return nil
}

func validateRunbookAttaches(domain string, targets []RunbookTarget, kinds map[string]bool) error {
	attached := map[string]bool{}
	for i, t := range targets {
		a := t.Attach
		if a == nil {
			continue
		}
		if err := validateRunbookAttach(domain, *a, kinds); err != nil {
			return fmt.Errorf("targets[%d] (%s): attach: %w", i, t.Kind, err)
		}
		if attached[a.Kind] {
			return fmt.Errorf("targets[%d] (%s): attach kind %s declared twice", i, t.Kind, a.Kind)
		}
		attached[a.Kind] = true
	}
	return nil
}

func validateRunbookAttach(domain string, a RunbookAttach, kinds map[string]bool) error {
	if err := token("kind", a.Kind); err != nil {
		return err
	}
	if a.Kind != domain && !strings.HasPrefix(a.Kind, domain+"_") {
		return fmt.Errorf("kind %q must be %s or start with %s_", a.Kind, domain, domain)
	}
	if slices.Contains(HostAttachKinds, a.Kind) {
		return fmt.Errorf("kind %q belongs to the host", a.Kind)
	}
	if !reI18nKey.MatchString(a.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", a.LabelKey)
	}
	for _, st := range a.SubjectTargets {
		if !kinds[st] {
			return fmt.Errorf("subject_targets %q is not a kind under targets", st)
		}
	}
	return nil
}

func validateRunbookTarget(doc *manifestDoc, domain string, t RunbookTarget) error {
	if err := token("kind", t.Kind); err != nil {
		return err
	}
	if t.Kind != domain && !strings.HasPrefix(t.Kind, domain+"_") {
		return fmt.Errorf("kind %q must be %s or start with %s_", t.Kind, domain, domain)
	}
	if !reI18nKey.MatchString(t.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", t.LabelKey)
	}
	if t.EmptyLabelKey != "" && !reI18nKey.MatchString(t.EmptyLabelKey) {
		return fmt.Errorf("empty_label_key %q is not an i18n key", t.EmptyLabelKey)
	}
	if strings.Contains(t.Path, "{") {
		return fmt.Errorf("path %q must not carry placeholders", t.Path)
	}
	if _, err := findRoute(doc, http.MethodGet, t.Path, false); err != nil {
		return err
	}
	for k, v := range t.Query {
		if !reField.MatchString(k) {
			return fmt.Errorf("query %q is not a parameter name", k)
		}
		if v != "$env" && v != "cluster" && !strings.HasPrefix(v, "=") {
			return fmt.Errorf("query %s source %q must be $env, cluster or =literal", k, v)
		}
	}
	if (t.ID == "") == (len(t.IDJoin) == 0) {
		return errors.New("exactly one of id and id_join is required")
	}
	if len(t.Name) == 0 {
		return errors.New("name is required")
	}
	fields := append(append(append([]string{t.ID, t.Items}, t.IDJoin...), t.Name...), t.Detail...)
	for _, f := range fields {
		if f != "" && !reField.MatchString(f) {
			return fmt.Errorf("%q is not a JSON field name", f)
		}
	}
	return nil
}

func validateRunbookStep(doc *manifestDoc, domain string, kinds map[string]bool, s RunbookStep) error {
	if !reStepKey.MatchString(s.Key) {
		return fmt.Errorf("key %q must match %s", s.Key, reStepKey)
	}
	if !strings.HasPrefix(s.Key, domain+".") {
		return fmt.Errorf("key %q must start with the domain prefix %q", s.Key, domain+".")
	}
	if !reI18nKey.MatchString(s.DescriptionKey) {
		return fmt.Errorf("description_key %q is not an i18n key", s.DescriptionKey)
	}
	if s.Kind != StepKindAction && s.Kind != StepKindDiagnostic {
		return fmt.Errorf("kind %q must be %s or %s", s.Kind, StepKindAction, StepKindDiagnostic)
	}
	if s.Destructive && s.Kind != StepKindAction {
		return errors.New("only an action can be destructive")
	}
	if s.Target != TargetEnvironment && !kinds[s.Target] {
		return fmt.Errorf("target %q is neither %s nor a kind under targets", s.Target, TargetEnvironment)
	}
	params, err := validateRunbookParams(kinds, s)
	if err != nil {
		return err
	}
	if len(s.Permissions) == 0 {
		return errors.New("permissions is required")
	}
	for _, p := range s.Permissions {
		if err := ownPermission(doc, p); err != nil {
			return err
		}
	}
	return validateRunbookCall(doc, s, params)
}

func validateRunbookParams(kinds map[string]bool, s RunbookStep) (map[string]bool, error) {
	names := map[string]bool{}
	pinned := 0
	for _, p := range s.Params {
		if !reField.MatchString(p.Name) {
			return nil, fmt.Errorf("param %q is not a parameter name", p.Name)
		}
		if names[p.Name] {
			return nil, fmt.Errorf("param %s declared twice", p.Name)
		}
		names[p.Name] = true
		if !slices.Contains(paramTypes, p.Type) {
			return nil, fmt.Errorf("param %s type %q is not one of %v", p.Name, p.Type, paramTypes)
		}
		if err := validateParamShape(kinds, p); err != nil {
			return nil, fmt.Errorf("param %s: %w", p.Name, err)
		}
		if p.Type == ParamTarget && p.Target == s.Target {
			pinned++
		}
	}
	if s.Target != TargetEnvironment && pinned != 1 {
		return nil, fmt.Errorf("target %s needs exactly one target param of that kind, has %d", s.Target, pinned)
	}
	return names, nil
}

func validateParamShape(kinds map[string]bool, p RunbookParam) error {
	if p.Type == ParamTarget {
		if !kinds[p.Target] {
			return fmt.Errorf("target %q is not a kind under targets", p.Target)
		}
	} else if p.Target != "" {
		return fmt.Errorf("target only applies to type %s", ParamTarget)
	}
	if (p.Type == ParamEnum) == (len(p.Options) == 0) {
		return fmt.Errorf("options are required for type %s and only for it", ParamEnum)
	}
	if (p.Min != nil || p.Max != nil) && p.Type != ParamInt {
		return fmt.Errorf("min and max only apply to type %s", ParamInt)
	}
	if (p.Min == nil) != (p.Max == nil) {
		return errors.New("min and max come together")
	}
	if p.Min != nil && *p.Min > *p.Max {
		return fmt.Errorf("min %d is above max %d", *p.Min, *p.Max)
	}
	return nil
}

func validateRunbookCall(doc *manifestDoc, s RunbookStep, params map[string]bool) error {
	call := s.Call
	if !slices.Contains(callVerbs, call.Method) {
		return fmt.Errorf("call.method %q must be one of %v", call.Method, callVerbs)
	}
	if s.Kind == StepKindDiagnostic && call.Method != http.MethodGet {
		return errors.New("a diagnostic is read-only, so call.method must be GET")
	}
	route, err := findRoute(doc, call.Method, call.Path, false)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	if route.Requires != "" && !slices.Contains(s.Permissions, route.Requires) {
		return fmt.Errorf("call: the route requires %q, which permissions does not list", route.Requires)
	}
	for _, ph := range rePlace.FindAllString(call.Path, -1) {
		name := ph[1 : len(ph)-1]
		if name == "$env" {
			continue
		}
		base, part, dotted := strings.Cut(name, ".")
		if !params[base] || (dotted && part != "namespace" && part != "name") {
			return fmt.Errorf("call.path placeholder %s names no param", ph)
		}
	}
	for k, v := range call.Query {
		if !reField.MatchString(k) {
			return fmt.Errorf("call.query %q is not a parameter name", k)
		}
		if v != "$env" && !strings.HasPrefix(v, "=") && !params[v] {
			return fmt.Errorf("call.query %s source %q is not $env, =literal or a param", k, v)
		}
	}
	if len(call.Body) > 0 && call.Method == http.MethodGet {
		return errors.New("call.body needs a method that carries one")
	}
	for _, b := range call.Body {
		if !reField.MatchString(b.JSON) || !params[b.Param] || !slices.Contains(bodyTypes, b.Type) {
			return fmt.Errorf("call.body field %q must name a JSON field, a param and a type in %v", b.JSON, bodyTypes)
		}
	}
	if call.PickID != "" && call.Text != "" {
		return errors.New("call.pick_id and call.text are exclusive")
	}
	if call.PickID != "" && !params[call.PickID] {
		return fmt.Errorf("call.pick_id %q is not a param", call.PickID)
	}
	if call.Text != "" && !reField.MatchString(call.Text) {
		return fmt.Errorf("call.text %q is not a JSON field name", call.Text)
	}
	return nil
}
