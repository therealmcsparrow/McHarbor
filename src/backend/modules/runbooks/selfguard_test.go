// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"errors"
	"testing"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

func selfGuardCase(t *testing.T, key string, protected bool) (*fixture, *Run, Step) {
	t.Helper()
	f := newFixture(t, []StepInput{
		{Kind: StepAction, Title: "Take it down", ActionKey: key, Required: true},
	}, rbac.PermRunbooksRun, rbac.PermContainersManage)
	if protected {
		f.rt.labels = map[string]string{coredocker.ProtectedLabel: "true"}
	}
	return f, f.start(t), f.runbook.Steps[0]
}

func (f *fixture) recordAs(ctx context.Context, run *Run, step Step) error {
	_, err := f.svc.RecordStep(ctx, newRequestCtx(ctx), run.ID, step.ID, StepInputResult{State: StateDone},
		Operator{UserID: actor, Username: actorName})
	return err
}

// TestRestartAndStopRefuseAProtectedContainerWithoutHostSelf: the runbook step is a
// second door onto the same Docker calls the containers module guards, so it asks the same
// question and answers with the key to grant, before any runtime call.
func TestRestartAndStopRefuseAProtectedContainerWithoutHostSelf(t *testing.T) {
	for _, key := range []string{"container.restart", "container.stop", "container.start"} {
		t.Run(key, func(t *testing.T) {
			f, run, step := selfGuardCase(t, key, true)
			for name, ctx := range map[string]context.Context{
				"no user behind the run": context.Background(),
				"user without the key":   selfguard.WithDecision(context.Background(), false),
			} {
				err := f.recordAs(ctx, run, step)
				var denied *ForbiddenError
				if !errors.As(err, &denied) || denied.Permission != rbac.PermHostSelfManage {
					t.Fatalf("%s: want a refusal naming host_self.manage, got %v", name, err)
				}
			}
			if f.rt.restarts != 0 || f.rt.stops != 0 {
				t.Fatalf("a refused step reached Docker: %d restart(s), %d stop(s)", f.rt.restarts, f.rt.stops)
			}
		})
	}
}

// TestRestartAndStopRunOnAProtectedContainerWithHostSelf: the grant lets the same step through.
func TestRestartAndStopRunOnAProtectedContainerWithHostSelf(t *testing.T) {
	f, run, step := selfGuardCase(t, "container.restart", true)
	if err := f.recordAs(selfguard.WithDecision(context.Background(), true), run, step); err != nil {
		t.Fatalf("host_self.manage holder refused: %v", err)
	}
	if f.rt.restarts != 1 {
		t.Fatalf("restarts = %d, want 1", f.rt.restarts)
	}
}

// TestRestartOfAnOrdinaryContainerNeedsNoHostSelf: the guard costs an ordinary container nothing.
func TestRestartOfAnOrdinaryContainerNeedsNoHostSelf(t *testing.T) {
	f, run, step := selfGuardCase(t, "container.restart", false)
	if err := f.recordAs(context.Background(), run, step); err != nil {
		t.Fatalf("ordinary container refused: %v", err)
	}
	if f.rt.restarts != 1 {
		t.Fatalf("restarts = %d, want 1", f.rt.restarts)
	}
}
