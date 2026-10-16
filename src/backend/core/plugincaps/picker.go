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

// FieldTypePluginSelect is the workflow field type whose options come from a
// picker-source, named by the field's source.plugin and source.target.
const FieldTypePluginSelect = "plugin-select"

// PickerList is the list a picker-source resolves to: a runbook-provider target's
// route and item shape, plus the key the route itself enforces, so the host can gate
// the picker on exactly what the list request will be checked against.
type PickerList struct {
	Path       string            `json:"path"`
	Query      map[string]string `json:"query,omitempty"`
	Items      string            `json:"items,omitempty"`
	ID         string            `json:"id,omitempty"`
	IDJoin     []string          `json:"idJoin,omitempty"`
	Name       []string          `json:"name"`
	Detail     []string          `json:"detail,omitempty"`
	LabelKey   string            `json:"labelKey"`
	I18nNS     string            `json:"i18nNs"`
	Permission string            `json:"permission"`
}

// reNodeKey is the workflow node-key alphabet the node catalog already enforces.
var reNodeKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// findTarget returns the runbook-provider target of kind declared anywhere in the
// manifest, with the provider's i18n namespace, so declaration order does not matter.
func findTarget(doc *manifestDoc, kind string) (RunbookTarget, string, bool) {
	for _, c := range doc.Contributes.Capabilities {
		if c.Kind != KindRunbookProvider {
			continue
		}
		for _, t := range c.Targets {
			if t.Kind == kind {
				return t, c.I18nNS, true
			}
		}
	}
	return RunbookTarget{}, "", false
}

// validatePickerSource holds a picker to a target the same manifest declares, whose list
// route declares a permission the plugin owns: a picker the host cannot gate honestly
// would either fail on a 403 or list objects the operator may not see.
func validatePickerSource(doc *manifestDoc, c Capability) error {
	if err := token("target", c.Target); err != nil {
		return err
	}
	t, _, ok := findTarget(doc, c.Target)
	if !ok {
		return fmt.Errorf("target %q is not a targets[].kind of this plugin's runbook-provider", c.Target)
	}
	route, err := findRoute(doc, http.MethodGet, t.Path, false)
	if err != nil {
		return err
	}
	if route.Requires == "" {
		return fmt.Errorf("GET %s declares no requires_permission, so the picker has nothing to gate on", t.Path)
	}
	return ownPermission(doc, route.Requires)
}

// resolvePicker builds the JSON descriptor of an already validated picker-source.
func resolvePicker(doc *manifestDoc, target string) *PickerList {
	t, ns, ok := findTarget(doc, target)
	if !ok {
		return nil
	}
	route, err := findRoute(doc, http.MethodGet, t.Path, false)
	if err != nil {
		return nil
	}
	return &PickerList{Path: t.Path, Query: t.Query, Items: t.Items, ID: t.ID, IDJoin: t.IDJoin,
		Name: t.Name, Detail: t.Detail, LabelKey: t.LabelKey, I18nNS: ns, Permission: route.Requires}
}

// validateNodeAlias allows a bare action only, pointing at a node the manifest lists or at
// one of the plugin's own channel types, so an alias can never name another plugin's node,
// another plugin's channel or a composite key.
func validateNodeAlias(doc *manifestDoc, c Capability) error {
	if !reNodeKey.MatchString(c.Alias) {
		return fmt.Errorf("alias %q must match %s", c.Alias, reNodeKey)
	}
	if (c.Node == "") == (c.ChannelType == "") {
		return errors.New("exactly one of node or channel_type is required")
	}
	if c.ChannelType != "" {
		return ownChannelType(doc, c.ChannelType)
	}
	if !slices.Contains(doc.Contributes.Nodes, c.Node) {
		return fmt.Errorf("node %q is not listed under contributes.nodes", c.Node)
	}
	return nil
}

// FieldSource is the source a plugin-select field names.
type FieldSource struct {
	Plugin string
	Target string
}

// ErrUnknownSource is returned when a plugin-select field names no picker-source the
// registering plugin declares.
var ErrUnknownSource = errors.New("plugincaps: plugin-select source is not declared")

// CheckFieldSource validates a plugin-select field of slug's node against caps, the
// same plugin's parsed capabilities: a node may only list through its own plugin's
// declared pickers, never another plugin's.
func CheckFieldSource(slug string, caps []Capability, src FieldSource) error {
	if src.Plugin != "" && src.Plugin != slug {
		return fmt.Errorf("%w: source.plugin %q is not the registering plugin %q", ErrUnknownSource, src.Plugin, slug)
	}
	for _, c := range caps {
		if c.Kind == KindPickerSource && c.Target == strings.TrimSpace(src.Target) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s declares no picker-source with target %q", ErrUnknownSource, slug, src.Target)
}

// NodeAliasAction resolves a bare stored action to the `plugin:<slug>:<node>` action of the
// loaded plugin that claims it. Composite actions are never re-resolved.
func (r *Registry) NodeAliasAction(action string) (string, bool) {
	if action == "" || strings.HasPrefix(action, "plugin:") {
		return "", false
	}
	c, ok := r.Provider(KindWorkflowNodeAlias, action)
	if !ok || c.Node == "" {
		return "", false
	}
	return "plugin:" + c.Plugin + ":" + c.Node, true
}

// Of returns slug's capabilities, for code validating one plugin's own declarations.
func (r *Registry) Of(slug string) []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, len(r.bySlug[slug]))
	copy(out, r.bySlug[slug])
	return out
}
