// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import "testing"

// TestConsoleSubjectLookup pins what the console gateway relies on: a declared guest kind
// resolves with its consoles and gate key, and nothing resolves once its plugin unloads.
func TestConsoleSubjectLookup(t *testing.T) {
	reg := NewRegistry()
	reg.Set("addon-demo", []Capability{{
		Kind: KindConsoleSubject, SubjectType: "widget",
		Consoles: []string{"vnc", "spice"}, Permission: "demo.console",
	}})

	c, ok := reg.ConsoleSubject("widget")
	if !ok || c.Permission != "demo.console" || c.Plugin != "addon-demo" {
		t.Fatalf("widget: want addon-demo gated on demo.console, got %+v (%v)", c, ok)
	}
	if !c.ServesConsole("spice") || c.ServesConsole("tty") {
		t.Errorf("widget: want spice served and tty refused, got %v", c.Consoles)
	}
	if _, ok := reg.ConsoleSubject(""); ok {
		t.Error("an empty guest kind must never resolve")
	}
	if _, ok := reg.ConsoleSubject("gadget"); ok {
		t.Error("an undeclared guest kind must not resolve")
	}

	reg.Remove("addon-demo")
	if _, ok := reg.ConsoleSubject("widget"); ok {
		t.Error("a console subject must stop resolving once its plugin unloads")
	}
}

// TestLifecycleSubjectTypes pins the declared-kind set the lifecycle service validates against.
func TestLifecycleSubjectTypes(t *testing.T) {
	reg := NewRegistry()
	reg.Set("addon-demo", []Capability{
		{Kind: KindLifecycleSubject, SubjectType: "widget", Page: "/plugins/addon-demo"},
		{Kind: KindConsoleSubject, SubjectType: "gadget", Consoles: []string{"tty"}, Permission: "demo.console"},
	})
	if !reg.IsLifecycleSubject("widget") {
		t.Error("widget is declared as a lifecycle subject")
	}
	if reg.IsLifecycleSubject("gadget") {
		t.Error("a console subject is not a lifecycle subject")
	}
	reg.Remove("addon-demo")
	if reg.IsLifecycleSubject("widget") {
		t.Error("an unloaded plugin's lifecycle subject must not validate")
	}
}
