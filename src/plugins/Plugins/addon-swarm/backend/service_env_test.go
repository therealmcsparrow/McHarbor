// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type fakePool struct{ cli *client.Client }

func (p fakePool) Get(string) (any, error) { return p.cli, nil }

type quietLogger struct{}

func (quietLogger) Info(string, ...any)  {}
func (quietLogger) Warn(string, ...any)  {}
func (quietLogger) Error(string, ...any) {}
func (quietLogger) Debug(string, ...any) {}

// swarmDaemon answers the service inspect, task, network and update calls, serving one
// service per name and recording the environment of every update.
type swarmDaemon struct {
	mu      sync.Mutex
	updates [][]string
}

var swarmServices = map[string]string{
	"web": `{"ID":"svc-web","Version":{"Index":7},"Spec":{"Name":"web","TaskTemplate":{"ContainerSpec":{"Image":"nginx:1.27",` +
		`"Env":["DB_PASSWORD=hunter2hunter2","MODE=blue","API_TOKEN=tok-abc"]}}}}`,
	"prod_mcharbor": `{"ID":"svc-prod","Version":{"Index":4},"Spec":{"Name":"prod_mcharbor","Labels":{"com.docker.stack.namespace":"prod"},` +
		`"TaskTemplate":{"ContainerSpec":{"Image":"registry.local/platform:7","Env":["MCHARBOR_SECRET=stack-secret-value","LOG_LEVEL=info"]}}}}`,
	"copier": `{"ID":"svc-cp","Version":{"Index":2},"Spec":{"Name":"copier","TaskTemplate":{"ContainerSpec":{"Image":"example/app:1",` +
		`"Env":["COPIED=swarm-registered-root-key-8c3d","MODE=blue"]}}}}`,
	"mcharbor": `{"ID":"svc-mch","Version":{"Index":3},"Spec":{"Name":"mcharbor","TaskTemplate":{"ContainerSpec":{"Image":"ghcr.io/therealmcsparrow/mcharbor:latest",` +
		`"Env":["MCHARBOR_SECRET=root-secret-value","DB_PASSWORD=app-pass"]}}}}`,
}

