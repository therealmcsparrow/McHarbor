// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package compute_console is the browser's way into a guest console.
//
//	browser ──WS──► backend /api/compute/console/{ticket} ──agent WS──► guest socket
//	 noVNC / xterm.js          this package                    vnc.sock / tty
//
// The client never talks to the agent directly, and this is the only part of
// the compute capability that had to stay in core: a plugin contributes HTTP
// routes, not WebSocket upgrades, and the console is a hijacked byte stream in
// both directions.
//
// # Why a ticket rather than a token in the URL
//
// A WebSocket upgrade cannot carry an Authorization header from the browser —
// the API simply has no place to put one. The obvious workaround is to append
// the session token to the URL, and it is a bad one: URLs land in proxy access
// logs, in browser history, and in any reverse proxy the operator runs in
// front of McHarbor. A leaked console URL would then be a leaked session.
//
// So the permission check happens on an ordinary authenticated POST, which
// issues a ticket: 32 bytes of crypto/rand, valid for 30 seconds, usable
// exactly once, and bound to the user, the guest and the console kind it was
// issued for. The worst a leaked ticket buys is a race for one console session
// on one guest, inside half a minute.
package compute_console

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// ticketTTL is how long a ticket is usable.
//
// Thirty seconds is generous for "POST then immediately upgrade" and short
// enough that a ticket captured from a log is almost always already dead.
const ticketTTL = 30 * time.Second

// maxTicketsPerUser bounds how many unredeemed tickets one user may hold, so a
// client bug that requests tickets in a loop cannot grow the store without
// limit.
const maxTicketsPerUser = 32

// maxViewersPerGuest caps concurrent console sessions on one guest.
//
// VNC has no multi-seat awareness: a second cursor moving on its own with
// nothing to explain it is confusing, so the UI shows a viewer count and this
// bounds it. It is not a security control — the permission check is — it is a
// usability one.
const maxViewersPerGuest = 4

// maxChannelsPerSession bounds a ticket batch. SPICE opens one WebSocket per
// channel — main, display, inputs, cursor, playback, record and a port or two —
// so eight covers every channel QEMU offers with room to spare, and anything
// larger is a client asking for tickets it has no socket to spend on.
const maxChannelsPerSession = 8

// sessionSeparator joins a batch ticket's session id to its secret. It is not
// in the base64url alphabet, so a single ticket can never be mistaken for one.
const sessionSeparator = "."

var (
	// ErrTicketInvalid covers unknown, expired and already-redeemed tickets
	// alike. They are deliberately not distinguished: telling a caller that a
	// ticket existed but expired confirms a valid ticket value.
	ErrTicketInvalid = errors.New("console ticket is not valid")
	// ErrTooManyViewers means the guest already has the maximum sessions open.
	ErrTooManyViewers = errors.New("this guest already has the maximum number of console viewers")
)

// Ticket is one issued console authorization.
type Ticket struct {
	// value is the secret. It is never logged, never stored and never returned
	// again after issue; the table holds a digest of it.
	value string

	UserID    string
	EnvID     string
	GuestKind string
	GuestID   string
	Kind      string
	// Command is the shell for an exec session, chosen server-side.
	Command []string
	// Session groups the channel tickets of one console session, so the
	// viewer count sees one viewer however many sockets it opens. Empty for a
	// single ticket, which is a session of its own.
	Session string

	IssuedAt  time.Time
	ExpiresAt time.Time
}

// TicketStore holds unredeemed tickets in the shared database.
//
// They used to live in a map on one node, which was correct while one node served every
// request and wrong the moment two did: the browser's WebSocket upgrade can land on a
// node that never issued the ticket, and the console would refuse a credential it had
// just minted. The table is the fix, and it costs a ticket nothing — it is written once
// and deleted once, inside thirty seconds.
//
// What is stored is a SHA-256 digest of the value, never the value, so a database dump, a
// replica or a backup carries no usable console credential.
//
// Viewer counts stay in memory deliberately. A viewer is an open WebSocket on one node,
// so a shared count would have to be reconciled against sockets no other node can see,
// and a node that died mid-session would leak a seat forever. The cap is therefore per
// node, which the readiness gate says in so many words.
type TicketStore struct {
	database *sql.DB
	logger   *slog.Logger

	mu sync.Mutex
	// viewers holds, per guest on this node, the open channel count of each
	// live session: the cap and the "2 viewers" badge count sessions, and a
	// session leaves only when its last channel closes.
	viewers map[string]map[string]int

	// stop ends the janitor. Every goroutine gets a lifecycle.
	stopOnce sync.Once
	stop     chan struct{}
}

// NewTicketStore builds a store over the shared database and starts its expiry janitor.
func NewTicketStore(database *sql.DB, logger *slog.Logger) *TicketStore {
	store := &TicketStore{
		database: database,
		logger:   logger,
		viewers:  make(map[string]map[string]int),
		stop:     make(chan struct{}),
	}
	go store.janitor()
	return store
}

