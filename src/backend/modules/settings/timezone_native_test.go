// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"errors"
	"testing"
)

func TestTimezoneChangeIsRefusedOnANativeInstall(t *testing.T) {
	SetNativeHost(t.TempDir(), func(context.Context) error { return nil })
	t.Cleanup(func() { nativeHost.set = false })

	// No database: the refusal must come before anything is saved or recreated.
	svc := &Service{}
	_, err := svc.UpdateTimezone(context.Background(), TimezoneInput{Timezone: "Europe/Amsterdam"})
	if !errors.Is(err, ErrTimezoneNative) {
		t.Fatalf("UpdateTimezone on a native install = %v, want ErrTimezoneNative", err)
	}
}
