// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestOpenExactExternalIDMatch(t *testing.T) {
	for _, api := range []string{"v1", "v2"} {
		t.Run(api, func(t *testing.T) {
			f := newFakeGLPI(t)
			f.looseFilter = true
			ev := incident.Event{DedupKey: "a1:e1:c1", State: incident.StateFiring, Severity: incident.SeverityCritical, Title: "Disk full", Message: "95%"}
			ext := externalID(ev)
			f.seed(fakeTicket{Name: "longer key", Status: 1, ExternalID: ext + "ff"})
			f.seed(fakeTicket{Name: "solved one", Status: 5, ExternalID: ext})
			f.seed(fakeTicket{Name: "trashed one", Status: 1, ExternalID: ext, Deleted: true})
			ref, err := NewClient(f.srv.Client()).Open(context.Background(), f.config(api), ev)
			if err != nil {
				t.Fatal(err)
			}
			if f.created != 1 {
				t.Fatalf("created = %d, want a new ticket (no seed is a live exact match)", f.created)
			}
			again, err := NewClient(f.srv.Client()).Open(context.Background(), f.config(api), ev)
			if err != nil {
				t.Fatal(err)
			}
			if f.created != 1 || again.ExternalID != ref.ExternalID {
				t.Fatalf("retry created a duplicate: created=%d %s vs %s", f.created, again.ExternalID, ref.ExternalID)
			}
			got := f.tickets[mustAtoi(t, ref.ExternalID)]
			if got.Urgency != 5 || got.Impact != 5 || got.Priority != 6 || got.Type != 1 {
				t.Fatalf("severity mapping: %+v", got)
			}
			if got.ExternalID != ext || !strings.HasPrefix(ext, "mcharbor:") || len(ext) != len("mcharbor:")+40 {
				t.Fatalf("externalid = %q", got.ExternalID)
			}
		})
	}
}

func TestCreate207IsError(t *testing.T) {
	f := newFakeGLPI(t)
	f.multiStatus = `[{"id":false,"message":"You don't have permission to perform this action."}]`
	_, err := NewClient(f.srv.Client()).Open(context.Background(), f.config("v1"),
		incident.Event{DedupKey: "k", State: incident.StateFiring, Severity: incident.SeverityInfo, Title: "t"})
	if !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "permission to perform") {
		t.Fatalf("vendor message lost: %v", err)
	}
}

func TestLifecycleBothAPIs(t *testing.T) {
	refs := map[string]incident.Ref{}
	for _, api := range []string{"v1", "v2"} {
		t.Run(api, func(t *testing.T) {
			f := newFakeGLPI(t)
			cfg := f.config(api)
			cfg["close_on_resolve"] = "true"
			cfg["entities_id"] = "1"
			cfg["itilcategories_id"] = "7"
			c := NewClient(f.srv.Client())
			ev := incident.Event{DedupKey: "k", State: incident.StateFiring, Severity: incident.SeverityWarning, Title: "t", Message: "m"}
			ref, err := c.Open(context.Background(), cfg, ev)
			if err != nil {
				t.Fatal(err)
			}
			again, err := c.Open(context.Background(), cfg, ev)
			if err != nil || again != ref || f.created != 1 {
				t.Fatalf("%s: Open not idempotent: %+v vs %+v created=%d err=%v", api, again, ref, f.created, err)
			}
			if err := c.Update(context.Background(), cfg, ref, ev); err != nil {
				t.Fatal(err)
			}
			ev.State = incident.StateResolved
			if err := c.Resolve(context.Background(), cfg, ref, ev); err != nil {
				t.Fatal(err)
			}
			tk := f.tickets[mustAtoi(t, ref.ExternalID)]
			if tk.Followups != 1 || tk.Solutions != 1 || tk.Status != 6 {
				t.Fatalf("%s: %+v", api, tk)
			}
			if tk.Urgency != 4 || tk.Impact != 3 || tk.Priority != 4 || tk.Entity != 1 || tk.Category != 7 || tk.ExternalID != externalID(ev) {
				t.Fatalf("%s: create fields %+v", api, tk)
			}
			if ref.URL != f.srv.URL+"/front/ticket.form.php?id="+ref.ExternalID || ref.Number != ref.ExternalID {
				t.Fatalf("url = %s", ref.URL)
			}
			st, err := c.Poll(context.Background(), cfg, []incident.Ref{ref})
			if err != nil || len(st) != 1 || st[0].State != incident.RemoteClosed || st[0].Ref != ref {
				t.Fatalf("%s: poll %+v %v", api, st, err)
			}
			refs[api] = incident.Ref{ExternalID: ref.ExternalID, Number: ref.Number, URL: strings.TrimPrefix(ref.URL, f.srv.URL)}
		})
	}
	if refs["v1"] != refs["v2"] {
		t.Fatalf("v1 and v2 refs diverge: %+v vs %+v", refs["v1"], refs["v2"])
	}
}

