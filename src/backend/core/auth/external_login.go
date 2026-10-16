// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"fmt"
	"time"
)

// External-directory sign-ins (LDAP) check the credential outside this package, so
// they cannot go through LoginFrom. These four steps are LoginFrom taken apart at
// the points where the directory is consulted, reusing the same lockout primitives,
// login_attempts rows and result shapes so the two paths cannot drift.

// invalidCredentialsResult is the generic refusal both paths give for a wrong
// credential, an unknown user and an account that may not sign in.
func invalidCredentialsResult() *AuthResult {
	return &AuthResult{Success: false, Error: "Invalid username or password"}
}

// accountLockedResult is the per-account lock refusal LoginFrom gives.
func accountLockedResult(retryAfter time.Duration) *AuthResult {
	return &AuthResult{
		Success:           false,
		Error:             "Account temporarily locked due to too many failed attempts",
		Locked:            true,
		RetryAfterSeconds: int64(retryAfter.Seconds()),
	}
}

// externalFailed records one failed external attempt and counts it against the
// client address, returning the address lock it armed, if any.
func (s *Service) externalFailed(ctx context.Context, attempted, userID, reason string, client LoginClient, policy LockoutPolicy) time.Duration {
	s.recordLoginAttempt(ctx, attempted, client, reason, false, userID)
	return s.RecordIPFailureContext(ctx, client.IP, policy)
}

// externalAccount reads the lockout key and state of the account an external
// identity is linked to.
func (s *Service) externalAccount(parent context.Context, userID string) (username string, active bool, err error) {
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()
	err = s.db.QueryRowContext(ctx, `SELECT username, is_active FROM users WHERE id = ?`, userID).Scan(&username, &active)
	if err != nil {
		return "", false, fmt.Errorf("reading linked account: %w", err)
	}
	return username, active, nil
}

// ExternalAddressGate refuses an external sign-in from a locked client address
// before the directory is contacted, so a locked address costs the directory
// nothing; nil means proceed.
func (s *Service) ExternalAddressGate(attempted string, client LoginClient) *AuthResult {
	return s.ExternalAddressGateContext(context.Background(), attempted, client)
}

// ExternalAddressGateContext is ExternalAddressGate bound to the caller's context, so a cancelled request stops the address gate.
func (s *Service) ExternalAddressGateContext(ctx context.Context, attempted string, client LoginClient) *AuthResult {
	if s == nil || s.db == nil {
		return nil
	}
	attempted = NormalizeUsername(attempted)
	policy := LoadLockoutPolicy(ctx, s.db)
	if locked, retryAfter := s.CheckIPLockoutContext(ctx, client.IP, policy); locked {
		s.recordLoginAttempt(ctx, attempted, client, loginReasonIPLocked, false, "")
		return ipLockedResult(retryAfter)
	}
	return nil
}

// ExternalAccountGate refuses an external sign-in for the account userID while it
// is disabled or locked, before the user's own bind, so a locked account never
// costs a directory bind or trips the directory's own lockout; nil means proceed.
// An empty userID (no linked account yet) always proceeds, as an unknown local
// username does. A read error fails open, as CheckLockout does.
func (s *Service) ExternalAccountGate(attempted, userID string, client LoginClient) *AuthResult {
	return s.ExternalAccountGateContext(context.Background(), attempted, userID, client)
}

// ExternalAccountGateContext is ExternalAccountGate bound to the caller's context, so a cancelled request stops the account gate.
func (s *Service) ExternalAccountGateContext(ctx context.Context, attempted, userID string, client LoginClient) *AuthResult {
	if s == nil || s.db == nil || userID == "" {
		return nil
	}
	attempted = NormalizeUsername(attempted)
	account, active, err := s.externalAccount(ctx, userID)
	if err != nil {
		return nil
	}
	policy := LoadLockoutPolicy(ctx, s.db)
	if !active {
		if ipLock := s.externalFailed(ctx, attempted, userID, "disabled", client, policy); ipLock > 0 {
			return ipLockedResult(ipLock)
		}
		return &AuthResult{Success: false, Error: "Account is disabled", Disabled: true}
	}
	if s.IsServicePrincipalContext(ctx, userID) {
		if ipLock := s.externalFailed(ctx, attempted, userID, "service_principal", client, policy); ipLock > 0 {
			return ipLockedResult(ipLock)
		}
		return invalidCredentialsResult()
	}
	if locked, _, retryAfter := s.CheckLockoutContext(ctx, account, policy); locked {
		if ipLock := s.externalFailed(ctx, attempted, userID, "locked", client, policy); ipLock > retryAfter {
			retryAfter = ipLock
		}
		return accountLockedResult(retryAfter)
	}
	return nil
}

// ExternalLoginFailed records a rejected external credential against the client
// address and, when userID names the linked account, against that account too,
// and returns the refusal LoginFrom would have given for a wrong password.
func (s *Service) ExternalLoginFailed(attempted, userID, reason string, client LoginClient) *AuthResult {
	return s.ExternalLoginFailedContext(context.Background(), attempted, userID, reason, client)
}

// ExternalLoginFailedContext is ExternalLoginFailed bound to the caller's context, so a cancelled request stops the failure bookkeeping.
func (s *Service) ExternalLoginFailedContext(ctx context.Context, attempted, userID, reason string, client LoginClient) *AuthResult {
	if s == nil || s.db == nil {
		return invalidCredentialsResult()
	}
	attempted = NormalizeUsername(attempted)
	if reason == "" {
		reason = "invalid_credentials"
	}
	policy := LoadLockoutPolicy(ctx, s.db)
	if userID != "" {
		if account, _, err := s.externalAccount(ctx, userID); err == nil {
			if nextLock := s.RecordFailedAttemptContext(ctx, account, policy); nextLock > 0 {
				if ipLock := s.externalFailed(ctx, attempted, userID, "locked", client, policy); ipLock > nextLock {
					nextLock = ipLock
				}
				return accountLockedResult(nextLock)
			}
		}
	}
	if ipLock := s.externalFailed(ctx, attempted, userID, reason, client, policy); ipLock > 0 {
		return ipLockedResult(ipLock)
	}
	return invalidCredentialsResult()
}

// ExternalLoginSucceeded finishes an external sign-in the way LoginFrom finishes a
// password one: the failure streak is cleared, last_login stamped and a session
// minted. Callers must have passed ExternalAccountGate for userID first.
func (s *Service) ExternalLoginSucceeded(userID string) (*AuthResult, error) {
	return s.ExternalLoginSucceededContext(context.Background(), userID)
}

// ExternalLoginSucceededContext is ExternalLoginSucceeded bound to the caller's context, so a cancelled request stops the session mint.
func (s *Service) ExternalLoginSucceededContext(ctx context.Context, userID string) (*AuthResult, error) {
	user, err := s.ValidateUserByIDContext(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading external account: %w", err)
	}
	if user == nil {
		return invalidCredentialsResult(), nil
	}
	s.ClearFailedAttemptsContext(ctx, userID)
	if _, err := s.db.ExecContext(ctx, "UPDATE users SET last_login = ? WHERE id = ?", time.Now().UTC().Format(time.RFC3339), userID); err != nil {
		return nil, fmt.Errorf("updating last login: %w", err)
	}
	sessionID, err := s.CreateSessionContext(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}
	return &AuthResult{Success: true, SessionID: sessionID, User: user}, nil
}
