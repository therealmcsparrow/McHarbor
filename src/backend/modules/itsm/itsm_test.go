// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package itsm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

var errBadSig = errors.New("bad signature")

type fakeVendor struct {
	mu        sync.Mutex
	calls     []string
	failOpen  int
	permanent bool
	partial   bool
	updateBad bool
	updateErr int
	notFound  bool
	verifyErr error
	parsed    []incident.RemoteStatus
	lastCfg   map[string]string
}

func (v *fakeVendor) record(s string, cfg map[string]string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls = append(v.calls, s)
	v.lastCfg = cfg
}

func (v *fakeVendor) Open(_ context.Context, cfg map[string]string, _ incident.Event) (incident.Ref, error) {
	v.record("open", cfg)
	switch {
	case v.permanent:
		return incident.Ref{}, errors.New("400 bad request")
	case v.failOpen > 0:
		v.failOpen--
		return incident.Ref{}, fmt.Errorf("%w: 503", incident.ErrRetryable)
	case v.partial:
		return incident.Ref{ExternalID: "EXT-1", Number: "INC1"}, fmt.Errorf("%w: attaching note failed", incident.ErrRetryable)
	}
	return incident.Ref{ExternalID: "EXT-1", Number: "INC1", URL: "https://itsm/INC1"}, nil
}

func (v *fakeVendor) Update(_ context.Context, cfg map[string]string, ref incident.Ref, _ incident.Event) error {
	v.record("update:"+ref.ExternalID, cfg)
	switch {
	case v.notFound:
		return incident.ErrNotFound
	case v.updateBad:
		return errors.New("422 unprocessable")
	case v.updateErr > 0:
		v.updateErr--
		return fmt.Errorf("%w: 502", incident.ErrRetryable)
	}
	return nil
}

func (v *fakeVendor) Resolve(_ context.Context, cfg map[string]string, ref incident.Ref, _ incident.Event) error {
	v.record("resolve:"+ref.ExternalID, cfg)
	return nil
}

func (v *fakeVendor) VerifyWebhook(map[string]string, http.Header, []byte) error { return v.verifyErr }

func (v *fakeVendor) ParseWebhook(context.Context, map[string]string, http.Header, []byte) ([]incident.RemoteStatus, error) {
	return v.parsed, nil
}

func (v *fakeVendor) Poll(_ context.Context, _ map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	out := []incident.RemoteStatus{}
	for _, r := range refs {
		out = append(out, incident.RemoteStatus{Ref: r, State: incident.RemoteAcknowledged, Assignee: "poller", UpdatedAt: time.Now()})
	}
	return out, nil
}

func (v *fakeVendor) callList() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.calls...)
}

type fakeDispatcher struct {
	db     *sql.DB
	vendor *fakeVendor
}

func (d fakeDispatcher) IncidentClient(t string) (incident.LifecycleClient, bool) {
	return d.vendor, t == "itsm"
}
func (d fakeDispatcher) WebhookParser(t string) (incident.WebhookParser, bool) {
	return d.vendor, t == "itsm"
}
func (d fakeDispatcher) Poller(t string) (incident.Poller, bool) { return d.vendor, t == "itsm" }
func (d fakeDispatcher) ChannelConfig(ctx context.Context, id string) (string, map[string]string, error) {
	var t string
	err := d.db.QueryRowContext(ctx, `SELECT channel_type FROM communication_channels WHERE id = ? AND enabled = 1`, id).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, notify.ErrChannelTargetNotFound
	}
	return t, map[string]string{"instance_url": "https://itsm"}, err
}

type harness struct {
	db     *sql.DB
	svc    *Service
	vendor *fakeVendor
	now    time.Time
	token  string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "itsm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ch1", "chat"} {
		typ := "itsm"
		if id == "chat" {
			typ = "slack"
		}
		if _, err := database.Exec(`INSERT INTO communication_channels (id, name, channel_type, method, is_default, enabled, config_enc, created_at, updated_at)
			VALUES (?, ?, ?, 'plugin', 0, 1, '', '2026-09-26T00:00:00Z', '2026-09-26T00:00:00Z')`, id, id, typ); err != nil {
			t.Fatal(err)
		}
	}
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{db: database, vendor: &fakeVendor{}, now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	h.svc = NewService(database, fakeDispatcher{db: database, vendor: h.vendor}, enc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.svc.now = func() time.Time { return h.now }
	h.svc.store.now = h.svc.now
	res, err := h.svc.RotateToken(context.Background(), "ch1", "https://mcharbor.example")
	if err != nil {
		t.Fatal(err)
	}
	h.token = res.URL[strings.LastIndex(res.URL, "/")+1:]
	return h
}

func (h *harness) drain(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		h.svc.ProcessOnce(context.Background())
		h.now = h.now.Add(maxBackoff + time.Minute)
	}
}

