// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package mfa implements time-based one-time-password (TOTP)
// multi-factor authentication: enrollment, verification, recovery
// codes, and the short-lived challenges that gate the second step of
// an interactive login.
package mfa

// Status is the per-user MFA state returned to the profile UI. It
// deliberately carries no secret material — only whether MFA is on and
// when it was last exercised.
type Status struct {
	Enabled     bool   `json:"enabled"`
	EnrolledAt  string `json:"enrolledAt,omitempty"`
	LastUsedAt  string `json:"lastUsedAt,omitempty"`
	HasRecovery bool   `json:"hasRecovery"`
	// RecoveryRemaining is how many unused recovery codes are left,
	// so the UI can nudge the user to regenerate before they run out.
	RecoveryRemaining int `json:"recoveryRemaining"`
}

// Enrollment is the one-and-only-time payload handed to the client
// when enrollment begins. The secret and the recovery codes are never
// retrievable again — the client renders the QR from OtpauthURI and
// the user is expected to store the recovery codes themselves.
type Enrollment struct {
	Secret        string   `json:"secret"`
	OtpauthURI    string   `json:"otpauthUri"`
	RecoveryCodes []string `json:"recoveryCodes"`
}

// confirmRequest is the JSON body for POST /auth/mfa/enroll/confirm.
type confirmRequest struct {
	Code string `json:"code"`
}

// pendingSecret is the decrypted state of a `user_mfa` row.
type pendingSecret struct {
	Secret        string
	Enabled       bool
	RecoveryCodes []string
	LastUsedAt    string
}
