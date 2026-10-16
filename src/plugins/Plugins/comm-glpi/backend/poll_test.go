// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestPollOmits404AndMapsStates(t *testing.T) {
	for _, api := range []string{"v1", "v2"} {
		t.Run(api, func(t *testing.T) {
			f := newFakeGLPI(t)
			want := map[string]incident.RemoteState{}
			var refs []incident.Ref
			for i, st := range []int{1, 2, 3, 4, 5, 6, 2, 1, 5} {
				tk := fakeTicket{Name: "x", Status: st, ExternalID: "mcharbor:a", Deleted: i == 6}
				id := itoa(f.seed(tk))
				s, _ := remoteState(tk.Status, tk.Deleted)
				want[id] = s
				refs = append(refs, incident.Ref{ExternalID: id})
			}
			refs = append(refs, incident.Ref{ExternalID: "424242"}, incident.Ref{ExternalID: "not-a-number"})
			got, err := NewClient(f.srv.Client()).Poll(context.Background(), f.config(api), refs)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("got %d statuses, want %d: %+v", len(got), len(want), got)
			}
			for _, st := range got {
				if want[st.Ref.ExternalID] != st.State || st.UpdatedAt.IsZero() {
					t.Errorf("%s: %s, want %s", st.Ref.ExternalID, st.State, want[st.Ref.ExternalID])
				}
			}
			if api == "v1" && (f.inits != 1 || f.killed != 1) {
				t.Fatalf("one session per batch: inits=%d killed=%d", f.inits, f.killed)
			}
		})
	}
}

func TestPollOneBadTicketDoesNotHideOthers(t *testing.T) {
	f := newFakeGLPI(t)
	good := f.seed(fakeTicket{Name: "ok", Status: 2})
	bad := f.seed(fakeTicket{Name: "hidden", Status: 2})
	f.forbidden = map[int]bool{bad: true}
	c := NewClient(f.srv.Client())
	got, err := c.Poll(context.Background(), f.config("v1"), []incident.Ref{{ExternalID: itoa(good)}, {ExternalID: itoa(bad)}})
	if err != nil || len(got) != 1 || got[0].Ref.ExternalID != itoa(good) {
		t.Fatalf("got %+v err %v", got, err)
	}
	got, err = c.Poll(context.Background(), f.config("v1"), []incident.Ref{{ExternalID: itoa(bad)}})
	if !errors.Is(err, incident.ErrOperatorMessage) || got != nil {
		t.Fatalf("all reads failed but got %+v err %v", got, err)
	}
	got, err = c.Poll(context.Background(), f.config("v1"), []incident.Ref{{ExternalID: "424242"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("a deleted ticket must be omitted, not an error: %+v %v", got, err)
	}
}
