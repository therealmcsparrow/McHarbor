// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notify

import (
	"context"
	"errors"
	"testing"
)

// TestResolveChannelTargetAnyOfNeverFallsBackToAnotherType: a caller asking for "any
// channel that can carry this" (an email) gets the default among those types, and a
// miss is a miss rather than the estate default of some unrelated provider.
func TestResolveChannelTargetAnyOfNeverFallsBackToAnotherType(t *testing.T) {
	db, enc := openDispatcherTestDB(t)
	insertCommunicationChannel(t, db, "chat-default", "Chat", "chat", true, true)
	insertCommunicationChannel(t, db, "mail-b", "B mail", "pigeon", false, true)
	insertCommunicationChannel(t, db, "mail-a", "A mail", "raven", false, true)
	dispatcher := NewDispatcher(db, enc)

	target, err := dispatcher.resolveChannelTarget(context.Background(), "", "", "pigeon", "raven")
	if err != nil || target.ID != "mail-a" {
		t.Fatalf("want the first of the named types, got %+v %v", target, err)
	}
	if _, err := dispatcher.resolveChannelTarget(context.Background(), "", "", "owl"); !errors.Is(err, ErrChannelTargetNotFound) {
		t.Fatalf("want ErrChannelTargetNotFound, never the chat default, got %v", err)
	}
	// A stored channel id still wins over the type set, as it always has.
	target, err = dispatcher.resolveChannelTarget(context.Background(), "mail-b", "", "raven")
	if err != nil || target.ID != "mail-b" {
		t.Fatalf("want the stored channel, got %+v %v", target, err)
	}
}
