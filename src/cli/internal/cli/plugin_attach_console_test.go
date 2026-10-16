// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"strings"
	"testing"
)

// TestAttachConsoleDefaultIsTheFirstDeclaredTerminal: the default comes from the
// declaration, skipping graphical kinds, so a VM listing vnc first still attaches serial.
func TestAttachConsoleDefaultIsTheFirstDeclaredTerminal(t *testing.T) {
	for _, tc := range []struct {
		declared []string
		want     string
	}{
		{[]string{"vnc", "serial", "spice"}, "serial"},
		{[]string{"tty", "exec"}, "tty"},
		{[]string{"exec", "tty"}, "exec"},
		{[]string{"vnc", "spice"}, ""},
		{nil, ""},
	} {
		def, help := attachConsoleFlag(tc.declared)
		if def != tc.want {
			t.Errorf("%v: default = %q, want %q", tc.declared, def, tc.want)
		}
		for _, kind := range tc.declared {
			if !strings.Contains(help, kind) {
				t.Errorf("%v: help %q does not name declared kind %s", tc.declared, help, kind)
			}
		}
	}
}

// TestAttachConsoleFollowsTheDeclaration: a kind is accepted only when declared and a
// terminal, and every refusal names the declared kinds rather than a hard-coded set.
func TestAttachConsoleFollowsTheDeclaration(t *testing.T) {
	ct := []string{"tty", "exec"}
	vm := []string{"vnc", "serial", "spice"}
	for _, kind := range []string{"tty", "exec"} {
		if err := checkAttachConsole(ct, kind); err != nil {
			t.Fatalf("ct %s refused: %v", kind, err)
		}
		if err := checkAttachConsole(vm, kind); err == nil {
			t.Fatalf("vm %s accepted; it is not declared", kind)
		} else if !strings.Contains(err.Error(), "serial") {
			t.Fatalf("vm %s refusal %q does not name the declared terminal", kind, err)
		}
	}
	for _, kind := range []string{"serial", "vnc"} {
		if err := checkAttachConsole(ct, kind); err == nil {
			t.Fatalf("ct %s accepted; it is not declared", kind)
		} else if !strings.Contains(err.Error(), "tty, exec") {
			t.Fatalf("ct %s refusal %q does not name the declared kinds", kind, err)
		}
	}
	if err := checkAttachConsole(vm, "serial"); err != nil {
		t.Fatalf("vm serial refused: %v", err)
	}
	if err := checkAttachConsole(vm, "vnc"); err == nil || !strings.Contains(err.Error(), "not a terminal") {
		t.Fatalf("vm vnc in a terminal: got %v, want a not-a-terminal refusal", err)
	}
	if err := checkAttachConsole([]string{"vnc"}, "vnc"); err == nil || !strings.Contains(err.Error(), "declared: vnc") {
		t.Fatalf("graphical-only guest: got %v, want a refusal naming what is declared", err)
	}
	if err := checkAttachConsole(nil, "serial"); err == nil {
		t.Fatal("a guest kind with no declaration accepted serial")
	}
}
