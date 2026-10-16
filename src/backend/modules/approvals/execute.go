// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// captureWriter records a replayed response so the execute call can report it.
type captureWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCaptureWriter() *captureWriter {
	return &captureWriter{header: http.Header{}, status: http.StatusOK}
}

func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *captureWriter) WriteHeader(status int)      { c.status = status }
func (c *captureWriter) Flush()                      {}

// replay serves the stored request through the finished router with the
// executor's own identity in context. Every middleware and route gate runs
// again, so a permission removed since the request was held is refused now.
func replay(ctx context.Context, root http.Handler, from *http.Request, req *Request, body []byte) *captureWriter {
	// A nil route context makes chi route the replay from the top rather than
	// resume the execute route's own routing state.
	ctx = context.WithValue(ctx, chi.RouteCtxKey, (*chi.Context)(nil))
	ctx = withReplay(ctx, req.ID)
	out, err := http.NewRequestWithContext(ctx, req.Method, req.Path, bytes.NewReader(body))
	cw := newCaptureWriter()
	if err != nil {
		cw.status = http.StatusBadRequest
		return cw
	}
	if len(body) > 0 {
		out.Header.Set("Content-Type", "application/json")
	}
	if lang := from.Header.Get("Accept-Language"); lang != "" {
		out.Header.Set("Accept-Language", lang)
	}
	out.RemoteAddr = from.RemoteAddr
	root.ServeHTTP(cw, out)
	return cw
}

// HandleExecute replays an approved request exactly once, as its requester.
// The claim is atomic, so a second call — or a race — finds it already spent.
func (h *Handler) HandleExecute(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if h.app.Root == nil {
		h.app.Logger.Error("approvals: execute before the router was built")
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	req, body, err := h.svc.Claim(r.Context(), chi.URLParam(r, "id"), user.ID)
	if err != nil {
		h.writeError(w, r, "claim", err)
		return
	}
	cw := replay(r.Context(), h.app.Root, r, req, body)

	var parsed any
	code := ""
	if cw.body.Len() > 0 && json.Unmarshal(cw.body.Bytes(), &parsed) == nil {
		if m, ok := parsed.(map[string]any); ok {
			code, _ = m["code"].(string)
		}
	}
	done, err := h.svc.Finish(r.Context(), req.ID, cw.status, code)
	if err != nil {
		h.writeError(w, r, "finish", err)
		return
	}
	severity := "success"
	if cw.status >= 400 {
		severity = "error"
	}
	h.auditRequest(r, "approval.execute", severity, done, fmt.Sprintf("result_status=%d result_code=%s", cw.status, code))
	response.OK(w, ExecuteResult{Approval: done, ResultStatus: cw.status, Response: parsed})
}
