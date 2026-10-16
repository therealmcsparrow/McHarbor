// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package response is the Swarm plugin's copy of the host's response writer.
//
// It exists so the ported handlers keep their call sites unchanged. Every one
// of the ~190 `response.OK` / `response.BadRequestCode` / `response.ParseEnvID`
// calls in this plugin was written against `core/response` when Swarm was a
// host module; re-declaring the same twelve functions here — over the plugin's
// own message catalog rather than the host's — moved 5,600 lines of handler
// and service code without touching a single statement inside them.
//
// It is a trimmed copy, not the whole file: the pagination helpers, the
// string-message variants (`BadRequest`, `NotFound`, …) and `NoContent` have
// no caller in this plugin and were left behind rather than carried across as
// dead code.
//
// The wire shape is identical to the host's — `{ success, data?, error?,
// code? }` — because the frontend does not know or care which process answered.
package response

import (
	"encoding/json"
	"net/http"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/i18n"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	hostresponse "github.com/therealmcsparrow/mcharbor/core/response"
)

// APIResponse matches the frontend contract: { success, data?, error?, message?, code? }.
type APIResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
	// Detail is the failure's redacted cause on an authenticated request, and
	// RequestID ties the failure to the server log line carrying the raw error.
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Swarm answers carry join tokens, unlock keys and secret metadata. None
	// of it belongs in a shared cache, and the host's own response writer sets
	// this for the same reason.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// OK sends { success: true, data: ... } with 200.
func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

// Created sends { success: true, data: ... } with 201.
func Created(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: data})
}

// ErrCode sends a failure envelope with the given status, translating the
// message from the request's Accept-Language.
func ErrCode(w http.ResponseWriter, r *http.Request, status int, code i18n.MsgCode) {
	writeJSON(w, status, APIResponse{
		Success:   false,
		Error:     i18n.T(i18n.FromRequest(r), code),
		Code:      string(code),
		RequestID: requestid.From(r.Context()),
	})
}

// ErrCodeCause is ErrCode plus err's redacted text as detail, so the operator
// sees why the daemon refused and not only which operation failed.
func ErrCodeCause(w http.ResponseWriter, r *http.Request, status int, code i18n.MsgCode, err error) {
	msg := i18n.T(i18n.FromRequest(r), code)
	writeJSON(w, status, APIResponse{
		Success:   false,
		Error:     msg,
		Code:      string(code),
		Detail:    hostresponse.DetailFor(r, err, msg),
		RequestID: requestid.From(r.Context()),
	})
}

// BadRequestCause sends 400 with a translated code and err's redacted detail.
func BadRequestCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusBadRequest, code, err)
}

// NotFoundCause sends 404 with a translated code and err's redacted detail.
func NotFoundCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusNotFound, code, err)
}

// ConflictCause sends 409 with a translated code and err's redacted detail.
func ConflictCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusConflict, code, err)
}

// InternalErrorCause sends 500 with a translated code and err's redacted detail.
func InternalErrorCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusInternalServerError, code, err)
}

// ErrCodeWithData sends a failure envelope that still carries a data payload.
//
// One caller: a stack deploy that rejected services by name, where a bare code
// would leave the UI showing "deploy failed" over a list it cannot render.
func ErrCodeWithData(w http.ResponseWriter, r *http.Request, status int, code i18n.MsgCode, data any) {
	writeJSON(w, status, APIResponse{
		Success:   false,
		Error:     i18n.T(i18n.FromRequest(r), code),
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
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"message": i18n.T(i18n.FromRequest(r), code)},
		Code:    string(code),
	})
}

// DecodeBody parses a JSON request body into target.
func DecodeBody(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

// ParseEnvID extracts the environment ID from the ?env= query param.
func ParseEnvID(r *http.Request) string {
	return r.URL.Query().Get("env")
}
