// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/nodelink"
)

const (
	// fireForwardAttempt bounds one offer round across the peers. The first round runs
	// on the plugin's call path, which must not stall its lifecycle recording.
	fireForwardAttempt = 3 * time.Second
	// fireRetryWindow is how long a fire that found no leader keeps being retried.
	fireRetryWindow = 60 * time.Second
	// fireRetryFirst and fireRetryMax bound the backoff between retries.
	fireRetryFirst = 500 * time.Millisecond
	fireRetryMax   = 8 * time.Second
	// fireMaxPending caps fires waiting to be retried, so a leaderless cluster under a
	// storm of plugin events costs bounded memory and goroutines.
	fireMaxPending = 256
)

// triggerFireLink is the half of the node link the relay sends through.
type triggerFireLink interface {
	ForwardTriggerFire(ctx context.Context, fire nodelink.TriggerFire) (nodelink.TriggerFireResult, error)
}

// fireRelay carries plugin trigger fires from a node that does not lead the workflow
// trigger to the one that does. The fire ids the leader has evaluated live in the shared
// ledger, not here, so they survive the leader changing.
type fireRelay struct {
	link   triggerFireLink
	ledger *fireLedger

	first, max, window time.Duration
	attempt            time.Duration

	pending chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup

	// mu orders wg.Add against close: once closed is set no retry is added, so wg.Wait
	// never races a late Add.
	mu     sync.Mutex
	closed bool
}

func newFireRelay(link triggerFireLink, database *sql.DB) *fireRelay {
	r := &fireRelay{
		link:    link,
		first:   fireRetryFirst,
		max:     fireRetryMax,
		window:  fireRetryWindow,
		attempt: fireForwardAttempt,
		pending: make(chan struct{}, fireMaxPending),
		done:    make(chan struct{}),
	}
	if database != nil {
		r.ledger = newFireLedger(database)
	}
	return r
}

// track registers one retry goroutine, refusing once the relay is closing.
func (r *fireRelay) track() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	r.wg.Add(1)
	return true
}

// close stops every pending retry and waits for them to end.
func (r *fireRelay) close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.done)
	}
	r.mu.Unlock()
	r.wg.Wait()
}

// forwardPluginFire hands matched bindings to the leader. One bounded attempt runs on the
// caller's path so the plugin still learns how many runs started; a failure moves the fire
// to a background retry and returns 0, because the fire is not lost yet.
func (ts *TriggerService) forwardPluginFire(ctx context.Context, source, actionKey string, payload map[string]interface{}, matched []PluginTriggerBinding) (int, error) {
	fire := nodelink.TriggerFire{
		FireID:    xid.New().String(),
		ActionKey: actionKey,
		Source:    source,
		Origin:    ts.nodeID,
		Payload:   payload,
		Bindings:  make([]nodelink.TriggerFireBinding, 0, len(matched)),
	}
	for _, b := range matched {
		fire.Bindings = append(fire.Bindings, nodelink.TriggerFireBinding{WorkflowID: b.WorkflowID, NodeID: b.NodeID})
	}
	if ts.fires == nil || ts.fires.link == nil {
		ts.logger.Warn("workflow trigger: not leader and no node link to forward the plugin fire; dropped",
			"node", ts.nodeID, "action", actionKey, "fire", fire.FireID, "bindings", len(matched))
		return 0, nil
	}
	attemptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ts.fires.attempt)
	res, err := ts.fires.link.ForwardTriggerFire(attemptCtx, fire)
	cancel()
	if err == nil {
		ts.logger.Info("workflow trigger: plugin fire forwarded to the leader", "fire", fire.FireID,
			"action", actionKey, "leader", res.ExecutedBy, "started", res.Started, "duplicate", res.Duplicate)
		return res.Started, nil
	}
	if errors.Is(err, nodelink.ErrTriggerFireUnsendable) || errors.Is(err, nodelink.ErrFireNotAllowed) {
		ts.logger.Error("workflow trigger: plugin fire cannot be forwarded; dropped", "fire", fire.FireID,
			"action", actionKey, "error", err)
		return 0, err
	}
	ts.logger.Warn("workflow trigger: forwarding the plugin fire failed; retrying in the background",
		"fire", fire.FireID, "action", actionKey, "error", err)
	ts.retryPluginFire(ctx, fire)
	return 0, nil
}

