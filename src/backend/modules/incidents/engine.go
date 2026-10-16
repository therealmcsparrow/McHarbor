// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Engine cadences.
const (
	scanInterval      = 30 * time.Second
	behaviourInterval = time.Hour
	renarrateAfter    = 30 * time.Minute
)

// Coordinator elects the one node that scans, so a cluster opens each
// incident once.
type Coordinator interface {
	LeaderOf(ctx context.Context, name string) bool
}

// Engine runs detection, evidence, proactive narration, resolution and
// behavioural detection in the background.
type Engine struct {
	Service   *Service
	Detector  *Detector
	Behaviour *Behaviour
	Coord     Coordinator
	Logger    *slog.Logger

	lastBehaviour time.Time
	done          chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// Start runs the engine until Stop or ctx ends.
func (e *Engine) Start(ctx context.Context) {
	e.done = make(chan struct{})
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(scanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.done:
				return
			case <-ticker.C:
				e.Tick(ctx)
			}
		}
	}()
}

// Stop ends the engine and waits for a tick in flight.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		if e.done != nil {
			close(e.done)
		}
	})
	e.wg.Wait()
}

func (e *Engine) log() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

// Tick runs one pass. It is exported so tests drive the engine without timers.
func (e *Engine) Tick(ctx context.Context) {
	if e.Coord != nil && !e.Coord.LeaderOf(ctx, "incidents") {
		return
	}
	res, err := e.Detector.Scan(ctx)
	if err != nil {
		e.log().Error("incidents: scan failed", "error", err)
		return
	}
	for _, in := range res.Opened {
		e.opened(ctx, in)
	}
	for _, in := range res.Escalated {
		e.escalated(ctx, in)
	}
	e.resolveQuiet(ctx)
	now := e.Service.now()
	if e.Behaviour != nil && now.Sub(e.lastBehaviour) >= behaviourInterval {
		e.lastBehaviour = now
		if _, err := e.Behaviour.Run(ctx); err != nil {
			e.log().Error("incidents: behaviour detection failed", "error", err)
		}
	}
}

func (e *Engine) opened(ctx context.Context, in *Incident) {
	e.Service.audit("incident.opened", in, map[string]any{"signal": in.Signal, "signals": in.SignalCount})
	ev, err := e.Service.Assembler.Assemble(ctx, in)
	if err != nil {
		e.log().Warn("incidents: assembling evidence failed", "incident", in.ID, "error", err)
		return
	}
	if err := e.Service.Store.SaveEvidence(ctx, in.ID, ev); err != nil {
		e.log().Warn("incidents: saving evidence failed", "incident", in.ID, "error", err)
		return
	}
	e.proactive(ctx, in.ID, in.EnvID)
}

// escalated re-narrates an incident whose signal got worse — a single exit
// that became a crash loop — at most once per half hour.
func (e *Engine) escalated(ctx context.Context, in *Incident) {
	if in.NarratedAt != "" {
		if at, err := time.Parse(time.RFC3339, in.NarratedAt); err == nil && e.Service.now().Sub(at) < renarrateAfter {
			return
		}
	}
	e.proactive(ctx, in.ID, in.EnvID)
}

func (e *Engine) proactive(ctx context.Context, id, envID string) {
	st, err := e.Service.Store.Settings(ctx, envID)
	if err != nil || !st.NarrateOptIn || !st.Proactive {
		return
	}
	if _, err := e.Service.Narrate(ctx, id, "system:narrator", true); err != nil &&
		!errors.Is(err, ErrCapReached) && !errors.Is(err, ErrInProgress) && !errors.Is(err, ErrNoModel) {
		e.log().Warn("incidents: proactive narration failed", "incident", id, "error", err)
	}
}

func (e *Engine) resolveQuiet(ctx context.Context) {
	quiet, err := e.Service.Store.Quiet(ctx, e.Service.now().Add(-quietToResolve))
	if err != nil {
		e.log().Error("incidents: listing quiet incidents failed", "error", err)
		return
	}
	for i := range quiet {
		in := &quiet[i]
		settled, err := e.Detector.Settled(ctx, in)
		if err != nil {
			e.log().Warn("incidents: checking recovery failed", "incident", in.ID, "error", err)
			continue
		}
		if !settled {
			continue
		}
		if _, err := e.Service.ResolveNow(ctx, in, "system:narrator"); err != nil {
			e.log().Warn("incidents: resolving failed", "incident", in.ID, "error", err)
			continue
		}
		st, err := e.Service.Store.Settings(ctx, in.EnvID)
		if err == nil && st.NarrateOptIn && st.Proactive {
			if _, err := e.Service.Summarise(ctx, in.ID, "system:narrator", true); err != nil && !errors.Is(err, ErrNoModel) {
				e.log().Warn("incidents: post-incident summary failed", "incident", in.ID, "error", err)
			}
		}
	}
}
