// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"strings"
	"testing"
)

func TestPrivilegePlanFollowsDetectedMode(t *testing.T) {
	cases := []struct {
		mode      Escalation
		wantOK    bool
		wantHead  string
		wantPTY   bool
		wantStdin string
	}{
		{EscalateNone, true, "sh -c ", false, ""},
		{EscalateSudo, true, "sudo -n sh -c ", false, ""},
		{EscalateSudoPassword, true, "sudo -S -p '' sh -c ", false, "ssh-pw\n"},
		{EscalateSu, true, "su -c ", true, "root-pw\n"},
		{EscalateUnavailable, false, "", false, ""},
	}
	for _, c := range cases {
		p := privilege{Mode: c.mode, sshPassword: "ssh-pw", rootPassword: "root-pw"}
		plan, ok := p.plan("apt-get install -y docker")
		if ok != c.wantOK {
			t.Fatalf("%s: ok = %v, want %v", c.mode, ok, c.wantOK)
		}
		if !ok {
			continue
		}
		if !strings.HasPrefix(plan.line, c.wantHead) || plan.pty != c.wantPTY || plan.stdin != c.wantStdin {
			t.Errorf("%s: got %+v", c.mode, plan)
		}
	}
}
