// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin_runtime

import (
	"slices"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

// TestAttachCommandsCarryTheDeclaredConsoles pins where the CLI's --console list comes
// from: the attaching plugin's own console-subject, in declared order, and nothing else.
func TestAttachCommandsCarryTheDeclaredConsoles(t *testing.T) {
	reg := plugincaps.NewRegistry()
	reg.Set("addon-demo", []plugincaps.Capability{{
		Kind: plugincaps.KindConsoleSubject, SubjectType: "widget",
		Consoles: []string{"vnc", "serial"}, Permission: "demo.console",
	}})

	if got := declaredConsoles(reg, "addon-demo", "widget"); !slices.Equal(got, []string{"vnc", "serial"}) {
		t.Fatalf("addon-demo widget consoles = %v, want [vnc serial] in declared order", got)
	}
	if got := declaredConsoles(reg, "addon-other", "widget"); got != nil {
		t.Fatalf("another plugin's declaration leaked onto addon-other's command: %v", got)
	}
	if got := declaredConsoles(reg, "addon-demo", "gadget"); got != nil {
		t.Fatalf("an undeclared guest kind got consoles: %v", got)
	}
}
