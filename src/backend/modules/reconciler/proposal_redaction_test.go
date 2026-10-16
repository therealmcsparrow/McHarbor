// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	errdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const livePassword = "hunter2-live-password"

// envDocker serves one container, "api", whose env carries a credential the plan must never store.
type envDocker struct{}

func (envDocker) ContainerInspect(_ context.Context, name string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if name != "api" {
		return client.ContainerInspectResult{}, errdefs.ErrNotFound.WithMessage("No such container: " + name)
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{
		Name:  "/api",
		State: &container.State{Running: true, Status: "running"},
		Config: &container.Config{
			Image:  "nginx:1",
			Env:    []string{"DB_PASSWORD=" + livePassword, "MODE=blue"},
			Labels: map[string]string{"team": "core"},
		},
		HostConfig: &container.HostConfig{},
	}}, nil
}

func (envDocker) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, nil
}

func (envDocker) ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{}, errdefs.ErrNotFound.WithMessage("no such image")
}

// recordingActuator keeps the recreate request so a test can read what would reach Docker.
type recordingActuator struct {
	fakeActuator
	mu       sync.Mutex
	recreate []docker.RecreateRequest
}

func (a *recordingActuator) Recreate(_ context.Context, _, name string, req docker.RecreateRequest) error {
	a.mu.Lock()
	a.recreate = append(a.recreate, req)
	a.mu.Unlock()
	return a.record("recreate", name)
}

func envProposalFixture(t *testing.T) (*Responder, *recordingActuator, *DesiredState) {
	t.Helper()
	loop, database, _, _ := newLoopFixture(t)
	if _, err := database.Exec(`INSERT INTO desired_states (id, name, container_name, image_ref, desired_status, config, created_at, updated_at)
		VALUES ('ds-env', 'ds-env', 'api', 'nginx:1', 'running', '{"env":{"MODE":"green"}}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	r, _ := withResponder(t, loop, database)
	act := &recordingActuator{}
	r.Actuator = act
	r.ReaderFor = func(string) (dockerReader, error) { return envDocker{}, nil }
	ds, err := r.Service.ByID(t.Context(), "ds-env")
	if err != nil || ds == nil {
		t.Fatalf("loading ds-env: %v", err)
	}
	return r, act, ds
}

// TestStoredProposalCarriesNoEnvValues is H2: the recreate step copied the container's whole
// Config.Env into reconcile_proposals.steps, which viewers list and the audit trail repeats.
func TestStoredProposalCarriesNoEnvValues(t *testing.T) {
	r, _, ds := envProposalFixture(t)
	p, err := r.ProposeNow(t.Context(), ds)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := r.Proposals.db.QueryRow(`SELECT steps FROM reconcile_proposals WHERE id = ?`, p.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, livePassword) || strings.Contains(raw, "DB_PASSWORD") || strings.Contains(raw, "green") {
		t.Fatalf("stored steps carry env beyond the changed key's name: %s", raw)
	}
	got, err := r.Proposals.Get(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var recreate *Step
	for i := range got.Steps {
		if got.Steps[i].Action == ActionRecreate {
			recreate = &got.Steps[i]
		}
	}
	if recreate == nil || recreate.Recreate == nil {
		t.Fatalf("plan = %+v, want a recreate", got.Steps)
	}
	if want := []string{"MODE=" + envredact.Marker}; !slices.Equal(recreate.Recreate.Env, want) {
		t.Fatalf("returned recreate env = %q, want %q", recreate.Recreate.Env, want)
	}
}

// TestLegacyProposalRowIsRedactedOnRead covers rows stored before redaction on write.
func TestLegacyProposalRowIsRedactedOnRead(t *testing.T) {
	r, _, ds := envProposalFixture(t)
	p, err := r.ProposeNow(t.Context(), ds)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `[{"action":"recreate","target":"api","fields":["env.MODE"],"recreate":{"env":["DB_PASSWORD=` + livePassword + `","MODE=green"]}}]`
	if _, err := r.Proposals.db.Exec(`UPDATE reconcile_proposals SET steps = ? WHERE id = ?`, legacy, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := r.Proposals.Get(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if env := got.Steps[0].Recreate.Env; !slices.Equal(env, []string{"MODE=" + envredact.Marker}) {
		t.Fatalf("legacy row returned env %q", env)
	}
}

// TestApproveRebuildsEnvFromTheLiveContainer is the other half: the stored plan has no values,
// so approving must read them again, and the marker must never reach Docker.
func TestApproveRebuildsEnvFromTheLiveContainer(t *testing.T) {
	r, act, ds := envProposalFixture(t)
	p, err := r.ProposeNow(t.Context(), ds)
	if err != nil {
		t.Fatal(err)
	}
	done, err := r.Approve(t.Context(), p.ID, "u1")
	if err != nil || done.Status != ProposalApplied {
		t.Fatalf("approve: %+v %v", done, err)
	}
	act.mu.Lock()
	defer act.mu.Unlock()
	if len(act.recreate) != 1 {
		t.Fatalf("recreates = %d, want 1", len(act.recreate))
	}
	want := []string{"DB_PASSWORD=" + livePassword, "MODE=green"}
	if !slices.Equal(act.recreate[0].Env, want) {
		t.Fatalf("recreate env = %q, want %q", act.recreate[0].Env, want)
	}
}

// TestApplyRefusesAWithheldEnvValue is the backstop behind rehydration.
func TestApplyRefusesAWithheldEnvValue(t *testing.T) {
	act := &recordingActuator{}
	step := Step{Action: ActionRecreate, Target: "api", Recreate: &docker.RecreateRequest{Env: []string{"MODE=" + envredact.Marker}}}
	if err := applyStep(t.Context(), act, "", step); err == nil {
		t.Fatal("a recreate carrying the marker was applied")
	}
	if len(act.recreate) != 0 {
		t.Fatalf("the marker reached the actuator: %+v", act.recreate)
	}
}

// TestGetProposalFromAnotherEnvironmentIs404 scopes the read to the proposal's own environment.
func TestGetProposalFromAnotherEnvironmentIs404(t *testing.T) {
	r, _, ds := envProposalFixture(t)
	far := *ds
	far.ID, far.EnvID = "ds-far", "env-far"
	p, err := r.Proposals.Create(t.Context(), &far, OriginPropose, &DriftReport{HasDrift: true}, []Step{})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(&router.AppDeps{DB: r.Proposals.db, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, r)
	mux := chi.NewRouter()
	mux.Get("/reconciler/proposals/{pid}", h.HandleGetProposal)
	get := func(env string) int {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/reconciler/proposals/"+p.ID+"?env="+env, nil))
		return w.Code
	}
	if code := get("other-env"); code != http.StatusNotFound {
		t.Fatalf("proposal read from another environment: HTTP %d, want 404", code)
	}
	if code := get("env-far"); code != http.StatusOK {
		t.Fatalf("proposal read from its own environment: HTTP %d, want 200", code)
	}
}
