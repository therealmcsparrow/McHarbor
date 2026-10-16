// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

type fakeSource struct {
	mu      sync.Mutex
	calls   int
	filters []string
	msgs    []chan events.Message
	errs    []chan error
	ctxs    []context.Context
}

func (f *fakeSource) Events(ctx context.Context, opts client.EventsListOptions) client.EventsResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	for v := range opts.Filters["type"] {
		f.filters = append(f.filters, v)
	}
	m, e := make(chan events.Message, 16), make(chan error, 1)
	f.msgs, f.errs, f.ctxs = append(f.msgs, m), append(f.errs, e), append(f.ctxs, ctx)
	return client.EventsResult{Messages: m, Err: e}
}

func (f *fakeSource) stream(i int) (chan events.Message, chan error, context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.msgs[i], f.errs[i], f.ctxs[i]
}

func receive(t *testing.T, s *Subscription) events.Message {
	t.Helper()
	select {
	case m, ok := <-s.Events:
		if !ok {
			t.Fatalf("subscription ended: %v", s.Err())
		}
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
	}
	return events.Message{}
}

func ended(t *testing.T, s *Subscription) error {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-s.Events:
			if !ok {
				return s.Err()
			}
		case <-deadline:
			t.Fatal("the subscription did not end")
		}
	}
}

func newTestBroker(t *testing.T) (*Broker, *fakeSource) {
	t.Helper()
	src := &fakeSource{}
	life, cancel := context.WithCancel(context.Background())
	b := NewBroker(life, func(string) (EventSource, error) { return src, nil })
	t.Cleanup(func() {
		cancel()
		stopped := make(chan struct{})
		go func() {
			b.Wait()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("the broker's pump did not stop")
		}
	})
	return b, src
}

func TestTabsOfOneEnvironmentShareOneDockerSubscription(t *testing.T) {
	b, src := newTestBroker(t)
	var subs []*Subscription
	var leaves []func()
	for i := 0; i < 5; i++ {
		s, leave, err := b.Subscribe("env1", "")
		if err != nil {
			t.Fatal(err)
		}
		subs, leaves = append(subs, s), append(leaves, leave)
	}
	if src.calls != 1 || b.Streams() != 1 {
		t.Fatalf("five tabs opened %d Docker subscriptions", src.calls)
	}
	msgs, _, ctx := src.stream(0)
	msgs <- events.Message{Type: "container", Action: "start"}
	for _, s := range subs {
		if m := receive(t, s); m.Action != "start" {
			t.Fatalf("got %+v", m)
		}
	}

	if _, _, err := b.Subscribe("env1", "image"); err != nil {
		t.Fatal(err)
	}
	if src.calls != 2 || src.filters[0] != "image" {
		t.Fatalf("a different filter must get its own stream: calls %d filters %v", src.calls, src.filters)
	}

	for _, leave := range leaves[:4] {
		leave()
	}
	if ctx.Err() != nil {
		t.Fatal("the stream stopped while a tab was still open")
	}
	leaves[4]()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the Docker subscription outlived its last tab")
	}
	if _, _, err := b.Subscribe("env1", ""); err != nil || src.calls != 3 {
		t.Fatalf("a tab after the stream closed must open a new one: calls %d %v", src.calls, err)
	}
}

func TestAStreamErrorEndsEveryTabSoTheyReconnect(t *testing.T) {
	b, src := newTestBroker(t)
	a, _, err := b.Subscribe("env1", "")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := b.Subscribe("env1", "")
	if err != nil {
		t.Fatal(err)
	}
	_, errs, _ := src.stream(0)
	errs <- errors.New("agent disconnected")
	for _, s := range []*Subscription{a, c} {
		if err := ended(t, s); err == nil || err.Error() != "agent disconnected" {
			t.Fatalf("ended with %v", err)
		}
	}
	if b.Streams() != 0 {
		t.Fatal("a failed stream stayed registered")
	}
}

func TestASlowTabIsDroppedWithoutStallingTheOthers(t *testing.T) {
	b, src := newTestBroker(t)
	slow, _, err := b.Subscribe("env1", "")
	if err != nil {
		t.Fatal(err)
	}
	fast, _, err := b.Subscribe("env1", "")
	if err != nil {
		t.Fatal(err)
	}
	msgs, _, _ := src.stream(0)
	for i := 0; i < subscriberBuffer+20; i++ {
		select {
		case msgs <- events.Message{Action: "tick"}:
		case <-time.After(2 * time.Second):
			t.Fatalf("the broker stopped reading its stream after %d messages", i)
		}
		receive(t, fast)
	}
	if err := ended(t, slow); !errors.Is(err, ErrSlowSubscriber) {
		t.Fatalf("the slow tab ended with %v", err)
	}
}
