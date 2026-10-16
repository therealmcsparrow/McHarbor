// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

type fakeLoadedRuntime map[string]pluginruntime.Loaded

func (f fakeLoadedRuntime) Loaded() map[string]pluginruntime.Loaded { return f }

func TestPluginLoadedViewReadsTheRuntimeLazily(t *testing.T) {
	var current any
	view := PluginLoadedView(func() any { return current })

	if got := view(); len(got) != 0 {
		t.Fatalf("before the runtime boots = %v, want empty", got)
	}

	current = fakeLoadedRuntime{
		"addon-lxc":     {Slug: "addon-lxc", Loaded: true},
		"addon-kvm":     {Slug: "addon-kvm", Loaded: false, Err: "abi mismatch"},
		"identity-ldap": {Slug: "identity-ldap", Loaded: true, Err: "late failure"},
	}
	got := view()
	if !got["addon-lxc"] || got["addon-kvm"] || got["identity-ldap"] {
		t.Fatalf("loaded view = %v, want only addon-lxc", got)
	}
}
