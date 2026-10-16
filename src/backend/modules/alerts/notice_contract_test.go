// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/alertnotice"
)

// TestAlertNoticeConstantsMatchCore pins the copies push reads from
// core/alertnotice to the values this engine actually emits, so a container
// alert keeps deep-linking and a resolve keeps rendering as a resolve.
func TestAlertNoticeConstantsMatchCore(t *testing.T) {
	if alertnotice.SourceContainer != sourceContainer {
		t.Fatalf("alertnotice.SourceContainer = %q, engine emits %q", alertnotice.SourceContainer, sourceContainer)
	}
	if alertnotice.StateResolved != EventStateResolved {
		t.Fatalf("alertnotice.StateResolved = %q, engine emits %q", alertnotice.StateResolved, EventStateResolved)
	}
}
