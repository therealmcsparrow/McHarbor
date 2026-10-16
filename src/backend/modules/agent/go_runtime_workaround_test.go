// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"strings"
	"testing"
)

// The agent's systemd unit carries the Go runtime workaround for the Green Tea GC's AVX-512
// scan path, and the scripts give Docker's own Go services the same drop-in on such a CPU.
// See .references/bugfix/2026-10-05-1958-zen4-avx512-go-gc-heap-corruption.md.
func TestAgentInstallsCarryTheGoRuntimeWorkaround(t *testing.T) {
	const unitLine = `Environment="GODEBUG=cpu.avx512f=off,cpu.avx512vl=off"`
	inTempWorkdir(t)
	writeBinary(t, localAgentDir, "mcharbor-agent-linux-amd64")
	scripts := map[string]string{
		"install script": renderInstallScript("tok", "https://mcharbor.example", true, false, "binary", "", ""),
		"ssh deploy":     buildBinaryDeployCmd(context.Background(), "https://mcharbor.example", "tok", "linux", "amd64", true, "", ""),
	}
	for name, script := range scripts {
		// A line of its own: the Docker drop-in's printf carries the same text inline.
		if !strings.Contains(script, "\n"+unitLine+"\n") {
			t.Errorf("%s: the agent unit lacks %s", name, unitLine)
		}
		if !strings.Contains(script, "grep -qw avx512f /proc/cpuinfo") || !strings.Contains(script, "mcharbor-go-avx512.conf") {
			t.Errorf("%s: Docker and containerd get no drop-in on an AVX-512 CPU", name)
		}
		if strings.Contains(script, "systemctl restart docker") || strings.Contains(script, "systemctl restart containerd") {
			t.Errorf("%s: installing the agent must not restart Docker, which stops every container", name)
		}
	}
}
