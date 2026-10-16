// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compute_console

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/agentrpc"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func spiceTicket() Ticket {
	return Ticket{UserID: "user-1", EnvID: "env-1", GuestKind: "vm", GuestID: "g1", Kind: "spice"}
}

// TestBatchTicketsShareOneSession: every channel ticket of a batch authorizes the
// same guest for the same user until the same moment, and each is still single-use.
func TestBatchTicketsShareOneSession(t *testing.T) {
	s, _ := store(t)
	ctx := context.Background()

	values, err := s.IssueBatch(ctx, spiceTicket(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 5 {
		t.Fatalf("asked for 5 channel tickets, got %d", len(values))
	}

	seen := map[string]bool{}
	var session string
	var expiry string
	for i, value := range values {
		if seen[value] {
			t.Fatalf("channel ticket %d repeats an earlier value", i)
		}
		seen[value] = true
		redeemed, err := s.Redeem(ctx, value)
		if err != nil {
			t.Fatalf("channel ticket %d must redeem: %v", i, err)
		}
		if redeemed.UserID != "user-1" || redeemed.GuestID != "g1" || redeemed.Kind != "spice" {
			t.Fatalf("channel ticket %d authorizes %+v", i, redeemed)
		}
		if redeemed.Session == "" {
			t.Fatalf("channel ticket %d lost its session", i)
		}
		if i == 0 {
			session, expiry = redeemed.Session, redeemed.ExpiresAt.String()
		} else if redeemed.Session != session || redeemed.ExpiresAt.String() != expiry {
			t.Fatalf("channel ticket %d is bound to session %q expiring %s, want %q expiring %s",
				i, redeemed.Session, redeemed.ExpiresAt, session, expiry)
		}
		if _, err := s.Redeem(ctx, value); !errors.Is(err, ErrTicketInvalid) {
			t.Fatalf("channel ticket %d must be single-use, got %v", i, err)
		}
	}
}

// TestBatchSessionCannotBeRewritten: the session prefix is covered by the digest, so
// grafting a secret onto another session's id is an unknown ticket, not a reassignment.
func TestBatchSessionCannotBeRewritten(t *testing.T) {
	s, _ := store(t)
	ctx := context.Background()

	values, err := s.IssueBatch(ctx, spiceTicket(), 2)
	if err != nil {
		t.Fatal(err)
	}
	_, secret, _ := strings.Cut(values[0], sessionSeparator)
	if _, err := s.Redeem(ctx, "d0000000000000000000"+sessionSeparator+secret); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("a rewritten session prefix must not redeem, got %v", err)
	}
}

// TestSingleTicketHasNoSession keeps VNC and serial exactly as they were.
func TestSingleTicketHasNoSession(t *testing.T) {
	s, _ := store(t)
	ctx := context.Background()

	value, err := s.Issue(ctx, Ticket{UserID: "user-1", EnvID: "env-1", GuestKind: "vm", GuestID: "g1", Kind: "vnc"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(value, sessionSeparator) {
		t.Fatalf("a single ticket must not carry a session prefix: %q", value)
	}
	redeemed, err := s.Redeem(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	if redeemed.Session != "" {
		t.Fatalf("a single ticket redeemed into session %q", redeemed.Session)
	}
}

func TestBatchSizeIsBounded(t *testing.T) {
	s, _ := store(t)
	for _, n := range []int{0, -1, maxChannelsPerSession + 1} {
		if _, err := s.IssueBatch(context.Background(), spiceTicket(), n); err == nil {
			t.Fatalf("a batch of %d must be refused", n)
		}
	}
}

// TestSessionCountsOnce: every channel of one SPICE session is one viewer, the cap
// counts sessions, and a session's later channels are never refused by it.
func TestSessionCountsOnce(t *testing.T) {
	s, _ := store(t)

	var sessions []string
	for i := range maxViewersPerGuest {
		session := "spice-" + string(rune('a'+i))
		for range 5 {
			if _, err := s.AddViewer("env-1", "vm", "g1", session); err != nil {
				t.Fatalf("channel of session %d refused: %v", i, err)
			}
		}
		sessions = append(sessions, session)
	}
	if got := s.Viewers("env-1", "vm", "g1"); got != maxViewersPerGuest {
		t.Fatalf("%d sessions of five channels each must read as %d viewers, got %d",
			maxViewersPerGuest, maxViewersPerGuest, got)
	}

	if _, err := s.AddViewer("env-1", "vm", "g1", sessions[0]); err != nil {
		t.Fatalf("a channel joining an admitted session must not hit the cap: %v", err)
	}
	if _, err := s.AddViewer("env-1", "vm", "g1", "spice-new"); !errors.Is(err, ErrTooManyViewers) {
		t.Fatalf("a fifth session must be refused, got %v", err)
	}
	if _, err := s.AddViewer("env-1", "vm", "g1", ""); !errors.Is(err, ErrTooManyViewers) {
		t.Fatalf("a single-socket console past the cap must be refused, got %v", err)
	}

	// Closing all but one channel keeps the session; closing the last frees the seat.
	for range 5 {
		s.RemoveViewer("env-1", "vm", "g1", sessions[0])
	}
	if got := s.Viewers("env-1", "vm", "g1"); got != maxViewersPerGuest {
		t.Fatalf("a session with one channel still open must still count, got %d", got)
	}
	s.RemoveViewer("env-1", "vm", "g1", sessions[0])
	if got := s.Viewers("env-1", "vm", "g1"); got != maxViewersPerGuest-1 {
		t.Fatalf("closing a session's last channel must free its seat, got %d", got)
	}
	if _, err := s.AddViewer("env-1", "vm", "g1", "spice-new"); err != nil {
		t.Fatalf("a freed seat must admit a new session: %v", err)
	}
}

// TestSingleSocketViewersCountSeparately is the VNC and serial behaviour: each
// socket is its own viewer, and the key AddViewer hands back releases exactly it.
func TestSingleSocketViewersCountSeparately(t *testing.T) {
	s, _ := store(t)

	var keys []string
	for i := range maxViewersPerGuest {
		key, err := s.AddViewer("env-1", "vm", "g1", "")
		if err != nil {
			t.Fatalf("viewer %d refused: %v", i, err)
		}
		keys = append(keys, key)
	}
	if got := s.Viewers("env-1", "vm", "g1"); got != maxViewersPerGuest {
		t.Fatalf("want %d viewers, got %d", maxViewersPerGuest, got)
	}
	if _, err := s.AddViewer("env-1", "vm", "g1", ""); !errors.Is(err, ErrTooManyViewers) {
		t.Fatalf("a fifth VNC viewer must be refused, got %v", err)
	}
	s.RemoveViewer("env-1", "vm", "g1", keys[0])
	if got := s.Viewers("env-1", "vm", "g1"); got != maxViewersPerGuest-1 {
		t.Fatalf("releasing one socket must free one seat, got %d", got)
	}
	for _, key := range keys[1:] {
		s.RemoveViewer("env-1", "vm", "g1", key)
	}
	if got := s.Viewers("env-1", "vm", "g1"); got != 0 {
		t.Fatalf("every socket released, got %d viewers", got)
	}
}

func TestChannelCountValidation(t *testing.T) {
	cases := []struct {
		kind     string
		channels int
		want     bool
	}{
		{"vnc", 0, true},
		{"vnc", 1, true},
		{"vnc", 2, false},
		{"serial", 3, false},
		{"exec", 2, false},
		{"spice", 0, true},
		{"spice", 1, true},
		{"spice", maxChannelsPerSession, true},
		{"spice", maxChannelsPerSession + 1, false},
		{"spice", -1, false},
	}
	for _, c := range cases {
		if got := validChannelCount(c.kind, c.channels); got != c.want {
			t.Errorf("validChannelCount(%q, %d) = %v, want %v", c.kind, c.channels, got, c.want)
		}
	}
}

// TestIssueHandlerReturnsBatchOnlyWhenAsked drives the route: a SPICE request with
// channels answers a tickets array, a VNC request answers the one ticket it always did,
// and a VNC request asking for channels is refused before any ticket is minted.
func TestIssueHandlerReturnsBatchOnlyWhenAsked(t *testing.T) {
	s, database := store(t)
	h := consoleHandlerFor(t, database, s)

	issue := func(body string) (*httptest.ResponseRecorder, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/api/compute/console/ticket", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
		rec := httptest.NewRecorder()
		h.HandleIssueTicket(rec, req)
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decoding %s: %v", rec.Body.String(), err)
			}
		}
		return rec, envelope.Data
	}

	rec, data := issue(`{"envId":"envA","guestKind":"vm","guestId":"g1","kind":"spice","channels":6}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("spice batch: %d %s", rec.Code, rec.Body.String())
	}
	tickets, ok := data["tickets"].([]any)
	if !ok || len(tickets) != 6 {
		t.Fatalf("spice batch must answer 6 tickets, got %v", data["tickets"])
	}
	if data["ticket"] != tickets[0] {
		t.Fatalf("ticket must be the batch's first, got %v", data["ticket"])
	}

	rec, data = issue(`{"envId":"envA","guestKind":"vm","guestId":"g1","kind":"vnc"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("vnc: %d %s", rec.Code, rec.Body.String())
	}
	if _, has := data["tickets"]; has {
		t.Fatalf("a vnc ticket must keep its old shape, got %v", data)
	}
	if value, _ := data["ticket"].(string); value == "" || strings.Contains(value, sessionSeparator) {
		t.Fatalf("a vnc ticket must be a plain single ticket, got %q", value)
	}

	rec, _ = issue(`{"envId":"envA","guestKind":"vm","guestId":"g1","kind":"vnc","channels":3}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a vnc batch must be refused with 400, got %d", rec.Code)
	}
}

// availableCompute answers Available and nothing else, which is all ticket issue asks.
type availableCompute struct{}

func (availableCompute) Available(context.Context, string) (bool, string) { return true, "" }

func (availableCompute) OpenConsole(context.Context, string, agentrpc.ConsoleRequest) (agentrpc.ConsoleStream, error) {
	return nil, errors.New("ticket tests never open a console")
}

// consoleHandlerFor builds a gateway over database whose user u1 holds vm.console.
func consoleHandlerFor(t *testing.T, database *sql.DB, tickets *TicketStore) *Handler {
	t.Helper()
	if err := rbac.UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('c', 'c', '["vm.console"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'u1', 'c')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	return &Handler{
		app: &router.AppDeps{
			RBACService: rbac.NewService(database),
			Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		tickets:  tickets,
		gateway:  availableCompute{},
		subjects: declaredSubjects(),
	}
}

// TestUpgraderEchoesTheBinarySubprotocol: spice-html5 requests "binary" on every
// channel socket and the browser drops a socket whose protocol is not echoed.
func TestUpgraderEchoesTheBinarySubprotocol(t *testing.T) {
	_, database := store(t)
	h := NewHandler(&router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, nil)
	t.Cleanup(h.tickets.Close)
	found := false
	for _, protocol := range h.upgrader.Subprotocols {
		found = found || protocol == "binary"
	}
	if !found {
		t.Fatalf("the console upgrader must offer the binary subprotocol, has %v", h.upgrader.Subprotocols)
	}
}
