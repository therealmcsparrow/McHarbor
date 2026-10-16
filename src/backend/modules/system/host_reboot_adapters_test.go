// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

// fakeGuestAdapter is one guest family with a fixed up-set, recording what it was asked.
type fakeGuestAdapter struct {
	family    string
	up        []rebootGuest
	shutdowns []string
	asked     int
	escalate  bool
}

func (f *fakeGuestAdapter) kind() string { return f.family }

func (f *fakeGuestAdapter) running(context.Context) ([]rebootGuest, error) {
	f.asked++
	return slices.Clone(f.up), nil
}

func (f *fakeGuestAdapter) requestShutdown(_ context.Context, g rebootGuest) (bool, error) {
	f.shutdowns = append(f.shutdowns, g.ID)
	return f.escalate, nil
}

// TestGuestRebootHostDispatchesThroughRegisteredAdapters guards the lookup: every step
// reaches a guest only through its own family's adapter, an undrained kind is refused, an
// escalation is a failure, and StillRunning asks only the families it has guests of.
func TestGuestRebootHostDispatchesThroughRegisteredAdapters(t *testing.T) {
	ctx := context.Background()
	alpha := &fakeGuestAdapter{family: "alpha", up: []rebootGuest{{Kind: "alpha", ID: "a1", Name: "one", Graceful: true}}}
	beta := &fakeGuestAdapter{family: "beta", up: []rebootGuest{{Kind: "beta", ID: "b1", Name: "three", Graceful: true}}}
	host := &guestRebootHost{adapters: []rebootGuestAdapter{alpha, beta}}

	running, err := host.RunningGuests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, g := range running {
		ids = append(ids, g.Kind+"/"+g.ID)
	}
	if !slices.Equal(ids, []string{"alpha/a1", "beta/b1"}) {
		t.Fatalf("running guests: got %v", ids)
	}
	for _, g := range running {
		if err := host.RequestShutdown(ctx, g); err != nil {
			t.Fatalf("shutdown %s: %v", g.ID, err)
		}
	}
	if !slices.Equal(alpha.shutdowns, []string{"a1"}) || !slices.Equal(beta.shutdowns, []string{"b1"}) {
		t.Errorf("each shutdown must reach its own family only: alpha %v, beta %v", alpha.shutdowns, beta.shutdowns)
	}
	if err := host.RequestShutdown(ctx, rebootGuest{Kind: "gamma", ID: "g1"}); !errors.Is(err, errNoGuestAdapter) {
		t.Errorf("an undrained guest kind must be refused, got %v", err)
	}
	beta.escalate = true
	if err := host.RequestShutdown(ctx, running[1]); err == nil {
		t.Error("an escalation to a hard stop must fail the request")
	}

	alpha.asked, beta.asked = 0, 0
	beta.up = nil
	still, err := host.StillRunning(ctx, running[:1])
	if err != nil {
		t.Fatal(err)
	}
	if len(still) != 1 || still[0].ID != "a1" {
		t.Errorf("still running: got %+v", still)
	}
	if alpha.asked != 1 || beta.asked != 0 {
		t.Errorf("only the family with guests in question may be asked: alpha %d, beta %d", alpha.asked, beta.asked)
	}
}

// drainPlugin is a plugin router serving one reboot-drain pair, recording who called and how.
type drainPlugin struct {
	mu        sync.Mutex
	callers   []string
	envs      []string
	stopped   []string
	forbidden bool
	hang      bool
	escalate  bool
	mux       *chi.Mux
}

func newDrainPlugin() *drainPlugin {
	p := &drainPlugin{mux: chi.NewRouter()}
	respond := func(w http.ResponseWriter, status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, body); err != nil {
			return
		}
	}
	p.mux.Get("/demo/reboot-drain", func(w http.ResponseWriter, r *http.Request) {
		if !p.record(r) {
			respond(w, http.StatusForbidden, `{"success":false,"error":"forbidden"}`)
			return
		}
		respond(w, http.StatusOK, `{"success":true,"data":[{"id":"m1","name":"one","graceful":true},{"id":"m2","name":"two","graceful":false},{"id":""}]}`)
	})
	p.mux.Post("/demo/reboot-drain/stop", func(w http.ResponseWriter, r *http.Request) {
		if !p.record(r) {
			respond(w, http.StatusForbidden, `{"success":false,"error":"forbidden"}`)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respond(w, http.StatusBadRequest, `{"success":false,"code":"err.invalid_body"}`)
			return
		}
		p.mu.Lock()
		p.stopped = append(p.stopped, body.ID)
		hang, escalate := p.hang, p.escalate
		p.mu.Unlock()
		if hang {
			<-r.Context().Done()
			respond(w, http.StatusBadGateway, `{"success":false,"code":"err.vm.agent_failed"}`)
			return
		}
		if escalate {
			respond(w, http.StatusOK, `{"success":true,"data":{"escalated":true}}`)
			return
		}
		respond(w, http.StatusOK, `{"success":true,"data":{"escalated":false}}`)
	})
	return p
}

