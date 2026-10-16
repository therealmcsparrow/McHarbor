// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"regexp"
	"slices"
	"strings"
)

// reAttachKind is the shape of a stored attach kind, the same token alphabet the manifest
// validator holds a declared kind to, so a well-formed kind nobody declares reads as an orphan.
var reAttachKind = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// attachKind is one attach kind a loaded provider declares. Selector is the target kind
// whose list the editor offers as the subject, and Subjects the target kinds a run for that
// subject may execute steps against.
type attachKind struct {
	Kind     string
	LabelKey string
	LabelNS  string
	Plugin   string
	Selector string
	Subjects []string
}

// hostAttachKinds are the attach kinds the host owns and the only ones stored without a provider.
var hostAttachKinds = []string{AttachAlertCondition, AttachContainer, AttachStack, AttachEnvironment}

// attachKinds lists every attach kind the loaded providers declare. Two plugins claiming a
// kind resolve to the first in sort order, as the registry does for a domain.
func (s *Service) attachKinds() []attachKind {
	var out []attachKind
	seen := map[string]bool{}
	for _, c := range s.providers() {
		for _, t := range c.Targets {
			a := t.Attach
			if a == nil || seen[a.Kind] || slices.Contains(hostAttachKinds, a.Kind) {
				continue
			}
			seen[a.Kind] = true
			subjects := a.SubjectTargets
			if len(subjects) == 0 {
				subjects = []string{t.Kind}
			}
			out = append(out, attachKind{
				Kind: a.Kind, LabelKey: a.LabelKey, LabelNS: c.I18nNS, Plugin: c.Plugin,
				Selector: t.Kind, Subjects: slices.Clone(subjects),
			})
		}
	}
	return out
}

func (s *Service) declaredAttach(kind string) (attachKind, bool) {
	for _, a := range s.attachKinds() {
		if a.Kind == kind {
			return a, true
		}
	}
	return attachKind{}, false
}

// isSubjectAttach reports whether kind attaches a runbook to one object a provider lists,
// as opposed to an alert condition, a stack, a container or an environment.
func (s *Service) isSubjectAttach(kind string) bool {
	_, ok := s.declaredAttach(kind)
	return ok
}

// attachKindStatus classifies a stored or submitted kind: host, declared by a loaded
// provider, an orphan whose provider is gone (a well-formed kind nobody declares), or invalid.
func (s *Service) attachKindStatus(kind string) (host, declared, orphan bool) {
	switch {
	case slices.Contains(hostAttachKinds, kind):
		return true, false, false
	case s.isSubjectAttach(kind):
		return false, true, false
	case reAttachKind.MatchString(strings.TrimSpace(kind)) && kind == strings.TrimSpace(kind):
		return false, false, true
	}
	return false, false, false
}

// AttachKindViews reports the attach kinds a loaded provider declares, so the editor offers
// exactly the ones it can fill and titles each in the plugin's own namespace.
func (s *Service) AttachKindViews() []AttachKindView {
	out := []AttachKindView{}
	for _, a := range s.attachKinds() {
		out = append(out, AttachKindView{Kind: a.Kind, LabelKey: a.LabelKey, LabelNS: a.LabelNS, Selector: a.Selector})
	}
	slices.SortFunc(out, func(a, b AttachKindView) int { return strings.Compare(a.Kind, b.Kind) })
	return out
}
