// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"errors"
	"time"

	"modernc.org/sqlite"
)

// SQLite result codes that mean "another connection got there first".
//
// SQLITE_BUSY_SNAPSHOT is the important one: a deferred transaction that
// has already read cannot be upgraded to a writer once another connection
// has committed, so SQLite fails it immediately. The busy handler is
// deliberately NOT invoked for that code — waiting cannot help, because the
// read snapshot is permanently stale — which is why `_busy_timeout` does not
// save a read-then-write transaction. The only remedy is to roll back and
// run the whole unit of work again.
const (
	sqliteBusy              = 5
	sqliteLocked            = 6
	sqliteLockedSharedCache = 261
	sqliteBusySnapshot      = 517
)

// IsBusy reports whether err is transient write contention that re-running
// the transaction can resolve. MySQL and Postgres resolve this contention
// inside the server, so their drivers never match and always report false.
func IsBusy(err error) bool {
	var serr *sqlite.Error
	if !errors.As(err, &serr) {
		return false
	}
	switch serr.Code() {
	case sqliteBusy, sqliteLocked, sqliteLockedSharedCache, sqliteBusySnapshot:
		return true
	default:
		return false
	}
}

// RetryOnBusy runs fn until it succeeds, fails with a non-busy error, or the
// attempt budget runs out, backing off 5ms and doubling between attempts.
//
// fn must be a complete unit of work — open the transaction, do the reads and
// writes, commit — because a busy failure discards everything the previous
// attempt read. Returns the last error when every attempt was contended, and
// ctx.Err() if the context is cancelled while backing off.
func RetryOnBusy(ctx context.Context, attempts int, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	delay := 5 * time.Millisecond
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		err = fn()
		if !IsBusy(err) {
			return err
		}
		if attempt == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return err
}
