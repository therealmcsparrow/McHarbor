// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

func runFailCause(t *testing.T, authed bool) apiEnvelope {
	t.Helper()
	h := requestid.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authed {
			r = r.WithContext(response.WithDetail(r.Context()))
		}
		failCause(w, r, http.StatusInternalServerError, "err.test.failed", errors.New("dial tcp 10.0.0.5:6443: connection refused"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	var env apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.RequestID == "" || env.RequestID != rec.Header().Get(requestid.Header) {
		t.Fatalf("requestId %q does not match header %q", env.RequestID, rec.Header().Get(requestid.Header))
	}
	return env
}

func TestFailCauseCarriesDetailOnlyWhenAuthenticated(t *testing.T) {
	if env := runFailCause(t, true); env.Detail == "" || env.Code != "err.test.failed" {
		t.Fatalf("authenticated failure lost its detail: %+v", env)
	}
	if env := runFailCause(t, false); env.Detail != "" {
		t.Fatalf("unauthenticated failure leaked detail %q", env.Detail)
	}
}
