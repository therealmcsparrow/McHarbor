// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// The runbook-provider descriptor types below mirror the host's plugincaps.RunbookTarget,
// RunbookStep and their parts, so the shared fixtures in src/contracts/plugincaps keep the
// two validators in step.

type pluginRunbookTarget struct {
	Kind     string               `yaml:"kind"`
	LabelKey string               `yaml:"label_key"`
	EmptyKey string               `yaml:"empty_label_key"`
	Path     string               `yaml:"path"`
	Query    map[string]string    `yaml:"query"`
	Items    string               `yaml:"items"`
	ID       string               `yaml:"id"`
	IDJoin   []string             `yaml:"id_join"`
	Name     []string             `yaml:"name"`
	Detail   []string             `yaml:"detail"`
	Attach   *pluginRunbookAttach `yaml:"attach"`
}

type pluginRunbookAttach struct {
	Kind           string   `yaml:"kind"`
	LabelKey       string   `yaml:"label_key"`
	SubjectTargets []string `yaml:"subject_targets"`
}

var pluginRunbookHostAttach = []string{"alert_condition", "container", "stack", "environment"}

type pluginRunbookParam struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Target  string   `yaml:"target"`
	Options []string `yaml:"options"`
	Min     *int     `yaml:"min"`
	Max     *int     `yaml:"max"`
}

type pluginRunbookBodyField struct {
	JSON  string `yaml:"json"`
	Param string `yaml:"param"`
	Type  string `yaml:"type"`
}

type pluginRunbookCall struct {
	Method string                   `yaml:"method"`
	Path   string                   `yaml:"path"`
	Query  map[string]string        `yaml:"query"`
	Body   []pluginRunbookBodyField `yaml:"body"`
	PickID string                   `yaml:"pick_id"`
	Text   string                   `yaml:"text"`
}

type pluginRunbookStep struct {
	Key            string               `yaml:"key"`
	DescriptionKey string               `yaml:"description_key"`
	Kind           string               `yaml:"kind"`
	Target         string               `yaml:"target"`
	Destructive    bool                 `yaml:"destructive"`
	Permissions    []string             `yaml:"permissions"`
	Params         []pluginRunbookParam `yaml:"params"`
	Call           pluginRunbookCall    `yaml:"call"`
}

