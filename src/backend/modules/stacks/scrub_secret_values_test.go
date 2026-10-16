// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"strings"
	"testing"
)

// The one error that named the missing device read `custom device "[redacted]"`,
// because every value of eight characters or more was scrubbed. Only secrets are.
func TestComposeOutputKeepsPlainValuesAndScrubsSecrets(t *testing.T) {
	declared := map[string]string{
		"RADIO_DEVICE":  "/dev/ttyUSB0",
		"BACKBONE_IF":   "enp3s0f0",
		"DB_PASSWORD":   "hunter2hunter2",
		"UPSTREAM_HOST": "${secret:matter-upstream}",
		"DATABASE_URL":  "postgres://app:s3cretpass@db:5432/app",
	}
	resolved := map[string]string{}
	for k, v := range declared {
		resolved[k] = v
	}
	resolved["UPSTREAM_HOST"] = "resolved-vault-value-77"

	text := `Error response from daemon: error gathering device information while adding custom device "/dev/ttyUSB0": no such file or directory` +
		"\nbackbone enp3s0f0 password hunter2hunter2 token resolved-vault-value-77 url postgres://app:s3cretpass@db:5432/app"
	got := scrubComposeOutput(text, secretEnvValues(declared, resolved))

	for _, keep := range []string{`custom device "/dev/ttyUSB0"`, "backbone enp3s0f0"} {
		if !strings.Contains(got, keep) {
			t.Errorf("a plain value was scrubbed, want %q in %q", keep, got)
		}
	}
	for _, leak := range []string{"hunter2hunter2", "resolved-vault-value-77", "s3cretpass"} {
		if strings.Contains(got, leak) {
			t.Errorf("secret %q reached the output: %q", leak, got)
		}
	}
}
