// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// The cold-migration relay.
//
// Agents connect outbound only, so two of them cannot reach each other; both
// can reach McHarbor. The source PUTs a disk here and the target GETs it, and
// the relay joins the two requests with a synchronous pipe: nothing is written
// to McHarbor's disk and nothing is buffered beyond one copy chunk, so the
// slower host sets the pace and a 500 GB disk costs McHarbor no memory.
//
// Each disk gets a grant with two tokens, one per direction, each usable once.
// The source only ever sees the send token and the target the receive token.

// migrationRelayPath is where agents redeem relay grants. A path rather than a
// URL: only the agent knows which address it reaches McHarbor on.
const migrationRelayPath = "/api/vms/migrations/internal/"

// relayGrantTTL bounds how long an unredeemed grant stays usable.
const relayGrantTTL = 2 * time.Hour

// relayWaitTimeout bounds how long one side waits for the other to arrive. The
// source may spend a while flattening a disk before its upload begins.
const relayWaitTimeout = 2 * time.Hour

var (
	// errRelayGrant covers an unknown grant, a wrong token and a reused one.
	errRelayGrant = errors.New("relay: grant not valid")
	// errRelayTimeout means the other half never arrived.
	errRelayTimeout = errors.New("relay: the other host never connected")
)

// relaySlot is one disk's rendezvous.
type relaySlot struct {
	sendToken    string
	receiveToken string
	expires      time.Time
	sendUsed     bool
	receiveUsed  bool

	// format and size are set by the upload before ready closes, and read by
	// the download only after it has seen ready closed.
	format string
	size   int64

	reader *io.PipeReader
	writer *io.PipeWriter
	// ready closes when the upload has arrived, attached when the download has.
	ready    chan struct{}
	attached chan struct{}
	once     sync.Once
}

// abort ends both sides of a slot with a reason.
func (s *relaySlot) abort(cause error) {
	s.once.Do(func() {
		// CloseWithError on an io.Pipe end always returns nil; both ends are
		// closed so whichever side is blocked wakes with the cause.
		if err := errors.Join(s.writer.CloseWithError(cause), s.reader.CloseWithError(cause)); err != nil {
			return
		}
	})
}

// migrationRelay holds the live grants.
type migrationRelay struct {
	mu    sync.Mutex
	slots map[string]*relaySlot
}

// newMigrationRelay builds an empty relay.
func newMigrationRelay() *migrationRelay {
	return &migrationRelay{slots: make(map[string]*relaySlot)}
}

// relayGrant is what a migration hands its two agents for one disk.
type relayGrant struct {
	id           string
	sendToken    string
	receiveToken string
}

// issue opens a slot for one disk.
func (r *migrationRelay) issue() (relayGrant, error) {
	id, err := newSecret(16)
	if err != nil {
		return relayGrant{}, err
	}
	sendToken, err := newSecret(32)
	if err != nil {
		return relayGrant{}, err
	}
	receiveToken, err := newSecret(32)
	if err != nil {
		return relayGrant{}, err
	}
	reader, writer := io.Pipe()
	slot := &relaySlot{
		sendToken: sendToken, receiveToken: receiveToken,
		expires: time.Now().Add(relayGrantTTL),
		reader:  reader, writer: writer,
		ready: make(chan struct{}), attached: make(chan struct{}),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	r.slots[id] = slot
	return relayGrant{id: id, sendToken: sendToken, receiveToken: receiveToken}, nil
}

// redeem hands out a slot for one direction, once. Constant-time, because the
// relay routes are unauthenticated and the token is the only credential.
func (r *migrationRelay) redeem(id, token string, send bool) (*relaySlot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	slot, ok := r.slots[id]
	if !ok || token == "" {
		return nil, errRelayGrant
	}
	want, used := slot.receiveToken, &slot.receiveUsed
	if send {
		want, used = slot.sendToken, &slot.sendUsed
	}
	if *used || subtle.ConstantTimeCompare([]byte(want), []byte(token)) != 1 {
		return nil, errRelayGrant
	}
	*used = true
	return slot, nil
}

// revoke closes a slot, ending any transfer still using it.
func (r *migrationRelay) revoke(id string) {
	r.mu.Lock()
	slot, ok := r.slots[id]
	delete(r.slots, id)
	r.mu.Unlock()
	if ok {
		slot.abort(errors.New("relay: the migration was cancelled"))
	}
}

// sweepLocked drops expired slots. Called under the lock.
func (r *migrationRelay) sweepLocked() {
	now := time.Now()
	for id, slot := range r.slots {
		if now.After(slot.expires) {
			slot.abort(errRelayTimeout)
			delete(r.slots, id)
		}
	}
}

// upload feeds one disk into its slot, waiting for the download to attach.
func (r *migrationRelay) upload(ctx context.Context, slot *relaySlot, format string, size int64, body io.Reader) (int64, error) {
	format = strings.TrimSpace(format)
	if format != "qcow2" && format != "raw" {
		format = "raw"
	}
	slot.format, slot.size = format, size
	close(slot.ready)

	wait := time.NewTimer(relayWaitTimeout)
	defer wait.Stop()
	select {
	case <-slot.attached:
	case <-wait.C:
		slot.abort(errRelayTimeout)
		return 0, errRelayTimeout
	case <-ctx.Done():
		slot.abort(ctx.Err())
		return 0, ctx.Err()
	}

	written, err := io.Copy(slot.writer, body)
	if err != nil {
		slot.abort(err)
		return written, err
	}
	if size > 0 && written != size {
		err := errors.New("relay: the upload ended before its announced length")
		slot.abort(err)
		return written, err
	}
	return written, slot.writer.Close()
}

// download waits for the upload and returns the stream with its format and size.
func (r *migrationRelay) download(ctx context.Context, slot *relaySlot) (io.Reader, string, int64, error) {
	wait := time.NewTimer(relayWaitTimeout)
	defer wait.Stop()
	select {
	case <-slot.ready:
	case <-wait.C:
		slot.abort(errRelayTimeout)
		return nil, "", 0, errRelayTimeout
	case <-ctx.Done():
		slot.abort(ctx.Err())
		return nil, "", 0, ctx.Err()
	}
	close(slot.attached)
	return slot.reader, slot.format, slot.size, nil
}