// retryPluginFire keeps offering fire with exponential backoff until it is delivered, the
// retry window closes, or the trigger service stops. If this node wins the leadership in
// the meantime it evaluates the fire itself, through the same shared ledger. The
// plugin's cancellation is dropped: its call has returned, and the fire outlives it.
func (ts *TriggerService) retryPluginFire(ctx context.Context, fire nodelink.TriggerFire) {
	r := ts.fires
	base := context.WithoutCancel(ctx)
	select {
	case r.pending <- struct{}{}:
	default:
		ts.logger.Error("workflow trigger: too many plugin fires waiting for a leader; dropped",
			"fire", fire.FireID, "action", fire.ActionKey, "pending", cap(r.pending))
		return
	}
	if !r.track() {
		<-r.pending
		ts.logger.Warn("workflow trigger: shutting down with a plugin fire undelivered", "fire", fire.FireID,
			"action", fire.ActionKey, "attempts", 1)
		return
	}
	// goroutine lifecycle: ends on delivery, when the retry window closes, or when Stop closes r.done.
	go func() {
		defer r.wg.Done()
		defer func() { <-r.pending }()
		deadline := time.Now().Add(r.window)
		delay := r.first
		timer := time.NewTimer(delay)
		defer timer.Stop()
		for attempt := 2; ; attempt++ {
			select {
			case <-r.done:
				ts.logger.Warn("workflow trigger: shutting down with a plugin fire undelivered", "fire", fire.FireID,
					"action", fire.ActionKey, "attempts", attempt-1)
				return
			case <-timer.C:
			}
			res, err := ts.deliverPluginFire(base, fire)
			if err == nil {
				ts.logger.Info("workflow trigger: plugin fire delivered after retry", "fire", fire.FireID,
					"action", fire.ActionKey, "leader", res.ExecutedBy, "started", res.Started,
					"duplicate", res.Duplicate, "attempt", attempt)
				return
			}
			if errors.Is(err, nodelink.ErrFireNotAllowed) || errors.Is(err, nodelink.ErrTriggerFireUnsendable) {
				ts.logger.Error("workflow trigger: plugin fire refused; dropped", "fire", fire.FireID,
					"action", fire.ActionKey, "error", err)
				return
			}
			if !time.Now().Add(delay).Before(deadline) {
				ts.logger.Error("workflow trigger: no leader accepted the plugin fire; dropped", "fire", fire.FireID,
					"action", fire.ActionKey, "origin", fire.Origin, "attempts", attempt, "error", err)
				return
			}
			ts.logger.Warn("workflow trigger: plugin fire retry failed", "fire", fire.FireID,
				"action", fire.ActionKey, "attempt", attempt, "error", err)
			delay = min(delay*2, r.max)
			timer.Reset(delay)
		}
	}()
}

// deliverPluginFire makes one delivery attempt: locally when this node now leads, and
// over the link otherwise.
func (ts *TriggerService) deliverPluginFire(ctx context.Context, fire nodelink.TriggerFire) (nodelink.TriggerFireResult, error) {
	ctx, cancel := context.WithTimeout(ctx, ts.fires.attempt)
	defer cancel()
	if ts.leaderFor(ctx) {
		res, err := ts.ReceiveTriggerFire(ctx, fire)
		res.ExecutedBy = ts.nodeID
		return res, err
	}
	return ts.fires.link.ForwardTriggerFire(ctx, fire)
}

// LeadsWorkflowTrigger answers another node's signed leader probe.
func (ts *TriggerService) LeadsWorkflowTrigger(ctx context.Context) bool {
	return ts.leaderFor(ctx)
}

// ReceiveTriggerFire evaluates a fire forwarded by another node. Only the workflow-trigger
// leader accepts it, only an action key its source may fire is evaluated, a fire id is
// claimed in the shared ledger before any run starts, and only the bindings the sender's
// plugin matched and that still exist here are started.
func (ts *TriggerService) ReceiveTriggerFire(ctx context.Context, fire nodelink.TriggerFire) (nodelink.TriggerFireResult, error) {
	if !ts.leaderFor(ctx) {
		return nodelink.TriggerFireResult{}, nodelink.ErrNotLeader
	}
	if err := fireAllowed(fire.Source, fire.ActionKey); err != nil {
		return nodelink.TriggerFireResult{}, err
	}
	ledger := ts.fireLedger()
	if ledger == nil {
		return nodelink.TriggerFireResult{}, fmt.Errorf("workflows: no fire ledger to de-duplicate fire %s", fire.FireID)
	}
	fresh, err := ledger.claim(ctx, fire)
	if err != nil {
		return nodelink.TriggerFireResult{}, err
	}
	if !fresh {
		ts.logger.Info("workflow trigger: duplicate plugin fire ignored", "fire", fire.FireID,
			"action", fire.ActionKey, "origin", fire.Origin)
		return nodelink.TriggerFireResult{FireID: fire.FireID, Duplicate: true}, nil
	}
	bindings, err := ts.PluginTriggerBindings(ctx, fire.ActionKey)
	if err != nil {
		if forgetErr := ledger.forget(context.WithoutCancel(ctx), fire.FireID); forgetErr != nil {
			ts.logger.Warn("workflow trigger: releasing a fire id after a failed evaluation", "fire", fire.FireID, "error", forgetErr)
		}
		return nodelink.TriggerFireResult{}, err
	}
	accepted := make(map[[2]string]bool, len(fire.Bindings))
	for _, b := range fire.Bindings {
		accepted[[2]string{b.WorkflowID, b.NodeID}] = true
	}
	keep := make([]PluginTriggerBinding, 0, len(fire.Bindings))
	for _, b := range bindings {
		if accepted[[2]string{b.WorkflowID, b.NodeID}] {
			keep = append(keep, b)
		}
	}
	started := ts.startPluginRuns(ctx, fire.ActionKey, fire.Payload, keep)
	ts.logger.Info("workflow trigger: forwarded plugin fire evaluated", "fire", fire.FireID,
		"action", fire.ActionKey, "source", fire.Source, "origin", fire.Origin, "started", started)
	return nodelink.TriggerFireResult{FireID: fire.FireID, Started: started}, nil
}