var (
	rePluginRunbookStepKey = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)
	rePluginRunbookI18nKey = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`)
	rePluginRunbookNS      = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	pluginRunbookParams    = []string{"target", "enum", "int", "string", "bool"}
	pluginRunbookBody      = []string{"int", "string", "bool"}
	pluginRunbookVerbs     = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
)

const pluginRunbookEnvironment = "environment"

func validateCapRunbook(m pluginManifest, c pluginCapDecl) error {
	if err := capToken("domain", c.Domain); err != nil {
		return err
	}
	if len(c.Steps) > 0 || len(c.Targets) > 0 {
		if !rePluginRunbookNS.MatchString(c.I18nNS) {
			return fmt.Errorf("i18n_ns %q is required with steps or targets and must match %s", c.I18nNS, rePluginRunbookNS)
		}
	} else if c.I18nNS != "" {
		return errors.New("i18n_ns needs steps or targets to describe")
	}
	kinds := map[string]bool{}
	for i, t := range c.Targets {
		if err := validateRunbookTarget(m, c.Domain, t); err != nil {
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
		if err := validateRunbookStep(m, c.Domain, kinds, s); err != nil {
			return fmt.Errorf("steps[%d] (%s): %w", i, s.Key, err)
		}
		if seen[s.Key] {
			return fmt.Errorf("steps[%d]: key %s declared twice", i, s.Key)
		}
		seen[s.Key] = true
	}
	return nil
}

func validateRunbookAttaches(domain string, targets []pluginRunbookTarget, kinds map[string]bool) error {
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

func validateRunbookAttach(domain string, a pluginRunbookAttach, kinds map[string]bool) error {
	if err := capToken("kind", a.Kind); err != nil {
		return err
	}
	if a.Kind != domain && !strings.HasPrefix(a.Kind, domain+"_") {
		return fmt.Errorf("kind %q must be %s or start with %s_", a.Kind, domain, domain)
	}
	if slices.Contains(pluginRunbookHostAttach, a.Kind) {
		return fmt.Errorf("kind %q belongs to the host", a.Kind)
	}
	if !rePluginRunbookI18nKey.MatchString(a.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", a.LabelKey)
	}
	for _, st := range a.SubjectTargets {
		if !kinds[st] {
			return fmt.Errorf("subject_targets %q is not a kind under targets", st)
		}
	}
	return nil
}

func validateRunbookTarget(m pluginManifest, domain string, t pluginRunbookTarget) error {
	if err := capToken("kind", t.Kind); err != nil {
		return err
	}
	if t.Kind != domain && !strings.HasPrefix(t.Kind, domain+"_") {
		return fmt.Errorf("kind %q must be %s or start with %s_", t.Kind, domain, domain)
	}
	if !rePluginRunbookI18nKey.MatchString(t.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", t.LabelKey)
	}
	if t.EmptyKey != "" && !rePluginRunbookI18nKey.MatchString(t.EmptyKey) {
		return fmt.Errorf("empty_label_key %q is not an i18n key", t.EmptyKey)
	}
	if strings.Contains(t.Path, "{") {
		return fmt.Errorf("path %q must not carry placeholders", t.Path)
	}
	if _, err := capFindRoute(m, http.MethodGet, t.Path, false); err != nil {
		return err
	}
	for k, v := range t.Query {
		if !rePluginCapField.MatchString(k) {
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
		if f != "" && !rePluginCapField.MatchString(f) {
			return fmt.Errorf("%q is not a JSON field name", f)
		}
	}
	return nil
}

func validateRunbookStep(m pluginManifest, domain string, kinds map[string]bool, s pluginRunbookStep) error {
	if !rePluginRunbookStepKey.MatchString(s.Key) {
		return fmt.Errorf("key %q must match %s", s.Key, rePluginRunbookStepKey)
	}
	if !strings.HasPrefix(s.Key, domain+".") {
		return fmt.Errorf("key %q must start with the domain prefix %q", s.Key, domain+".")
	}
	if !rePluginRunbookI18nKey.MatchString(s.DescriptionKey) {
		return fmt.Errorf("description_key %q is not an i18n key", s.DescriptionKey)
	}
	if s.Kind != "action" && s.Kind != "diagnostic" {
		return fmt.Errorf("kind %q must be action or diagnostic", s.Kind)
	}
	if s.Destructive && s.Kind != "action" {
		return errors.New("only an action can be destructive")
	}
	if s.Target != pluginRunbookEnvironment && !kinds[s.Target] {
		return fmt.Errorf("target %q is neither %s nor a kind under targets", s.Target, pluginRunbookEnvironment)
	}
	params, err := validateRunbookParams(kinds, s)
	if err != nil {
		return err
	}
	if len(s.Permissions) == 0 {
		return errors.New("permissions is required")
	}
	for _, p := range s.Permissions {
		if err := capOwnPermission(m, p); err != nil {
			return err
		}
	}
	return validateRunbookCall(m, s, params)
}

func validateRunbookParams(kinds map[string]bool, s pluginRunbookStep) (map[string]bool, error) {
	names := map[string]bool{}
	pinned := 0
	for _, p := range s.Params {
		if !rePluginCapField.MatchString(p.Name) {
			return nil, fmt.Errorf("param %q is not a parameter name", p.Name)
		}
		if names[p.Name] {
			return nil, fmt.Errorf("param %s declared twice", p.Name)
		}
		names[p.Name] = true
		if !slices.Contains(pluginRunbookParams, p.Type) {
			return nil, fmt.Errorf("param %s type %q is not one of %v", p.Name, p.Type, pluginRunbookParams)
		}
		if err := validateRunbookParamShape(kinds, p); err != nil {
			return nil, fmt.Errorf("param %s: %w", p.Name, err)
		}
		if p.Type == "target" && p.Target == s.Target {
			pinned++
		}
	}
	if s.Target != pluginRunbookEnvironment && pinned != 1 {
		return nil, fmt.Errorf("target %s needs exactly one target param of that kind, has %d", s.Target, pinned)
	}
	return names, nil
}

func validateRunbookParamShape(kinds map[string]bool, p pluginRunbookParam) error {
	if p.Type == "target" {
		if !kinds[p.Target] {
			return fmt.Errorf("target %q is not a kind under targets", p.Target)
		}
	} else if p.Target != "" {
		return errors.New("target only applies to type target")
	}
	if (p.Type == "enum") == (len(p.Options) == 0) {
		return errors.New("options are required for type enum and only for it")
	}
	if (p.Min != nil || p.Max != nil) && p.Type != "int" {
		return errors.New("min and max only apply to type int")
	}
	if (p.Min == nil) != (p.Max == nil) {
		return errors.New("min and max come together")
	}
	if p.Min != nil && *p.Min > *p.Max {
		return fmt.Errorf("min %d is above max %d", *p.Min, *p.Max)
	}
	return nil
}

func validateRunbookCall(m pluginManifest, s pluginRunbookStep, params map[string]bool) error {
	call := s.Call
	if !slices.Contains(pluginRunbookVerbs, call.Method) {
		return fmt.Errorf("call.method %q must be one of %v", call.Method, pluginRunbookVerbs)
	}
	if s.Kind == "diagnostic" && call.Method != http.MethodGet {
		return errors.New("a diagnostic is read-only, so call.method must be GET")
	}
	route, err := capFindRoute(m, call.Method, call.Path, false)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	if route.Requires != "" && !slices.Contains(s.Permissions, route.Requires) {
		return fmt.Errorf("call: the route requires %q, which permissions does not list", route.Requires)
	}
	for _, ph := range rePluginCapPlace.FindAllString(call.Path, -1) {
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
		if !rePluginCapField.MatchString(k) {
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
		if !rePluginCapField.MatchString(b.JSON) || !params[b.Param] || !slices.Contains(pluginRunbookBody, b.Type) {
			return fmt.Errorf("call.body field %q must name a JSON field, a param and a type in %v", b.JSON, pluginRunbookBody)
		}
	}
	if call.PickID != "" && call.Text != "" {
		return errors.New("call.pick_id and call.text are exclusive")
	}
	if call.PickID != "" && !params[call.PickID] {
		return fmt.Errorf("call.pick_id %q is not a param", call.PickID)
	}
	if call.Text != "" && !rePluginCapField.MatchString(call.Text) {
		return fmt.Errorf("call.text %q is not a JSON field name", call.Text)
	}
	return nil
}
