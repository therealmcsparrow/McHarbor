// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"strings"
	"testing"
)

// TestValidationFailuresAnswerAReasonKeyNotProse pins the refusal shape: every
// validation failure maps to a stable reason key, never the formatted message
// that names the offending branch or path.
func TestValidationFailuresAnswerAReasonKeyNotProse(t *testing.T) {
	ok := DefaultSettings()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"confirm passes", func() error { s := ok; s.ConfirmPasses = 0; return s.Validate() }(), "confirm_passes_range"},
		{"actions per pass", func() error { s := ok; s.MaxActionsPerPass = MaxActionsCap + 1; return s.Validate() }(), "actions_per_pass_range"},
		{"cooldown", func() error { s := ok; s.CooldownSeconds = -1; return s.Validate() }(), "cooldown_range"},
		{"failure threshold", func() error { s := ok; s.FailureThreshold = 0; return s.Validate() }(), "failure_threshold_range"},
		{"required fields", (&SourceInput{}).Validate(), "source_fields_required"},
		{"branch", (&SourceInput{Name: "n", RepoID: "r", EnvID: "e", Branch: "bad branch"}).Validate(), "invalid_branch"},
		{"path", (&SourceInput{Name: "n", RepoID: "r", EnvID: "e", Path: "../etc"}).Validate(), "path_escapes_repo"},
	}
	for _, c := range cases {
		if c.err == nil {
			t.Fatalf("%s: expected a validation failure", c.name)
		}
		got := validationReason(c.err)
		if got != c.want {
			t.Errorf("%s: reason = %q, want %q", c.name, got, c.want)
		}
		if strings.Contains(got, " ") {
			t.Errorf("%s: reason %q reads as prose, not a key", c.name, got)
		}
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("default settings must validate: %v", err)
	}
}
