// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

type restCall struct {
	Operation string         `json:"operation"`
	Class     string         `json:"class"`
	Key       any            `json:"key"`
	Stimulus  string         `json:"stimulus"`
	Fields    map[string]any `json:"fields"`
}

// fakeITop is a rest.php stand-in that records every decoded operation and refuses any request
// whose token is missing from the form body or leaks into the query string.
func fakeITop(t *testing.T, h func(restCall) string) (*httptest.Server, map[string]string, *[]restCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []restCall
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webservices/rest.php" || r.URL.Query().Get("version") != "1.3" {
			t.Errorf("path %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.PostForm.Get("auth_token") != "tok" || r.URL.Query().Get("auth_token") != "" {
			writeBody(t, w, `{"code":1,"message":"Error: Invalid login"}`)
			return
		}
		var c restCall
		if err := json.Unmarshal([]byte(r.PostForm.Get("json_data")), &c); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		writeBody(t, w, h(c))
	}))
	t.Cleanup(srv.Close)
	return srv, map[string]string{
		"instance_url": srv.URL, "auth_mode": "token", "auth_token": "tok", "ticket_class": "Incident",
		"org_name": "Demo", "team_id": "18", "agent_id": "57", "service_id": "3", "resolution_code": "other",
	}, &calls
}

func writeBody(t *testing.T, w http.ResponseWriter, s string) {
	t.Helper()
	if _, err := w.Write([]byte(s)); err != nil {
		t.Error(err)
	}
}

func TestCode100IsOperatorMessage(t *testing.T) {
	cases := []string{
		`{"code":100,"message":"Error: Missing mandatory attribute 'caller_id'"}`,
		`{"code":0,"message":"","objects":{"Incident::0":{"code":100,"message":"Missing mandatory attribute 'caller_id'","class":"Incident","key":"0"}}}`,
	}
	for _, body := range cases {
		srv, cfg, _ := fakeITop(t, func(restCall) string { return body })
		parsed, err := parseConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewClient(srv.Client()).rest(context.Background(), parsed, map[string]any{"operation": "core/create"})
		if !errors.Is(err, incident.ErrOperatorMessage) || errors.Is(err, incident.ErrRetryable) || !strings.Contains(err.Error(), "caller_id") {
			t.Fatalf("body %s: err = %v", body, err)
		}
	}
}

func TestCode1NamesCredentialsNotSecret(t *testing.T) {
	srv, cfg, _ := fakeITop(t, func(restCall) string { return `{"code":0}` })
	cfg["auth_token"] = "wrong-secret-token"
	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewClient(srv.Client()).rest(context.Background(), parsed, map[string]any{"operation": "list_operations"})
	if !errors.Is(err, incident.ErrOperatorMessage) || strings.Contains(err.Error(), "wrong-secret-token") {
		t.Fatalf("err = %v", err)
	}
}

func TestSecretRedactedFromVendorMessage(t *testing.T) {
	srv, cfg, _ := fakeITop(t, func(restCall) string { return `{"code":100,"message":"bad token tok here"}` })
	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewClient(srv.Client()).rest(context.Background(), parsed, map[string]any{"operation": "core/get"})
	if err == nil || strings.Contains(err.Error(), " tok ") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPStatusClassification(t *testing.T) {
	cases := []struct {
		code  int
		check func(error) bool
	}{
		{429, func(err error) bool {
			var ra *incident.RetryAfterError
			return errors.As(err, &ra) && ra.After == 7*time.Second && errors.Is(err, incident.ErrRetryable)
		}},
		{503, func(err error) bool { return errors.Is(err, incident.ErrRetryable) }},
		{404, func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && !errors.Is(err, incident.ErrNotFound)
		}},
		{302, func(err error) bool { return errors.Is(err, incident.ErrOperatorMessage) }},
	}
	for _, tc := range cases {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "7")
			w.Header().Set("Location", "https://elsewhere.example/")
			w.WriteHeader(tc.code)
		}))
		hc := srv.Client()
		hc.CheckRedirect = NewClient(nil).http.CheckRedirect
		parsed, err := parseConfig(map[string]string{"instance_url": srv.URL, "auth_token": "tok", "org_name": "o", "team_id": "1", "agent_id": "1", "service_id": "1"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewClient(hc).rest(context.Background(), parsed, map[string]any{"operation": "list_operations"})
		if err == nil || !tc.check(err) {
			t.Errorf("status %d: %v", tc.code, err)
		}
		srv.Close()
	}
}

func TestOQLQuote(t *testing.T) {
	if got := oqlQuote(`x' OR 1=1 --\`); got != `'x\' OR 1=1 --\\'` {
		t.Fatalf("got %s", got)
	}
	srv, cfg, calls := fakeITop(t, func(restCall) string {
		return `{"code":0,"objects":{"Organization::1":{"code":0,"class":"Organization","key":"1","fields":{"id":"1"}}}}`
	})
	cfg["org_name"] = `x' OR 1=1 --`
	if err := NewClient(srv.Client()).Test(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	var orgOQL string
	for _, c := range *calls {
		if c.Class == "Organization" {
			orgOQL, _ = c.Key.(string)
		}
	}
	if orgOQL != `SELECT Organization WHERE name = 'x\' OR 1=1 --'` {
		t.Fatalf("organization OQL = %q", orgOQL)
	}
}

func TestParseConfigRejects(t *testing.T) {
	good := map[string]string{"instance_url": "https://itop.example.fr/itop/", "auth_token": "t", "org_name": "o", "team_id": "1", "agent_id": "2", "service_id": "3"}
	cfg, err := parseConfig(good)
	if err != nil || cfg.BaseURL != "https://itop.example.fr/itop" || cfg.Class != "Incident" || cfg.Origin != "monitoring" {
		t.Fatalf("good config: %+v %v", cfg, err)
	}
	bad := []map[string]string{
		{"instance_url": "http://itop.lan"},
		{"instance_url": "https://user:pw@itop.lan"},
		{"instance_url": "https://itop.lan/?x=1"},
		{"instance_url": "file:///etc/passwd"},
		{"ticket_class": "Person WHERE 1=1"},
		{"auth_mode": "basic"},
		{"team_id": "0"},
		{"agent_id": "1 OR 1=1"},
		{"severity_map": `{"crit":{"impact":"1"}}`},
		{"severity_map": `{"critical":{"impact":"4"}}`},
	}
	for _, patch := range bad {
		m := map[string]string{}
		for k, v := range good {
			m[k] = v
		}
		for k, v := range patch {
			m[k] = v
		}
		if _, err := parseConfig(m); !errors.Is(err, incident.ErrOperatorMessage) {
			t.Errorf("%v accepted: %v", patch, err)
		}
	}
	m := map[string]string{"severity_map": `{"warning":{"impact":3,"urgency":"04"}}`}
	for k, v := range good {
		m[k] = v
	}
	cfg, err = parseConfig(m)
	if err != nil || cfg.Severity[incident.SeverityWarning] != (severityFields{Impact: "3", Urgency: "4"}) || cfg.Severity[incident.SeverityCritical].Impact != "1" {
		t.Fatalf("severity override: %+v %v", cfg.Severity, err)
	}
}
