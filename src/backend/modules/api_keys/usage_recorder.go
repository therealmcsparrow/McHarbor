// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/apikeyusage"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

const (
	// usageQueueCapacity bounds the in-memory backlog. Sized so a burst
	// lasting a few seconds is absorbed without touching the request path,
	// and so a wedged database costs a few megabytes rather than the heap.
	usageQueueCapacity = 4096

	// usageBatchSize caps one transaction. SQLite is far happier with one
	// transaction of a hundred inserts than a hundred transactions.
	usageBatchSize = 200

	// usageFlushInterval is how long a partial batch waits for company.
	// Short enough that the history looks live, long enough to batch.
	usageFlushInterval = 500 * time.Millisecond

	// usageDropLogInterval rate-limits the dropped-event warning so a
	// sustained overload cannot itself become the load.
	usageDropLogInterval = time.Minute
)

// UsageRecorder is the bounded asynchronous writer behind the usage Sink.
//
// The contract it keeps is availability, not completeness: this is
// operational telemetry, and a database stall must never turn into request
// latency. Record therefore never blocks — a full queue drops the event and
// increments a counter that is logged periodically, so the gap is visible
// rather than silent.
//
// One goroutine owns all writes; batches commit in a single transaction
// through the repository's busy-retry helper, since SQLite will reject a
// concurrent writer rather than wait.
type UsageRecorder struct {
	db     *sql.DB
	logger *slog.Logger

	queue chan apikeyusage.Event

	dropped     atomic.Int64
	lastDropLog atomic.Int64 // unix nanos

	startOnce sync.Once
	stopOnce  sync.Once
	done      chan struct{}
	stopped   chan struct{}
}

// NewUsageRecorder builds a recorder. It does not start until Start is
// called, so a caller may wire it before the background context exists.
func NewUsageRecorder(db *sql.DB, logger *slog.Logger) *UsageRecorder {
	queue := make(chan apikeyusage.Event, usageQueueCapacity)
	telemetry.RegisterQueue("api_key_usage",
		func() float64 { return float64(len(queue)) },
		func() float64 { return float64(cap(queue)) })
	return &UsageRecorder{
		db:      db,
		logger:  logger,
		queue:   queue,
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

// Record implements apikeyusage.Sink. It is called on the request goroutine
// and must not block, so a full queue drops.
func (rec *UsageRecorder) Record(event apikeyusage.Event) {
	if rec == nil || event.APIKeyID == "" {
		return
	}

	select {
	case rec.queue <- event:
	default:
		rec.noteDrop()
	}
}

// noteDrop counts a dropped event and logs at most once per interval.
func (rec *UsageRecorder) noteDrop() {
	total := rec.dropped.Add(1)
	now := time.Now().UnixNano()
	last := rec.lastDropLog.Load()
	if now-last < int64(usageDropLogInterval) {
		return
	}
	if !rec.lastDropLog.CompareAndSwap(last, now) {
		return
	}
	if rec.logger != nil {
		rec.logger.Warn("api key usage: event queue full, dropping telemetry",
			"droppedTotal", total, "capacity", usageQueueCapacity)
	}
}

// Start launches the writer goroutine. Safe to call more than once.
func (rec *UsageRecorder) Start(ctx context.Context) {
	if rec == nil {
		return
	}
	rec.startOnce.Do(func() {
		go rec.run(ctx)
	})
}

// Stop signals the writer to drain and waits briefly for it. Called from a
// deferred shutdown hook, so the last few requests before a restart are not
// lost silently.
func (rec *UsageRecorder) Stop() {
	if rec == nil {
		return
	}
	rec.stopOnce.Do(func() {
		close(rec.done)
	})

	select {
	case <-rec.stopped:
	case <-time.After(5 * time.Second):
		if rec.logger != nil {
			rec.logger.Warn("api key usage: writer did not drain before shutdown deadline",
				"queued", len(rec.queue))
		}
	}
}

// run is the single writer. It batches by size or by tick, whichever comes
// first, and drains what it can on shutdown.
func (rec *UsageRecorder) run(ctx context.Context) {
	defer close(rec.stopped)

	ticker := time.NewTicker(usageFlushInterval)
	defer ticker.Stop()

	batch := make([]apikeyusage.Event, 0, usageBatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		rec.write(ctx, batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			rec.drain(ctx, &batch)
			flush()
			return
		case <-rec.done:
			rec.drain(ctx, &batch)
			flush()
			return
		case event := <-rec.queue:
			batch = append(batch, event)
			if len(batch) >= usageBatchSize {
				flush()
			}
		case <-ticker.C:
			quiesce.Run(flush)
		}
	}
}

// drain moves whatever is already queued into the batch without waiting,
// so shutdown does not discard events that have already been accepted.
func (rec *UsageRecorder) drain(ctx context.Context, batch *[]apikeyusage.Event) {
	for {
		select {
		case event := <-rec.queue:
			*batch = append(*batch, event)
			if len(*batch) >= usageBatchSize {
				rec.write(ctx, *batch)
				*batch = (*batch)[:0]
			}
		default:
			return
		}
	}
}

// write commits one batch. A failure is logged and the batch discarded:
// retrying telemetry forever would trade the availability this component
// exists to protect for completeness it never promised.
func (rec *UsageRecorder) write(ctx context.Context, batch []apikeyusage.Event) {
	if len(batch) == 0 || rec.db == nil {
		return
	}

	// The shutdown flush runs after ctx is cancelled, and it must still commit accepted events.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	err := coredb.RetryOnBusy(ctx, 5, func() error {
		return rec.writeOnce(ctx, batch)
	})
	if err != nil && rec.logger != nil {
		rec.logger.Error("api key usage: batch insert failed",
			"error", err, "events", len(batch))
	}
}