func (h *harness) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := h.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func fire(key string) incident.Event {
	return incident.Event{DedupKey: key, State: incident.StateFiring, Severity: incident.SeverityCritical, Title: "t", AlertID: "a"}
}

func resolved(key string) incident.Event {
	ev := fire(key)
	ev.State = incident.StateResolved
	return ev
}

func TestStoreOneLiveLinkPerKey(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	s := h.svc.store
	first, created, err := s.EnsureLink(ctx, "ch1", "itsm", "a:e:s", "a", "e", "s")
	if err != nil || !created {
		t.Fatalf("first EnsureLink: created=%v err=%v", created, err)
	}
	again, created, err := s.EnsureLink(ctx, "ch1", "itsm", "a:e:s", "a", "e", "s")
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("second EnsureLink must return the live link: created=%v err=%v", created, err)
	}
	if err := s.MarkResolved(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	fresh, created, err := s.EnsureLink(ctx, "ch1", "itsm", "a:e:s", "a", "e", "s")
	if err != nil || !created || fresh.ID == first.ID {
		t.Fatalf("after resolve a new link must be created: created=%v err=%v", created, err)
	}
}

// TestResolveWaitsForOpen is Review Focus 1: a recovery that arrives while the
// open is still retrying waits behind it and is sent with the ticket's id.
func TestResolveWaitsForOpen(t *testing.T) {
	h := newHarness(t)
	h.vendor.failOpen = 2
	ctx := context.Background()
	if err := h.svc.Emit(ctx, "ch1", fire("a:e:s")); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Emit(ctx, "ch1", resolved("a:e:s")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 10)
	got := h.vendor.callList()
	if len(got) != 4 || got[3] != "resolve:EXT-1" {
		t.Fatalf("want open,open,open,resolve:EXT-1, got %v", got)
	}
	link, err := h.svc.store.LinkByExternal(ctx, "ch1", "EXT-1")
	if err != nil || link.LocalState != LinkResolved {
		t.Fatalf("link state = %s %v", link.LocalState, err)
	}
	if !strings.HasSuffix(h.vendor.lastCfg["_webhook_url"], "/api/incident-webhooks/"+h.token) || h.vendor.lastCfg["_webhook_secret"] == "" {
		t.Fatalf("host keys were not injected: %v", h.vendor.lastCfg)
	}
}

func TestPermanentErrorParks(t *testing.T) {
	h := newHarness(t)
	h.vendor.permanent = true
	if err := h.svc.Emit(context.Background(), "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 3)
	if n := h.count(t, `SELECT COUNT(*) FROM incident_outbox WHERE parked = 1`); n != 1 {
		t.Fatalf("parked = %d, want 1", n)
	}
	if calls := h.vendor.callList(); len(calls) != 1 {
		t.Fatalf("a permanent error must not retry: %v", calls)
	}
	link, err := h.svc.store.LinkByKey(context.Background(), "ch1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if link.LastError != "delivery failed — see server log" {
		t.Fatalf("the vendor's own text reached the operator: %q", link.LastError)
	}
}

// TestOpenPartialSuccessKeepsRef checks a ticket created before a follow-up
// failure is kept and never opened twice.
func TestOpenPartialSuccessKeepsRef(t *testing.T) {
	h := newHarness(t)
	h.vendor.partial = true
	ctx := context.Background()
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 3)
	link, err := h.svc.store.LinkByKey(ctx, "ch1", "k")
	if err != nil || link.ExternalID != "EXT-1" || link.LocalState != LinkOpen {
		t.Fatalf("link: %+v %v", link, err)
	}
	if calls := h.vendor.callList(); len(calls) != 1 {
		t.Fatalf("a created ticket was opened again: %v", calls)
	}
}

