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

func failureBody(t *testing.T, authenticated bool) apiEnvelope {
	t.Helper()
	handler := &Handler{logger: quietLogger{}}
	served := requestid.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authenticated {
			r = r.WithContext(response.WithDetail(r.Context()))
		}
		handler.respondErr(w, r, "test", errors.New("qemu exploded while opening the disk"))
	}))
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	var body apiEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an envelope: %v", err)
	}
	if body.Success {
		t.Fatalf("a failure was written as a success: %s", recorder.Body.String())
	}
	return body
}

func TestRespondErrCarriesDetailAndRequestID(t *testing.T) {
	body := failureBody(t, true)
	if body.Detail == "" {
		t.Fatal("an authenticated failure carries no detail")
	}
	if body.RequestID == "" {
		t.Fatal("a failure carries no request id")
	}
}

func TestRespondErrWithholdsDetailWithoutAuthentication(t *testing.T) {
	body := failureBody(t, false)
	if body.Detail != "" {
		t.Fatalf("an unauthenticated failure leaked detail %q", body.Detail)
	}
	if body.RequestID == "" {
		t.Fatal("the request id must still be present")
	}
}
