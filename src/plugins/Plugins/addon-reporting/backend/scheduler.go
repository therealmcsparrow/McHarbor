// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The scheduler.
//
// One goroutine, a sixty-second tick, and a run per due schedule. It is
// started in Init and cancelled in Shutdown, because every `go func()` in
// this codebase gets lifecycle management and a generation still fanning
// out across loopback after the host has torn the plugin down would write
// to a closed database handle.
//
// Failure semantics are deliberately plain: no retry. A schedule that
// could not deliver this hour delivers next hour, and the last run row
// says what went wrong in between. This matches the notification
// contract's fire-and-forget stance and the backup scheduler's behaviour;
// a retry loop here would double-send to the targets that *did* work.
//
// Known limitation, accepted: `container_backups` avoids double-firing in
// an HA deployment through a leader coordinator, and `HostContext` exposes
// no coordinator handle. On a multi-node deployment every node would fire
// this. Correct on single-node, which is the shipping configuration.

const (
	schedulerTick = 60 * time.Second
	// A single generation fans out across twenty loopback endpoints, some
	// of them over the agent transport. Ten minutes is generous for the
	// slowest honest run and short enough that a wedged one cannot pin a
	// goroutine for the life of the process.
	scheduleRunTimeout = 10 * time.Minute
	// How long before a cached session's real expiry it stops being
	// handed out. Comfortably longer than scheduleRunTimeout, so a run
	// that starts on a cached session can never outlive it mid-collection.
	sessionRetireEarly = 15 * time.Minute
)

// cachedSession is one live host session, reused across runs by the same
// author until it approaches expiry. See sessionCookie for why.
type cachedSession struct {
	cookie string
	until  time.Time
}

type scheduler struct {
	// onRun, when set, is told about every finished run so the
	// schedule-ran workflow trigger can fire.
	onRun func(Schedule, ScheduleRun, *StoredReport)
	svc   *service
	store *store
	host  plugin.HostContext
	audit func(event string, fields map[string]any)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// running guards against a slow schedule being started twice: the
	// tick that follows a two-minute generation would otherwise find the
	// same row still due if writing next_run_at had failed.
	mu      sync.Mutex
	running map[string]bool

	// sessions is keyed by the schedule author's user id, not by schedule
	// id: two schedules owned by the same person see exactly the same
	// estate, so they should share one session rather than mint two.
	sessionMu sync.Mutex
	sessions  map[string]cachedSession
}

func newScheduler(svc *service, host plugin.HostContext, audit func(string, map[string]any)) *scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &scheduler{
		svc: svc, store: svc.store, host: host, audit: audit,
		ctx: ctx, cancel: cancel,
		running:  map[string]bool{},
		sessions: map[string]cachedSession{},
	}
}

func (s *scheduler) start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(schedulerTick)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.tick()
			}
		}
	}()
}

func (s *scheduler) stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *scheduler) tick() {
	now := time.Now().UTC()
	due, err := s.store.dueSchedules(s.ctx, now)
	if err != nil {
		s.logError("reporting: read due schedules failed", err)
		return
	}
	for _, sc := range due {
		s.launch(sc, now)
	}
}

// launch claims a schedule and runs it on its own goroutine.
//
// next_run_at is advanced *before* the run starts, not after. A report
// that takes two minutes would otherwise still be due on the next tick,
// and the guard below would be the only thing standing between one
// schedule and a queue of duplicate runs.
func (s *scheduler) launch(sc Schedule, now time.Time) {
	if !s.claim(sc.ID) {
		return
	}
	next, err := nextCronRun(sc.Cron, now)
	if err != nil {
		// A schedule whose expression stopped parsing is disabled by
		// clearing its next run rather than by flipping `enabled`: the
		// operator's intent to have it on is preserved, and the UI shows
		// "never" against an expression it can point at.
		s.logError("reporting: schedule has an unreadable expression", err,
			"schedule_id", sc.ID, "cron", sc.Cron)
		if markErr := s.store.markRan(s.ctx, sc.ID, sc.LastRunAt, time.Time{}); markErr != nil {
			s.logError("reporting: clear next run failed", markErr, "schedule_id", sc.ID)
		}
		s.release(sc.ID)
		return
	}
	if err := s.store.markRan(s.ctx, sc.ID, now, next); err != nil {
		s.logError("reporting: advance schedule failed", err, "schedule_id", sc.ID)
		s.release(sc.ID)
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.release(sc.ID)
		ctx, cancel := context.WithTimeout(s.ctx, scheduleRunTimeout)
		defer cancel()
		s.run(ctx, sc)
	}()
}

