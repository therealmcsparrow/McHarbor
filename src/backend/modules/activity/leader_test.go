// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

import (
	"context"
	"testing"
)

// TestStreamsFollowLeadership guards multi-node installs: a node that is not the
// elected collector must not open the local event stream, or every event is stored twice.
func TestStreamsFollowLeadership(t *testing.T) {
	c := &Collector{}
	if !c.mayStream(context.Background(), "") {
		t.Fatal("without a leader check a single node always streams")
	}
	leading := false
	c.SetLeader(func(context.Context) bool { return leading })
	if c.mayStream(context.Background(), "") {
		t.Fatal("a follower must not stream the local environment")
	}
	leading = true
	if !c.mayStream(context.Background(), "") {
		t.Fatal("the leader streams")
	}
}
