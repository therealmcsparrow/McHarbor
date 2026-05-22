// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"sync"
)

// ExecutionEvent is a single SSE event pushed to subscribers.
type ExecutionEvent struct {
	Event string // e.g. "node.started", "node.completed", "run.completed"
	Data  []byte // JSON payload
}

// Hub is an in-memory pub/sub for workflow execution events.
// The trigger service publishes events; SSE handlers subscribe.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan ExecutionEvent]struct{} // workflowID -> set of channels
}

// NewHub creates a new event hub.
func NewHub() *Hub {
	return &Hub{
		subs: make(map[string]map[chan ExecutionEvent]struct{}),
	}
}

// Subscribe returns a channel that receives events for the given workflow.
// Call Unsubscribe when done to prevent leaks.
func (h *Hub) Subscribe(workflowID string) chan ExecutionEvent {
	ch := make(chan ExecutionEvent, 64)
	h.mu.Lock()
	if h.subs[workflowID] == nil {
		h.subs[workflowID] = make(map[chan ExecutionEvent]struct{})
	}
	h.subs[workflowID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a channel from the workflow's subscriber set.
func (h *Hub) Unsubscribe(workflowID string, ch chan ExecutionEvent) {
	shouldClose := false
	h.mu.Lock()
	if m, ok := h.subs[workflowID]; ok {
		if _, exists := m[ch]; exists {
			delete(m, ch)
			shouldClose = true
			if len(m) == 0 {
				delete(h.subs, workflowID)
			}
		}
	}
	h.mu.Unlock()
	if shouldClose {
		close(ch)
	}
}

// Publish sends an event to all subscribers of the given workflow.
// Non-blocking: if a subscriber is slow, the event is dropped for that subscriber.
func (h *Hub) Publish(workflowID string, evt ExecutionEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[workflowID] {
		select {
		case ch <- evt:
		default:
			// Subscriber too slow, skip
		}
	}
}
