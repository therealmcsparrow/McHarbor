// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A failed install used to end on "installation failed" with the cause only in
// the server log; the operator must read it in the install dialog.
func TestInstallFailureNamesTheReason(t *testing.T) {
	err := errors.New("starting stack: Container hass-otbr  Creating\nError response from daemon: error gathering device information while adding custom device \"/dev/ttyUSB0\": no such file or directory")
	got := installFailureMessage(context.Background(), err)
	if !strings.HasPrefix(got, "installation failed: ") || !strings.Contains(got, `custom device "/dev/ttyUSB0": no such file or directory`) {
		t.Fatalf("message = %q", got)
	}
}
