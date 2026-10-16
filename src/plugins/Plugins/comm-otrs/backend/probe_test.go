// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestProbeDetectsIgnoredDedupField(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketSearch"] = func(map[string]any) any { return map[string]any{"TicketID": []string{"1"}} }
	err := NewClient().Test(context.Background(), testConfig(srv.URL))
	if err == nil || !strings.Contains(err.Error(), "not searchable") || !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("want not-searchable operator error, got %v", err)
	}
}

func TestProbeHappyPath(t *testing.T) {
	f, srv := newFakeGI(t)
	var probes []map[string]any
	f.handlers["POST /TicketSearch"] = func(b map[string]any) any {
		probes = append(probes, b)
		return map[string]any{}
	}
	if err := NewClient().Test(context.Background(), testConfig(srv.URL)); err != nil {
		t.Fatal(err)
	}
	if n := f.count("POST /Ticket"); n != 0 {
		t.Fatalf("Test created %d tickets", n)
	}
	f.locked(func() {
		if len(probes) != 2 {
			t.Fatalf("probes %v", probes)
		}
		df, _ := probes[1]["DynamicField_McHarborDedupKey"].(map[string]any)
		if eq, _ := df["Equals"].(string); !strings.HasPrefix(eq, "mcharbor-probe-") || len(eq) != len("mcharbor-probe-")+16 {
			t.Fatalf("probe key %v", df)
		}
	})
}

func TestProbeForcesFreshLogin(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketSearch"] = func(map[string]any) any { return map[string]any{} }
	c := NewClient()
	for i := 0; i < 2; i++ {
		if err := c.Test(context.Background(), testConfig(srv.URL)); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("POST /Session"); n != 2 {
		t.Fatalf("Test must prove the login each time: logins = %d", n)
	}
}

func TestSendCreatesTicketWithoutDedupField(t *testing.T) {
	f, srv := newFakeGI(t)
	var got map[string]any
	f.handlers["POST /Ticket"] = func(b map[string]any) any { got = b; return map[string]any{"TicketID": "5", "TicketNumber": "N5"} }
	err := NewClient().Send(context.Background(), testConfig(srv.URL), plugin.NotificationMessage{Title: "Backup failed", Message: "disk full", Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	f.locked(func() {
		if _, has := got["DynamicField"]; has {
			t.Fatalf("plain message carried a dedup field: %v", got)
		}
		tk := got["Ticket"].(map[string]any)
		if tk["Priority"] != "5 very high" || tk["Title"] != "Backup failed" {
			t.Fatalf("ticket %v", tk)
		}
	})
}
