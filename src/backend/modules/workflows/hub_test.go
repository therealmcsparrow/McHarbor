// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"testing"
	"time"
)

func TestHubUnsubscribeClosesChannelWithoutBlocking(t *testing.T) {
	hub := NewHub()
	ch := hub.Subscribe("wf-1")

	done := make(chan struct{})
	go func() {
		hub.Unsubscribe("wf-1", ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("unsubscribe blocked")
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected subscription channel to be closed")
		}
	default:
		t.Fatal("expected closed subscription channel to read immediately")
	}
}