func TestResolveIsIdempotent(t *testing.T) {
	for _, api := range []string{"v1", "v2"} {
		t.Run(api, func(t *testing.T) {
			f := newFakeGLPI(t)
			id := f.seed(fakeTicket{Name: "x", Status: 5, ExternalID: "mcharbor:ab"})
			cfg := f.config(api)
			cfg["close_on_resolve"] = "true"
			err := NewClient(f.srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: itoa(id)}, incident.Event{Title: "t"})
			if err != nil {
				t.Fatal(err)
			}
			if f.tickets[id].Status != 6 || f.tickets[id].Solutions != 0 {
				t.Fatalf("%+v", f.tickets[id])
			}
		})
	}
}

func TestResolveRefusedOnOpenTicketStaysError(t *testing.T) {
	f := newFakeGLPI(t)
	c := NewClient(f.srv.Client())
	err := c.Resolve(context.Background(), f.config("v1"), incident.Ref{ExternalID: "999999"}, incident.Event{Title: "t"})
	if err == nil {
		t.Fatal("solution refused for a missing parent was reported as success")
	}
}

func TestTestChecksEntity(t *testing.T) {
	f := newFakeGLPI(t)
	cfg := f.config("v1")
	cfg["entities_id"] = "1"
	c := NewClient(f.srv.Client())
	if err := c.Test(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	cfg["entities_id"] = "3"
	err := c.Test(context.Background(), cfg)
	if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "entity 3 is not active") {
		t.Fatalf("err = %v", err)
	}
	if f.created != 0 || f.killed != 2 {
		t.Fatalf("created=%d killed=%d", f.created, f.killed)
	}
}

func TestTestV2NoCreate(t *testing.T) {
	f := newFakeGLPI(t)
	if err := NewClient(f.srv.Client()).Test(context.Background(), f.config("v2")); err != nil {
		t.Fatal(err)
	}
	if f.created != 0 || f.tokenCalls != 1 {
		t.Fatalf("created=%d tokenCalls=%d", f.created, f.tokenCalls)
	}
}

