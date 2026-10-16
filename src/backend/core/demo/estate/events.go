// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"sync"
	"time"
)

// Event is one Docker-style event emitted when simulated state changes; the
// Docker API layer renders it as an /events document.
type Event struct {
	Type       string
	Action     string
	ActorID    string
	Attributes map[string]string
	Time       time.Time
}

type broadcaster struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a channel of this host's events and a cancel func that must
// be called when the reader stops; a slow reader loses events rather than
// blocking the mutation that produced them.
func (h *Host) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 256)
	h.events.mu.Lock()
	h.events.subs[ch] = struct{}{}
	h.events.mu.Unlock()
	return ch, func() {
		h.events.mu.Lock()
		delete(h.events.subs, ch)
		h.events.mu.Unlock()
	}
}

func (h *Host) emit(ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	h.events.mu.Lock()
	defer h.events.mu.Unlock()
	for ch := range h.events.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (h *Host) emitContainer(c *Container, action string) {
	attrs := map[string]string{"name": c.Name, "image": c.Image}
	for k, v := range c.Labels {
		attrs[k] = v
	}
	if action == "die" {
		attrs["exitCode"] = itoa(c.ExitCode)
	}
	h.emit(Event{Type: "container", Action: action, ActorID: c.ID, Attributes: attrs})
}
