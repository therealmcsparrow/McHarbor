// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package response

import (
	"encoding/json"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// ApiResponse matches the frontend contract: { success, data?, error?, message?, code? }
type ApiResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
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
	json.NewEncoder(w).Encode(v)
}

// OK sends { success: true, data: ... } with 200.
func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, ApiResponse{Success: true, Data: data})
}

// Created sends { success: true, data: ... } with 201.
func Created(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, ApiResponse{Success: true, Data: data})
}

// NoContent sends 204 with no body.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Err sends { success: false, error: ... } with the given status.
func Err(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ApiResponse{Success: false, Error: msg})
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
	lang := i18n.FromRequest(r)
	writeJSON(w, status, ApiResponse{Success: false, Error: i18n.T(lang, code), Code: string(code)})
}

// BadRequestCode sends 400 with a translated message code.
func BadRequestCode(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	ErrCode(w, r, http.StatusBadRequest, code)
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

// OKMsg sends { success: true, data: { message: "..." }, code: ... } with 200.
func OKMsg(w http.ResponseWriter, r *http.Request, code i18n.MsgCode) {
	lang := i18n.FromRequest(r)
	writeJSON(w, http.StatusOK, ApiResponse{
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
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

// ParsePagination extracts page/per_page query params with defaults.
func ParsePagination(r *http.Request) (page, perPage int) {
	page = 1
	perPage = 25

	if v := r.URL.Query().Get("page"); v != "" {
		if p := parseInt(v); p > 0 {
			page = p
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if pp := parseInt(v); pp > 0 && pp <= 100 {
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