// runNow fires a schedule immediately without disturbing its cron.
// Returns false when the schedule is already running, so "Run now"
// pressed twice does not produce two overlapping generations.
func (s *scheduler) runNow(sc Schedule) bool {
	if !s.claim(sc.ID) {
		return false
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.release(sc.ID)
		ctx, cancel := context.WithTimeout(s.ctx, scheduleRunTimeout)
		defer cancel()
		s.run(ctx, sc)
	}()
	return true
}

func (s *scheduler) claim(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[id] {
		return false
	}
	s.running[id] = true
	return true
}

func (s *scheduler) release(id string) {
	s.mu.Lock()
	delete(s.running, id)
	s.mu.Unlock()
}

// --- the run ----------------------------------------------------------

func (s *scheduler) run(ctx context.Context, sc Schedule) {
	started := time.Now().UTC()
	run := ScheduleRun{
		ScheduleID: sc.ID, Status: RunRunning,
		Deliveries: []DeliveryOutcome{}, StartedAt: started,
	}
	// The run row is written before generation begins so the page shows
	// "running" instead of last week's result for the minute a report
	// takes to build.
	if err := s.store.upsertRun(ctx, run, nil); err != nil {
		s.logError("reporting: open scheduled run failed", err, "schedule_id", sc.ID)
	}

	rec, err := s.generate(ctx, sc)
	if err != nil {
		run.Status, run.Code = RunFailed, scheduleFailureCode(err)
		run.FinishedAt = time.Now().UTC()
		s.logError("reporting: scheduled generation failed", err,
			"schedule_id", sc.ID, "type", sc.ReportType)
		if err := s.store.upsertRun(ctx, run, nil); err != nil {
			s.logError("reporting: record failed run", err, "schedule_id", sc.ID)
		}
		s.auditRun(sc, run)
		s.notifyRun(sc, run, nil)
		return
	}

	run.Title = rec.Title
	data, filename, contentType, err := s.svc.render(ctx, rec, sc.Format,
		s.store.settings(ctx, s.host.DB.KV()))
	if err != nil {
		run.Status, run.Code = RunFailed, codeExport
		run.FinishedAt = time.Now().UTC()
		s.logError("reporting: scheduled export failed", err, "schedule_id", sc.ID)
		// The document is still stored: a render failure is a formatting
		// problem, and the report itself is readable in the UI.
		if err := s.store.upsertRun(ctx, run, rec.Document); err != nil {
			s.logError("reporting: record failed run", err, "schedule_id", sc.ID)
		}
		s.auditRun(sc, run)
		s.notifyRun(sc, run, &rec)
		return
	}

	targets, err := s.store.scheduleTargets(ctx, sc.ID)
	if err != nil {
		s.logError("reporting: read schedule targets failed", err, "schedule_id", sc.ID)
		targets = nil
	}
	run.Deliveries = s.deliver(ctx, sc, targets, deliverable{
		data: data, filename: filename, contentType: contentType, report: rec,
	})
	run.Status = deliveryStatus(run.Deliveries)
	run.FinishedAt = time.Now().UTC()

	if err := s.store.upsertRun(ctx, run, rec.Document); err != nil {
		s.logError("reporting: record scheduled run failed", err, "schedule_id", sc.ID)
	}
	s.auditRun(sc, run)
	s.notifyRun(sc, run, &rec)
}

// notifyRun hands a finished run to the schedule-ran workflow trigger.
func (s *scheduler) notifyRun(sc Schedule, run ScheduleRun, rec *StoredReport) {
	if s.onRun != nil {
		s.onRun(sc, run, rec)
	}
}

