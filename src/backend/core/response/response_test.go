// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package response

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

func decodeAPIResponse(t *testing.T, rec *httptest.ResponseRecorder) APIResponse {
	t.Helper()

	var resp APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestSuccessResponses(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		rec := httptest.NewRecorder()
		OK(rec, map[string]string{"id": "abc"})

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q", got)
		}
		resp := decodeAPIResponse(t, rec)
		if !resp.Success {
			t.Fatal("response should be successful")
		}
	})

	t.Run("Created", func(t *testing.T) {
		rec := httptest.NewRecorder()
		Created(rec, "created")

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		if !decodeAPIResponse(t, rec).Success {
			t.Fatal("response should be successful")
		}
	})

	t.Run("NoContent", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NoContent(rec)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("body length = %d, want 0", rec.Body.Len())
		}
	})
}

func TestErrorResponses(t *testing.T) {
	tests := []struct {
		name       string
		call       func(http.ResponseWriter)
		wantStatus int
		wantError  string
	}{
		{name: "Err", call: func(w http.ResponseWriter) { Err(w, http.StatusTeapot, "teapot") }, wantStatus: http.StatusTeapot, wantError: "teapot"},
		{name: "BadRequest", call: func(w http.ResponseWriter) { BadRequest(w, "bad") }, wantStatus: http.StatusBadRequest, wantError: "bad"},
		{name: "Unauthorized", call: func(w http.ResponseWriter) { Unauthorized(w, "no") }, wantStatus: http.StatusUnauthorized, wantError: "no"},
		{name: "UnauthorizedDefault", call: func(w http.ResponseWriter) { Unauthorized(w, "") }, wantStatus: http.StatusUnauthorized, wantError: "Unauthorized"},
		{name: "Forbidden", call: func(w http.ResponseWriter) { Forbidden(w, "forbidden") }, wantStatus: http.StatusForbidden, wantError: "forbidden"},
		{name: "NotFound", call: func(w http.ResponseWriter) { NotFound(w, "gone") }, wantStatus: http.StatusNotFound, wantError: "gone"},
		{name: "NotFoundDefault", call: func(w http.ResponseWriter) { NotFound(w, "") }, wantStatus: http.StatusNotFound, wantError: "Not found"},
		{name: "Conflict", call: func(w http.ResponseWriter) { Conflict(w, "conflict") }, wantStatus: http.StatusConflict, wantError: "conflict"},
		{name: "InternalError", call: func(w http.ResponseWriter) { InternalError(w, "boom") }, wantStatus: http.StatusInternalServerError, wantError: "boom"},
		{name: "InternalErrorDefault", call: func(w http.ResponseWriter) { InternalError(w, "") }, wantStatus: http.StatusInternalServerError, wantError: "Internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.call(rec)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			resp := decodeAPIResponse(t, rec)
			if resp.Success {
				t.Fatal("response should not be successful")
			}
			if resp.Error != tt.wantError {
				t.Fatalf("error = %q, want %q", resp.Error, tt.wantError)
			}
		})
	}
}

func TestCodeBasedResponses(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(i18n.WithLang(req.Context(), i18n.EN))

	tests := []struct {
		name       string
		call       func(http.ResponseWriter, *http.Request)
		wantStatus int
		wantCode   i18n.MsgCode
	}{
		{name: "ErrCode", call: func(w http.ResponseWriter, r *http.Request) { ErrCode(w, r, http.StatusTeapot, i18n.ErrUnauthorized) }, wantStatus: http.StatusTeapot, wantCode: i18n.ErrUnauthorized},
		{name: "BadRequestCode", call: func(w http.ResponseWriter, r *http.Request) { BadRequestCode(w, r, i18n.ErrInvalidBody) }, wantStatus: http.StatusBadRequest, wantCode: i18n.ErrInvalidBody},
		{name: "UnauthorizedCode", call: func(w http.ResponseWriter, r *http.Request) { UnauthorizedCode(w, r, i18n.ErrUnauthorized) }, wantStatus: http.StatusUnauthorized, wantCode: i18n.ErrUnauthorized},
		{name: "ForbiddenCode", call: func(w http.ResponseWriter, r *http.Request) { ForbiddenCode(w, r, i18n.ErrForbidden) }, wantStatus: http.StatusForbidden, wantCode: i18n.ErrForbidden},
		{name: "NotFoundCode", call: func(w http.ResponseWriter, r *http.Request) { NotFoundCode(w, r, i18n.ErrNotFound) }, wantStatus: http.StatusNotFound, wantCode: i18n.ErrNotFound},
		{name: "ConflictCode", call: func(w http.ResponseWriter, r *http.Request) { ConflictCode(w, r, i18n.ErrAuthUsernameTaken) }, wantStatus: http.StatusConflict, wantCode: i18n.ErrAuthUsernameTaken},
		{name: "InternalErrorCode", call: func(w http.ResponseWriter, r *http.Request) { InternalErrorCode(w, r, i18n.ErrInternalServer) }, wantStatus: http.StatusInternalServerError, wantCode: i18n.ErrInternalServer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.call(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			resp := decodeAPIResponse(t, rec)
			if resp.Success {
				t.Fatal("response should not be successful")
			}
			if resp.Code != string(tt.wantCode) {
				t.Fatalf("code = %q, want %q", resp.Code, tt.wantCode)
			}
			if resp.Error != i18n.T(i18n.EN, tt.wantCode) {
				t.Fatalf("error = %q, want translated message", resp.Error)
			}
		})
	}
}

