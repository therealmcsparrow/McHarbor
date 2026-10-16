// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package selfguard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// TestAllowedFailsClosedForAnUndecidedUser holds the contract every caller leans on: no
// user is McHarbor acting for itself, but a user with no decision attached is refused.
func TestAllowedFailsClosedForAnUndecidedUser(t *testing.T) {
	if ok, err := Allowed(context.Background()); err != nil || !ok {
		t.Fatalf("no principal: Allowed = %v, %v; want true", ok, err)
	}
	userCtx := context.WithValue(context.Background(), auth.UserContextKey, &auth.User{ID: "u1"})
	if ok, err := Allowed(userCtx); err != nil || ok {
		t.Fatal("a user with no decision attached was allowed")
	}
	systemCtx := context.WithValue(context.Background(), auth.UserContextKey, &auth.User{ID: "system"})
	if ok, err := Allowed(systemCtx); err != nil || !ok {
		t.Fatal("the system user was refused")
	}
	if ok, err := Allowed(WithDecision(userCtx, true)); err != nil || !ok {
		t.Fatal("an attached allow decision was ignored")
	}
}

func TestRefusalIsErrRequiredAndDenyNamesTheKey(t *testing.T) {
	ref := &Refusal{Code: i18n.ErrHostSelfContainerRequired, Target: "app"}
	if !errors.Is(ref, ErrRequired) {
		t.Fatal("a Refusal does not match ErrRequired")
	}
	rec := httptest.NewRecorder()
	Deny(rec, httptest.NewRequest(http.MethodPost, "/", nil), ref)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
	var body struct {
		Code string     `json:"code"`
		Data DeniedData `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != string(i18n.ErrHostSelfContainerRequired) || body.Data.Permission != "host_self.manage" {
		t.Fatalf("body %+v does not name the code and host_self.manage", body)
	}
}