// generate builds the report as the user who owns the schedule.
//
// This is the whole reason a schedule stores a user id. Collection runs
// over loopback against the host's own API, and the host authenticates by
// session cookie — so a background run with no session would either read
// nothing or, far worse, have to read the database directly and bypass
// every role, ABAC rule and environment scope the interface enforces.
// Issuing a session for the schedule's author keeps the guarantee the
// interactive path already makes: a report can only ever contain what the
// person who asked for it may already see.
//
// Persist is false: a scheduled run keeps its document on its own run row
// and never enters the report library.
func (s *scheduler) generate(ctx context.Context, sc Schedule) (StoredReport, error) {
	end := time.Now()
	days := sc.PeriodDays
	if days < 1 {
		days = 30
	}
	rt, known := reportTypeByKey(sc.ReportType)
	if !known {
		return StoredReport{}, fmt.Errorf("reporting: schedule %s names unknown type %q",
			sc.ID, sc.ReportType)
	}

	in := GenerateInput{
		ReportType:    rt.Key,
		EnvironmentID: sc.EnvironmentID,
		Environment:   sc.Environment,
		PeriodStart:   end.AddDate(0, 0, -days),
		PeriodEnd:     end,
		Levels:        sc.Levels,
		Options:       resolveOptions(rt, sc.Options),
		Persist:       false,
		Subject:       Subject{Kind: sc.SubjectKind, ID: sc.SubjectID, Name: sc.SubjectName},
		Cookie:        s.sessionCookie(sc),
		User:          orDash(sc.CreatedBy, "schedule"),
		Source:        "schedule",
		// A scheduled report runs as its author, so a plugin-contributed
		// type re-checks that author's own permission — the same answer
		// they would get pressing Generate by hand. An author whose
		// access was revoked after the schedule was saved stops getting
		// the report, which is the point.
		UserID:         sc.CreatedByID,
		Prompt:         sc.Prompt,
		AIConnectionID: sc.AIConnectionID,
	}
	if rt.PointInTime {
		in.PeriodStart = end
	}
	return s.svc.Generate(ctx, in)
}

// sessionCookie returns a cookie header naming a live session for the
// schedule's author, minting one only when the cached one is spent.
//
// The cache is not an optimisation, it is hygiene. The host has no
// "destroy this session" callback on the plugin contract and prunes
// expired sessions only lazily, when someone happens to present one — so
// a machine session that nobody ever presents again is a row that sits
// there until the database is touched by hand. Minting one per *run*
// would leave a daily report with a row a day, for ever. Minting one per
// session lifetime bounds it to a handful.
//
// An empty string is a legitimate result rather than a failure: when the
// host runs with authentication disabled its middleware injects a system
// user and no cookie is needed at all. When authentication *is* on and
// the session cannot be issued, collection finds every source
// unavailable, the report says so in its Methodology annex, and the log
// line below is the evidence for why.
func (s *scheduler) sessionCookie(sc Schedule) string {
	userID := strings.TrimSpace(sc.CreatedByID)
	if s.host.Sessions == nil || userID == "" {
		return ""
	}

	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if cached, ok := s.sessions[userID]; ok && time.Now().Before(cached.until) {
		return cached.cookie
	}

	id, expiresInSec, err := s.host.Sessions.IssueSession(userID)
	if err != nil || id == "" {
		s.logError("reporting: could not issue a session for a scheduled run", err,
			"schedule_id", sc.ID)
		return ""
	}
	cookie := s.host.Sessions.SessionCookieName() + "=" + id
	// Retire the cached session well before the host would: a run that
	// starts five minutes before expiry must not have its collection cut
	// off half way through, which would produce a report full of
	// "not permitted" rather than an honest failure.
	lifetime := time.Duration(expiresInSec) * time.Second
	if lifetime <= sessionRetireEarly {
		// A very short-lived session is still usable right now; it just
		// must not be cached, or the next run reuses a dead one.
		return cookie
	}
	s.sessions[userID] = cachedSession{cookie: cookie, until: time.Now().Add(lifetime - sessionRetireEarly)}
	return cookie
}

// --- delivery ---------------------------------------------------------

// deliverable is the finished artefact plus the record it came from, so
// every target formats its own message from the same material.
type deliverable struct {
	data        []byte
	filename    string
	contentType string
	report      StoredReport
}

