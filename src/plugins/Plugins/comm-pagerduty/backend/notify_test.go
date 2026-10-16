// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestTestReportsRegionOnUnauthorized(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/abilities" || r.Header.Get("Authorization") != "Token token=RESTKEY" {
			t.Errorf("unexpected probe %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	cfg := baseCfg()
	cfg["region"] = "eu"
	cfg["rest_api_key"] = "RESTKEY"
	err := c.Test(context.Background(), cfg)
	if err == nil || !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "EU service region") {
		t.Fatalf("want operator message naming the region, got %v", err)
	}
	if strings.Contains(err.Error(), "RESTKEY") {
		t.Fatal("REST key leaked")
	}
}

func TestTestWithoutRESTKeyMakesNoCall(t *testing.T) {
	c, _ := testClient(t, func(_ http.ResponseWriter, r *http.Request) { t.Errorf("unexpected call %s", r.URL.Path) })
	if err := c.Test(context.Background(), baseCfg()); err != nil {
		t.Fatal(err)
	}
}

func TestSendUsesStableNotifyKey(t *testing.T) {
	rec := &enqueueRecorder{}
	c, _ := testClient(t, rec.handler(t))
	msg := plugin.NotificationMessage{Title: "Backup failed", Message: "job nightly", Level: "error"}
	for i := 0; i < 2; i++ {
		if err := c.Send(context.Background(), baseCfg(), msg); err != nil {
			t.Fatal(err)
		}
	}
	msg.Message = "job weekly"
	if err := c.Send(context.Background(), baseCfg(), msg); err != nil {
		t.Fatal(err)
	}
	k := rec.bodies[0]["dedup_key"].(string)
	if !strings.HasPrefix(k, "mcharbor-notify:") || k != rec.bodies[1]["dedup_key"] || k == rec.bodies[2]["dedup_key"] {
		t.Fatalf("notify keys %v", rec.bodies)
	}
	if s := rec.bodies[0]["payload"].(map[string]any)["severity"]; s != "critical" {
		t.Fatalf("error level severity %v", s)
	}
}
