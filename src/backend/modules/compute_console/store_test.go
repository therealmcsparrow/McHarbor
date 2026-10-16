// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compute_console

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

func store(t *testing.T) (*TicketStore, *sql.DB) {
	t.Helper()
	database, err := db.Open(db.Config{Driver: db.DriverSQLite, Path: filepath.Join(t.TempDir(), "c.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	s := NewTicketStore(database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(s.Close)
	return s, database
}

func aTicket() Ticket {
	return Ticket{UserID: "user-1", EnvID: "env-1", GuestKind: "ct", GuestID: "100", Kind: "exec",
		Command: []string{"/bin/sh"}}
}

// TestTicketIsSingleUse is the property the whole design rests on. Moving the store from
// a map on one node to a shared table is exactly the change that could have lost it.
func TestTicketIsSingleUse(t *testing.T) {
	s, _ := store(t)
	ctx := context.Background()

	value, err := s.Issue(ctx, aTicket())
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.Redeem(ctx, value)
	if err != nil {
		t.Fatalf("the first redemption must succeed: %v", err)
	}
	if first.UserID != "user-1" || first.EnvID != "env-1" || first.GuestID != "100" || first.Kind != "exec" {
		t.Fatalf("a ticket must authorize exactly what it was issued for, got %+v", first)
	}
	if len(first.Command) != 1 || first.Command[0] != "/bin/sh" {
		t.Fatalf("the server-chosen command must survive the round trip, got %v", first.Command)
	}

	if _, err := s.Redeem(ctx, value); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("a redeemed ticket must never be redeemable again, got %v", err)
	}
}

// TestConcurrentRedemptionYieldsOneWinner is the same property under the race the shared
// table introduced: two nodes upgrading the same console URL at the same moment.
func TestConcurrentRedemptionYieldsOneWinner(t *testing.T) {
	s, database := store(t)
	ctx := context.Background()
	// A second store over the same database stands in for a second node.
	other := NewTicketStore(database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(other.Close)

	value, err := s.Issue(ctx, aTicket())
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]error, 2)
	stores := []*TicketStore{s, other}
	start := make(chan struct{})
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, results[i] = stores[i].Redeem(ctx, value)
		}(i)
	}
	close(start)
	wg.Wait()

	won := 0
	for _, err := range results {
		if err == nil {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("exactly one of two nodes must redeem a ticket, %d did", won)
	}
}

// TestOnlyOneCallerConsumesATicket tests the arbitration on its own, because the
// interesting case cannot be reached through Redeem: two nodes that both passed the SELECT
// and then both reach the DELETE. Exactly one row count of 1 is what stops the second from
// opening a console on a credential that was already spent.
func TestOnlyOneCallerConsumesATicket(t *testing.T) {
	s, _ := store(t)
	ctx := context.Background()
	value, err := s.Issue(ctx, aTicket())
	if err != nil {
		t.Fatal(err)
	}
	digest := digestOf(value)

	first, err := s.consume(ctx, digest)
	if err != nil || !first {
		t.Fatalf("the first caller must consume the ticket, got %v (%v)", first, err)
	}
	second, err := s.consume(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}
	if second {
		t.Fatal("a second caller must not consume a ticket that was already spent")
	}
}

// TestUnknownAndExpiredTicketsAreIndistinguishable keeps the refusal from confirming that
// a captured value was once real.
func TestUnknownAndExpiredTicketsAreIndistinguishable(t *testing.T) {
	s, database := store(t)
	ctx := context.Background()

	if _, err := s.Redeem(ctx, "never-issued"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("an unknown ticket must be ErrTicketInvalid, got %v", err)
	}
	if _, err := s.Redeem(ctx, ""); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("an empty ticket must be ErrTicketInvalid, got %v", err)
	}

	value, err := s.Issue(ctx, aTicket())
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if _, err := database.Exec("UPDATE console_tickets SET expires_at = ? WHERE digest = ?", past, digestOf(value)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Redeem(ctx, value); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("an expired ticket must be ErrTicketInvalid, got %v", err)
	}
}

// TestTicketValueIsNotStored guards the reason the table holds a digest: a database dump
// must not contain a usable console credential.
func TestTicketValueIsNotStored(t *testing.T) {
	s, database := store(t)
	value, err := s.Issue(context.Background(), aTicket())
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM console_tickets WHERE digest = ?", value).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("the ticket value must never be stored, only a digest of it")
	}
}

// TestOutstandingTicketsAreBounded proves a client bug that loops on ticket requests
// degrades rather than growing the table without limit.
func TestOutstandingTicketsAreBounded(t *testing.T) {
	s, database := store(t)
	ctx := context.Background()
	for i := 0; i < maxTicketsPerUser+5; i++ {
		if _, err := s.Issue(ctx, aTicket()); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM console_tickets WHERE user_id = ?", "user-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count > maxTicketsPerUser {
		t.Fatalf("outstanding tickets must stay bounded at %d, found %d", maxTicketsPerUser, count)
	}
}
