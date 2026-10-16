// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package response writes the { success, data, error, code } JSON envelope every handler returns.
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
)

// APIResponse matches the frontend contract: { success, data?, error?, message?, code?, detail?, requestId? }.
// Detail is the redacted cause behind a coded failure; RequestID ties a failure to its server log line.
type APIResponse struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
	Code      string `json:"code,omitempty"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// PaginatedData wraps list responses with pagination metadata.
type PaginatedData struct {
	Items      any   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalPages int   `json:"total_pages"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("response: writing JSON body failed", "error", err)
	}
}

// OK sends { success: true, data: ... } with 200.
func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

// Created sends { success: true, data: ... } with 201.
func Created(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: data})
}

// NoContent sends 204 with no body.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Err sends { success: false, error: ... } with the given status.
func Err(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, APIResponse{Success: false, Error: msg})
}

// BadRequest sends 400.
func BadRequest(w http.ResponseWriter, msg string) {
	Err(w, http.StatusBadRequest, msg)
}

// Unauthorized sends 401.
func Unauthorized(w http.ResponseWriter, msg string) {
	if msg == "" {
		msg = "Unauthorized"
	}
	Err(w, http.StatusUnauthorized, msg)
}

// Forbidden sends 403.
func Forbidden(w http.ResponseWriter, msg string) {
	Err(w, http.StatusForbidden, msg)
}

// NotFound sends 404.
func NotFound(w http.ResponseWriter, msg string) {
	if msg == "" {
		msg = "Not found"
	}
	Err(w, http.StatusNotFound, msg)
}

// Conflict sends 409.
func Conflict(w http.ResponseWriter, msg string) {
	Err(w, http.StatusConflict, msg)
}

// InternalError sends 500.
func InternalError(w http.ResponseWriter, msg string) {
	if msg == "" {
		msg = "Internal server error"
	}
	Err(w, http.StatusInternalServerError, msg)
}

// --- Code-based helpers (i18n-aware) ---

// ErrCode sends { success: false, error: ..., code: ... } with the given status,
// translating the message based on the request's Accept-Language.
func ErrCode(w http.ResponseWriter, r *http.Request, status int, code i18n.MsgCode) {
	lang := i18n.FromContext(r.Context())
	writeJSON(w, status, APIResponse{
		Success:   false,
		Error:     i18n.T(lang, code),
		Code:      string(code),
		RequestID: requestid.From(r.Context()),
	})
}

// ErrCodeWithData sends a failure envelope that still carries a data payload.
//
// Most failures are a single code. Some are a code plus structure the client
// needs in order to say anything useful — a stack deploy that rejected four
// services by name, for instance, where a bare code would leave the UI showing
// "deploy failed" over a list it cannot render.
func ErrCodeWithData(w http.ResponseWriter, r *http.Request, status int, code i18n.MsgCode, data any) {
	lang := i18n.FromContext(r.Context())
	writeJSON(w, status, APIResponse{
		Success:   false,
		Error:     i18n.T(lang, code),
		Code:      string(code),
		Data:      data,
		RequestID: requestid.From(r.Context()),
	})
}

// BadRequestCode sends 400 with a translated message code.
func BadRequestCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusBadRequest, code)
}

// BadRequestCodeWithData sends 400 with a translated code and a data payload.
func BadRequestCodeWithData(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, data any) {
	ErrCodeWithData(w, r, http.StatusBadRequest, code, data)
}

// UnauthorizedCode sends 401 with a translated message code.
func UnauthorizedCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusUnauthorized, code)
}

// ForbiddenCode sends 403 with a translated message code.
func ForbiddenCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusForbidden, code)
}

// NotFoundCode sends 404 with a translated message code.
func NotFoundCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusNotFound, code)
}

// ConflictCode sends 409 with a translated message code.
func ConflictCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusConflict, code)
}

// InternalErrorCode sends 500 with a translated message code.
func InternalErrorCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusInternalServerError, code)
}

// ServiceUnavailableCode sends 503 with a translated message code.
//
// Use it when an upstream McHarbor depends on is unreachable or is refusing
// service, rather than InternalErrorCode: 500 asserts that McHarbor itself
// broke, which sends anyone reading the status line - a reverse proxy, an
// uptime probe, a log sweep for 5xx - after the wrong fault.
func ServiceUnavailableCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusServiceUnavailable, code)
}

// OKMsg sends { success: true, data: { message: "..." }, code: ... } with 200.
func OKMsg(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	lang := i18n.FromContext(r.Context())
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"message": i18n.T(lang, code)},
		Code:    string(code),
	})
}

// Paginated sends a paginated list response.
func Paginated(w http.ResponseWriter, items any, total int64, page, perPage int) {
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}
	OK(w, PaginatedData{
		Items:      items,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

// DecodeBody parses JSON request body into target struct.
func DecodeBody(r *http.Request, target any) error {
	defer func() {
		if err := r.Body.Close(); err != nil {
			slog.Debug("response: closing request body failed", "error", err)
		}
	}()
	return json.NewDecoder(r.Body).Decode(target)
}

// ParsePagination extracts page/per_page query params with defaults.
//
// The default per-page size is 25 and the maximum is 1000. Endpoints
// that serve very large result sets (audit logs, container events)
// typically pair this with lazy loading on the client and request
// per_page values between 50 and 500.
func ParsePagination(r *http.Request) (page, perPage int) {
	page = 1
	perPage = 25

	if v := r.URL.Query().Get("page"); v != "" {
		if p := parseInt(v); p > 0 {
			page = p
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if pp := parseInt(v); pp > 0 && pp <= 1000 {
			perPage = pp
		}
	}
	return page, perPage
}

// ParseEnvID extracts the environment ID from ?env= query param.
func ParseEnvID(r *http.Request) string {
	return r.URL.Query().Get("env")
}

// ParseInt parses a string to int, returning 0 on failure.
func ParseInt(s string) int {
	return parseInt(s)
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// NoStore marks a response as uncacheable. Call it before the OK/Created
// helper that writes the body — once the status line is out, headers are
// fixed.
//
// Reserved for responses that carry a plaintext credential: the stack
// env-var reveal, and the secrets vault's reveal endpoint. A value that
// reaches a shared proxy cache or a browser's back-forward cache has
// outlived the access check that released it.
func NoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}
