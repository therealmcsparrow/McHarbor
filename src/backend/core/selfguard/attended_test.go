// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package selfguard

import (
	"context"
	"errors"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// TestAttendedRefusesABareContext is the difference from Allowed: a context with no
// decision attached (a job, a reconciler pass) is refused, even one carrying the system user.
func TestAttendedRefusesABareContext(t *testing.T) {
	bare := context.Background()
	if ok, err := Allowed(bare); !ok || err != nil {
		t.Fatal("Allowed must keep answering yes to a bare context (the self-update flow)")
	}
	system := context.WithValue(bare, auth.UserContextKey, &auth.User{ID: "system"})
	for name, ctx := range map[string]context.Context{"bare": bare, "system user": system} {
		if ok, err := Attended(ctx); ok || err != nil {
			t.Errorf("Attended(%s) = %v, %v; want false", name, ok, err)
		}
		if err := RequireAttended(ctx, i18n.ErrHostSelfContainerRequired, "app", nil); !errors.Is(err, ErrRequired) {
			t.Errorf("RequireAttended(%s) = %v; want a Refusal", name, err)
		}
	}
	if err := RequireAttended(WithDecision(bare, true), i18n.ErrHostSelfContainerRequired, "app", nil); err != nil {
		t.Errorf("RequireAttended with an allowing decision = %v", err)
	}
	if err := RequireAttended(WithDecision(bare, false), i18n.ErrHostSelfContainerRequired, "app", nil); !errors.Is(err, ErrRequired) {
		t.Errorf("RequireAttended with a refusing decision = %v", err)
	}
}