// deliver fans out to every target concurrently.
//
// Concurrently because a schedule with four targets should take as long
// as its slowest one, not the sum; and because one unreachable webhook
// must not delay the storage upload behind it.
func (s *scheduler) deliver(
	ctx context.Context, sc Schedule, targets []ScheduleTarget, art deliverable,
) []DeliveryOutcome {
	if len(targets) == 0 {
		// The service refuses to save a schedule with no targets, so this
		// can only mean the rows were removed underneath it.
		return []DeliveryOutcome{}
	}
	channels := s.commChannels()
	locations := s.storageLocations()

	out := make([]DeliveryOutcome, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(idx int, target ScheduleTarget) {
			defer wg.Done()
			switch target.Kind {
			case TargetStorage:
				out[idx] = s.deliverStorage(ctx, sc, target, art, locations)
			case TargetComm:
				out[idx] = s.deliverComm(ctx, sc, target, art, channels)
			default:
				out[idx] = DeliveryOutcome{
					Kind: target.Kind, RefID: target.RefID,
					Name: target.RefID, Code: codeDeliveryTargetGone,
				}
			}
		}(i, t)
	}
	wg.Wait()
	return out
}

func (s *scheduler) deliverStorage(
	ctx context.Context, sc Schedule, target ScheduleTarget,
	art deliverable, locations map[string]string,
) DeliveryOutcome {
	res := DeliveryOutcome{
		Kind: TargetStorage, RefID: target.RefID,
		Name: orDash(locations[target.RefID], target.RefID),
	}
	if s.host.Storage == nil {
		res.Code = codeDeliveryUnavailable
		return res
	}
	if _, known := locations[target.RefID]; !known {
		res.Code = codeDeliveryTargetGone
		return res
	}
	// A dated filename under the folder the operator chose, or under the
	// plugin's default when they chose none. The *plugin* keeps only the
	// last run; a storage location is where an operator sends a report
	// precisely because they want the history, so the filename carries the
	// date rather than the folder being overwritten every month.
	folder := target.Path
	if folder == "" {
		folder = defaultStoragePath(sc.Name)
	}
	// The name is the operator's pattern when they wrote one, and the
	// exporter's dated name otherwise. The extension always comes from
	// the bytes that were rendered, never from the pattern, so a file
	// cannot end up claiming a format it is not.
	key := folder + "/" + applyFilenamePattern(target.Filename, art.filename, sc, art.report, time.Now())
	if err := s.host.Storage.Upload(ctx, target.RefID, key, art.data); err != nil {
		s.logError("reporting: scheduled upload failed", err,
			"schedule_id", sc.ID, "location_id", target.RefID)
		res.Code = codeDeliveryFailed
		return res
	}
	res.OK, res.Attached = true, true
	return res
}

func (s *scheduler) deliverComm(
	ctx context.Context, sc Schedule, target ScheduleTarget,
	art deliverable, channels map[string]plugin.CommsChannelRef,
) DeliveryOutcome {
	ch, known := channels[target.RefID]
	res := DeliveryOutcome{
		Kind: TargetComm, RefID: target.RefID,
		Name: orDash(ch.Name, target.RefID),
	}
	if s.host.Comms == nil {
		res.Code = codeDeliveryUnavailable
		return res
	}
	if !known {
		res.Code = codeDeliveryTargetGone
		return res
	}

	msg := plugin.CommsMessage{
		Title:   art.report.Title,
		Message: scheduleMessageBody(sc, art, ch.SupportsFileAttachments),
		Level:   "info",
	}
	// An alternative destination for this schedule only; the channel
	// itself is untouched and its next send goes where it always did.
	// A stored override on a channel that has since lost the capability
	// is dropped rather than sent: the host would refuse it anyway, and
	// failing the whole delivery would cost the operator a report over a
	// field they can no longer see.
	if target.Recipient != "" && ch.Recipient.Supported {
		msg.Recipient = target.Recipient
	}
	// The host drops Files for an adapter that cannot carry them, but
	// sending several megabytes across the boundary to be discarded is
	// waste — and the body above has already been written to suit.
	if ch.SupportsFileAttachments {
		msg.Files = []plugin.CommsFile{{
			Filename: art.filename, ContentType: art.contentType, Data: art.data,
		}}
	}
	if err := s.host.Comms.Send(ctx, target.RefID, msg); err != nil {
		s.logError("reporting: scheduled send failed", err,
			"schedule_id", sc.ID, "channel_id", target.RefID)
		res.Code = codeDeliveryFailed
		return res
	}
	res.OK = true
	res.Attached = ch.SupportsFileAttachments
	return res
}

