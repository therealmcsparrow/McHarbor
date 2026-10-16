// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/permissions"
)

func testClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, "mh_test_key", false, "test")
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return c
}

// TestDecodeUnwrapsTheSuccessEnvelope checks the payload inside data is what a
// resource receives, and that auth and the /api prefix reach the server.
func TestDecodeUnwrapsTheSuccessEnvelope(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/roles/r1" {
			t.Errorf("path = %q, want /api/roles/r1", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer mh_test_key" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"r1","name":"ops","description":"","permissions":["containers.view"],"isSystem":false,"createdAt":"x","updatedAt":"y"}}`))
	})
	var role client.RolesRole
	if err := decode(apiCall(c.RolesGet(context.Background(), "r1")), "rolesGet", &role); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if role.Name != "ops" || len(role.Permissions) != 1 {
		t.Fatalf("decoded %+v", role)
	}
}

// TestForbiddenNamesThePermission is the point of the permission map: a 403
// must tell the operator which key to grant, not just that they lack one.
func TestForbiddenNamesThePermission(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"error":"Permission denied","code":"err.forbidden"}`))
	})
	err := decode(apiCall(c.RolesCreate(context.Background(), client.RolesCreateJSONRequestBody{Name: "x"})), "rolesCreate", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Status != http.StatusForbidden {
		t.Fatalf("status = %d", apiErr.Status)
	}
	if !strings.Contains(err.Error(), "roles.manage") || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("message %q does not name roles.manage and the server's reason", err.Error())
	}
}

// TestPermissionMapCoversTier1Operations guards the generated map against a
// spec that stopped carrying x-mcharbor-permission.
func TestPermissionMapCoversTier1Operations(t *testing.T) {
	for op, want := range map[string]string{
		"rolesCreate":        "roles.manage",
		"environmentsCreate": "environments.manage",
		"stacksDelete":       "stacks.delete",
		"usersAssignRole":    "users.manage",
		"apiKeysCreate":      "",
	} {
		keys := permissions.For(op)
		if want == "" {
			continue
		}
		if !contains(keys, want) {
			t.Errorf("permissions.For(%q) = %v, want it to include %q", op, keys, want)
		}
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// TestNotFoundIsRecognised is what lets Read drop a deleted object from state.
func TestNotFoundIsRecognised(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"Role not found","code":"err.role.not_found"}`))
	})
	err := decode(apiCall(c.RolesGet(context.Background(), "gone")), "rolesGet", &client.RolesRole{})
	if !isNotFound(err) {
		t.Fatalf("isNotFound(%v) = false", err)
	}
}

// TestSuccessStatusWithFailureEnvelopeIsAnError covers handlers that answer
// 200 with success=false, which must not be mistaken for a result.
func TestSuccessStatusWithFailureEnvelopeIsAnError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error":"Stack is protected"}`))
	})
	if err := decode(apiCall(c.RolesGet(context.Background(), "r1")), "rolesGet", &client.RolesRole{}); err == nil {
		t.Fatal("decode accepted success=false")
	}
}

// TestNoContentAndNonJSON check the two bodies that are not an envelope.
func TestNoContentAndNonJSON(t *testing.T) {
	noContent := testClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	if err := decode(apiCall(noContent.RolesDelete(context.Background(), "r1")), "rolesDelete", nil); err != nil {
		t.Fatalf("204: %v", err)
	}
	html := testClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>proxy page</html>")) })
	if err := decode(apiCall(html.RolesGet(context.Background(), "r1")), "rolesGet", &client.RolesRole{}); err == nil {
		t.Fatal("decode accepted a non-JSON body")
	}
}

// TestAPIKeyCreateResultIsFlat pins the real wire shape: McHarbor embeds the
// key record, so fields sit beside the secret rather than under "APIKey".
func TestAPIKeyCreateResultIsFlat(t *testing.T) {
	raw := rawData(`{"id":"k1","name":"ci","keyPrefix":"mh_ab","isActive":true,"roles":[{"id":"r1","name":"ops"}],"scopes":[],"environmentIds":[],"key":"mh_secret"}`)
	var key client.ApiKeysAPIKey
	var secret apiKeySecret
	if err := raw.into(&key, &secret); err != nil {
		t.Fatalf("into: %v", err)
	}
	if key.Id != "k1" || secret.Key != "mh_secret" || len(key.Roles) != 1 {
		t.Fatalf("key=%+v secret=%+v", key, secret)
	}
}
