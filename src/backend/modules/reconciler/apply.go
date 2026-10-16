// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package reconciler detects drift between declared and running state and proposes fixes.
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// Actuator is everything a plan may do to an environment. It is kept apart
// from the detector's read-only interface so observing can never act.
type Actuator interface {
	Start(ctx context.Context, envID, name string) error
	Stop(ctx context.Context, envID, name string) error
	Remove(ctx context.Context, envID, name string) error
	Recreate(ctx context.Context, envID, name string, req docker.RecreateRequest) error
	StackUp(ctx context.Context, stack string) error
}

// StepResult is what applying one step did.
type StepResult struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Step result statuses.
const (
	StepDone    = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
	StepManual  = "manual"
)

// Apply runs the applicable steps in order and stops at the first failure, so a
// plan never carries on from a state it did not expect. Manual steps are listed
// as left to a person; later steps after a failure are listed as skipped.
func Apply(ctx context.Context, act Actuator, envID string, steps []Step) ([]StepResult, error) {
	results := make([]StepResult, 0, len(steps))
	var failure error
	for _, s := range steps {
		r := StepResult{Action: s.Action, Target: s.Target}
		switch {
		case !s.Applicable():
			r.Status = StepManual
		case failure != nil:
			r.Status = StepSkipped
		default:
			if err := applyStep(ctx, act, envID, s); err != nil {
				r.Status, r.Error = StepFailed, err.Error()
				failure = fmt.Errorf("%s %s: %w", s.Action, s.Target, err)
			} else {
				r.Status = StepDone
			}
		}
		results = append(results, r)
	}
	return results, failure
}

func applyStep(ctx context.Context, act Actuator, envID string, s Step) error {
	switch s.Action {
	case ActionStart:
		return act.Start(ctx, envID, s.Target)
	case ActionStop:
		return act.Stop(ctx, envID, s.Target)
	case ActionRemove:
		return act.Remove(ctx, envID, s.Target)
	case ActionRecreate:
		if s.Recreate == nil {
			return errors.New("recreate step without overrides")
		}
		for _, kv := range s.Recreate.Env {
			if _, v, _ := strings.Cut(kv, "="); v == envredact.Marker {
				return errors.New("recreate step carries a withheld env value")
			}
		}
		return act.Recreate(ctx, envID, s.Target, *s.Recreate)
	case ActionStackUp:
		return act.StackUp(ctx, s.Target)
	default:
		return fmt.Errorf("unknown action %q", s.Action)
	}
}
