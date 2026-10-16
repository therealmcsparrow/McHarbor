// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"errors"

	"github.com/therealmcsparrow/mcharbor/core/failreason"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// installFailureMessage is the last line of a failed install's progress: a
// self-protection refusal in the operator's language, otherwise the reason the
// stack would not deploy, such as a missing device or a port already bound.
func installFailureMessage(ctx context.Context, err error) string {
	var refusal *selfguard.Refusal
	if errors.As(err, &refusal) {
		return i18n.T(i18n.FromContext(ctx), refusal.Code)
	}
	if reason := failreason.Of(err); reason != "" {
		return "installation failed: " + reason
	}
	return "installation failed"
}
