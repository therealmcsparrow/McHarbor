// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestClientIsOneWay(t *testing.T) {
	var c any = NewClient()
	if _, ok := c.(incident.LifecycleClient); !ok {
		t.Fatal("comm-alertmanager-webhook must implement LifecycleClient")
	}
	if _, ok := c.(incident.WebhookParser); ok {
		t.Fatal("comm-alertmanager-webhook must not implement WebhookParser; receivers have no common signed callback")
	}
	if _, ok := c.(incident.Poller); ok {
		t.Fatal("comm-alertmanager-webhook must not implement Poller; the protocol has no read API")
	}
}
