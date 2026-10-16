// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import "errors"

// MFA sentinel errors. The mfa module produces them and the auth module's
// login handler maps them to HTTP responses; keeping them here lets the
// two modules agree on identity without importing each other.
var (
	// ErrMFANotEnrolled is returned when a verify/confirm/disable is
	// attempted for a user with no active enrollment.
	ErrMFANotEnrolled = errors.New("mfa: not enrolled")
	// ErrMFAInvalidCode covers a wrong TOTP code, a wrong recovery
	// code, and a replayed code.
	ErrMFAInvalidCode = errors.New("mfa: invalid code")
	// ErrMFAChallengeInvalid is returned for an unknown, expired, or
	// already-consumed login challenge.
	ErrMFAChallengeInvalid = errors.New("mfa: invalid challenge")
)
