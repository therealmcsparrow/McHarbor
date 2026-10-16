// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package deviceauth implements the OAuth-style device authorization flow the
// CLI signs in with: the terminal shows a short code, a signed-in user approves
// it in the browser, and the terminal receives a new API key exactly once.
package deviceauth

import (
	"errors"

	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
)

// Errors a poll can end in; each maps to an OAuth device-flow error string.
var (
	ErrPending  = errors.New("deviceauth: authorization pending")
	ErrSlowDown = errors.New("deviceauth: polling too fast")
	ErrExpired  = errors.New("deviceauth: code expired")
	ErrDenied   = errors.New("deviceauth: request denied")
	ErrInvalid  = errors.New("deviceauth: unknown or already used code")
	ErrNotFound = errors.New("deviceauth: no pending request with that code")
)

// StartInput describes the terminal asking to sign in, shown to the approver so
// they can tell their own request from someone else's.
type StartInput struct {
	Host   string `json:"host"`
	OSUser string `json:"osUser"`
}

// StartResult is what the terminal needs to run the flow. DeviceCode is a
// secret; the user code is the part a person types.
type StartResult struct {
	DeviceCode       string `json:"deviceCode"`
	UserCode         string `json:"userCode"`
	VerificationPath string `json:"verificationPath"`
	ExpiresIn        int    `json:"expiresIn"`
	Interval         int    `json:"interval"`
}

// TokenInput is one poll.
type TokenInput struct {
	DeviceCode string `json:"deviceCode"`
}

// Request is a pending sign-in as the approver sees it.
type Request struct {
	UserCode  string `json:"userCode"`
	Host      string `json:"host"`
	OSUser    string `json:"osUser"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
}

// ApproveInput is the key the approver agrees to issue, in the same shape as
// creating an API key by hand.
type ApproveInput struct {
	Name           string   `json:"name"`
	RoleIDs        []string `json:"roleIds"`
	Scopes         []string `json:"scopes"`
	EnvironmentIDs []string `json:"environmentIds"`
	ExpiresAt      *string  `json:"expiresAt"`
}

// TokenResult is the one response that ever carries the new key.
type TokenResult = apikeys.CreateAPIKeyResult
