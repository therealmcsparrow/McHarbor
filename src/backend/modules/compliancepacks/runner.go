// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// runBudget bounds one sweep. A CIS sweep over an agent environment is many
// one-shot calls at ~2s each, so it cannot live inside a request's context; the
// runner owns its own deadline and the handler answers as soon as the row exists.
const runBudget = 10 * time.Minute

// ErrFrameworkNotRunnable is returned for a framework that is a mapping rather
// than a check suite, so the handler can answer 400 instead of recording an empty
// run that looks like a clean sweep.
var ErrFrameworkNotRunnable = errors.New("framework has no mechanical checks")

// ErrUnknownFramework is returned for a framework key no pack declares.
var ErrUnknownFramework = errors.New("unknown framework")

// Runner executes a pack against an environment and persists the result.
type Runner struct {
	svc      *Service
	gatherer *Gatherer
	logger   *slog.Logger
}

// NewRunner returns a Runner writing through svc and measuring through gatherer.
func NewRunner(svc *Service, gatherer *Gatherer, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{svc: svc, gatherer: gatherer, logger: logger}
}

// Begin records the run row and returns its ID without measuring anything, so a
// request can answer immediately and the operator sees a `running` row rather than
// waiting minutes for a daemon sweep.
func (r *Runner) Begin(ctx context.Context, framework, envID, trigger, actor string) (string, error) {
	f, ok := FrameworkByKey(framework)
	if !ok {
		return "", ErrUnknownFramework
	}
	if !f.Runnable {
		return "", ErrFrameworkNotRunnable
	}
	return r.svc.StartRun(ctx, framework, envID, trigger, actor)
}

// Execute measures, evaluates and records. A failure closes the run with its
// message rather than leaving a row stuck at `running`, because an unfinished run
// is indistinguishable from a crashed one.
func (r *Runner) Execute(ctx context.Context, runID, framework, envID string) error {
	f, ok := FrameworkByKey(framework)
	if !ok {
		return ErrUnknownFramework
	}
	runCtx, cancel := context.WithTimeout(ctx, runBudget)
	defer cancel()
	in := r.gatherer.Gather(runCtx, envID)
	results := Evaluate(f, in)
	if err := r.svc.SaveResults(ctx, runID, results); err != nil {
		r.logger.Error("compliance pack: saving results failed", "run", runID, "error", err)
		if failErr := r.svc.FailRun(ctx, runID, "the check results could not be recorded"); failErr != nil {
			r.logger.Error("compliance pack: marking the run failed also failed", "run", runID, "error", failErr)
		}
		return fmt.Errorf("execute compliance pack run: %w", err)
	}
	r.logger.Info("compliance pack run recorded", "run", runID, "framework", framework,
		"environment", envID, "verdict", string(Worst(results)))
	return nil
}

// Run is Begin followed by Execute, for the scheduler, which has nobody to answer
// to and so waits for the sweep.
func (r *Runner) Run(ctx context.Context, framework, envID, trigger, actor string) (string, error) {
	runID, err := r.Begin(ctx, framework, envID, trigger, actor)
	if err != nil {
		return "", err
	}
	return runID, r.Execute(ctx, runID, framework, envID)
}
