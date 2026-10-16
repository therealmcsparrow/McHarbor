// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import "github.com/therealmcsparrow/mcharbor/core/failreason"

// failureText is a stream's terminal error_text: what failed, then why, so the
// install dialog's log names the cause (a signature that does not verify, a
// migration that failed) instead of only saying that the install failed.
func failureText(outcome string, err error) string {
	if reason := failreason.Of(err); reason != "" {
		return outcome + ": " + reason
	}
	return outcome
}
