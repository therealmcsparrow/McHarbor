// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

type handler struct {
	app *router.AppDeps
	svc *Service
}

// reportQuery is the parsed query string. Redaction defaults on for an export and
// off for the dashboard, because only the export leaves the operator's hands.
type reportQuery struct {
	sections []string
	redact   bool
	export   bool
}

func parseReportQuery(r *http.Request) (reportQuery, bool) {
	q := r.URL.Query()
	var raw []string
	if v := strings.TrimSpace(q.Get("sections")); v != "" {
		for _, part := range strings.Split(v, ",") {
			raw = append(raw, strings.TrimSpace(part))
		}
	}
	sections, ok := ParseSections(raw)
	if !ok {
		return reportQuery{}, false
	}
	out := reportQuery{sections: sections, export: q.Get("export") == "1"}
	out.redact = out.export
	switch q.Get("redact") {
	case "1", "true":
		out.redact = true
	case "0", "false":
		out.redact = false
	case "":
	default:
		return reportQuery{}, false
	}
	return out, true
}

func (h *handler) handlePosture(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	q, ok := parseReportQuery(r)
	if !ok {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "invalid_query"})
		return
	}
	report := h.svc.Report(r.Context(), q.sections, q.redact)
	if q.export {
		details, err := json.Marshal(map[string]any{"sections": q.sections, "redacted": q.redact})
		if err != nil {
			h.app.Logger.Error("compliance: marshal audit details", "error", err)
		}
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "compliance.report.export",
			Severity:   "info",
			EntityType: "compliance_report",
			EntityName: report.GeneratedAt,
			Details:    string(details),
		})
	}
	response.OK(w, report)
}
