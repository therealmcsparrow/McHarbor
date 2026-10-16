// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"errors"
	"fmt"
	"testing"
)

// The install and update streams' error_text is what the plugin dialog logs, so
// it carries the cause after the outcome.
func TestFailureTextCarriesTheCause(t *testing.T) {
	err := fmt.Errorf("installing: %w", errors.New("migration 004_tables.sql failed: table vms already exists"))
	got := failureText("plugin install failed", err)
	if got != "plugin install failed: installing: migration 004_tables.sql failed: table vms already exists" {
		t.Fatalf("failureText = %q", got)
	}
	if got := failureText("update failed", nil); got != "update failed" {
		t.Fatalf("failureText(nil) = %q", got)
	}
}
