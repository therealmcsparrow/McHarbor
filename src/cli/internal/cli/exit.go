// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package cli is the mcharbor command tree: global flags, contexts,
// credentials, output rendering and every command, over the generated client.
package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Exit codes are an API: scripts branch on them, so they are frozen at v1 and a
// change to any of them is a breaking change.
const (
	ExitOK        = 0
	ExitFailure   = 1
	ExitUsage     = 2
	ExitAuth      = 3
	ExitForbidden = 4
	ExitNotFound  = 5
	ExitConflict  = 6
	ExitRemote    = 7
	ExitTimeout   = 8
)

// Error is a failure carrying the exit code it maps to and, for a refused
// request, the permission keys the operation needs.
type Error struct {
	// Code is the process exit code.
	Code int `json:"exitCode"`
	// Message is the human explanation, the server's own when it gave one.
	Message string `json:"message"`
	// Operation is the operationId of the request that failed, when there was one.
	Operation string `json:"operation,omitempty"`
	// Status is the HTTP status the server answered with.
	Status int `json:"status,omitempty"`
	// ServerCode is the server's i18n error code, such as err.rbac.permission_denied.
	ServerCode string `json:"code,omitempty"`
	// Permissions lists the keys the operation's route enforces.
	Permissions []string `json:"permissions,omitempty"`
	// RequestID is the server's request id header, when it sent one.
	RequestID string `json:"requestId,omitempty"`
}

// Error renders one sentence an operator can act on; a 403 names the key to grant.
func (e *Error) Error() string {
	msg := e.Message
	if e.Operation != "" && e.Status != 0 {
		msg = fmt.Sprintf("%s: %s (HTTP %d)", e.Operation, msg, e.Status)
	}
	if e.ServerCode != "" {
		msg += " [" + e.ServerCode + "]"
	}
	if e.Code == ExitForbidden && len(e.Permissions) > 0 {
		msg += "; missing permission: " + strings.Join(e.Permissions, ", ")
	}
	return msg
}

// usageError is a mistake in how the command was invoked.
func usageError(format string, args ...any) *Error {
	return &Error{Code: ExitUsage, Message: fmt.Sprintf(format, args...)}
}

// codeForStatus maps an HTTP status to the frozen exit codes.
func codeForStatus(status int) int {
	switch {
	case status == http.StatusBadRequest, status == http.StatusUnprocessableEntity:
		return ExitUsage
	case status == http.StatusUnauthorized:
		return ExitAuth
	case status == http.StatusForbidden:
		return ExitForbidden
	case status == http.StatusNotFound:
		return ExitNotFound
	case status == http.StatusConflict:
		return ExitConflict
	case status == http.StatusRequestTimeout, status == http.StatusGatewayTimeout:
		return ExitTimeout
	case status >= 500:
		return ExitRemote
	default:
		return ExitFailure
	}
}

// ExitCode returns the exit code for err; cobra's own argument errors are usage.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ExitTimeout
	}
	msg := err.Error()
	for _, prefix := range []string{"unknown command", "unknown flag", "unknown shorthand", "accepts ", "requires at least", "requires exactly", "invalid argument", "flag needs an argument", "required flag"} {
		if strings.HasPrefix(msg, prefix) {
			return ExitUsage
		}
	}
	return ExitFailure
}

// asError normalises any error into an *Error for machine-readable output.
func asError(err error) *Error {
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr
	}
	return &Error{Code: ExitCode(err), Message: err.Error()}
}
