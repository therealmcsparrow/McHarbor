// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// failingAuthorizer is an RBAC check that errors rather than answering.
type failingAuthorizer struct{}

func (failingAuthorizer) HasPermission(context.Context, string, string, rbac.Permission) (bool, error) {
	return false, errors.New("sql: no such table: rbac_internal_users at /var/lib/mcharbor/mcharbor.db")
}

// captureDefaultLog routes slog's default logger into a buffer for the test.
func captureDefaultLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

// TestCallErrorNeverCarriesTheRawError keeps a lower layer's error text (table names,
// paths, upstream bodies) out of what the caller and the model see, while the server log
// still records it with the tool and the user.
func TestCallErrorNeverCarriesTheRawError(t *testing.T) {
	logged := captureDefaultLog(t)
	const leak = "open /var/lib/mcharbor/secrets/token: permission denied"
	Register(&Tool{
		Name:     "test.leaky_handler",
		ReadOnly: true,
		Handler: func(context.Context, Scope, json.RawMessage) (any, error) {
			return nil, errors.New(leak)
		},
	})
	Register(&Tool{
		Name:     "test.leaky_rbac",
		ReadOnly: true,
		Required: []rbac.Permission{rbac.PermContainersView},
		Handler:  func(context.Context, Scope, json.RawMessage) (any, error) { return "ok", nil },
	})

	for _, name := range []string{"test.leaky_handler", "test.leaky_rbac"} {
		_, ce := Call(t.Context(), failingAuthorizer{}, nil, nil, CallRequest{
			Name: name, Arguments: json.RawMessage(`{}`), Scope: Scope{UserID: "u-42"},
		})
		if ce == nil {
			t.Fatalf("%s: expected a CallError", name)
		}
		wire, err := json.Marshal(ce)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{leak, "rbac_internal_users", "/var/lib"} {
			if strings.Contains(string(wire), secret) || strings.Contains(ce.Message, secret) {
				t.Errorf("%s: the caller sees %q in %s", name, secret, wire)
			}
		}
		if strings.Contains(string(wire), `"details"`) {
			t.Errorf("%s: CallError still serialises details: %s", name, wire)
		}
		if !strings.Contains(logged.String(), name) || !strings.Contains(logged.String(), "u-42") {
			t.Errorf("%s: the real error was not logged with the tool and user: %s", name, logged.String())
		}
	}
	if !strings.Contains(logged.String(), "permission denied") || !strings.Contains(logged.String(), "rbac_internal_users") {
		t.Errorf("the log lost the real error: %s", logged.String())
	}
}