func TestSendHasNoExternalID(t *testing.T) {
	f := newFakeGLPI(t)
	err := NewClient(f.srv.Client()).Send(context.Background(), f.config("v1"),
		plugin.NotificationMessage{Title: "Backup <done>", Message: "ok", Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	if f.created != 1 {
		t.Fatalf("created = %d", f.created)
	}
	if _, ok := f.lastCreate["externalid"]; ok {
		t.Fatalf("Send wrote an externalid: %v", f.lastCreate)
	}
	if !strings.Contains(f.lastCreate["content"].(string), "Backup &lt;done&gt;") || num(f.lastCreate["urgency"]) != 5 {
		t.Fatalf("create = %v", f.lastCreate)
	}
}

func TestConfigValidation(t *testing.T) {
	base := map[string]string{"instance_url": "https://glpi.example.org/apirest.php/", "user_token": "UT"}
	cfg, err := parseConfig(base)
	if err != nil || cfg.BaseURL != "https://glpi.example.org" || cfg.API != "v1" {
		t.Fatalf("%+v %v", cfg, err)
	}
	bad := []map[string]string{
		{"instance_url": "http://glpi.lan", "user_token": "UT"},
		{"instance_url": "https://user:pw@glpi.lan", "user_token": "UT"},
		{"instance_url": "https://glpi.lan?x=1", "user_token": "UT"},
		{"instance_url": "file:///etc/passwd", "user_token": "UT"},
		{"instance_url": "https://glpi.lan"},
		{"instance_url": "https://glpi.lan", "api_version": "v3", "user_token": "UT"},
		{"instance_url": "https://glpi.lan", "api_version": "v2", "oauth_client_id": "a", "oauth_client_secret": "b", "oauth_username": "c"},
		{"instance_url": "https://glpi.lan", "api_version": "v2", "v2_path": "../apirest.php", "oauth_client_id": "a", "oauth_client_secret": "b", "oauth_username": "c", "oauth_password": "d"},
		{"instance_url": "https://glpi.lan", "user_token": "UT", "entities_id": "-1"},
		{"instance_url": "https://glpi.lan", "user_token": "UT", "severity_map": `{"crit":{"urgency":1,"impact":1,"priority":1}}`},
		{"instance_url": "https://glpi.lan", "user_token": "UT", "severity_map": `{"info":{"urgency":9,"impact":1,"priority":1}}`},
	}
	for i, m := range bad {
		if _, err := parseConfig(m); !errors.Is(err, incident.ErrOperatorMessage) {
			t.Errorf("case %d accepted: %v", i, err)
		}
	}
	cfg, err = parseConfig(map[string]string{"instance_url": "https://glpi.lan", "user_token": "UT", "severity_map": `{"info":{"urgency":3,"impact":3,"priority":3}}`})
	if err != nil || cfg.Severity[incident.SeverityInfo].Priority != 3 || cfg.Severity[incident.SeverityCritical].Priority != 6 {
		t.Fatalf("override: %+v %v", cfg.Severity, err)
	}
}

func TestErrorsClassifyAndRedact(t *testing.T) {
	const secret = "UT"
	cases := []struct {
		code  int
		body  string
		check func(error) bool
	}{
		{429, `[]`, func(err error) bool {
			var ra *incident.RetryAfterError
			return errors.As(err, &ra) && errors.Is(err, incident.ErrRetryable)
		}},
		{503, `[]`, func(err error) bool { return errors.Is(err, incident.ErrRetryable) }},
		{404, `["ERROR_ITEM_NOT_FOUND","Item not found"]`, func(err error) bool { return errors.Is(err, incident.ErrNotFound) }},
		{400, `["ERROR_GLPI_UPDATE","token UT-SECRET-UT is wrong"]`, func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && strings.Contains(err.Error(), "[redacted]")
		}},
		{302, ``, func(err error) bool { return errors.Is(err, incident.ErrOperatorMessage) }},
	}
	for _, tc := range cases {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.code == 302 {
				http.Redirect(w, r, "https://elsewhere.invalid/", http.StatusFound)
				return
			}
			w.WriteHeader(tc.code)
			if _, err := w.Write([]byte(tc.body)); err != nil {
				t.Error(err)
			}
		}))
		c := NewClient(srv.Client())
		_, err := c.do(context.Background(), http.MethodGet, srv.URL+"/apirest.php/Ticket/1", nil, nil, nil, secret)
		srv.Close()
		if err == nil || !tc.check(err) {
			t.Errorf("status %d: unexpected classification %v", tc.code, err)
		}
		if err != nil && strings.Contains(err.Error(), "UT-SECRET") {
			t.Errorf("status %d: secret leaked: %v", tc.code, err)
		}
	}
}

func TestDescribeEscapes(t *testing.T) {
	out := describe(incident.Event{Title: "<script>x</script>", Message: "a\nb", Severity: incident.SeverityInfo, Link: "javascript:alert(1)", Labels: map[string]string{"k": "<i>"}})
	if strings.Contains(out, "<script>") || strings.Contains(out, "javascript:") || strings.Contains(out, "<i>") || !strings.Contains(out, "a<br>b") {
		t.Fatalf("describe = %s", out)
	}
}
