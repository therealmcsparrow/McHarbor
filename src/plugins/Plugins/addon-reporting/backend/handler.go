// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HTTP layer.
//
// Handlers parse, validate, call the service, audit and respond —
// nothing else. No SQL, no rendering, no business rules. Errors are
// logged server-side with their detail and returned to the client as a
// stable code, never as err.Error(): a raw error can carry a path, a
// query or a hostname that has no business leaving the process.

type apiEnvelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"code,omitempty"`
	// Detail is err's redacted text on an authenticated request; RequestID
	// ties the failure to the server log line that carries the raw error.
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// Error codes. Deliberately coarse — they tell the UI which message to
// show, not what went wrong internally.
const (
	codeBadRequest  = "err.reporting.invalid_request"
	codeUnknownType = "err.reporting.unknown_type"
	codeNotFound    = "err.reporting.not_found"
	codeGenerate    = "err.reporting.generate_failed"
	codeExport      = "err.reporting.export_failed"
	codeInternal    = "err.reporting.internal"
	codeBadFormat   = "err.reporting.unsupported_format"
	// codeAIDisabled means the reporting plugin's own AIReports setting
	// is off. codeAIUnavailable and codeAIConnectionUnavailable are
	// distinct conditions that a single codeAIDisabled used to collapse
	// into this one message — including telling a user whose AIReports
	// setting was already on to go enable it, when the real cause was
	// that no AI provider was reachable on the host at all.
	codeAIDisabled              = "err.reporting.ai_disabled"
	codeAIUnavailable           = "err.reporting.ai_unavailable"
	codeAIConnectionUnavailable = "err.reporting.ai_connection_unavailable"
	codePromptRequired          = "err.reporting.prompt_required"
	codeAIPlanFailed            = "err.reporting.ai_plan_failed"
	codeAIAuthorFailed          = "err.reporting.ai_author_failed"
	msgReportDeleted            = "msg.reporting.deleted"

	// Schedules. codeNoTargets and codeBadTarget are separate because
	// they are different operator mistakes: one saved a schedule that
	// delivers nowhere, the other named a destination that is not a
	// channel or a location at all.
	codeNoTargets           = "err.reporting.schedule_no_targets"
	codeBadTarget           = "err.reporting.schedule_bad_target"
	codeBadCron             = "err.reporting.schedule_bad_cron"
	codeBadPath             = "err.reporting.schedule_bad_path"
	codeBadFilename         = "err.reporting.schedule_bad_filename"
	codeBadRecipient        = "err.reporting.schedule_bad_recipient"
	codeScheduleRunning     = "err.reporting.schedule_running"
	codeDeliveryFailed      = "err.reporting.delivery_failed"
	codeDeliveryUnavailable = "err.reporting.delivery_unavailable"
	codeDeliveryTargetGone  = "err.reporting.delivery_target_gone"

	msgScheduleDeleted    = "msg.reporting.schedule_deleted"
	msgScheduleRunStarted = "msg.reporting.schedule_run_started"
)

func writeJSON(w http.ResponseWriter, status int, body apiEnvelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func ok(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Data: data})
}

func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, apiEnvelope{Success: false, Code: code, Error: code})
}

// failCause is fail plus the request ID and err's redacted text, so the
// operator sees why the call failed and can quote the ID against the log.
func failCause(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	writeJSON(w, status, apiEnvelope{
		Success:   false,
		Code:      code,
		Error:     code,
		Detail:    response.DetailFor(r, err, code),
		RequestID: requestid.From(r.Context()),
	})
}

func (p *ReportingPlugin) logErr(msg string, err error, args ...any) {
	if p.host.Logger == nil {
		return
	}
	p.host.Logger.Error(msg, append([]any{"error", err}, args...)...)
}

func (p *ReportingPlugin) audit(event string, fields map[string]any) {
	p.auditAs("", event, fields)
}

// auditActorEmitter is the actor-attributing half of the host's audit handle.
//
// Asserted rather than required: a host that predates it still loads this
// plugin, the row just goes out unattributed as it always did.
type auditActorEmitter interface {
	EmitEvent(ev plugin.AuditEvent) error
}

// auditAs writes an audit row attributed to userID. A blank id is an
// unattributed row, which is what the scheduler and the workflow nodes want.
func (p *ReportingPlugin) auditAs(userID, event string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if actor, attributed := p.host.Audit.(auditActorEmitter); attributed {
		_ = actor.EmitEvent(plugin.AuditEvent{Action: event, UserID: userID, Fields: fields})
		return
	}
	_ = p.host.Audit.Emit(event, fields)
}

// --- catalog ---------------------------------------------------------

type catalogResponse struct {
	Types     []ReportType `json:"types"`
	Levels    []Level      `json:"levels"`
	PageSizes []pageSize   `json:"page_sizes"`
	Settings  Settings     `json:"settings"`
}

func (p *ReportingPlugin) handleCatalog(w http.ResponseWriter, r *http.Request) {
	ok(w, catalogResponse{
		Types:     catalog(),
		Levels:    []Level{Strategic, Tactical, Operational},
		PageSizes: pageSizes(),
		// The studio reads this for the default period and the cover
		// wording; it never draws a mark. Sending the marks anyway would
		// put several megabytes of base64 on a request the studio makes
		// on every visit. The settings page fetches them from /settings.
		Settings: p.svc.store.settings(r.Context(), p.host.DB.KV()).withoutMarks(),
	})
}

// --- library ---------------------------------------------------------

type listResponse struct {
	Items      []StoredReport `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	PerPage    int            `json:"per_page"`
	TotalPages int            `json:"total_pages"`
}

