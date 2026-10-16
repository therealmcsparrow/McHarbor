// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// offlineBatchSize bounds one replay message. 500 metric rows serialise to
// roughly 150 KB, far under the server's 17 MB read limit, while keeping the
// number of round trips for a long outage in the tens rather than thousands.
const offlineBatchSize = 500

// offlineAckTimeout is how long the agent waits for the server to confirm a
// batch. A server too old to know the message never answers, in which case
// the data stays local and the next connection tries again.
const offlineAckTimeout = 30 * time.Second

// offlineAcks routes offline_batch_ack messages from the connection's read
// loop to the goroutine waiting on that batch.
type offlineAcks struct {
	mu      sync.Mutex
	waiters map[string]chan *OfflineBatchPayload
}

func newOfflineAcks() *offlineAcks {
	return &offlineAcks{waiters: make(map[string]chan *OfflineBatchPayload)}
}

func (a *offlineAcks) register(batchID string) (chan *OfflineBatchPayload, func()) {
	ch := make(chan *OfflineBatchPayload, 1)
	a.mu.Lock()
	a.waiters[batchID] = ch
	a.mu.Unlock()
	return ch, func() {
		a.mu.Lock()
		delete(a.waiters, batchID)
		a.mu.Unlock()
	}
}

// deliver hands an ack to its waiter. Unknown batch ids (a late ack after the
// waiter timed out) are dropped.
func (a *offlineAcks) deliver(batchID string, payload *OfflineBatchPayload) {
	a.mu.Lock()
	ch := a.waiters[batchID]
	a.mu.Unlock()
	if ch == nil || payload == nil {
		return
	}
	select {
	case ch <- payload:
	default:
	}
}

// flushOffline replays everything in the local store to the server over conn,
// deleting each batch only after the server acknowledged it. Events go first
// so the lifecycle log is complete before metrics fill in, and the agent's own
// log goes last because it is the least time-sensitive.
func (a *Agent) flushOffline(ctx context.Context, conn *websocket.Conn) {
	if a.offline == nil {
		return
	}
	store := a.offline.Store()
	logs, events, metrics := store.Counts(ctx)
	if logs+events+metrics == 0 {
		return
	}
	a.logger.Info("replaying offline buffer", "logs", logs, "events", events, "metrics", metrics)

	start := time.Now()
	var sent int64
	for _, kind := range []string{offlineKindEvents, offlineKindMetrics, offlineKindLogs} {
		n, err := a.flushKind(ctx, conn, store, kind)
		sent += n
		if err != nil {
			a.logger.Warn("offline replay stopped; remaining rows stay buffered", "kind", kind, "error", err)
			return
		}
	}
	a.logger.Info("offline buffer replayed", "rows", sent, "duration", time.Since(start).Round(time.Millisecond))
}

func (a *Agent) flushKind(ctx context.Context, conn *websocket.Conn, store *OfflineStore, kind string) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		payload, ids, err := store.NextBatch(ctx, kind, offlineBatchSize)
		if err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		payload.BatchID = newOfflineID()

		ackCh, release := a.offlineAcks.register(payload.BatchID)
		writeMu.Lock()
		err = conn.WriteJSON(WSMessage{Type: MsgOfflineBatch, ID: payload.BatchID, Offline: &payload})
		writeMu.Unlock()
		if err != nil {
			release()
			return total, fmt.Errorf("sending offline batch: %w", err)
		}

		var ack *OfflineBatchPayload
		select {
		case <-ctx.Done():
			release()
			return total, ctx.Err()
		case <-time.After(offlineAckTimeout):
			release()
			return total, fmt.Errorf("no acknowledgement for batch %s within %s (server may predate offline replay)", payload.BatchID, offlineAckTimeout)
		case ack = <-ackCh:
			release()
		}
		if !ack.Accepted {
			return total, fmt.Errorf("server rejected batch %s: %s", payload.BatchID, ack.Error)
		}
		if err := store.DeleteIDs(ctx, kind, ids); err != nil {
			return total, err
		}
		total += int64(len(ids))
		a.logger.Debug("offline batch acknowledged", "kind", kind, "rows", len(ids), "remaining", payload.Remaining)
	}
}