func TestOKMsg(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(i18n.WithLang(req.Context(), i18n.EN))
	rec := httptest.NewRecorder()

	OKMsg(rec, req, i18n.MsgAuthLoggedOut)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
		Code    string            `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Fatal("response should be successful")
	}
	if resp.Code != string(i18n.MsgAuthLoggedOut) {
		t.Fatalf("code = %q, want %q", resp.Code, i18n.MsgAuthLoggedOut)
	}
	if resp.Data["message"] != i18n.T(i18n.EN, i18n.MsgAuthLoggedOut) {
		t.Fatalf("message = %q", resp.Data["message"])
	}
}

func TestPaginated(t *testing.T) {
	tests := []struct {
		name           string
		total          int64
		perPage        int
		wantTotalPages int
	}{
		{name: "exact", total: 50, perPage: 25, wantTotalPages: 2},
		{name: "remainder", total: 51, perPage: 25, wantTotalPages: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Paginated(rec, []string{"a"}, tt.total, 2, tt.perPage)

			var resp struct {
				Success bool          `json:"success"`
				Data    PaginatedData `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !resp.Success {
				t.Fatal("response should be successful")
			}
			if resp.Data.TotalPages != tt.wantTotalPages {
				t.Fatalf("total_pages = %d, want %d", resp.Data.TotalPages, tt.wantTotalPages)
			}
			if resp.Data.Page != 2 || resp.Data.PerPage != tt.perPage || resp.Data.Total != tt.total {
				t.Fatalf("pagination data = %#v", resp.Data)
			}
		})
	}
}

func TestDecodeBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":"demo"}`))
	var target struct {
		Name string `json:"name"`
	}

	if err := DecodeBody(req, &target); err != nil {
		t.Fatalf("DecodeBody valid JSON: %v", err)
	}
	if target.Name != "demo" {
		t.Fatalf("Name = %q, want demo", target.Name)
	}
}

func TestDecodeBodyReturnsDecodeErrorAndClosesBody(t *testing.T) {
	body := &closeTrackingReadCloser{Reader: bytes.NewBufferString(`{`)}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = body

	var target map[string]any
	if err := DecodeBody(req, &target); err == nil {
		t.Fatal("DecodeBody invalid JSON error = nil")
	}
	if !body.closed {
		t.Fatal("DecodeBody should close request body")
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		target      string
		wantPage    int
		wantPerPage int
	}{
		{target: "/", wantPage: 1, wantPerPage: 25},
		{target: "/?page=3&per_page=50", wantPage: 3, wantPerPage: 50},
		{target: "/?page=0&per_page=0", wantPage: 1, wantPerPage: 25},
		{target: "/?page=x&per_page=x", wantPage: 1, wantPerPage: 25},
		{target: "/?page=2&per_page=101", wantPage: 2, wantPerPage: 101},
		{target: "/?page=2&per_page=1000", wantPage: 2, wantPerPage: 1000},
		{target: "/?page=2&per_page=1001", wantPage: 2, wantPerPage: 25},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			page, perPage := ParsePagination(req)
			if page != tt.wantPage || perPage != tt.wantPerPage {
				t.Fatalf("ParsePagination(%q) = (%d, %d), want (%d, %d)", tt.target, page, perPage, tt.wantPage, tt.wantPerPage)
			}
		})
	}
}

func TestParseEnvID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?env=env-1", nil)
	if got := ParseEnvID(req); got != "env-1" {
		t.Fatalf("ParseEnvID = %q, want env-1", got)
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		value string
		want  int
	}{
		{value: "", want: 0},
		{value: "42", want: 42},
		{value: "0042", want: 42},
		{value: "4x", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := ParseInt(tt.value); got != tt.want {
				t.Fatalf("ParseInt(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

type closeTrackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *closeTrackingReadCloser) Close() error {
	r.closed = true
	return nil
}
