// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestVerifySharedSecret(t *testing.T) {
	c := NewClient(nil)
	cfg := map[string]string{"_webhook_secret": "s3cret-value"}
	cases := []struct {
		name   string
		cfg    map[string]string
		header string
		ok     bool
	}{
		{"missing header", cfg, "", false},
		{"wrong secret", cfg, "s3cret-valuf", false},
		{"prefix only", cfg, "s3cret", false},
		{"no channel secret", map[string]string{"_webhook_secret": ""}, "", false},
		{"no channel secret, any header", map[string]string{}, "anything", false},
		{"right secret", cfg, "s3cret-value", true},
	}
	for _, tc := range cases {
		h := http.Header{}
		if tc.header != "" {
			h.Set("X-McHarbor-Secret", tc.header)
		}
		err := c.VerifyWebhook(tc.cfg, h, []byte(`{}`))
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
}

func TestParseWebhookAssigned(t *testing.T) {
	body := `{"id":"42","ref":"I-000042","class":"Incident","title":"[MH-0123456789] Disque &amp; plein","status":"assigned","agent":"Jean-Fran&ccedil;ois Dupont","last_update":"2026-09-25 12:00:00"}`
	cfg := map[string]string{"instance_url": "https://itop.example.fr"}
	got, err := NewClient(nil).ParseWebhook(context.Background(), cfg, http.Header{}, []byte(body))
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err %v", got, err)
	}
	st := got[0]
	if st.State != incident.RemoteAcknowledged || st.Assignee != "Jean-François Dupont" || st.Ref.ExternalID != "42" || st.Ref.Number != "I-000042" {
		t.Fatalf("status = %+v", st)
	}
	if st.Ref.URL != "https://itop.example.fr/pages/UI.php?operation=details&class=Incident&id=42" {
		t.Fatalf("url = %s", st.Ref.URL)
	}
}

func TestParseWebhookDropsForeignAndUnknown(t *testing.T) {
	for _, body := range []string{
		`{"id":"42","title":"Printer jam","status":"assigned"}`,
		`{"id":"42","title":"[MH-0123456789] x","status":"frozen"}`,
		`{"id":"42) OR (1","title":"[MH-0123456789] x","status":"new"}`,
	} {
		got, err := NewClient(nil).ParseWebhook(context.Background(), map[string]string{}, http.Header{}, []byte(body))
		if err != nil || len(got) != 0 {
			t.Errorf("%s: got %+v err %v", body, got, err)
		}
	}
}

func TestPollOneQueryPerBatch(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(restCall) string {
		return `{"code":0,"objects":{
			"Incident::1":{"code":0,"class":"Incident","key":"1","fields":{"id":"1","ref":"I-1","status":"assigned","agent_id_friendlyname":"Ana"}},
			"Incident::2":{"code":0,"class":"Incident","key":"2","fields":{"id":"2","ref":"I-2","status":"closed","agent_id_friendlyname":""}},
			"Incident::3":{"code":0,"class":"Incident","key":"3","fields":{"id":"3","ref":"I-3","status":"new","agent_id_friendlyname":""}}}}`
	})
	got, err := NewClient(srv.Client()).Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "1"}, {ExternalID: "2"}, {ExternalID: "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].Key != "SELECT Incident WHERE id IN (1,2,3)" {
		t.Fatalf("calls = %+v", *calls)
	}
	want := map[string]incident.RemoteState{"1": incident.RemoteAcknowledged, "2": incident.RemoteClosed, "3": incident.RemoteOpen}
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	for _, st := range got {
		if want[st.Ref.ExternalID] != st.State {
			t.Errorf("%s: %s", st.Ref.ExternalID, st.State)
		}
	}
	if got[0].Assignee != "Ana" {
		t.Errorf("assignee = %q", got[0].Assignee)
	}
}

func TestPollRejectsNonNumericID(t *testing.T) {
	srv, cfg, calls := fakeITop(t, func(restCall) string { return `{"code":0,"objects":null}` })
	c := NewClient(srv.Client())
	if _, err := c.Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "1) OR (1=1"}}); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("a non-numeric id reached iTop: %+v", *calls)
	}
	if _, err := c.Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "7"}, {ExternalID: "7 OR 1=1"}, {ExternalID: "7"}}); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].Key != "SELECT Incident WHERE id IN (7)" {
		t.Fatalf("calls = %+v", *calls)
	}
}

func TestMarkerShape(t *testing.T) {
	m := marker("alert:env:subject")
	if !markerPattern.MatchString(m) || len(m) != 15 || m != marker("alert:env:subject") || m == marker("alert:env:other") {
		t.Fatalf("marker %q", m)
	}
	long := markerTitle(m, strings.Repeat("é", 400))
	if len([]rune(long)) != maxTitleRunes || !strings.HasPrefix(long, m+" ") {
		t.Fatalf("title %q", long)
	}
}
