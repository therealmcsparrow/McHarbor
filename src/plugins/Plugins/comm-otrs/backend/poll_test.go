// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestPollMapsStateTypes(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketGet"] = func(b map[string]any) any {
		if b["TicketID"] != "42,43" {
			t.Errorf("batch ids %v", b["TicketID"])
		}
		return map[string]any{"Ticket": []any{
			map[string]any{"TicketID": "42", "State": "merged", "StateType": "merged"},
			map[string]any{"TicketID": "43", "State": "Erledigt", "StateType": "closed"},
		}}
	}
	cfg := testConfig(srv.URL)
	got, err := NewClient().Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "42"}, {ExternalID: "43"}})
	if err != nil || len(got) != 2 || got[0].State != incident.RemoteClosed || got[1].State != incident.RemoteResolved {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestPollBatchesOf50AndSkipsDeleted(t *testing.T) {
	f, srv := newFakeGI(t)
	var batches []string
	f.handlers["POST /TicketGet"] = func(b map[string]any) any {
		ids := b["TicketID"].(string)
		batches = append(batches, ids)
		if slices.Contains(strings.Split(ids, ","), "7") {
			return map[string]any{"Error": map[string]any{"ErrorCode": "TicketGet.NotValidTicketID", "ErrorMessage": "x"}}
		}
		var out []any
		for _, id := range strings.Split(ids, ",") {
			out = append(out, map[string]any{"TicketID": id, "State": "open", "StateType": "open"})
		}
		return map[string]any{"Ticket": out}
	}
	var refs []incident.Ref
	for i := 1; i <= 60; i++ {
		refs = append(refs, incident.Ref{ExternalID: strconv.Itoa(i + 100)})
	}
	refs[0].ExternalID = "7"
	got, err := NewClient().Poll(context.Background(), testConfig(srv.URL), refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 59 {
		t.Fatalf("got %d statuses, want 59 (one deleted)", len(got))
	}
	for _, st := range got {
		if st.Ref.ExternalID == "7" {
			t.Fatal("deleted ticket reported")
		}
	}
	f.locked(func() {
		if len(strings.Split(batches[0], ",")) != 50 {
			t.Fatalf("first batch %d ids", len(strings.Split(batches[0], ",")))
		}
	})
}

func TestRemoteStateMapping(t *testing.T) {
	cfg, err := parseConfig(map[string]string{
		"instance_url": "https://x", "webservice": "W", "user_login": "u", "password": "p",
		"default_assignment_group": "Q", "customer_user": "c", "dedup_field": "D",
		"resolved_states": "Erledigt", "cancelled_states": "Storniert",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		state, stateType string
		want             incident.RemoteState
	}{
		{"new", "new", incident.RemoteOpen},
		{"open", "open", incident.RemoteAcknowledged},
		{"pending reminder", "pending reminder", incident.RemoteAcknowledged},
		{"pending auto close+", "", incident.RemoteAcknowledged},
		{"closed successful", "closed", incident.RemoteResolved},
		{"closed unsuccessful", "closed", incident.RemoteClosed},
		{"closed unsuccessful", "", incident.RemoteClosed},
		{"merged", "merged", incident.RemoteClosed},
		{"removed", "removed", incident.RemoteCancelled},
		{"Erledigt", "", incident.RemoteResolved},
		{"Storniert", "closed", incident.RemoteCancelled},
		{"whatever", "", incident.RemoteOpen},
	} {
		if got := cfg.remoteState(tc.state, tc.stateType); got != tc.want {
			t.Errorf("%q/%q: got %s want %s", tc.state, tc.stateType, got, tc.want)
		}
	}
}