func (d *swarmDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) >= 2 && parts[1] == "tasks":
		writeBody(w, `[]`)
	case len(parts) >= 2 && parts[1] == "networks":
		writeBody(w, `[]`)
	case len(parts) == 4 && parts[1] == "services" && parts[3] == "update":
		var spec struct {
			TaskTemplate struct {
				ContainerSpec struct {
					Env []string `json:"Env"`
				} `json:"ContainerSpec"`
			} `json:"TaskTemplate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		d.mu.Lock()
		d.updates = append(d.updates, spec.TaskTemplate.ContainerSpec.Env)
		d.mu.Unlock()
		writeBody(w, `{"Warnings":[]}`)
	case len(parts) == 3 && parts[1] == "services":
		doc, ok := swarmServices[parts[2]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writeBody(w, `{"message":"service not found"}`)
			return
		}
		writeBody(w, doc)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeBody(w, `{"message":"not found"}`)
	}
}

func writeBody(w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		panic(err) // panic-ok: test fixture; a failed write to an httptest recorder is a broken test
	}
}

func newEnvTestService(t *testing.T) (*Service, *swarmDaemon) {
	t.Helper()
	d := &swarmDaemon{}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(fakePool{cli: cli}, quietLogger{})
	s.cache["env1"] = capEntry{cap: Capability{Capable: true, ControlAvailable: true, NodeRole: "manager"}, at: time.Now()}
	return s, d
}

// TestServiceDetailWithholdsCredentials is the plugin half of F-1: swarm_services.view read
// every service's raw ContainerSpec.Env through the detail page, the CLI and runbooks.
func TestServiceDetailWithholdsCredentials(t *testing.T) {
	s, _ := newEnvTestService(t)
	detail, err := s.InspectService(context.Background(), "env1", "web")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"DB_PASSWORD=" + envredact.Marker, "MODE=blue", "API_TOKEN=" + envredact.Marker}
	if !slices.Equal(detail.Env, want) {
		t.Fatalf("detail env = %q\nwant       %q", detail.Env, want)
	}
}

// TestServiceEnvRevealKeepsMcHarborKeys is the manage-level read: values in the clear, except
// a McHarbor service's own keys.
func TestServiceEnvRevealKeepsMcHarborKeys(t *testing.T) {
	s, _ := newEnvTestService(t)
	env, err := s.ServiceEnv(context.Background(), "env1", "web")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(env.Env, []string{"DB_PASSWORD=hunter2hunter2", "MODE=blue", "API_TOKEN=tok-abc"}) {
		t.Fatalf("revealed env = %q", env.Env)
	}
	self, err := s.ServiceEnv(context.Background(), "env1", "mcharbor")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(self.Env, []string{"MCHARBOR_SECRET=" + envredact.Marker, "DB_PASSWORD=app-pass"}) {
		t.Fatalf("revealed McHarbor env = %q", self.Env)
	}
}

// TestServiceUpdateKeepsMaskedValues is the write path: a form filled from the masked detail
// must not write the marker over a credential.
func TestServiceUpdateKeepsMaskedValues(t *testing.T) {
	s, d := newEnvTestService(t)
	detail, err := s.InspectService(context.Background(), "env1", "web")
	if err != nil {
		t.Fatal(err)
	}
	edited := slices.Clone(detail.Env)
	edited[1] = "MODE=green"
	if err := s.UpdateService(context.Background(), "env1", "web", ServiceWriteRequest{Name: "web", Env: edited}); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.updates) != 1 || !slices.Equal(d.updates[0], []string{"DB_PASSWORD=hunter2hunter2", "MODE=green", "API_TOKEN=tok-abc"}) {
		t.Fatalf("updated env = %q", d.updates)
	}
}

// TestServiceEnvRouteIsManageGatedAndAudited reads the manifest: the reveal route must take
// swarm_services.manage and carry an audit declaration, or the host serves it to viewers
// and records nothing.
func TestServiceEnvRouteIsManageGatedAndAudited(t *testing.T) {
	raw, err := os.ReadFile("../plugin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	at := strings.Index(text, `path: "/api/swarm/services/{id}/env"`)
	if at < 0 {
		t.Fatal("the manifest declares no /api/swarm/services/{id}/env route")
	}
	block := text[at:]
	if end := strings.Index(block, "- method:"); end >= 0 {
		block = block[:end]
	}
	if !strings.HasSuffix(strings.TrimRight(text[:at], " "), "- method: GET\n") {
		t.Fatalf("the env route is not a GET:\n%s", text[max(0, at-40):at])
	}
	for _, want := range []string{
		`requires_permission: "swarm_services.manage"`, "audit_required: true",
		`audit_action: "plugin.swarm.service.env_revealed"`, `audit_entity_type: "swarm_service"`,
		`audit_severity: "warning"`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("env route lacks %s:\n%s", want, block)
		}
	}
}

// TestStackDeployedMcHarborServiceKeepsItsKeys is M6 of the 2026-09-30 review: the detector
// required the service name to be exactly "mcharbor", but `docker stack deploy` names it
// <stack>_<service>, so a stack-deployed McHarbor revealed its own keys.
func TestStackDeployedMcHarborServiceKeepsItsKeys(t *testing.T) {
	s, _ := newEnvTestService(t)
	env, err := s.ServiceEnv(context.Background(), "env1", "prod_mcharbor")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(env.Env, []string{"MCHARBOR_SECRET=" + envredact.Marker, "LOG_LEVEL=info"}) {
		t.Fatalf("revealed stack-deployed McHarbor env = %q", env.Env)
	}
}

// TestRegisteredPinnedValuesBindTheSwarmReveal is M6's pinned half: the plugin has no Pinned
// list, so the host's registered root-of-trust values must bind its policy anyway.
func TestRegisteredPinnedValuesBindTheSwarmReveal(t *testing.T) {
	envredact.RegisterPinned("swarm-registered-root-key-8c3d")
	s, _ := newEnvTestService(t)
	env, err := s.ServiceEnv(context.Background(), "env1", "copier")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(env.Env, []string{"COPIED=" + envredact.Marker, "MODE=blue"}) {
		t.Fatalf("revealed env with a registered pinned value = %q", env.Env)
	}
}

// TestServiceUpdateRefusesMovingAWithheldValue is M2 for Swarm: keeping McHarbor's key by
// its marker while changing the image would hand it to an image the editor chose.
func TestServiceUpdateRefusesMovingAWithheldValue(t *testing.T) {
	s, d := newEnvTestService(t)
	env := []string{"MCHARBOR_SECRET=" + envredact.Marker, "LOG_LEVEL=debug"}
	err := s.UpdateService(context.Background(), "env1", "prod_mcharbor", ServiceWriteRequest{Image: "attacker/exfil:1", Env: env})
	if !errors.Is(err, ErrMaskedValueMoved) {
		t.Fatalf("image change keeping McHarbor's key: err = %v, want ErrMaskedValueMoved", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.updates) != 0 {
		t.Fatalf("a refused update reached the daemon: %q", d.updates)
	}
}

type capturedAudit struct {
	events []plugin.AuditEvent
}

func (c *capturedAudit) Emit(string, map[string]any) error { return nil }

func (c *capturedAudit) EmitEvent(ev plugin.AuditEvent) error {
	c.events = append(c.events, ev)
	return nil
}

// TestServiceEnvRevealAuditNamesTheService is L1: the only row was the manifest fallback's,
// at "success" severity and with no entity id.
func TestServiceEnvRevealAuditNamesTheService(t *testing.T) {
	s, _ := newEnvTestService(t)
	sink := &capturedAudit{}
	h := &Handler{svc: s, logger: quietLogger{}, audits: sink}
	req := httptest.NewRequest(http.MethodGet, "/swarm/services/web/env?env=env1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "web")
	req = req.WithContext(context.WithValue(context.WithValue(req.Context(), chi.RouteCtxKey, rctx),
		auth.UserContextKey, &auth.User{ID: "ops", Username: "ops"}))
	rec := httptest.NewRecorder()
	h.HandleServiceEnv(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("env reveal: HTTP %d %s", rec.Code, rec.Body.String())
	}
	if len(sink.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(sink.events))
	}
	ev := sink.events[0]
	if ev.Severity != "warning" || ev.UserID != "ops" || ev.Fields["entity_id"] != "web" || ev.Fields["environment_id"] != "env1" {
		t.Fatalf("reveal audit event = %+v", ev)
	}
}