// record notes the caller and the environment, and reports whether the plugin's gate passes.
func (p *drainPlugin) record(r *http.Request) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	user := auth.UserFromContext(r.Context())
	id := ""
	if user != nil {
		id = user.ID
	}
	p.callers = append(p.callers, id)
	p.envs = append(p.envs, r.URL.Query().Get("env"))
	return !p.forbidden
}

// ServeHTTP answers as the host's plugin router does: chi reads the route path the caller
// set relative to /api.
func (p *drainPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mux.ServeHTTP(w, r)
}

func demoDrain() plugincaps.Capability {
	return plugincaps.Capability{
		Plugin: "addon-demo", Kind: plugincaps.KindRebootDrain, SubjectType: "machine",
		Routes: map[string]string{"running": "/api/demo/reboot-drain", "stop": "/api/demo/reboot-drain/stop"},
	}
}

// TestDrainAdapterCallsTheDeclaredRoutesAsThePrincipal drives the real adapter through a plugin
// router: both routes are reached in-process at their declared paths, as the reboot's principal
// and on its environment; the listed guests become that subject's guests; a refusal by the
// plugin's gate fails the drain; an escalation fails the stop; and a stop the plugin is still
// waiting on when this side stops listening counts as in progress, not failed.
func TestDrainAdapterCallsTheDeclaredRoutesAsThePrincipal(t *testing.T) {
	ctx := context.Background()
	plugin := newDrainPlugin()
	host := newGuestRebootHost([]plugincaps.Capability{demoDrain()}, plugin, "env-9", &auth.User{ID: "u-requester"})

	running, err := host.RunningGuests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 2 || running[0] != (rebootGuest{Kind: "machine", ID: "m1", Name: "one", Graceful: true}) || running[1].Graceful {
		t.Fatalf("running guests: %+v", running)
	}
	if err := host.RequestShutdown(ctx, running[0]); err != nil {
		t.Fatalf("stop: %v", err)
	}
	plugin.mu.Lock()
	if !slices.Equal(plugin.stopped, []string{"m1"}) {
		t.Errorf("stopped %v, want [m1]", plugin.stopped)
	}
	for i, caller := range plugin.callers {
		if caller != "u-requester" || plugin.envs[i] != "env-9" {
			t.Errorf("call %d as %q on %q, want u-requester on env-9", i, caller, plugin.envs[i])
		}
	}
	plugin.escalate = true
	plugin.mu.Unlock()
	if err := host.RequestShutdown(ctx, running[0]); err == nil || !strings.Contains(err.Error(), "hard stop") {
		t.Errorf("an escalated stop must fail, got %v", err)
	}

	plugin.mu.Lock()
	plugin.escalate, plugin.hang = false, true
	plugin.mu.Unlock()
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	a := host.adapters[0].(*drainAdapter)
	if _, err := a.requestShutdown(short, running[0]); !stillWaiting(err) {
		t.Errorf("a stop nobody waited for must read as still in progress, got %v", err)
	}

	plugin.mu.Lock()
	plugin.hang, plugin.forbidden = false, true
	plugin.mu.Unlock()
	if _, err := host.RunningGuests(ctx); err == nil || !strings.Contains(err.Error(), "addon-demo") {
		t.Errorf("a drain the plugin's gate refuses must fail naming the plugin, got %v", err)
	}
}

// TestDrainNeedsAPrincipal: with no requester and no user on the request there is nobody for
// the plugin's gate to judge, so the drain refuses rather than calling anonymously.
func TestDrainNeedsAPrincipal(t *testing.T) {
	plugin := newDrainPlugin()
	host := newGuestRebootHost([]plugincaps.Capability{demoDrain()}, plugin, "env-9", rebootPrincipal(context.Background(), ""))
	if _, err := host.RunningGuests(context.Background()); err == nil {
		t.Fatal("a drain without a principal must refuse")
	}
	if len(plugin.callers) != 0 {
		t.Fatalf("the plugin was called anonymously: %v", plugin.callers)
	}
	if u := rebootPrincipal(context.Background(), "user:u7"); u == nil || u.ID != "u7" {
		t.Fatalf("the reboot's requester must be the principal, got %+v", u)
	}
	withUser := context.WithValue(context.Background(), auth.UserContextKey, &auth.User{ID: "viewer"})
	if u := rebootPrincipal(withUser, ""); u == nil || u.ID != "viewer" {
		t.Fatalf("an inventory read acts as its own user, got %+v", u)
	}
}
