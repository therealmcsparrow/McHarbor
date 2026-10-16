// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

func TestValidateSettingValueMFARequirement(t *testing.T) {
	for _, value := range []string{"off", "admins", "all"} {
		if code, ok := validateSettingValue(rbac.SettingMFARequired, value); !ok {
			t.Fatalf("%q rejected with %s", value, code)
		}
	}
	for _, value := range []string{"", "on", "admin", "everyone"} {
		code, ok := validateSettingValue(rbac.SettingMFARequired, value)
		if ok || code != i18n.ErrSettingsMFARequirementInvalid {
			t.Fatalf("%q: ok=%v code=%s, want rejection with %s", value, ok, code, i18n.ErrSettingsMFARequirementInvalid)
		}
	}
}