func (p *ReportingPlugin) handleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)
	reportType := strings.TrimSpace(r.URL.Query().Get("type"))
	if reportType != "" {
		if _, ok := reportTypeByKey(reportType); !ok {
			fail(w, http.StatusBadRequest, codeUnknownType)
			return
		}
	}

	total, err := p.svc.store.count(r.Context(), reportType)
	if err != nil {
		p.logErr("reporting: count reports failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	items, err := p.svc.store.list(r.Context(), reportType, perPage, (page-1)*perPage)
	if err != nil {
		p.logErr("reporting: list reports failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	if items == nil {
		items = []StoredReport{}
	}
	pages := (total + perPage - 1) / perPage
	ok(w, listResponse{Items: items, Total: total, Page: page, PerPage: perPage, TotalPages: pages})
}

func (p *ReportingPlugin) handleGet(w http.ResponseWriter, r *http.Request) {
	rec, err := p.svc.store.get(r.Context(), chi.URLParam(r, "id"))
	switch {
	case errors.Is(err, errNotFound):
		fail(w, http.StatusNotFound, codeNotFound)
		return
	case err != nil:
		p.logErr("reporting: get report failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	ok(w, rec)
}

func (p *ReportingPlugin) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	removed, err := p.svc.store.delete(r.Context(), id)
	if err != nil {
		p.logErr("reporting: delete report failed", err, "id", id)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	if !removed {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}
	p.auditAs(requestUserID(r), "plugin.reporting.report.deleted", map[string]any{"report_id": id})
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Code: msgReportDeleted})
}

// --- generation ------------------------------------------------------

type generateRequest struct {
	Type          string   `json:"type"`
	EnvironmentID string   `json:"environment_id"`
	Environment   string   `json:"environment"`
	Levels        []string `json:"levels"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Days          int      `json:"days"`
	Options       []string `json:"options"`
	// The scope, when the report is narrowed to one object. An unknown or
	// unsupported kind narrows nothing rather than failing the request:
	// the whole-estate report is always a valid answer.
	SubjectKind string `json:"subject_kind"`
	SubjectID   string `json:"subject_id"`
	SubjectName string `json:"subject_name"`
	// Prompt is the user's question, used only by the ad-hoc type.
	Prompt string `json:"prompt"`
	// ReplayOf, ad-hoc only, names a stored report whose pinned plan
	// should be replayed instead of re-planning. A scheduled ad-hoc
	// report uses this so consecutive periods stay comparable rather
	// than possibly choosing different sources each run.
	ReplayOf string `json:"replay_of"`
	// AIConnectionID optionally pins generation to one configured AI
	// connection (GET /api/reporting/ai/connections lists the choices).
	// Empty means the host's default/favourite connection. Ignored by a
	// replay: a replayed ad-hoc report reuses whatever connection its
	// pinned plan recorded, not this field — see AdHocPlan.AIConnectionID.
	AIConnectionID string `json:"ai_connection_id"`
}

func (p *ReportingPlugin) handleGenerate(w http.ResponseWriter, r *http.Request) {
	p.generate(w, r, true)
}

func (p *ReportingPlugin) handlePreview(w http.ResponseWriter, r *http.Request) {
	p.generate(w, r, false)
}

// parseGenerate turns a request into a validated GenerateInput.
//
// It writes the failure response itself and reports false, so both the
// synchronous endpoint and the job endpoint get the same validation and
// the same error codes without repeating either.
func (p *ReportingPlugin) parseGenerate(w http.ResponseWriter, r *http.Request, persist bool) (ReportType, GenerateInput, bool) {
	var req generateRequest
	if r.Method == http.MethodPost {
		// A report request is small; anything large is a mistake or an
		// attack, and reading it costs nothing to refuse.
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			failCause(w, r, http.StatusBadRequest, codeBadRequest, err)
			return ReportType{}, GenerateInput{}, false
		}
	} else {
		q := r.URL.Query()
		req = generateRequest{
			Type: q.Get("type"), EnvironmentID: q.Get("env"), Environment: q.Get("environment"),
			From: q.Get("from"), To: q.Get("to"),
			SubjectKind: q.Get("subject_kind"), SubjectID: q.Get("subject_id"),
			SubjectName:    q.Get("subject_name"),
			Prompt:         q.Get("prompt"),
			ReplayOf:       q.Get("replay_of"),
			AIConnectionID: q.Get("ai_connection_id"),
		}
		if v, err := strconv.Atoi(q.Get("days")); err == nil {
			req.Days = v
		}
		if v := q.Get("levels"); v != "" {
			req.Levels = strings.Split(v, ",")
		}
	}

	rt, found := reportTypeByKey(strings.TrimSpace(req.Type))
	if !found {
		fail(w, http.StatusBadRequest, codeUnknownType)
		return ReportType{}, GenerateInput{}, false
	}

	prompt := strings.TrimSpace(req.Prompt)
	var pinnedPlan *AdHocPlan
	if rt.Key == TypeAdHoc {
		if replayOf := strings.TrimSpace(req.ReplayOf); replayOf != "" {
			// A replay id naming a report that is missing, not ad-hoc, or
			// has no pinned plan is not a request error: the request
			// simply falls through to a fresh pass-1 plan below, the same
			// as if replay_of had never been sent.
			stored, err := p.svc.store.get(r.Context(), replayOf)
			switch {
			case errors.Is(err, errNotFound):
				// fall through to fresh planning
			case err != nil:
				p.logErr("reporting: load report for ad-hoc replay failed", err, "replay_of", replayOf)
			case stored.ReportType == TypeAdHoc && len(stored.Plan) > 0:
				var plan AdHocPlan
				if err := json.Unmarshal(stored.Plan, &plan); err != nil {
					p.logErr("reporting: decode pinned ad-hoc plan failed", err, "replay_of", replayOf)
				} else {
					pinnedPlan = &plan
					if prompt == "" {
						// A scheduled regenerate may carry no prompt of its
						// own; the original question is what pass 2 needs
						// to write about, pinned plan or not.
						prompt = strings.TrimSpace(stored.Prompt)
					}
				}
			}
		}
		if prompt == "" {
			fail(w, http.StatusBadRequest, codePromptRequired)
			return ReportType{}, GenerateInput{}, false
		}
		// A question is a sentence, not a document. Anything longer is a
		// paste, and it would dominate the planner's context.
		if len(prompt) > 2000 {
			prompt = prompt[:2000]
		}
	}

	settings := p.svc.store.settings(r.Context(), p.host.DB.KV())
	from, to := resolvePeriod(req, rt, settings)

	return rt, GenerateInput{
		ReportType:     rt.Key,
		EnvironmentID:  strings.TrimSpace(req.EnvironmentID),
		Environment:    strings.TrimSpace(req.Environment),
		PeriodStart:    from,
		PeriodEnd:      to,
		Levels:         parseRequestLevels(req.Levels),
		Options:        resolveOptions(rt, req.Options),
		Prompt:         prompt,
		Plan:           pinnedPlan,
		Subject:        resolveSubject(rt, req),
		Persist:        persist,
		Cookie:         r.Header.Get("Cookie"),
		Authorization:  r.Header.Get("Authorization"),
		Lang:           r.Header.Get("Accept-Language"),
		User:           requestUser(r),
		UserID:         requestUserID(r),
		AIConnectionID: strings.TrimSpace(req.AIConnectionID),
	}, true
}

func (p *ReportingPlugin) generate(w http.ResponseWriter, r *http.Request, persist bool) {
	rt, in, okReq := p.parseGenerate(w, r, persist)
	if !okReq {
		return
	}

	rec, err := p.svc.Generate(r.Context(), in)
	if err != nil {
		switch {
		case errors.Is(err, errAIReportsOff):
			fail(w, http.StatusBadRequest, codeAIDisabled)
			return
		case errors.Is(err, errAIUnavailable):
			fail(w, http.StatusBadRequest, codeAIUnavailable)
			return
		case errors.Is(err, errAIConnectionUnavailable):
			fail(w, http.StatusBadRequest, codeAIConnectionUnavailable)
			return
		case errors.Is(err, errPlanFailed):
			failCause(w, r, http.StatusBadRequest, codeAIPlanFailed, err)
			return
		case errors.Is(err, errAuthorFailed):
			failCause(w, r, http.StatusBadRequest, codeAIAuthorFailed, err)
			return
		}
		p.logErr("reporting: generate failed", err, "type", rt.Key)
		failCause(w, r, http.StatusInternalServerError, codeGenerate, err)
		return
	}
	if persist {
		p.auditGenerated(requestUserID(r), rec)
	}
	ok(w, rec)
}

func (p *ReportingPlugin) auditGenerated(userID string, rec StoredReport) {
	p.auditAs(userID, "plugin.reporting.report.generated", map[string]any{
		"report_id":   rec.ID,
		"report_type": rec.ReportType,
		"environment": rec.Environment,
		"levels":      levelsCSV(rec.Levels),
		"score":       rec.OverallScore,
		// Empty when no AI actually wrote or polished this report (see
		// service.reportedAIConnectionID) — never a connection that was
		// merely offered. Recording it in the audit trail is what makes
		// a substitution (a replayed pin whose connection had gone
		// stale, see AdHocPlan.validate) visible without diffing rows.
		"ai_connection_id": rec.AIConnectionID,
	})
}

// resolveSubject accepts a scope only when the type offers that kind, so
// a stale studio tab asking for a scope a report cannot honour produces
// the estate-wide report rather than an empty one.
func resolveSubject(rt ReportType, req generateRequest) Subject {
	kind, known := parseSubjectKind(req.SubjectKind)
	if !known {
		// A plugin-contributed type narrows to the contributing
		// plugin's own object kinds, which this plugin deliberately
		// does not know: the provider does the narrowing. The catalog
		// entry is still the gate — supportsSubject below rejects any
		// kind the type did not declare — so an unknown kind is only
		// accepted when the type itself asked for it.
		kind = SubjectKind(strings.TrimSpace(strings.ToLower(req.SubjectKind)))
	}
	if kind == "" || !rt.supportsSubject(kind) {
		return Subject{}
	}
	sub := Subject{
		Kind: kind,
		ID:   strings.TrimSpace(req.SubjectID),
		Name: strings.TrimSpace(req.SubjectName),
	}
	if sub.empty() {
		return Subject{}
	}
	return sub
}

// --- scope picker ----------------------------------------------------

type subjectsResponse struct {
	Kind  SubjectKind     `json:"kind"`
	Items []SubjectOption `json:"items"`
}

// handleSubjects lists the objects the studio can narrow a report to.
//
// The listing runs as the calling user over loopback, so it can only
// ever offer objects that user could already open in the interface.
func (p *ReportingPlugin) handleSubjects(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("kind")))
	cookie, lang := r.Header.Get("Cookie"), r.Header.Get("Accept-Language")
	env := strings.TrimSpace(r.URL.Query().Get("env"))

	// A plugin-contributed report narrows to the contributing plugin's
	// own objects — a cluster, a namespace — which this plugin cannot
	// list and must not try to. The provider answers instead, through
	// the same handler and the same response shape, so the studio's
	// scope picker needs no idea which half produced the rows.
	if items, handled := listProviderSubjects(r.Context(), raw, cookie, lang, env, requestUserID(r)); handled {
		ok(w, subjectsResponse{Kind: SubjectKind(raw), Items: items})
		return
	}

	kind, known := parseSubjectKind(raw)
	if !known {
		fail(w, http.StatusBadRequest, codeBadRequest)
		return
	}
	api := newHostAPI(cookie, lang, env).withAuthorization(r.Header.Get("Authorization"))
	ok(w, subjectsResponse{Kind: kind, Items: listSubjects(r.Context(), api, kind)})
}

// --- generation jobs -------------------------------------------------

// handleJobStart begins a generation and answers immediately with the
// job's first status, so the studio can swap its configure dialog for a
// progress dialog without holding a request open for a minute.
func (p *ReportingPlugin) handleJobStart(w http.ResponseWriter, r *http.Request) {
	rt, in, okReq := p.parseGenerate(w, r, true)
	if !okReq {
		return
	}

	// Read before the closure: the job outlives this request, and reaching
	// for r from inside it would be a use after the handler returned.
	actor := requestUserID(r)

	st := p.jobs.start(rt.Key, func(ctx context.Context, prog *progress) (string, string) {
		in.Progress = prog
		rec, err := p.svc.Generate(ctx, in)
		if err != nil {
			switch {
			case errors.Is(err, errAIReportsOff):
				return "", codeAIDisabled
			case errors.Is(err, errAIUnavailable):
				return "", codeAIUnavailable
			case errors.Is(err, errAIConnectionUnavailable):
				return "", codeAIConnectionUnavailable
			case errors.Is(err, errPlanFailed):
				return "", codeAIPlanFailed
			case errors.Is(err, errAuthorFailed):
				return "", codeAIAuthorFailed
			}
			p.logErr("reporting: generate failed", err, "type", rt.Key)
			return "", codeGenerate
		}
		p.auditGenerated(actor, rec)
		return rec.ID, ""
	})
	writeJSON(w, http.StatusAccepted, apiEnvelope{Success: true, Data: st})
}

func (p *ReportingPlugin) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	st, found := p.jobs.get(chi.URLParam(r, "id"))
	if !found {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}
	ok(w, st)
}

// resolvePeriod works out the window, preferring an explicit range, then
// a day count, then the type's own default.
func resolvePeriod(req generateRequest, rt ReportType, s Settings) (time.Time, time.Time) {
	now := time.Now()
	if rt.PointInTime {
		return now, now
	}
	from, to := parseTS(req.From), parseTS(req.To)
	if !from.IsZero() && !to.IsZero() && to.After(from) {
		// A window longer than a year is almost always a mistyped year;
		// clamping is kinder than collecting a million audit rows.
		if to.Sub(from) > 366*24*time.Hour {
			from = to.AddDate(0, 0, -366)
		}
		return from, to
	}
	days := req.Days
	if days <= 0 {
		days = rt.DefaultDays
	}
	if days <= 0 {
		days = s.DefaultDays
	}
	if days < 1 {
		days = 1
	}
	if days > 366 {
		days = 366
	}
	return now.AddDate(0, 0, -days), now
}

func parseRequestLevels(raw []string) []Level {
	var out []Level
	for _, s := range raw {
		switch Level(strings.TrimSpace(strings.ToLower(s))) {
		case Strategic:
			out = append(out, Strategic)
		case Tactical:
			out = append(out, Tactical)
		case Operational:
			out = append(out, Operational)
		}
	}
	return out
}

// resolveOptions starts from the type's declared defaults and applies
// the caller's list, so an omitted option keeps its default rather than
// silently switching off.
func resolveOptions(rt ReportType, requested []string) map[string]bool {
	out := map[string]bool{}
	for _, o := range rt.Options {
		out[o.Key] = o.Default
	}
	if requested == nil {
		return out
	}
	for k := range out {
		out[k] = false
	}
	for _, k := range requested {
		k = strings.TrimSpace(k)
		if _, known := out[k]; known {
			out[k] = true
		}
	}
	return out
}

// --- export ----------------------------------------------------------

func (p *ReportingPlugin) handleExport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "pdf"
	}
	if format != "pdf" && format != "docx" {
		fail(w, http.StatusBadRequest, codeBadFormat)
		return
	}
	data, filename, contentType, err := p.svc.Export(r.Context(), id, format)
	switch {
	case errors.Is(err, errNotFound):
		fail(w, http.StatusNotFound, codeNotFound)
		return
	case err != nil:
		p.logErr("reporting: export failed", err, "id", id, "format", format)
		failCause(w, r, http.StatusInternalServerError, codeExport, err)
		return
	}

	p.auditAs(requestUserID(r), "plugin.reporting.report.exported", map[string]any{
		"report_id": id, "format": format, "bytes": len(data),
	})

	w.Header().Set("Content-Type", contentType)
	// The filename is built from a slug and a date, so it needs no
	// quoting games; it is still quoted so a space could never split it.
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// --- AI connections ----------------------------------------------------

// aiConnectionResponse mirrors plugin.AIConnection field-for-field — id,
// name, provider_type, is_default — and adds nothing else. No secret,
// no credential, no base URL: the SDK type already excludes them, and
// this response carries only what it carries.
type aiConnectionResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	IsDefault    bool   `json:"is_default"`
}

type aiConnectionsResponse struct {
	// Available mirrors host.AI.Available(), so the studio can tell "no
	// connections configured" (Available false, Connections empty) apart
	// from "AI works" without inferring it from an empty list alone —
	// Available can be false even with connections listed, if no
	// provider plugin has registered an adapter for any of them.
	Available   bool                   `json:"available"`
	Connections []aiConnectionResponse `json:"connections"`
}

// handleAIConnections lists the AI connections the studio may offer in
// a "which AI should write this report" selector. Read-only: it never
// creates, edits or deletes a connection — that stays in AI settings.
func (p *ReportingPlugin) handleAIConnections(w http.ResponseWriter, r *http.Request) {
	resp := aiConnectionsResponse{Connections: []aiConnectionResponse{}}
	if p.host.AI != nil {
		resp.Available = p.host.AI.Available()
		for _, c := range p.host.AI.Connections() {
			resp.Connections = append(resp.Connections, aiConnectionResponse{
				ID: c.ID, Name: c.Name, ProviderType: c.ProviderType, IsDefault: c.IsDefault,
			})
		}
	}
	ok(w, resp)
}

// --- settings --------------------------------------------------------

func (p *ReportingPlugin) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	ok(w, p.svc.store.settings(r.Context(), p.host.DB.KV()))
}

func (p *ReportingPlugin) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	// Two base64 marks ride in this body. At the per-mark ceiling the store
	// enforces (2 MiB decoded) that is ~5.4 MiB of base64 before anything
	// else, so the cap has to clear it — the old 16 KiB predated the marks
	// and rejected every upload.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&in); err != nil {
		failCause(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	saved, err := p.svc.store.saveSettings(r.Context(), p.host.DB.KV(), in)
	if err != nil {
		p.logErr("reporting: save settings failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	p.auditAs(requestUserID(r), "plugin.reporting.settings.updated", map[string]any{
		"organisation": saved.Organisation, "classification": saved.Classification,
		"retain_reports": saved.RetainReports, "ai_narrative": saved.AINarrative,
		// ai_reports is the one setting that authorises estate data
		// leaving the box to an external AI provider — it belongs in the
		// audit trail at least as much as ai_narrative does.
		"ai_reports": saved.AIReports,
	})
	ok(w, saved)
}

// --- request helpers -------------------------------------------------

// parsePagination mirrors the host's convention: default 25, max 100.
func parsePagination(r *http.Request) (page, perPage int) {
	page, perPage = 1, 25
	q := r.URL.Query()
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(q.Get("per_page")); err == nil && v > 0 {
		perPage = v
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

// requestUserID reads the authenticated account's id.
//
// Separate from requestUser because the two are for genuinely different
// things: the username goes on the cover for attribution, the id goes to
// a report provider so it can re-check its own permission key. Never
// rendered, never stored on the report.
func requestUserID(r *http.Request) string {
	if u := auth.RequireAuth(r); u != nil {
		return strings.TrimSpace(u.ID)
	}
	return ""
}

// requestUser reads the authenticated user the host's middleware put on
// the request. The plugin never authenticates anyone itself — by the
// time a handler runs, the host has already enforced the session and the
// permission declared in the manifest. This is only for attribution on
// the report's cover.
func requestUser(r *http.Request) string {
	if u := auth.RequireAuth(r); u != nil {
		if name := strings.TrimSpace(u.Username); name != "" {
			return name
		}
	}
	return ""
}