// scheduleMessageBody writes what the recipient reads.
//
// Two versions, because the honest thing to say differs: a channel that
// carries the file says what is attached, and one that cannot says where
// to find it instead of pretending. The headline is the report's own
// opening sentence, so the message is worth reading even when the
// document is not opened.
func scheduleMessageBody(sc Schedule, art deliverable, attached bool) string {
	rec := art.report
	lines := []string{
		fmt.Sprintf("%s — %s", rec.Title, fmtPeriod(Period{
			Start: rec.PeriodStart, End: rec.PeriodEnd,
			PointInTime: rec.PeriodStart.Equal(rec.PeriodEnd),
		})),
	}
	if env := strings.TrimSpace(rec.Environment); env != "" {
		lines = append(lines, "Environment: "+env)
	}
	if headline := strings.TrimSpace(rec.Headline); headline != "" {
		lines = append(lines, "", truncateText(headline, 400))
	}
	lines = append(lines, "")
	if attached {
		lines = append(lines, "The full report is attached as "+art.filename+".")
	} else {
		lines = append(lines,
			"This channel cannot carry file attachments, so the report is not attached.",
			"Open it in McHarbor under Reporting → Report Schedules → "+sc.Name+".")
	}
	return strings.Join(lines, "\n")
}

// deliveryStatus collapses the per-target outcomes into the run's own
// state. `partial` exists so "two of three worked" is not reported as
// either a clean success or a total failure.
func deliveryStatus(outcomes []DeliveryOutcome) string {
	if len(outcomes) == 0 {
		return RunFailed
	}
	ok := 0
	for _, o := range outcomes {
		if o.OK {
			ok++
		}
	}
	switch {
	case ok == len(outcomes):
		return RunSuccess
	case ok == 0:
		return RunFailed
	default:
		return RunPartial
	}
}

// --- host lookups -----------------------------------------------------

func (s *scheduler) commChannels() map[string]plugin.CommsChannelRef {
	out := map[string]plugin.CommsChannelRef{}
	if s.host.Comms == nil {
		return out
	}
	for _, c := range s.host.Comms.Channels() {
		out[c.ID] = c
	}
	return out
}

func (s *scheduler) storageLocations() map[string]string {
	out := map[string]string{}
	if s.host.Storage == nil {
		return out
	}
	for _, l := range s.host.Storage.Locations() {
		out[l.ID] = l.Name
	}
	return out
}

// --- plumbing ---------------------------------------------------------

// scheduleFailureCode maps a generation error to the code the UI shows.
// The same mapping the interactive handler uses, so an operator sees the
// same wording whether a report failed from the studio or from a cron.
func scheduleFailureCode(err error) string {
	switch {
	case errors.Is(err, errAIReportsOff):
		return codeAIDisabled
	case errors.Is(err, errAIUnavailable):
		return codeAIUnavailable
	case errors.Is(err, errAIConnectionUnavailable):
		return codeAIConnectionUnavailable
	case errors.Is(err, errPlanFailed):
		return codeAIPlanFailed
	case errors.Is(err, errAuthorFailed):
		return codeAIAuthorFailed
	default:
		return codeGenerate
	}
}

func (s *scheduler) auditRun(sc Schedule, run ScheduleRun) {
	if s.audit == nil {
		return
	}
	delivered := 0
	for _, o := range run.Deliveries {
		if o.OK {
			delivered++
		}
	}
	s.audit("plugin.reporting.schedule.ran", map[string]any{
		"schedule_id": sc.ID,
		"name":        sc.Name,
		"report_type": sc.ReportType,
		"status":      run.Status,
		"targets":     len(run.Deliveries),
		"delivered":   delivered,
		"code":        run.Code,
	})
}

func (s *scheduler) logError(msg string, err error, args ...any) {
	if s.host.Logger == nil {
		return
	}
	s.host.Logger.Error(msg, append([]any{"error", err}, args...)...)
}
