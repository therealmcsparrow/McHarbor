// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package itsm

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Outbox limits. Twelve attempts at the capped backoff spans a few hours,
// long enough for a vendor outage, short enough that a broken config surfaces.
const (
	maxAttempts  = 12
	baseBackoff  = 10 * time.Second
	maxBackoff   = 30 * time.Minute
	vendorCall   = 30 * time.Second
	tickInterval = 5 * time.Second
	batchSize    = 50
	pollBatch    = 50
)

// Coordinator elects the node that delivers, so one ticket operation is sent once.
type Coordinator interface {
	LeaderOf(ctx context.Context, name string) bool
}

// backoff is 2^attempts * 10s capped at 30 minutes, with ±20% jitter so a
// vendor recovering from an outage is not hit by every channel at once.
func backoff(attempts int) time.Duration {
	d := baseBackoff
	for i := 0; i < attempts && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	jitter := 0.8 + rand.Float64()*0.4 //nolint:gosec // retry-backoff jitter is not security-sensitive
	return time.Duration(float64(d) * jitter)
}

// Start runs the outbox and the poller until Stop.
func (s *Service) Start(ctx context.Context, coord Coordinator) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(tickInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if coord != nil && !coord.LeaderOf(ctx, "itsm") {
					continue
				}
				s.ProcessOnce(ctx)
				s.PollDue(ctx)
			}
		}
	}()
}

// Stop ends the worker and waits for the pass in flight.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// ProcessOnce delivers every due operation once. Each link's operations go
// strictly in sequence: a resolve never overtakes the open it depends on.
func (s *Service) ProcessOnce(ctx context.Context) {
	if n, err := s.store.OrphanDeletedChannels(ctx); err != nil {
		s.logger.Error("itsm: orphaning links of deleted channels failed", "error", err)
	} else if n > 0 {
		s.logger.Info("itsm: orphaned links of deleted channels", "links", n)
	}
	rows, err := s.store.Heads(ctx, s.now(), batchSize)
	if err != nil {
		s.logger.Error("itsm: reading the outbox failed", "error", err)
		return
	}
	for _, row := range rows {
		s.deliver(ctx, row)
	}
}