// TestRemoteCloseBlocksReopen is Review Focus 3: a ticket closed in the ITSM
// while the alert fires is not reopened until the alert recovers and fires again.
func TestRemoteCloseBlocksReopen(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.db.Exec(`INSERT INTO alert_states (state_key, alert_id, active, last_seen, created_at, updated_at) VALUES ('k', 'a', 1, 'x', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 1)
	if _, err := h.svc.ApplyRemote(ctx, "ch1", incident.RemoteStatus{Ref: incident.Ref{ExternalID: "EXT-1"}, State: incident.RemoteClosed, UpdatedAt: h.now}); err != nil {
		t.Fatal(err)
	}
	link, err := h.svc.store.LinkByKey(ctx, "ch1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if link.LocalState != LinkRemoteClosed {
		t.Fatalf("state = %s, want remote_closed", link.LocalState)
	}
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 2)
	if calls := h.vendor.callList(); len(calls) != 1 {
		t.Fatalf("a firing condition reopened a remotely closed ticket: %v", calls)
	}
	if err := h.svc.Emit(ctx, "ch1", resolved("k")); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 2)
	if calls := h.vendor.callList(); len(calls) != 2 || calls[1] != "open" {
		t.Fatalf("after recovery the next firing must open a fresh ticket: %v", calls)
	}
}

// TestStaleStatusFromPreviousIncidentIsIgnored guards vendors that reuse the dedup
// key: a late resolve from an earlier incident must not close the fresh ticket.
func TestStaleStatusFromPreviousIncidentIsIgnored(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 1)
	applied, err := h.svc.ApplyRemote(ctx, "ch1", incident.RemoteStatus{Ref: incident.Ref{ExternalID: "EXT-1"}, State: incident.RemoteResolved, UpdatedAt: h.now.Add(-time.Hour)})
	if err != nil || applied {
		t.Fatalf("stale status applied=%v err=%v", applied, err)
	}
	link, err := h.svc.store.LinkByKey(ctx, "ch1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if link.LocalState != LinkOpen {
		t.Fatalf("state = %s, want open", link.LocalState)
	}
	opened, err := time.Parse(time.RFC3339, link.OpenedAt)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := h.svc.ApplyRemote(ctx, "ch1", incident.RemoteStatus{Ref: incident.Ref{ExternalID: "EXT-1"}, State: incident.RemoteAcknowledged, UpdatedAt: opened.Add(-30 * time.Second)}); err != nil || !applied {
		t.Fatalf("a status within clock skew must apply: %v %v", applied, err)
	}
}

// TestChannelDeleteOrphansLinks is Review Focus 5: a deleted channel's links
// are orphaned, their queue dropped, and the vendor never contacted again.
func TestChannelDeleteOrphansLinks(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.vendor.failOpen = 5
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 1)
	if _, err := h.db.Exec(`DELETE FROM communication_channels WHERE id = 'ch1'`); err != nil {
		t.Fatal(err)
	}
	before := len(h.vendor.callList())
	h.drain(t, 3)
	if len(h.vendor.callList()) != before {
		t.Fatal("a deleted channel was contacted")
	}
	link, err := h.svc.store.LinkByKey(ctx, "ch1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if link.LocalState != LinkOrphaned || h.count(t, `SELECT COUNT(*) FROM incident_outbox`) != 0 {
		t.Fatalf("link %s, outbox %d", link.LocalState, h.count(t, `SELECT COUNT(*) FROM incident_outbox`))
	}
}

func TestApplyRemoteIgnoresStaleAndBackfills(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	link, _, err := h.svc.store.EnsureLink(ctx, "ch1", "itsm", "k", "a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := h.svc.ApplyRemote(ctx, "ch1", incident.RemoteStatus{DedupKey: "k", Ref: incident.Ref{ExternalID: "X9", Number: "N9"},
		State: incident.RemoteAcknowledged, Assignee: "ann", UpdatedAt: h.now})
	if err != nil || !ok {
		t.Fatal(err)
	}
	got, err := h.svc.store.LinkByID(ctx, link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalID != "X9" || got.ExternalNumber != "N9" {
		t.Fatalf("ref not backfilled: %+v", got)
	}
	if ok, err := h.svc.ApplyRemote(ctx, "ch1", incident.RemoteStatus{Ref: incident.Ref{ExternalID: "X9"}, State: incident.RemoteOpen,
		UpdatedAt: h.now.Add(-time.Hour)}); err != nil || ok {
		t.Fatal("an older status overwrote a newer one")
	}
	if got, err := h.svc.store.LinkByID(ctx, link.ID); err != nil || got.RemoteState != string(incident.RemoteAcknowledged) {
		t.Fatalf("remote state = %s", got.RemoteState)
	}
}

func webhookMux(h *harness) http.Handler {
	database := h.db
	hd := &Handler{app: &router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AuditLog: coreAudit.NewLogger(database, "s")}, svc: h.svc}
	mux := chi.NewRouter()
	mux.Post("/api/incident-webhooks/{token}", hd.HandleInbound)
	return mux
}

func (h *harness) postWebhook(token string, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	webhookMux(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/incident-webhooks/"+token, strings.NewReader(body)))
	return rec
}

// TestWebhookRejectsUniformly is Review Focus 4: an unknown token and a bad
// signature get the same answer, so the route reveals nothing.
func TestWebhookRejectsUniformly(t *testing.T) {
	h := newHarness(t)
	unknown := h.postWebhook("nope", `{}`)
	h.vendor.verifyErr = errBadSig
	badSig := h.postWebhook(h.token, `{}`)
	if unknown.Code != 401 || badSig.Code != 401 || unknown.Body.String() != badSig.Body.String() {
		t.Fatalf("answers differ: %d %s / %d %s", unknown.Code, unknown.Body, badSig.Code, badSig.Body)
	}
}

func TestWebhookIgnoresUnknownTickets(t *testing.T) {
	h := newHarness(t)
	h.vendor.parsed = []incident.RemoteStatus{{Ref: incident.Ref{ExternalID: "never-seen"}, State: incident.RemoteClosed}}
	if rec := h.postWebhook(h.token, `{}`); rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if n := h.count(t, `SELECT COUNT(*) FROM incident_links`); n != 0 {
		t.Fatalf("the webhook created %d links", n)
	}
	if n := h.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'incident.webhook.received'`); n != 1 {
		t.Fatalf("webhook audit rows = %d", n)
	}
}