func (rec *UsageRecorder) writeOnce(ctx context.Context, batch []apikeyusage.Event) (err error) {
	tx, err := rec.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning usage batch: %w", err)
	}
	defer func() {
		if err != nil {
			coredb.RollbackTx(tx)
		}
	}()

	stmt, prepErr := tx.PrepareContext(ctx, `
		INSERT INTO api_key_usage_events (
			id, api_key_id, occurred_at, method, route_pattern, status_code,
			duration_ms, auth_outcome, environment_id, peer_ip, client_ip,
			client_ip_source, user_agent, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if prepErr != nil {
		err = fmt.Errorf("preparing usage insert: %w", prepErr)
		return err
	}
	defer func() { logCloseErr(stmt.Close()) }()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, event := range batch {
		outcome := event.Outcome
		if !outcome.Valid() {
			outcome = apikeyusage.OutcomeAccepted
		}

		var envID any
		if event.EnvironmentID != "" {
			envID = event.EnvironmentID
		}

		if _, execErr := stmt.ExecContext(ctx,
			xid.New().String(),
			event.APIKeyID,
			event.OccurredAt.UTC().Format(time.RFC3339),
			event.Method,
			apikeyusage.NormalizeRoutePattern(event.RoutePattern),
			event.StatusCode,
			event.Duration.Milliseconds(),
			string(outcome),
			envID,
			event.PeerIP,
			event.ClientIP,
			event.ClientIPSource,
			apikeyusage.SanitizeUserAgent(event.UserAgent),
			now,
			now,
		); execErr != nil {
			err = fmt.Errorf("inserting usage event: %w", execErr)
			return err
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("committing usage batch: %w", commitErr)
		return err
	}
	return nil
}

// Dropped reports how many events have been discarded for lack of queue
// space since start. Exposed so a future health surface can show it.
func (rec *UsageRecorder) Dropped() int64 {
	if rec == nil {
		return 0
	}
	return rec.dropped.Load()
}
