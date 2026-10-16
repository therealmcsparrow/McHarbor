// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// Schedule routes.
//
// Same discipline as the rest of this plugin's HTTP layer: parse,
// validate, call the service, audit, respond. Every route is gated on
// `reporting.schedule` by the host from the manifest, before a request
// reaches anything here.

// scheduleRequest is the wire shape for create and update.
//
// It mirrors generateRequest field for field where the two overlap, so
// the studio's configure dialog can build one from the other without a
// translation layer that could drift.
type scheduleRequest struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	EnvironmentID string   `json:"environment_id"`
	Environment   string   `json:"environment"`
	Levels        []string `json:"levels"`
	Days          int      `json:"days"`
	Options       []string `json:"options"`
	SubjectKind   string   `json:"subject_kind"`
	SubjectID     string   `json:"subject_id"`
	SubjectName   string   `json:"subject_name"`
	Prompt        string   `json:"prompt"`

	AIConnectionID string `json:"ai_connection_id"`
	Format         string `json:"format"`
	Cron           string `json:"cron"`
	Enabled        *bool  `json:"enabled"`

	Targets []scheduleTargetRequest `json:"targets"`
}

type scheduleTargetRequest struct {
	Kind  string `json:"kind"`
	RefID string `json:"ref_id"`
	// Path is the destination folder for a storage target, relative to the
	// location's own base path. Ignored for a comm target.
	Path string `json:"path"`
	// Filename is the storage target's name pattern, before the format's
	// extension. Ignored for a comm target.
	Filename string `json:"filename"`
	// Recipient is the comm target's alternative destination. Ignored for
	// a storage target, and refused for a channel whose adapter declared
	// no overridable destination.
	Recipient string `json:"recipient"`
}

