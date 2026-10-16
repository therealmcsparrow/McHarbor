// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// eventQueueDepth bounds the buffer between the transport read loop and the
// worker. Lifecycle events are rare; a depth this size is only ever reached if
// the worker has stalled, and dropping then is better than stalling the agent.
const eventQueueDepth = 64

// eventApplyTimeout bounds one state write.
const eventApplyTimeout = 10 * time.Second

// guestEventWorker applies agent guest events to the VM table.
//
// It exists because the host calls the sink on the agent transport's read loop:
// writing to SQLite there would stall every other message from that agent, so
// events are handed to one owned goroutine instead.
type guestEventWorker struct {
	svc    *Service
	logger pluginLogger
	events chan GuestEvent
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

func newGuestEventWorker(svc *Service, logger pluginLogger) *guestEventWorker {
	return &guestEventWorker{
		svc:    svc,
		logger: logger,
		events: make(chan GuestEvent, eventQueueDepth),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start runs the worker until Stop is called.
func (w *guestEventWorker) Start() {
	go func() {
		defer close(w.done)
		for {
			select {
			case <-w.stop:
				return
			case event := <-w.events:
				w.apply(event)
			}
		}
	}()
}

// Stop ends the worker and waits for the in-flight event to finish.
func (w *guestEventWorker) Stop() {
	w.once.Do(func() {
		close(w.stop)
		<-w.done
	})
}

// Enqueue is the sink handed to the host. It never blocks: a full queue drops
// the event and says so, because the alternative is stalling the transport.
func (w *guestEventWorker) Enqueue(event GuestEvent) {
	select {
	case w.events <- event:
	default:
		w.logger.Warn("kvm: guest event dropped, queue full", "guest", event.GuestID, "kind", event.Kind)
	}
}

// applyPathMove rewrites every disk row that named a file the host moved.
func (w *guestEventWorker) applyPathMove(event GuestEvent) {
	from, to := event.Detail["from"], event.Detail["to"]
	if from == "" || to == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), eventApplyTimeout)
	defer cancel()
	if _, err := w.svc.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_disks SET volume_path = ?, updated_at = ?
		 WHERE volume_path = ?`,
		to, time.Now().UTC().Format(time.RFC3339), from); err != nil {
		w.logger.Error("kvm: recording a moved file's new path failed; "+
			"a guest attached to it will not start",
			"from", from, "to", to, "error", err)
	}
}

// applyVolumeMove records a disk's new path after a location change.
//
// Matched on guest and slot rather than on the old path: by the time this
// arrives the file is already gone from where the row says it is, and a guest
// with two disks in one pool produces two of these.
func (w *guestEventWorker) applyVolumeMove(event GuestEvent) {
	to := event.Detail["to"]
	if to == "" {
		return
	}
	slot, err := strconv.Atoi(event.Detail["slot"])
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), eventApplyTimeout)
	defer cancel()
	if _, err := w.svc.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_disks SET volume_path = ?, updated_at = ?
		 WHERE vm_id = ? AND slot = ?`,
		to, time.Now().UTC().Format(time.RFC3339), event.GuestID, slot); err != nil {
		w.logger.Error("kvm: recording a disk's new path after a relocation failed; "+
			"the guest will start from the old one",
			"guest", event.GuestID, "slot", slot, "path", to, "error", err)
		return
	}
	w.logger.Info("kvm: a disk moved with its location",
		"guest", event.GuestID, "slot", slot, "path", to)
}

// apply maps one QMP-derived event onto the stored state.
//
// Only events that genuinely settle a guest's state are applied. This is what
// finishes a shutdown: the server marks a guest "stopping" when it asks, and
// without this nothing ever moved it to "stopped", so a guest that had already
// gone stayed "shutting down" until somebody pressed Stop.
func (w *guestEventWorker) apply(event GuestEvent) {
	// A file moved with a location change. It names no guest — an ISO in the
	// image root is attached to as many as mounted it — so the rows are found
	// by the path they still carry.
	if event.Kind == "path_moved" {
		w.applyPathMove(event)
		return
	}
	if event.GuestKind != guestKindVM || event.GuestID == "" {
		return
	}

	// Kept before the state is applied, so the incident scan can tell a guest
	// that powered itself off from one the host killed, which state alone cannot.
	w.svc.recordVMEvent(event)

	// Delete progress is not a state change: it updates a number the list and
	// the dialog read, and the row keeps saying "deleting" until it is gone.
	if event.Kind == "delete_progress" {
		w.svc.recordDeleteProgress(event.GuestID, event.Detail["percent"], event.Detail["step"])
		return
	}

	if event.Kind == "snapshot_progress" {
		w.svc.recordSnapshotProgress(event.GuestID, event.Detail["percent"], event.Detail["step"])
		return
	}

	// A clone reports under the clone's id and a migration under the guest's;
	// neither is a state change.
	if event.Kind == "clone_progress" {
		w.svc.recordCloneProgress(event.GuestID, event.Detail["percent"], event.Detail["step"])
		return
	}
	if event.Kind == "migrate_progress" {
		w.svc.recordMigrationProgress(event.GuestID, event.Detail)
		return
	}

	if event.Kind == "backup_progress" {
		w.svc.recordBackupProgress(event.Detail)
		return
	}

	// A relocation moved this guest's disk. The host is the only thing that
	// knows the new path, and this row is the only thing that will look for
	// the disk at the next start.
	if event.Kind == "volume_moved" {
		w.applyVolumeMove(event)
		return
	}

	var state string
	switch event.Kind {
	case "shutdown", "powerdown":
		state = StateStopped
	case "stop":
		// QMP STOP is the vCPUs halting, which is a pause rather than a power
		// off — a guest paused by a block I/O error reports this too.
		state = StatePaused
	case "resume":
		state = StateRunning
	case "guest_panic":
		// Left in whatever state it is: the guest is wedged, not stopped, and
		// overwriting the state would hide that from the operator.
		w.logger.Warn("kvm: guest panicked", "guest", event.GuestID)
		return
	default:
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), eventApplyTimeout)
	defer cancel()
	if err := w.svc.setState(ctx, event.GuestID, state, ""); err != nil {
		w.logger.Warn("kvm: applying guest event failed",
			"guest", event.GuestID, "kind", event.Kind, "error", err)
		return
	}
	w.logger.Info("kvm: guest state updated from agent event",
		"guest", event.GuestID, "kind", event.Kind, "state", state)
}