// digestOf is what the table holds in place of the ticket value.
func digestOf(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func nowText() string { return time.Now().UTC().Format(time.RFC3339) }

// janitor drops expired rows.
//
// Redemption already refuses an expired ticket, so this is only about not
// accumulating dead rows. Any node may run it: once a row has expired it is
// nobody's.
func (s *TicketStore) janitor() {
	ticker := time.NewTicker(ticketTTL)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if _, err := s.database.ExecContext(ctx,
				"DELETE FROM console_tickets WHERE expires_at < ?", nowText()); err != nil {
				s.logger.Warn("compute console: pruning expired tickets failed", "error", err)
			}
			cancel()
		}
	}
}

// Close stops the janitor.
func (s *TicketStore) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// Issue mints a ticket. The returned string is the only time the value exists.
func (s *TicketStore) Issue(ctx context.Context, ticket Ticket) (string, error) {
	ticket.Session = ""
	now := time.Now().UTC()
	ticket.IssuedAt = now
	ticket.ExpiresAt = now.Add(ticketTTL)
	return s.store(ctx, ticket, "")
}

// IssueBatch mints n channel tickets for one console session, all bound to the
// same user, guest, kind and expiry, for a client that opens a socket per channel.
//
// Each value is the session id and a secret joined by a dot. The table holds the
// digest of the whole value, so the session cannot be swapped without the lookup
// missing; that is what lets redemption trust the prefix without a new column.
func (s *TicketStore) IssueBatch(ctx context.Context, ticket Ticket, n int) ([]string, error) {
	if n < 1 || n > maxChannelsPerSession {
		return nil, fmt.Errorf("compute console: a session takes 1 to %d channel tickets, not %d", maxChannelsPerSession, n)
	}
	now := time.Now().UTC()
	ticket.Session = xid.New().String()
	ticket.IssuedAt = now
	ticket.ExpiresAt = now.Add(ticketTTL)

	values := make([]string, 0, n)
	for range n {
		value, err := s.store(ctx, ticket, ticket.Session+sessionSeparator)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

// store writes one ticket row and returns its value, prefix first.
func (s *TicketStore) store(ctx context.Context, ticket Ticket, prefix string) (string, error) {
	// 32 bytes from crypto/rand. math/rand would be predictable enough to guess
	// given the 30-second window and the small keyspace a weaker source would
	// imply.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("compute console: generate ticket: %w", err)
	}
	value := prefix + base64.RawURLEncoding.EncodeToString(raw)
	ticket.value = value

	command := ""
	if len(ticket.Command) > 0 {
		encoded, err := json.Marshal(ticket.Command)
		if err != nil {
			return "", fmt.Errorf("compute console: encode ticket command: %w", err)
		}
		command = string(encoded)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.boundPerUser(ctx, ticket.UserID); err != nil {
		s.logger.Warn("compute console: bounding outstanding tickets failed", "error", err)
	}
	if _, err := s.database.ExecContext(ctx, `INSERT INTO console_tickets (id, digest, user_id, environment_id,
		guest_kind, guest_id, console_kind, command, issued_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), digestOf(value), ticket.UserID, ticket.EnvID, ticket.GuestKind, ticket.GuestID,
		ticket.Kind, command, ticket.IssuedAt.Format(time.RFC3339), ticket.ExpiresAt.Format(time.RFC3339)); err != nil {
		return "", fmt.Errorf("compute console: storing ticket: %w", err)
	}
	return value, nil
}

// boundPerUser drops this user's oldest unredeemed tickets past the cap, so a
// client bug that requests tickets in a loop degrades rather than locking the
// user out of their console.
func (s *TicketStore) boundPerUser(ctx context.Context, userID string) error {
	var outstanding int
	if err := s.database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM console_tickets WHERE user_id = ?", userID).Scan(&outstanding); err != nil {
		return fmt.Errorf("compute console: counting tickets: %w", err)
	}
	if outstanding < maxTicketsPerUser {
		return nil
	}
	stale, err := s.oldestDigests(ctx, userID, outstanding-maxTicketsPerUser+1)
	if err != nil {
		return err
	}
	// bounded by the tickets over maxTicketsPerUser, usually one; an incomplete eviction repeats on the next issue
	for _, digest := range stale {
		if _, err := s.database.ExecContext(ctx, "DELETE FROM console_tickets WHERE digest = ?", digest); err != nil {
			return fmt.Errorf("compute console: dropping an old ticket: %w", err)
		}
	}
	return nil
}

// oldestDigests reads the n oldest ticket digests for userID and closes the cursor before returning, so the deletes that follow are not blocked by an open read.
func (s *TicketStore) oldestDigests(ctx context.Context, userID string, n int) (stale []string, err error) {
	rows, err := s.database.QueryContext(ctx,
		"SELECT digest FROM console_tickets WHERE user_id = ? ORDER BY issued_at ASC LIMIT ?", userID, n)
	if err != nil {
		return nil, fmt.Errorf("compute console: listing oldest tickets: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			stale, err = nil, fmt.Errorf("compute console: listing oldest tickets: %w", closeErr)
		}
	}()
	for rows.Next() {
		var digest string
		if err := rows.Scan(&digest); err != nil {
			return nil, fmt.Errorf("compute console: scanning ticket: %w", err)
		}
		stale = append(stale, digest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compute console: listing oldest tickets: %w", err)
	}
	return stale, nil
}

// Redeem consumes a ticket, returning what it authorizes.
//
// The DELETE is the arbiter, not the SELECT: two nodes redeeming the same value
// at the same moment both read the row and exactly one deletes it, so the second
// is refused even though it read a valid ticket. Single use is the point — a
// ticket that survived its first use would let a captured URL be replayed for the
// whole TTL rather than losing the race.
func (s *TicketStore) Redeem(ctx context.Context, value string) (Ticket, error) {
	if value == "" {
		return Ticket{}, ErrTicketInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	digest := digestOf(value)
	var ticket Ticket
	var command, issued, expires string
	err := s.database.QueryRowContext(ctx, `SELECT user_id, environment_id, guest_kind, guest_id, console_kind,
		command, issued_at, expires_at FROM console_tickets WHERE digest = ?`, digest).
		Scan(&ticket.UserID, &ticket.EnvID, &ticket.GuestKind, &ticket.GuestID, &ticket.Kind,
			&command, &issued, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Ticket{}, ErrTicketInvalid
	}
	if err != nil {
		return Ticket{}, fmt.Errorf("compute console: reading ticket: %w", err)
	}

	consumed, err := s.consume(ctx, digest)
	if err != nil {
		return Ticket{}, err
	}
	if !consumed {
		return Ticket{}, ErrTicketInvalid
	}

	issuedAt, issuedErr := time.Parse(time.RFC3339, issued)
	if issuedErr != nil {
		s.logger.Warn("compute console: ticket issued_at is not RFC3339", "error", issuedErr)
	}
	ticket.IssuedAt = issuedAt
	expiresAt, err := time.Parse(time.RFC3339, expires)
	if err != nil || time.Now().UTC().After(expiresAt) {
		return Ticket{}, ErrTicketInvalid
	}
	ticket.ExpiresAt = expiresAt
	if session, _, batched := strings.Cut(value, sessionSeparator); batched {
		ticket.Session = session
	}
	if strings.TrimSpace(command) != "" {
		if err := json.Unmarshal([]byte(command), &ticket.Command); err != nil {
			return Ticket{}, fmt.Errorf("compute console: decoding ticket command: %w", err)
		}
	}
	return ticket, nil
}

// consume deletes the row and reports whether this caller was the one that deleted it.
//
// It is the only thing standing between two nodes that both read the same valid ticket
// and two live console sessions on one credential. Kept as its own method so the property
// can be tested directly: reading the row first and then deleting it is unavoidable —
// MySQL has no DELETE ... RETURNING — so the row count is the arbitration.
func (s *TicketStore) consume(ctx context.Context, digest string) (bool, error) {
	res, err := s.database.ExecContext(ctx, "DELETE FROM console_tickets WHERE digest = ?", digest)
	if err != nil {
		return false, fmt.Errorf("compute console: consuming ticket: %w", err)
	}
	return db.RowsAffected(res) == 1, nil
}

// guestKey identifies a guest for the viewer count.
func guestKey(envID, guestKind, guestID string) string {
	return envID + "/" + guestKind + "/" + guestID
}

// AddViewer registers one open channel of a session on this node.
//
// The cap counts sessions, not sockets: a SPICE client's display or inputs
// channel joining a session that is already admitted is never refused, and a
// new session is refused once the guest has the maximum. An empty session is
// a single-socket console and counts as a session of its own; the returned
// key is what RemoveViewer takes back.
func (s *TicketStore) AddViewer(envID, guestKind, guestID, session string) (string, error) {
	key := guestKey(envID, guestKind, guestID)
	if session == "" {
		session = "single:" + xid.New().String()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := s.viewers[key]
	if sessions[session] == 0 && len(sessions) >= maxViewersPerGuest {
		return "", ErrTooManyViewers
	}
	if sessions == nil {
		sessions = make(map[string]int)
		s.viewers[key] = sessions
	}
	sessions[session]++
	return session, nil
}

// RemoveViewer closes one channel of a session; the session stops counting
// when its last channel closes.
func (s *TicketStore) RemoveViewer(envID, guestKind, guestID, session string) {
	key := guestKey(envID, guestKind, guestID)

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := s.viewers[key]
	if sessions[session] <= 1 {
		delete(sessions, session)
	} else {
		sessions[session]--
	}
	if len(sessions) == 0 {
		delete(s.viewers, key)
	}
}

// Viewers reports how many sessions this node has open on a guest, for the UI badge.
func (s *TicketStore) Viewers(envID, guestKind, guestID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.viewers[guestKey(envID, guestKind, guestID)])
}
