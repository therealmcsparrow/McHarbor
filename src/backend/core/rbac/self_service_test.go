// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// TestRequireSelfServiceHoldsAPIKeysToAccountSelfManage: a session acting on its
// own account passes, a narrow automation key does not, a key carrying
// account.self.manage does.
func TestRequireSelfServiceHoldsAPIKeysToAccountSelfManage(t *testing.T) {
	svc := NewService(nil)
	h := RequireSelfService(svc)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	call := func(key *APIKeyPrincipal) int {
		req := httptest.NewRequest(http.MethodPost, "/api/mfa/enroll/begin", nil)
		ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"})
		if key != nil {
			ctx = WithAPIKey(ctx, key)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req.WithContext(ctx))
		return rec.Code
	}

	if got := call(nil); got != http.StatusOK {
		t.Fatalf("a session: got %d, want 200", got)
	}
	if got := call(&APIKeyPrincipal{KeyID: "narrow", Scopes: []Permission{PermContainersView}}); got != http.StatusForbidden {
		t.Fatalf("a key without account.self.manage: got %d, want 403", got)
	}
	if got := call(&APIKeyPrincipal{KeyID: "self", Scopes: []Permission{PermAccountSelfManage}}); got != http.StatusOK {
		t.Fatalf("a key with account.self.manage: got %d, want 200", got)
	}

	reqs := ProbeChain(RequireSelfService(svc))
	if len(reqs) != 1 || reqs[0].Scope != ScopeAccount || len(reqs[0].Keys) != 1 || reqs[0].Keys[0] != PermAccountSelfManage {
		t.Fatalf("the spec must say an API key needs account.self.manage, got %+v", reqs)
	}
}