func (s *Service) deliver(ctx context.Context, row OutboxRow) {
	link, err := s.store.LinkByID(ctx, row.LinkID)
	if errors.Is(err, ErrNoLink) || err == nil && link.LocalState == LinkOrphaned {
		if dropErr := s.store.DropQueue(ctx, row.LinkID); dropErr != nil {
			s.logger.Error("itsm: dropping queue failed", "link", row.LinkID, "error", dropErr)
		}
		return
	}
	if err != nil {
		s.logger.Error("itsm: reading link failed", "link", row.LinkID, "error", err)
		return
	}
	channelType, cfg, err := s.disp.ChannelConfig(ctx, row.ChannelID)
	if errors.Is(err, notify.ErrChannelTargetNotFound) {
		s.fail(ctx, row, link, fmt.Errorf("%w: channel disabled or missing", incident.ErrRetryable))
		return
	}
	if err != nil {
		s.fail(ctx, row, link, fmt.Errorf("%w: %w", incident.ErrRetryable, err))
		return
	}
	client, ok := s.disp.IncidentClient(channelType)
	if !ok {
		s.fail(ctx, row, link, fmt.Errorf("%w: channel type %s cannot open tickets", incident.ErrOperatorMessage, channelType))
		return
	}
	cfg, err = s.withHostKeys(ctx, row.ChannelID, cfg)
	if err != nil {
		s.fail(ctx, row, link, fmt.Errorf("%w: %w", incident.ErrRetryable, err))
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, vendorCall)
	defer cancel()
	ref := incident.Ref{ExternalID: link.ExternalID, Number: link.ExternalNumber, URL: link.ExternalURL}

	switch row.Op {
	case OpOpen:
		got, err := client.Open(callCtx, cfg, row.Event)
		if got.ExternalID != "" {
			// A ticket exists even if a follow-up call failed; retrying the
			// open would create a second one, so the error is only recorded.
			if markErr := s.store.MarkOpened(ctx, link.ID, got, SafeMessage(err)); markErr != nil {
				s.logger.Error("itsm: recording opened ticket failed", "link", link.ID, "error", markErr)
				return
			}
			if err != nil {
				s.logger.Warn("itsm: ticket opened with a follow-up failure", "channel", row.ChannelID, "link", link.ID, "error", err)
			}
			s.done(ctx, row)
			return
		}
		if err == nil {
			err = fmt.Errorf("%w: plugin returned no ticket id", incident.ErrOperatorMessage)
		}
		s.fail(ctx, row, link, err)
	case OpUpdate, OpResolve:
		if ref.ExternalID == "" {
			if err := s.store.Postpone(ctx, row.ID, s.now().Add(baseBackoff)); err != nil {
				s.logger.Error("itsm: postponing failed", "row", row.ID, "error", err)
			}
			return
		}
		if row.Op == OpUpdate {
			err = client.Update(callCtx, cfg, ref, row.Event)
		} else {
			err = client.Resolve(callCtx, cfg, ref, row.Event)
		}
		switch {
		case errors.Is(err, incident.ErrNotFound):
			if markErr := s.store.MarkOrphaned(ctx, link.ID); markErr != nil {
				s.logger.Error("itsm: orphaning link failed", "link", link.ID, "error", markErr)
			}
			if dropErr := s.store.DropQueue(ctx, link.ID); dropErr != nil {
				s.logger.Error("itsm: dropping queue failed", "link", link.ID, "error", dropErr)
			}
		case err != nil:
			s.fail(ctx, row, link, err)
		default:
			if row.Op == OpResolve {
				if markErr := s.store.MarkResolved(ctx, link.ID); markErr != nil {
					s.logger.Error("itsm: resolving link failed", "link", link.ID, "error", markErr)
				}
			} else if clearErr := s.store.SetError(ctx, link.ID, ""); clearErr != nil {
				s.logger.Error("itsm: clearing error failed", "link", link.ID, "error", clearErr)
			}
			s.done(ctx, row)
		}
	default:
		s.fail(ctx, row, link, fmt.Errorf("%w: unknown operation %q", incident.ErrOperatorMessage, row.Op))
	}
}

func (s *Service) done(ctx context.Context, row OutboxRow) {
	if err := s.store.Done(ctx, row.ID); err != nil {
		s.logger.Error("itsm: removing delivered row failed", "row", row.ID, "error", err)
	}
}

// fail retries a transient failure with backoff and parks anything else —
// or anything that has failed too often — with a message for the operator.
func (s *Service) fail(ctx context.Context, row OutboxRow, link Link, err error) {
	attempts := row.Attempts + 1
	s.logger.Warn("itsm: delivery failed", "channel", row.ChannelID, "link", link.ID, "op", row.Op, "attempt", attempts, "error", err)
	msg := SafeMessage(err)
	if errors.Is(err, incident.ErrRetryable) && attempts < maxAttempts {
		wait := backoff(attempts)
		var ra *incident.RetryAfterError
		if errors.As(err, &ra) && ra.After > 0 {
			wait = ra.After
		}
		if rerr := s.store.Retry(ctx, row.ID, attempts, s.now().Add(wait), msg); rerr != nil {
			s.logger.Error("itsm: rescheduling failed", "row", row.ID, "error", rerr)
		}
	} else if perr := s.store.Park(ctx, row.ID, msg); perr != nil {
		s.logger.Error("itsm: parking failed", "row", row.ID, "error", perr)
	}
	if serr := s.store.SetError(ctx, link.ID, msg); serr != nil && !errors.Is(serr, ErrNoLink) {
		s.logger.Error("itsm: recording link error failed", "link", link.ID, "error", serr)
	}
}

