// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseServicesOutputNeverEncodesNullServices(t *testing.T) {
	cases := map[string]string{
		"sysvinit with no service lines": "___INIT:sysvinit\n",
		"systemd with no units":          "___INIT:systemd",
		"no init system":                 "___INIT:none",
		"empty output":                   "",
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(parseServicesOutput(output))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(raw), `"services":[]`) {
				t.Fatalf("services must encode as [], got %s", raw)
			}
		})
	}
}
