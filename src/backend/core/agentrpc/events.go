// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentrpc

import (
	"sync/atomic"
)

// queueSize bounds one plugin's backlog of unsolicited frames. Guest events arrive a few at
// a time, so a full queue means the plugin's sink is stuck, and dropping is the only thing
// that keeps the agent's read loop, and every other plugin, moving.
const queueSize = 256

type frameEvent struct {
	envID   string
	msgType string
	frame   []byte
}

// subscription is one sink with its own queue and goroutine, so a slow plugin costs only
// itself.
type subscription struct {
	slug    string
	observe map[string]bool
	queue   chan frameEvent
	stop    chan struct{}
	dropped atomic.Uint64
}

// subscribe replaces key's subscription. observe nil means every observable frame; an
// empty map means none. A nil sink removes the subscription.
func (b *Bridge) subscribe(key string, observe map[string]bool, sink func(envID, msgType string, frame []byte)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if old, ok := b.subs[key]; ok {
		close(old.stop)
		delete(b.subs, key)
	}
	if sink == nil {
		return
	}
	s := &subscription{
		slug:    key,
		observe: observe,
		queue:   make(chan frameEvent, queueSize),
		stop:    make(chan struct{}),
	}
	b.subs[key] = s
	// goroutine lifecycle: ends when the subscription is replaced or removed and stop closes.
	go b.drain(s, sink)
}

// drain delivers one subscription's frames in order until it is stopped.
func (b *Bridge) drain(s *subscription, sink func(envID, msgType string, frame []byte)) {
	for {
		select {
		case <-s.stop:
			return
		case ev := <-s.queue:
			b.deliver(s, sink, ev)
		}
	}
}

// deliver runs plugin code, so a panic in it is contained to this frame and logged rather
// than taking the host process down.
func (b *Bridge) deliver(s *subscription, sink func(envID, msgType string, frame []byte), ev frameEvent) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("agentrpc: plugin event sink panicked", "origin", s.slug, "type", ev.msgType, "panic", r)
		}
	}()
	sink(ev.envID, ev.msgType, ev.frame)
}

// Unsubscribe drops slug's sink when the plugin unloads, so frames stop reaching code that
// is no longer serving anything.
func (b *Bridge) Unsubscribe(slug string) {
	b.subscribe(slug, nil, nil)
}

// Observe fans one unsolicited frame out to every subscription that may see it. It runs on
// an agent's read loop and never blocks: a full queue drops the frame and counts it.
func (b *Bridge) Observe(envID, msgType string, frame []byte) {
	b.mu.Lock()
	targets := make([]*subscription, 0, len(b.subs))
	for _, s := range b.subs {
		if s.observe == nil || s.observe[msgType] {
			targets = append(targets, s)
		}
	}
	b.mu.Unlock()
	for _, s := range targets {
		// Each plugin gets its own copy, so one that edits the bytes cannot corrupt
		// another's frame.
		ev := frameEvent{envID: envID, msgType: msgType, frame: append([]byte(nil), frame...)}
		select {
		case s.queue <- ev:
		default:
			n := s.dropped.Add(1)
			if n == 1 || n%100 == 0 {
				b.logger.Warn("agentrpc: plugin event queue full, frame dropped",
					"origin", s.slug, "type", msgType, "env", envID, "dropped_total", n)
			}
		}
	}
}

// Dropped reports how many frames slug's queue has refused since it subscribed.
func (b *Bridge) Dropped(slug string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.subs[slug]; ok {
		return s.dropped.Load()
	}
	return 0
}