// TestAckIsRecordedForTheAlert checks a remote acknowledgement lands on the
// alert state, which is what stops cooldown re-notification.
func TestAckIsRecordedForTheAlert(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.db.Exec(`INSERT INTO alert_states (state_key, alert_id, active, last_seen, created_at, updated_at) VALUES ('a:e:s', 'a', 1, 'x', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Emit(ctx, "ch1", fire("a:e:s")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 1)
	h.vendor.parsed = []incident.RemoteStatus{{Ref: incident.Ref{ExternalID: "EXT-1"}, State: incident.RemoteAcknowledged, Assignee: "jdoe", UpdatedAt: h.now}}
	h.postWebhook(h.token, `{}`)
	var by string
	if err := h.db.QueryRow(`SELECT acknowledged_by FROM alert_states WHERE state_key = 'a:e:s'`).Scan(&by); err != nil || by != "itsm:jdoe" {
		t.Fatalf("acknowledged_by = %q %v", by, err)
	}
}

// TestParkedOperationBlocksTheRest checks a parked update holds back the
// resolve behind it, so the ticket is never closed around a failure nobody saw.
func TestParkedOperationBlocksTheRest(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 1)
	h.vendor.updateBad = true
	upd := fire("k")
	upd.State = incident.StateUpdate
	if err := h.svc.Emit(ctx, "ch1", upd); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Emit(ctx, "ch1", resolved("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 3)
	for _, c := range h.vendor.callList() {
		if strings.HasPrefix(c, "resolve") {
			t.Fatalf("a resolve went out behind a parked update: %v", h.vendor.callList())
		}
	}
}

// TestRetryingUpdateIsNotOvertaken checks operations on one ticket keep their
// order across retries.
func TestRetryingUpdateIsNotOvertaken(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Emit(ctx, "ch1", fire("k")); err != nil {
		t.Fatal(err)
	}
	h.drain(t, 1)
	h.vendor.updateErr = 1
	upd := fire("k")
	upd.State = incident.StateUpdate
	if err := h.svc.Emit(ctx, "ch1", upd); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Emit(ctx, "ch1", resolved("k")); err != nil {
		t.Fatal(err)
	}
	h.svc.ProcessOnce(ctx)
	h.svc.ProcessOnce(ctx)
	h.drain(t, 3)
	got := h.vendor.callList()
	want := []string{"open", "update:EXT-1", "update:EXT-1", "resolve:EXT-1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
