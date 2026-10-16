// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package auth serves the login, session, profile and password-policy endpoints and the bridges plugins use for external login and MFA.
package auth

import (
	"context"
	"errors"
)

// A login that lands on an MFA-enrolled account has to raise a challenge and
// verify the code before a session is issued. The mfa module owns enrolment,
// challenges and verification; a module must not import another module, so
// auth declares the narrow contract here and main.go injects the bridge
// built in internal/bootstrap.
//
// The bridge translates the mfa module's sentinel errors into the ones below
// so errors.Is keeps working on this side of the boundary.

// MFAVerifier is the second-factor half of the login flow.
type MFAVerifier interface {
	// IsEnabled reports whether the account has a confirmed second factor.
	IsEnabled(ctx context.Context, userID string) (bool, error)
	// CreateChallenge opens a pending challenge and returns its id.
	CreateChallenge(ctx context.Context, userID string) (string, error)
	// PeekChallenge returns the challenge's user id without consuming it.
	PeekChallenge(ctx context.Context, challengeID string) (string, error)
	// VerifyForLogin checks a submitted code against the user's factor.
	VerifyForLogin(ctx context.Context, userID, code string) error
	// ConsumeChallenge burns the challenge so a code cannot be replayed.
	ConsumeChallenge(ctx context.Context, challengeID string) (string, error)
}

// Sentinels the login handler distinguishes. The bridge maps the mfa
// module's equivalents onto these.
var (
	// ErrMFAChallengeInvalid — unknown, expired or already-consumed challenge.
	ErrMFAChallengeInvalid = errors.New("mfa challenge invalid")
	// ErrMFAInvalidCode — the submitted code did not verify.
	ErrMFAInvalidCode = errors.New("mfa code invalid")
	// ErrMFANotEnrolled — the account has no confirmed second factor.
	ErrMFANotEnrolled = errors.New("mfa not enrolled")
)

// noMFA is the fallback when no bridge is injected. It reports every account
// as having no second factor, which matches a deployment without the MFA
// module mounted; it never silently accepts a code.
type noMFA struct{}

func (noMFA) IsEnabled(context.Context, string) (bool, error)         { return false, nil }
func (noMFA) CreateChallenge(context.Context, string) (string, error) { return "", ErrMFANotEnrolled }
func (noMFA) PeekChallenge(context.Context, string) (string, error) {
	return "", ErrMFAChallengeInvalid
}
func (noMFA) VerifyForLogin(context.Context, string, string) error { return ErrMFANotEnrolled }
func (noMFA) ConsumeChallenge(context.Context, string) (string, error) {
	return "", ErrMFAChallengeInvalid
}