type scheduleListResponse struct {
	Items      []Schedule `json:"items"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PerPage    int        `json:"per_page"`
	TotalPages int        `json:"total_pages"`
}

// --- list / read ------------------------------------------------------

func (p *ReportingPlugin) handleScheduleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)

	total, err := p.svc.store.countSchedules(r.Context())
	if err != nil {
		p.logErr("reporting: count schedules failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	items, err := p.svc.store.listSchedules(r.Context(), perPage, (page-1)*perPage)
	if err != nil {
		p.logErr("reporting: list schedules failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	for i := range items {
		p.hydrateSchedule(r, &items[i], false)
	}
	pages := (total + perPage - 1) / perPage
	ok(w, scheduleListResponse{
		Items: items, Total: total, Page: page, PerPage: perPage, TotalPages: pages,
	})
}

func (p *ReportingPlugin) handleScheduleGet(w http.ResponseWriter, r *http.Request) {
	sc, found := p.loadSchedule(w, r)
	if !found {
		return
	}
	p.hydrateSchedule(r, &sc, false)
	ok(w, sc)
}

// hydrateSchedule attaches the targets and the last run.
//
// Target names and file-attachment capability come from the host at read
// time rather than from the row: a channel renamed in settings must not
// keep showing its old name, and an adapter that gains file support in a
// rebuilt plugin must stop being labelled "link only" without anyone
// re-saving a schedule. A target whose host row has gone is marked
// missing rather than dropped — a schedule quietly delivering to fewer
// places than it lists is the failure mode this avoids.
func (p *ReportingPlugin) hydrateSchedule(r *http.Request, sc *Schedule, withDocument bool) {
	targets, err := p.svc.store.scheduleTargets(r.Context(), sc.ID)
	if err != nil {
		p.logErr("reporting: read schedule targets failed", err, "schedule_id", sc.ID)
		targets = []ScheduleTarget{}
	}
	channels, locations := p.deliveryIndex()
	for i := range targets {
		switch targets[i].Kind {
		case TargetComm:
			if ch, known := channels[targets[i].RefID]; known {
				targets[i].Name = ch.Name
				targets[i].SupportsFiles = ch.SupportsFileAttachments
				targets[i].SupportsRecipient = ch.SupportsRecipient
			} else {
				targets[i].Missing = true
			}
		case TargetStorage:
			if name, known := locations[targets[i].RefID]; known {
				targets[i].Name = name
			} else {
				targets[i].Missing = true
			}
		}
	}
	sc.Targets = targets

	run, err := p.svc.store.lastRun(r.Context(), sc.ID, withDocument)
	if err != nil {
		p.logErr("reporting: read last schedule run failed", err, "schedule_id", sc.ID)
		return
	}
	sc.LastRun = run
}

// --- create / update / delete -----------------------------------------

func (p *ReportingPlugin) handleScheduleCreate(w http.ResponseWriter, r *http.Request) {
	sc, okReq := p.parseSchedule(w, r, Schedule{ID: newID(), Enabled: true}, true)
	if !okReq {
		return
	}
	sc.CreatedByID, sc.CreatedBy = requestUserIdentity(r)

	if err := p.svc.store.insertSchedule(r.Context(), sc); err != nil {
		p.logErr("reporting: insert schedule failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	if err := p.svc.store.replaceTargets(r.Context(), sc.ID, sc.Targets); err != nil {
		p.logErr("reporting: save schedule targets failed", err, "schedule_id", sc.ID)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	p.auditAs(requestUserID(r), "plugin.reporting.schedule.created", map[string]any{
		"schedule_id": sc.ID, "name": sc.Name, "report_type": sc.ReportType,
		"cron": sc.Cron, "targets": len(sc.Targets), "enabled": sc.Enabled,
	})
	p.hydrateSchedule(r, &sc, false)
	writeJSON(w, http.StatusCreated, apiEnvelope{Success: true, Data: sc})
}

func (p *ReportingPlugin) handleScheduleUpdate(w http.ResponseWriter, r *http.Request) {
	existing, found := p.loadSchedule(w, r)
	if !found {
		return
	}
	// Authorship is not editable: the run collects as the schedule's
	// author, so letting an edit change it would let one user borrow
	// another's visibility.
	sc, okReq := p.parseSchedule(w, r, existing, false)
	if !okReq {
		return
	}

	updated, err := p.svc.store.updateSchedule(r.Context(), sc)
	if err != nil {
		p.logErr("reporting: update schedule failed", err, "schedule_id", sc.ID)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	if !updated {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}
	if err := p.svc.store.replaceTargets(r.Context(), sc.ID, sc.Targets); err != nil {
		p.logErr("reporting: save schedule targets failed", err, "schedule_id", sc.ID)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	p.auditAs(requestUserID(r), "plugin.reporting.schedule.updated", map[string]any{
		"schedule_id": sc.ID, "name": sc.Name, "report_type": sc.ReportType,
		"cron": sc.Cron, "targets": len(sc.Targets), "enabled": sc.Enabled,
	})
	p.hydrateSchedule(r, &sc, false)
	ok(w, sc)
}

func (p *ReportingPlugin) handleScheduleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	removed, err := p.svc.store.deleteSchedule(r.Context(), id)
	if err != nil {
		p.logErr("reporting: delete schedule failed", err, "schedule_id", id)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	if !removed {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}
	p.auditAs(requestUserID(r), "plugin.reporting.schedule.deleted", map[string]any{"schedule_id": id})
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Code: msgScheduleDeleted})
}

// parseSchedule validates a create or update body against `base`, which
// carries the fields an edit may not change (id, authorship).
//
// It writes the failure response itself and reports false, so create and
// update share one set of rules and one set of error codes. `isNew`
// decides only what an omitted `enabled` means: a new schedule is on, an
// edited one keeps whatever it had.
func (p *ReportingPlugin) parseSchedule(
	w http.ResponseWriter, r *http.Request, base Schedule, isNew bool,
) (Schedule, bool) {
	var req scheduleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		failCause(w, r, http.StatusBadRequest, codeBadRequest, err)
		return Schedule{}, false
	}

	rt, known := reportTypeByKey(strings.TrimSpace(req.Type))
	if !known {
		fail(w, http.StatusBadRequest, codeUnknownType)
		return Schedule{}, false
	}

	// The ≥1-target rule lives here rather than only in the form: a
	// schedule that delivers nowhere produces a document nobody reads and
	// is indistinguishable from a schedule that is quietly broken.
	channels, _ := p.deliveryIndex()
	capable := make(map[string]bool, len(channels))
	for id, ch := range channels {
		capable[id] = ch.SupportsRecipient
	}
	targets, targetErr := parseScheduleTargets(req.Targets, capable)
	if targetErr != nil {
		// A bad path and a bad destination are different operator
		// mistakes, and telling them apart is the difference between
		// "fix the folder you typed" and "that is not a destination".
		code := codeBadTarget
		switch {
		case errors.Is(targetErr, errBadPath):
			code = codeBadPath
		case errors.Is(targetErr, errBadFilename):
			code = codeBadFilename
		case errors.Is(targetErr, errBadRecipient):
			code = codeBadRecipient
		}
		failCause(w, r, http.StatusBadRequest, code, targetErr)
		return Schedule{}, false
	}
	if len(targets) == 0 {
		fail(w, http.StatusBadRequest, codeNoTargets)
		return Schedule{}, false
	}

	cron := strings.TrimSpace(req.Cron)
	next, err := nextCronRun(cron, time.Now())
	if err != nil {
		failCause(w, r, http.StatusBadRequest, codeBadCron, err)
		return Schedule{}, false
	}

	prompt := strings.TrimSpace(req.Prompt)
	if rt.Key == TypeAdHoc {
		if prompt == "" {
			fail(w, http.StatusBadRequest, codePromptRequired)
			return Schedule{}, false
		}
		if len(prompt) > 2000 {
			prompt = prompt[:2000]
		}
	} else {
		// A prompt on a fixed type would be stored and never read. Drop it
		// rather than keep a field that lies about what the run does.
		prompt = ""
	}

	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format != "docx" {
		format = "pdf"
	}

	enabled := base.Enabled
	if isNew {
		enabled = true
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	days := req.Days
	if days <= 0 {
		days = rt.DefaultDays
	}
	if days <= 0 {
		days = 30
	}
	days = clampInt(days, 1, 366)

	sc := base
	sc.Name = clampText(req.Name, 80, defaultScheduleName(rt))
	sc.ReportType = rt.Key
	sc.EnvironmentID = ""
	sc.Environment = ""
	if rt.EnvironmentScoped {
		sc.EnvironmentID = strings.TrimSpace(req.EnvironmentID)
		sc.Environment = clampText(req.Environment, 80, "")
	}
	sc.PeriodDays = days
	sc.Levels = rt.resolveLevels(parseRequestLevels(req.Levels))
	sc.Options = normaliseOptions(rt, req.Options)
	sc.Format = format
	sc.Prompt = prompt
	sc.AIConnectionID = strings.TrimSpace(req.AIConnectionID)
	sc.Cron = cron
	sc.Enabled = enabled
	sc.NextRunAt = next
	sc.Targets = targets

	subject := resolveSubject(rt, generateRequest{
		SubjectKind: req.SubjectKind, SubjectID: req.SubjectID, SubjectName: req.SubjectName,
	})
	sc.SubjectKind, sc.SubjectID, sc.SubjectName = subject.Kind, subject.ID, subject.Name

	// A disabled schedule has no next run: leaving one would make the
	// listing say a switched-off schedule fires on Tuesday.
	if !sc.Enabled {
		sc.NextRunAt = time.Time{}
	}
	return sc, true
}

// normaliseOptions keeps only the keys the type actually declares, so an
// option removed from the catalog cannot linger in a saved schedule and
// silently do nothing forever.
func normaliseOptions(rt ReportType, requested []string) []string {
	resolved := resolveOptions(rt, requested)
	out := []string{}
	for _, o := range rt.Options {
		if resolved[o.Key] {
			out = append(out, o.Key)
		}
	}
	return out
}

// parseScheduleTargets validates the submitted target set. capable maps
// a channel id to whether its adapter accepts an alternative recipient,
// so an override can be refused at the edge rather than at 3am.
func parseScheduleTargets(raw []scheduleTargetRequest, capable map[string]bool) ([]ScheduleTarget, error) {
	// Deduplicated: the same channel picked twice would send the same
	// report twice and count as two successes.
	seen := map[string]bool{}
	out := []ScheduleTarget{}
	for _, t := range raw {
		kind := ScheduleTargetKind(strings.ToLower(strings.TrimSpace(t.Kind)))
		if kind != TargetComm && kind != TargetStorage {
			return nil, errBadTarget
		}
		ref := strings.TrimSpace(t.RefID)
		if ref == "" {
			return nil, errBadTarget
		}
		key := string(kind) + ":" + ref
		if seen[key] {
			continue
		}
		seen[key] = true

		target := ScheduleTarget{Kind: kind, RefID: ref}
		switch kind {
		case TargetStorage:
			// Normalises typing, refuses traversal. Empty stays empty and
			// means "the plugin's default folder". A path on a channel is
			// dropped rather than kept as a field that lies about the run.
			path, err := sanitiseStoragePath(t.Path)
			if err != nil {
				return nil, err
			}
			target.Path = path
			name, err := sanitiseFilenamePattern(t.Filename)
			if err != nil {
				return nil, err
			}
			target.Filename = name
		case TargetComm:
			recipient, err := sanitiseRecipient(t.Recipient)
			if err != nil {
				return nil, err
			}
			// A recipient the channel cannot honour is refused rather
			// than stored and ignored. Storing it would leave an
			// operator looking at a filled-in field and a report that
			// went somewhere else entirely.
			if recipient != "" && !capable[ref] {
				return nil, errBadRecipient
			}
			target.Recipient = recipient
		}
		out = append(out, target)
		if len(out) >= 20 {
			break
		}
	}
	return out, nil
}

func defaultScheduleName(rt ReportType) string {
	return reportTitle(rt)
}

// --- run now / last run -----------------------------------------------

func (p *ReportingPlugin) handleScheduleRunNow(w http.ResponseWriter, r *http.Request) {
	sc, found := p.loadSchedule(w, r)
	if !found {
		return
	}
	if p.sched == nil || !p.sched.runNow(sc) {
		// Already running. Not an error the operator needs to fix — the
		// run they asked for is happening — so it is a conflict, not a
		// failure.
		fail(w, http.StatusConflict, codeScheduleRunning)
		return
	}
	p.auditAs(requestUserID(r), "plugin.reporting.schedule.run_requested", map[string]any{
		"schedule_id": sc.ID, "name": sc.Name,
	})
	writeJSON(w, http.StatusAccepted, apiEnvelope{Success: true, Code: msgScheduleRunStarted})
}

func (p *ReportingPlugin) handleScheduleRun(w http.ResponseWriter, r *http.Request) {
	sc, found := p.loadSchedule(w, r)
	if !found {
		return
	}
	run, err := p.svc.store.lastRun(r.Context(), sc.ID, true)
	if err != nil {
		p.logErr("reporting: read schedule run failed", err, "schedule_id", sc.ID)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	if run == nil {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}
	ok(w, run)
}

// handleScheduleRunExport renders the last run to a file.
//
// It re-renders from the stored document rather than keeping the bytes
// that were delivered: the document is the record, and a settings change
// (paper size, organisation, mark) should apply to a download taken
// afterwards the same way it does everywhere else.
func (p *ReportingPlugin) handleScheduleRunExport(w http.ResponseWriter, r *http.Request) {
	sc, found := p.loadSchedule(w, r)
	if !found {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = sc.Format
	}
	if format != "pdf" && format != "docx" {
		fail(w, http.StatusBadRequest, codeBadFormat)
		return
	}

	run, err := p.svc.store.lastRun(r.Context(), sc.ID, true)
	if err != nil {
		p.logErr("reporting: read schedule run failed", err, "schedule_id", sc.ID)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return
	}
	if run == nil || run.Document == nil {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}

	rec := StoredReport{
		ID: sc.ID, ReportType: sc.ReportType, Title: run.Title,
		EnvironmentID: sc.EnvironmentID, Environment: sc.Environment,
		PeriodStart: run.Document.Meta.PeriodStart, PeriodEnd: run.Document.Meta.PeriodEnd,
		Document: run.Document,
	}
	data, filename, contentType, err := p.svc.render(r.Context(), rec, format,
		p.svc.store.settings(r.Context(), p.host.DB.KV()))
	if err != nil {
		p.logErr("reporting: export schedule run failed", err, "schedule_id", sc.ID)
		failCause(w, r, http.StatusInternalServerError, codeExport, err)
		return
	}

	p.auditAs(requestUserID(r), "plugin.reporting.schedule.exported", map[string]any{
		"schedule_id": sc.ID, "format": format, "bytes": len(data),
	})
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// --- delivery targets -------------------------------------------------

type deliveryTarget struct {
	Kind ScheduleTargetKind `json:"kind"`
	ID   string             `json:"id"`
	Name string             `json:"name"`
	// Type is the channel type or location type, so the picker can show a
	// tile's provider alongside its name.
	Type string `json:"type"`
	// SupportsFiles is false for a channel whose adapter cannot transmit
	// a file. The picker shows it as "link only" rather than hiding it:
	// the operator should see the choice and why it is limited.
	SupportsFiles bool `json:"supports_files"`

	// SupportsRecipient is true for a channel whose adapter declared an
	// overridable destination - a mail `to`, a Telegram chat. False for
	// a webhook-bound channel where the URL is both the destination and
	// the secret, and for every storage location, which is retargeted
	// with a folder instead.
	SupportsRecipient bool `json:"supports_recipient"`
	// RecipientKind is "email", "phone", "chat_id" or "topic", so the
	// picker can validate and label in the provider's own vocabulary
	// rather than asking for "a recipient" and hoping.
	RecipientKind string `json:"recipient_kind,omitempty"`
	// RecipientLabel and RecipientPlaceholder are the adapter's own
	// English source strings, used only where this plugin's bundle has
	// nothing more specific to say.
	RecipientLabel       string `json:"recipient_label,omitempty"`
	RecipientPlaceholder string `json:"recipient_placeholder,omitempty"`
	// RecipientMultiple reports whether a comma-separated list is
	// accepted.
	RecipientMultiple bool `json:"recipient_multiple,omitempty"`
}

type deliveryTargetsResponse struct {
	Channels  []deliveryTarget `json:"channels"`
	Locations []deliveryTarget `json:"locations"`
}

// handleDeliveryTargets lists what a schedule may deliver to: id, name
// and type only. No credential, no endpoint, no bucket path — the host's
// handles are built precisely so a plugin never sees one.
func (p *ReportingPlugin) handleDeliveryTargets(w http.ResponseWriter, r *http.Request) {
	resp := deliveryTargetsResponse{Channels: []deliveryTarget{}, Locations: []deliveryTarget{}}
	if p.host.Comms != nil {
		for _, c := range p.host.Comms.Channels() {
			resp.Channels = append(resp.Channels, deliveryTarget{
				Kind: TargetComm, ID: c.ID, Name: c.Name, Type: c.ChannelType,
				SupportsFiles:        c.SupportsFileAttachments,
				SupportsRecipient:    c.Recipient.Supported,
				RecipientKind:        c.Recipient.Kind,
				RecipientLabel:       c.Recipient.Label,
				RecipientPlaceholder: c.Recipient.Placeholder,
				RecipientMultiple:    c.Recipient.Multiple,
			})
		}
	}
	if p.host.Storage != nil {
		for _, l := range p.host.Storage.Locations() {
			resp.Locations = append(resp.Locations, deliveryTarget{
				Kind: TargetStorage, ID: l.ID, Name: l.Name, Type: l.LocationType,
				// A storage location always receives the document itself.
				SupportsFiles: true,
			})
		}
	}
	ok(w, resp)
}

func (p *ReportingPlugin) deliveryIndex() (map[string]commChannelInfo, map[string]string) {
	channels := map[string]commChannelInfo{}
	if p.host.Comms != nil {
		for _, c := range p.host.Comms.Channels() {
			channels[c.ID] = commChannelInfo{
				Name:                    c.Name,
				SupportsFileAttachments: c.SupportsFileAttachments,
				SupportsRecipient:       c.Recipient.Supported,
				RecipientKind:           c.Recipient.Kind,
				RecipientLabel:          c.Recipient.Label,
				RecipientPlaceholder:    c.Recipient.Placeholder,
				RecipientMultiple:       c.Recipient.Multiple,
			}
		}
	}
	locations := map[string]string{}
	if p.host.Storage != nil {
		for _, l := range p.host.Storage.Locations() {
			locations[l.ID] = l.Name
		}
	}
	return channels, locations
}

// commChannelInfo is the handful of fields hydrateSchedule and the
// target picker need, kept local so the handler does not carry the
// SDK's whole channel ref around.
type commChannelInfo struct {
	Name                    string
	SupportsFileAttachments bool

	// The adapter's declared alternative destination, or the zero
	// value for a webhook- or token-bound channel that has none.
	SupportsRecipient    bool
	RecipientKind        string
	RecipientLabel       string
	RecipientPlaceholder string
	RecipientMultiple    bool
}

// --- shared -----------------------------------------------------------

func (p *ReportingPlugin) loadSchedule(w http.ResponseWriter, r *http.Request) (Schedule, bool) {
	sc, err := p.svc.store.getSchedule(r.Context(), chi.URLParam(r, "id"))
	switch {
	case errors.Is(err, errScheduleNotFound):
		fail(w, http.StatusNotFound, codeNotFound)
		return Schedule{}, false
	case err != nil:
		p.logErr("reporting: get schedule failed", err)
		failCause(w, r, http.StatusInternalServerError, codeInternal, err)
		return Schedule{}, false
	}
	return sc, true
}

// requestUserIdentity returns the authenticated user's id and display
// name. The id is what a scheduled run needs — it collects as that user
// through a host-issued session — and the name is only for the listing.
func requestUserIdentity(r *http.Request) (id, name string) {
	u := auth.RequireAuth(r)
	if u == nil {
		return "", ""
	}
	return u.ID, strings.TrimSpace(u.Username)
}
