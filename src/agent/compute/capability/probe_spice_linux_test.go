// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package capability

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fakeQEMU stands in for a modular QEMU build: it greets on QMP like the real
// binary, and with NO_SPICE set it refuses -spice the way Ubuntu's does when
// qemu-system-modules-spice is not installed.
const fakeQEMU = `#!/bin/sh
for a in "$@"; do
  if [ "$a" = "-spice" ] && [ -n "$NO_SPICE" ]; then
    echo "qemu-system-x86_64: -spice: There is no option group 'spice'" >&2
    exit 1
  fi
done
echo '{"QMP": {"version": {"qemu": {"major": 8}}, "capabilities": []}}'
cat >/dev/null
`

func writeFakeQEMU(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "qemu-system-x86_64")
	if err := os.WriteFile(path, []byte(fakeQEMU), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSPICEProbeRefusesAModularBuildWithoutTheModule is the homelab01 failure:
// query-spice was in the schema, -spice was not usable, and every guest with
// SPICE on failed to start.
func TestSPICEProbeRefusesAModularBuildWithoutTheModule(t *testing.T) {
	path := writeFakeQEMU(t)

	t.Setenv("NO_SPICE", "1")
	if qemuSPICEUsable(t.Context(), path) {
		t.Fatal("a QEMU that rejects -spice was reported as able to serve SPICE")
	}

	t.Setenv("NO_SPICE", "")
	if !qemuSPICEUsable(t.Context(), path) {
		t.Fatal("a QEMU that starts with -spice was reported as unable to serve SPICE")
	}
}

func TestWithoutCommandDropsOnlyTheNamedVerb(t *testing.T) {
	got := withoutCommand([]string{"query-cpus-fast", "query-spice", "set_link"}, "query-spice")
	if !slices.Equal(got, []string{"query-cpus-fast", "set_link"}) {
		t.Fatalf("withoutCommand = %v", got)
	}
}
