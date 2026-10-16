// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package events streams Docker daemon events to browsers over Server-Sent Events.
package events

import (
	"context"
	"errors"
	"sync"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// subscriberBuffer is how far one tab may fall behind before it is dropped.
// Dropping a slow tab, which then reconnects, is better than letting it stall
// the stream every other tab of that environment shares.
const subscriberBuffer = 256

// ErrSlowSubscriber ends a subscription that fell too far behind.
var ErrSlowSubscriber = errors.New("event subscriber fell behind")

// EventSource is the one Docker call the broker makes.
type EventSource interface {
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
}

// Broker shares one Docker events subscription per environment and filter
// among every open tab. Without it each tab opens its own, and on an agent
// environment every event crosses the agent link once per tab.
type Broker struct {
	source func(envID string) (EventSource, error)
	life   context.Context //nolint:containedctx // lifecycle root for the shared event streams, outliving any one subscriber

	mu      sync.Mutex
	streams map[string]*stream
	wg      sync.WaitGroup
}

type stream struct {
	cancel context.CancelFunc
	subs   map[*Subscription]struct{}
}

// Subscription is one tab's view of a shared stream.
type Subscription struct {
	Events <-chan events.Message
	events chan events.Message
	err    error
	once   sync.Once
}

// Err is why the subscription ended, once Events is closed.
func (s *Subscription) Err() error { return s.err }

func (s *Subscription) end(err error) {
	s.once.Do(func() {
		s.err = err
		close(s.events)
	})
}

// NewBroker returns a Broker whose streams stop when life ends.
func NewBroker(life context.Context, source func(envID string) (EventSource, error)) *Broker {
	return &Broker{source: source, life: life, streams: map[string]*stream{}}
}

// Wait blocks until every stream has stopped.
func (b *Broker) Wait() { b.wg.Wait() }

// Subscribe joins the shared stream for envID and eventType, starting it if
// this is the first tab. Call the returned function to leave; the stream stops
// when its last tab leaves.
func (b *Broker) Subscribe(envID, eventType string) (*Subscription, func(), error) {
	key := envID + "\x00" + eventType
	sub := &Subscription{events: make(chan events.Message, subscriberBuffer)}
	sub.Events = sub.events

	b.mu.Lock()
	st, ok := b.streams[key]
	if !ok {
		src, err := b.source(envID)
		if err != nil {
			b.mu.Unlock()
			return nil, nil, err
		}
		ctx, cancel := context.WithCancel(b.life)
		st = &stream{cancel: cancel, subs: map[*Subscription]struct{}{}}
		b.streams[key] = st
		args := make(client.Filters)
		if eventType != "" {
			args.Add("type", eventType)
		}
		// No timeout: this is the long-lived event stream, held open for as
		// long as any tab is subscribed; the broker's lifetime bounds it.
		res := src.Events(ctx, client.EventsListOptions{Filters: args})
		b.wg.Add(1)
		go b.pump(ctx, key, st, res.Messages, res.Err)
	}
	st.subs[sub] = struct{}{}
	b.mu.Unlock()

	leave := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := st.subs[sub]; !ok {
			return
		}
		delete(st.subs, sub)
		sub.end(nil)
		if len(st.subs) == 0 && b.streams[key] == st {
			delete(b.streams, key)
			st.cancel()
		}
	}
	return sub, leave, nil
}

// pump fans one Docker stream out to its subscribers until the stream ends,
// then ends every subscription with the stream's error so their tabs reconnect.
func (b *Broker) pump(ctx context.Context, key string, st *stream, msgs <-chan events.Message, errs <-chan error) {
	defer b.wg.Done()
	var cause error
	for cause == nil {
		select {
		case <-ctx.Done():
			cause = ctx.Err()
		case err, ok := <-errs:
			if !ok || err == nil {
				err = context.Canceled
			}
			cause = err
		case m, ok := <-msgs:
			if !ok {
				cause = context.Canceled
				break
			}
			b.mu.Lock()
			for sub := range st.subs {
				select {
				case sub.events <- m:
				default:
					delete(st.subs, sub)
					sub.end(ErrSlowSubscriber)
				}
			}
			b.mu.Unlock()
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.streams[key] == st {
		delete(b.streams, key)
	}
	for sub := range st.subs {
		delete(st.subs, sub)
		sub.end(cause)
	}
	st.cancel()
}

// Streams reports how many shared Docker subscriptions are open.
func (b *Broker) Streams() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.streams)
}
