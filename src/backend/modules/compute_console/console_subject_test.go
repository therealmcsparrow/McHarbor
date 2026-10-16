// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compute_console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

// declaredSubjects stands in for the loaded plugins: one guest kind with graphical
// consoles gated on vm.console, one with text consoles gated on ct.console.
func declaredSubjects() *plugincaps.Registry {
	reg := plugincaps.NewRegistry()
	reg.Set("addon-graphical", []plugincaps.Capability{{
		Kind: plugincaps.KindConsoleSubject, SubjectType: "vm",
		Consoles: []string{"vnc", "serial", "spice"}, Permission: "vm.console",
	}})
	reg.Set("addon-text", []plugincaps.Capability{{
		Kind: plugincaps.KindConsoleSubject, SubjectType: "ct",
		Consoles: []string{"tty", "exec"}, Permission: "ct.console",
	}})
	return reg
}

// TestTicketGuestKindsComeFromDeclarations drives the ticket route against declared
// subjects: an undeclared guest kind or a console its plugin does not serve is refused
// before any permission check, and a declared kind is gated on the key its plugin names.
func TestTicketGuestKindsComeFromDeclarations(t *testing.T) {
	s, database := store(t)
	h := consoleHandlerFor(t, database, s)

	issue := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/compute/console/ticket", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
		rec := httptest.NewRecorder()
		h.HandleIssueTicket(rec, req)
		return rec.Code
	}

	cases := []struct {
		name string
		body string
		want int
	}{
		{"declared kind and console, key held", `{"envId":"envA","guestKind":"vm","guestId":"g1","kind":"serial"}`, http.StatusOK},
		{"guest kind no plugin declares", `{"envId":"envA","guestKind":"pod","guestId":"g1","kind":"tty"}`, http.StatusBadRequest},
		{"console the declaring plugin does not serve", `{"envId":"envA","guestKind":"vm","guestId":"g1","kind":"tty"}`, http.StatusBadRequest},
		{"declared kind gated on a key the user lacks", `{"envId":"envA","guestKind":"ct","guestId":"c1","kind":"tty"}`, http.StatusForbidden},
	}
	for _, c := range cases {
		if got := issue(c.body); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}

	h.subjects = plugincaps.NewRegistry()
	if got := issue(`{"envId":"envA","guestKind":"vm","guestId":"g1","kind":"vnc"}`); got != http.StatusBadRequest {
		t.Errorf("with the declaring plugin unloaded a console must be refused, got %d", got)
	}
}
