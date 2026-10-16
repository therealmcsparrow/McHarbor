// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"strings"
	"testing"
)

// TestStandardRedactionRemovesAWholeJWT guards the ordering bug where the
// entropy mask replaced a JWT's long signature first, leaving the header and
// claims in a shape the JWT pattern no longer matched — and sent to the model.
func TestStandardRedactionRemovesAWholeJWT(t *testing.T) {
	line := "auth ok token=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U done"
	out := Redact(map[string]any{"log": line}, RedactStandard).(map[string]any)["log"].(string)
	for _, part := range []string{"eyJhbGciOiJIUzI1NiJ9", "eyJzdWIiOiIxMjM0NTY3ODkwIn0"} {
		if strings.Contains(out, part) {
			t.Fatalf("a JWT segment survived redaction: %q", out)
		}
	}
	if !strings.Contains(out, "auth ok") || !strings.Contains(out, "done") {
		t.Fatalf("redaction removed the surrounding text: %q", out)
	}
	creds := Redact("postgres://app:hunter2secret@db:5432/app", RedactStandard).(string)
	if strings.Contains(creds, "hunter2secret") {
		t.Fatalf("a connection password survived: %q", creds)
	}
}
