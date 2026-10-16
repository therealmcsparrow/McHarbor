// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package response

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
)

func TestInternalErrorCauseCarriesRedactedDetailAndRequestID(t *testing.T) {
	var rec *httptest.ResponseRecorder
	h := requestid.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		InternalErrorCause(w, r.WithContext(WithDetail(r.Context())), i18n.ErrInternalServer,
			errors.New("dial unix /var/run/docker.sock: connect: permission denied (password=hunter2)"))
	}))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	resp := decodeAPIResponse(t, rec)
	if resp.Code != string(i18n.ErrInternalServer) || resp.Error == "" {
		t.Fatalf("code/error not set: %+v", resp)
	}
	if !strings.Contains(resp.Detail, "permission denied") {
		t.Fatalf("detail lost the cause: %q", resp.Detail)
	}
	if strings.Contains(resp.Detail, "docker.sock") || strings.Contains(resp.Detail, "hunter2") {
		t.Fatalf("detail not redacted: %q", resp.Detail)
	}
	if resp.RequestID == "" || resp.RequestID != rec.Header().Get(requestid.Header) {
		t.Fatalf("requestId %q does not match header %q", resp.RequestID, rec.Header().Get(requestid.Header))
	}
}

func TestDetailOmittedWhenItRepeatsTheSummary(t *testing.T) {
	if got := Detail(errors.New("Not found"), "Not found"); got != "" {
		t.Fatalf("Detail = %q, want empty", got)
	}
	if got := Detail(nil, "x"); got != "" {
		t.Fatalf("Detail(nil) = %q, want empty", got)
	}
}

func TestCauseWithheldFromUnauthenticatedRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	InternalErrorCause(rec, req, i18n.ErrInternalServer, errors.New("ldap: user alice not found"))
	if resp := decodeAPIResponse(t, rec); resp.Detail != "" {
		t.Fatalf("pre-auth request received detail %q", resp.Detail)
	}
}

func TestDetailOmitsNoRows(t *testing.T) {
	if got := Detail(fmt.Errorf("load stack: %w", sql.ErrNoRows), "Stack not found"); got != "" {
		t.Fatalf("Detail = %q, want empty", got)
	}
}
