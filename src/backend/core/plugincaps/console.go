// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"fmt"
	"slices"
)

// ConsoleKinds are the console streams the agent can open. They are the agent protocol's
// vocabulary, not a plugin's, so a console-subject may only pick from them.
func ConsoleKinds() []string {
	return []string{"vnc", "spice", "serial", "tty", "exec"}
}

// validateConsoleSubject holds a console subject to agent console kinds, each named once,
// and to a gate key the plugin adds itself, since opening a console is root in the guest.
func validateConsoleSubject(doc *manifestDoc, c Capability) error {
	if err := token("subject_type", c.SubjectType); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, kind := range c.Consoles {
		if !slices.Contains(ConsoleKinds(), kind) {
			return fmt.Errorf("consoles: %q is not an agent console kind (known: %v)", kind, ConsoleKinds())
		}
		if seen[kind] {
			return fmt.Errorf("consoles: %q is listed twice", kind)
		}
		seen[kind] = true
	}
	return addedPermission(doc, c.Permission)
}

// addedPermission requires key to be one the plugin adds, so a console gate can never be
// borrowed from the host or from another plugin.
func addedPermission(doc *manifestDoc, key string) error {
	if !rePerm.MatchString(key) {
		return fmt.Errorf("permission %q is not a permission key", key)
	}
	for _, p := range doc.AddsPermissions {
		if p.Key == key {
			return nil
		}
	}
	return fmt.Errorf("permission %q is not in adds_permissions", key)
}

// ConsoleSubject returns the declaration of guestKind, and false when no loaded plugin
// serves consoles for it, which is what makes an uninstalled plugin's kind unreachable.
func (r *Registry) ConsoleSubject(guestKind string) (Capability, bool) {
	if guestKind == "" {
		return Capability{}, false
	}
	return r.Provider(KindConsoleSubject, guestKind)
}

// ServesConsole reports whether a console-subject lists kind among its consoles.
func (c Capability) ServesConsole(kind string) bool {
	return c.Kind == KindConsoleSubject && slices.Contains(c.Consoles, kind)
}

// IsLifecycleSubject reports whether a loaded plugin declares subjectType a lifecycle
// subject, which is what lets its events onto the timeline.
func (r *Registry) IsLifecycleSubject(subjectType string) bool {
	if subjectType == "" {
		return false
	}
	_, ok := r.Provider(KindLifecycleSubject, subjectType)
	return ok
}
