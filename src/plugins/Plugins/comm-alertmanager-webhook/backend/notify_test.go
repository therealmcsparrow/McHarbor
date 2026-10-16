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

func TestTestDoesNotPostByDefault(t *testing.T) {
	c, base := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Test sent a request with the default config")
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := c.Test(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}); err != nil {
		t.Fatal(err)
	}
}

func TestTestSendsPairWhenEnabled(t *testing.T) {
	c, base, got := capture(t)
	if err := c.Test(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true", "test_sends_alert": "true"}); err != nil {
		t.Fatal(err)
	}
	p := got.all()
	if len(p) != 2 || p[0].Status != "firing" || p[1].Status != "resolved" || p[0].Alerts[0].Fingerprint != p[1].Alerts[0].Fingerprint {
		t.Fatalf("test pair wrong: %+v", p)
	}
	if p[0].Alerts[0].Labels["alertname"] != testAlertName || p[0].Alerts[0].Labels["severity"] != "info" {
		t.Fatalf("test alert labels %v", p[0].Alerts[0].Labels)
	}
}

func TestTestRefusesBlockedAndInvalid(t *testing.T) {
	c := NewClient()
	err := c.Test(context.Background(), map[string]string{"webhook_url": "http://169.254.169.254/latest/", "allow_http": "true"})
	if !errors.Is(err, errBlockedAddress) || !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("metadata probe not refused: %v", err)
	}
	if err := c.Test(context.Background(), map[string]string{"webhook_url": "http://receiver.example/"}); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("plain http accepted: %v", err)
	}
	c2 := newClient(true)
	err = c2.Test(context.Background(), map[string]string{"webhook_url": "http://127.0.0.1:1/tok-SECRET/", "allow_http": "true"})
	if err == nil || strings.Contains(err.Error(), "tok-SECRET") {
		t.Fatalf("unreachable receiver: %v", err)
	}
}

func TestSendResolvesImmediately(t *testing.T) {
	c, base, got := capture(t)
	msg := plugin.NotificationMessage{Title: "Backup done", Message: "", Level: "error"}
	if err := c.Send(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}, msg); err != nil {
		t.Fatal(err)
	}
	p := got.all()
	if len(p) != 2 || p[0].Status != "firing" || p[1].Status != "resolved" || p[0].Alerts[0].Fingerprint != p[1].Alerts[0].Fingerprint {
		t.Fatalf("send pair wrong: %+v", p)
	}
	al := p[0].Alerts[0]
	if al.Labels["alertname"] != notifyAlertName || al.Labels["severity"] != "critical" || al.Annotations["description"] != "Backup done" {
		t.Fatalf("send alert %+v", al)
	}
	if p[1].Alerts[0].StartsAt != al.StartsAt {
		t.Fatal("startsAt drifted between the send pair")
	}
}

func TestSendFailureStopsBeforeResolve(t *testing.T) {
	calls := 0
	c, base := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	})
	err := c.Send(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}, plugin.NotificationMessage{Title: "x"})
	if err == nil || calls != 1 {
		t.Fatalf("err %v calls %d", err, calls)
	}
}