// PollDue polls each channel whose plugin can poll, on the channel's own
// poll_interval_minutes (default 5; 0 turns polling off).
func (s *Service) PollDue(ctx context.Context) {
	ids, err := s.enabledChannelIDs(ctx)
	if err != nil {
		s.logger.Error("itsm: listing channels failed", "error", err)
		return
	}
	for _, id := range ids {
		s.pollChannel(ctx, id, false)
	}
}

// enabledChannelIDs reads the ids up front and closes the cursor before any
// polling starts, because a poll writes and SQLite would hold both open.
func (s *Service) enabledChannelIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM communication_channels WHERE enabled = 1 LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			s.logger.Error("itsm: closing channel rows failed", "error", cerr)
		}
	}()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func pollInterval(cfg map[string]string) time.Duration {
	v := cfg["poll_interval_minutes"]
	if v == "" {
		return 5 * time.Minute
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 5 * time.Minute
	}
	return time.Duration(n) * time.Minute
}

func (s *Service) pollChannel(ctx context.Context, channelID string, force bool) {
	channelType, cfg, err := s.disp.ChannelConfig(ctx, channelID)
	if err != nil {
		return
	}
	poller, ok := s.disp.Poller(channelType)
	if !ok {
		return
	}
	interval := pollInterval(cfg)
	s.pollMu.Lock()
	last := s.lastPolled[channelID]
	due := force || (interval > 0 && s.now().Sub(last) >= interval)
	s.pollMu.Unlock()
	if !due {
		return
	}
	links, err := s.store.OpenLinks(ctx, channelID, 1000)
	if err != nil || len(links) == 0 {
		return
	}
	cfg, err = s.withHostKeys(ctx, channelID, cfg)
	if err != nil {
		s.logger.Error("itsm: preparing poll failed", "channel", channelID, "error", err)
		return
	}
	for start := 0; start < len(links); start += pollBatch {
		end := min(start+pollBatch, len(links))
		refs := make([]incident.Ref, 0, end-start)
		for _, l := range links[start:end] {
			refs = append(refs, incident.Ref{ExternalID: l.ExternalID, Number: l.ExternalNumber, URL: l.ExternalURL})
		}
		callCtx, cancel := context.WithTimeout(ctx, vendorCall)
		statuses, err := poller.Poll(callCtx, cfg, refs)
		cancel()
		if err != nil {
			s.logger.Warn("itsm: poll failed", "channel", channelID, "error", err)
			return
		}
		for _, st := range statuses {
			if _, err := s.ApplyRemote(ctx, channelID, st); err != nil {
				s.logger.Error("itsm: applying polled status failed", "channel", channelID, "error", err)
			}
		}
	}
	s.pollMu.Lock()
	s.lastPolled[channelID] = s.now()
	s.pollMu.Unlock()
}

// Resync polls one link's ticket now, when its channel can poll.
func (s *Service) Resync(ctx context.Context, linkID string) error {
	link, err := s.store.LinkByID(ctx, linkID)
	if err != nil {
		return err
	}
	channelType, cfg, err := s.disp.ChannelConfig(ctx, link.ChannelID)
	if err != nil {
		return err
	}
	poller, ok := s.disp.Poller(channelType)
	if !ok || link.ExternalID == "" {
		return ErrCannotPoll
	}
	cfg, err = s.withHostKeys(ctx, link.ChannelID, cfg)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, vendorCall)
	defer cancel()
	statuses, err := poller.Poll(callCtx, cfg, []incident.Ref{{ExternalID: link.ExternalID, Number: link.ExternalNumber, URL: link.ExternalURL}})
	if err != nil {
		return fmt.Errorf("itsm: resync poll: %w", err)
	}
	for _, st := range statuses {
		if _, err := s.ApplyRemote(ctx, link.ChannelID, st); err != nil {
			return err
		}
	}
	return nil
}

// ErrCannotPoll is returned when a resync is asked of a channel that cannot poll.
var ErrCannotPoll = errors.New("itsm: this channel cannot be polled")
