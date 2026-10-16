// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newPolicyTestService(t *testing.T) *HostUpdateService {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "host-update.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO environments (id, name, connection_type) VALUES ('env-1', 'local', 'socket')`); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	agentPool := coreagent.NewAgentPool(logger)
	pool := coredocker.NewClientPool(database, agentPool, logger)
	return NewHostUpdateService(NewServiceWithAgent(pool, agentPool, logger), agentPool, database, logger)
}

func TestHostUpdatePolicyRoundTrip(t *testing.T) {
	svc := newPolicyTestService(t)
	ctx := context.Background()

	state, err := svc.State(ctx, EnvTarget("env-1"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Schedule.SkipUnavailable || state.Schedule.WithNewPackages || state.Schedule.RemoveUnused {
		t.Fatalf("every option must default to off, got %+v", state.Schedule)
	}

	if err := svc.SaveSchedule(ctx, EnvTarget("env-1"), HostUpdateSchedule{
		Enabled: true, Time: "04:30", Days: []string{"mon"}, SkipUnavailable: true, RemoveUnused: true,
	}); err != nil {
		t.Fatal(err)
	}
	state, err = svc.State(ctx, EnvTarget("env-1"))
	if err != nil {
		t.Fatal(err)
	}
	if !state.Schedule.SkipUnavailable || state.Schedule.WithNewPackages || !state.Schedule.RemoveUnused {
		t.Fatalf("stored schedule did not round-trip: %+v", state.Schedule)
	}

	stored := svc.applyPolicy(ctx, EnvTarget("env-1"), HostUpdateOverrides{})
	if !stored.SkipUnavailable || stored.WithNewPackages || !stored.RemoveUnused {
		t.Fatalf("apply must use the stored policy, got %+v", stored)
	}

	off, on := false, true
	overridden := svc.applyPolicy(ctx, EnvTarget("env-1"), HostUpdateOverrides{SkipUnavailable: &off, WithNewPackages: &on, RemoveUnused: &off})
	if overridden.SkipUnavailable || !overridden.WithNewPackages || overridden.RemoveUnused {
		t.Fatalf("a per-run override must win, got %+v", overridden)
	}

	if err := svc.SaveSchedule(ctx, EnvTarget("env-1"), HostUpdateSchedule{Time: "04:30", Days: []string{"mon"}}); err != nil {
		t.Fatal(err)
	}
	if cleared := svc.applyPolicy(ctx, EnvTarget("env-1"), HostUpdateOverrides{}); cleared.SkipUnavailable || cleared.RemoveUnused {
		t.Fatalf("turning the option off must persist, got %+v", cleared)
	}
}

func TestSelfHostPolicyIsSeparateFromEnvironments(t *testing.T) {
	svc := newPolicyTestService(t)
	ctx := context.Background()

	if err := svc.SaveSchedule(ctx, SelfTarget(), HostUpdateSchedule{
		Enabled: true, Time: "02:15", Days: []string{"sun"}, WithNewPackages: true, SkipUnavailable: true, RemoveUnused: true,
	}); err != nil {
		t.Fatal(err)
	}
	self, err := svc.State(ctx, SelfTarget())
	if err != nil {
		t.Fatal(err)
	}
	if !self.Schedule.Enabled || self.Schedule.Time != "02:15" || !self.Schedule.SkipUnavailable || !self.Schedule.WithNewPackages || !self.Schedule.RemoveUnused {
		t.Fatalf("self schedule did not round-trip: %+v", self.Schedule)
	}
	env, err := svc.State(ctx, EnvTarget("env-1"))
	if err != nil {
		t.Fatal(err)
	}
	if env.Schedule.Enabled || env.Schedule.SkipUnavailable || env.Schedule.RemoveUnused || env.Schedule.Time == "02:15" {
		t.Fatalf("saving the self host leaked into an environment: %+v", env.Schedule)
	}
	if opts := svc.applyPolicy(ctx, SelfTarget(), HostUpdateOverrides{}); !opts.SkipUnavailable || !opts.WithNewPackages || !opts.RemoveUnused {
		t.Fatalf("self apply must use the self policy, got %+v", opts)
	}
}

func TestActiveJobIsPerTarget(t *testing.T) {
	svc := newPolicyTestService(t)
	job := &HostUpdateJob{ID: "job-self", Target: SelfTarget()}
	svc.jobs[job.ID] = job
	if got := svc.ActiveJob(EnvTarget("env-1")); got != nil {
		t.Fatalf("an environment must not reattach to the self host's upgrade, got %s", got.ID)
	}
	if got := svc.ActiveJob(SelfTarget()); got == nil || got.ID != "job-self" {
		t.Fatalf("the self host's running job was not found: %v", got)
	}
}

func TestScheduleQueryReadsBothTables(t *testing.T) {
	svc := newPolicyTestService(t)
	var logs strings.Builder
	svc.logger = slog.New(slog.NewTextHandler(&logs, nil))
	svc.runDueChecks(context.Background())
	if strings.Contains(logs.String(), "reading host update schedules failed") {
		t.Fatalf("the schedule query failed: %s", logs.String())
	}
}
