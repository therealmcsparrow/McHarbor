// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// PasswordPolicyResponse is the client-facing shape of the runtime
// password policy. It mirrors core/auth.PasswordPolicy field for
// field so the browser can pre-validate a candidate with exactly
// the rules the server will apply.
//
// MaxAgeDays and HistorySize are informational: neither can be
// checked client-side (the browser has no access to the user's
// password history), but the UI states them so a "password reused"
// rejection is not a surprise.
type PasswordPolicyResponse struct {
	MinLength     int  `json:"minLength"`
	RequireUpper  bool `json:"requireUpper"`
	RequireLower  bool `json:"requireLower"`
	RequireDigit  bool `json:"requireDigit"`
	RequireSymbol bool `json:"requireSymbol"`
	MaxAgeDays    int  `json:"maxAgeDays"`
	HistorySize   int  `json:"historySize"`
}

// HandlePasswordPolicy returns the effective password policy.
//
// Deliberately unauthenticated (it sits in the rate-limited auth
// group next to /auth/status and /auth/setup): the first-run setup
// wizard and the token-based reset page both run logged out and
// must be able to state the rules before the user starts typing.
// The payload carries complexity requirements only — no secrets,
// no user data, and nothing an attacker could not learn by
// submitting two passwords.
func (h *Handler) HandlePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	policy := auth.LoadPasswordPolicy(r.Context(), h.app.DB)
	response.OK(w, PasswordPolicyResponse{
		MinLength:     policy.MinLength,
		RequireUpper:  policy.RequireUpper,
		RequireLower:  policy.RequireLower,
		RequireDigit:  policy.RequireDigit,
		RequireSymbol: policy.RequireSymbol,
		MaxAgeDays:    policy.MaxAgeDays,
		HistorySize:   policy.HistorySize,
	})
}
