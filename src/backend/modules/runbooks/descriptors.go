// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// pluginCatalog is every action and diagnostic the loaded runbook providers declare.
// The descriptors were validated when the manifest loaded, so nothing here re-checks
// a route or a permission; it only translates them into catalog entries.
func (s *Service) pluginCatalog() (actions, diagnostics []catalogEntry) {
	for _, c := range s.providers() {
		for _, st := range c.Steps {
			e := entryFromStep(c, st)
			if st.Kind == plugincaps.StepKindAction {
				actions = append(actions, e)
			} else {
				diagnostics = append(diagnostics, e)
			}
		}
	}
	return actions, diagnostics
}

// targetSources is every selector kind the loaded providers can list, each keyed by
// kind. Containers and stacks are not here: they are host objects listed through host routes.
func (s *Service) targetSources() map[string]targetSource {
	out := map[string]targetSource{}
	for _, c := range s.providers() {
		for _, t := range c.Targets {
			out[t.Kind] = sourceFromTarget(c.Domain, t)
		}
	}
	return out
}

func entryFromStep(c plugincaps.Capability, s plugincaps.RunbookStep) catalogEntry {
	required := make([]rbac.Permission, 0, len(s.Permissions))
	for _, p := range s.Permissions {
		required = append(required, rbac.Permission(p))
	}
	call := &pluginCall{Method: s.Call.Method, Path: s.Call.Path, PickID: s.Call.PickID, Text: s.Call.Text}
	if len(s.Call.Query) > 0 {
		call.Query = copyMap(s.Call.Query)
	}
	for _, b := range s.Call.Body {
		call.Body = append(call.Body, bodyField{JSON: b.JSON, Param: b.Param, Type: b.Type})
	}
	var specs []ParamSpec
	for _, p := range s.Params {
		specs = append(specs, ParamSpec{
			Name: p.Name, Type: p.Type, Target: p.Target, Options: p.Options, Min: p.Min, Max: p.Max, Required: p.Required,
		})
	}
	return catalogEntry{
		Key: s.Key, DescriptionKey: s.DescriptionKey, DescriptionNS: c.I18nNS,
		ParamSpecs: specs, Target: s.Target, Required: required, Destructive: s.Destructive,
		Domain: c.Domain, Call: call, Plugin: c.Plugin,
	}
}

func sourceFromTarget(domain string, t plugincaps.RunbookTarget) targetSource {
	src := targetSource{
		Domain: domain, Path: t.Path, Items: t.Items, ID: t.ID,
		IDJoin: t.IDJoin, Name: t.Name, Detail: t.Detail,
	}
	if len(t.Query) > 0 {
		src.Query = copyMap(t.Query)
	}
	return src
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
