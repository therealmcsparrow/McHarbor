// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package itsm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Dispatcher is what the service needs from core/notify: the channel's
// decrypted config and the optional interfaces its plugin implements.
type Dispatcher interface {
	IncidentClient(channelType string) (incident.LifecycleClient, bool)
	WebhookParser(channelType string) (incident.WebhookParser, bool)
	Poller(channelType string) (incident.Poller, bool)
	ChannelConfig(ctx context.Context, channelID string) (string, map[string]string, error)
}

// StatusTrigger receives every applied remote status, for workflows.
type StatusTrigger func(ctx context.Context, payload map[string]any)

// Service is the host side of the ITSM lifecycle.
type Service struct {
	db      *sql.DB
	store   *Store
	disp    Dispatcher
	enc     *encryption.Service
	logger  *slog.Logger
	now     func() time.Time
	trigger atomic.Pointer[StatusTrigger]
	acked   atomic.Pointer[func(stateKey string)]

	cancel     context.CancelFunc
	wg         sync.WaitGroup
	lastPolled map[string]time.Time
	pollMu     sync.Mutex
}

// NewService returns the ITSM service over database and the notification dispatcher.
func NewService(database *sql.DB, disp Dispatcher, enc *encryption.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: database, store: NewStore(database), disp: disp, enc: enc, logger: logger, now: time.Now,
		lastPolled: map[string]time.Time{}}
}

// SetStatusTrigger installs the workflow hook fired on every applied remote status.
// It is atomic because main wires it after the poller has already started.
func (s *Service) SetStatusTrigger(fn StatusTrigger) { s.trigger.Store(&fn) }

// SetAcknowledgedHook installs the callback run after an ITSM acknowledgement is written to
// alert_states, so a cache elsewhere can drop its stale copy without this module importing it.
func (s *Service) SetAcknowledgedHook(fn func(stateKey string)) { s.acked.Store(&fn) }

// Store exposes the link store to the handler.
func (s *Service) Store() *Store { return s.store }

// Supports reports whether a channel type's plugin can open tickets.
func (s *Service) Supports(channelType string) bool {
	if s == nil || s.disp == nil {
		return false
	}
	_, ok := s.disp.IncidentClient(channelType)
	return ok
}

// ChannelType resolves an enabled channel's type.
func (s *Service) ChannelType(ctx context.Context, channelID string) (string, error) {
	t, _, err := s.disp.ChannelConfig(ctx, channelID)
	return t, err
}

// Emit records a lifecycle transition and queues what it implies for the
// channel's ticket; delivery happens in the outbox, never in the caller.
func (s *Service) Emit(ctx context.Context, channelID string, ev incident.Event) error {
	if err := ev.Validate(); err != nil {
		return err
	}
	channelType, cfg, err := s.disp.ChannelConfig(ctx, channelID)
	if err != nil {
		return fmt.Errorf("itsm: resolving channel: %w", err)
	}
	switch ev.State {
	case incident.StateFiring:
		link, created, err := s.store.EnsureLink(ctx, channelID, channelType, ev.DedupKey, ev.AlertID, ev.EnvironmentID, ev.SubjectID)
		if err != nil {
			return err
		}
		switch {
		case created:
			return s.store.Enqueue(ctx, link.ID, channelID, OpOpen, ev)
		case link.LocalState == LinkOpen && cfg["update_on_repeat"] != "false":
			return s.store.Enqueue(ctx, link.ID, channelID, OpUpdate, ev)
		}
		return nil
	case incident.StateResolved:
		link, err := s.store.LiveLink(ctx, channelID, ev.DedupKey)
		if errors.Is(err, ErrNoLink) {
			return nil
		}
		if err != nil {
			return err
		}
		if link.LocalState == LinkRemoteClosed {
			return s.store.MarkResolved(ctx, link.ID)
		}
		return s.store.Enqueue(ctx, link.ID, channelID, OpResolve, ev)
	case incident.StateUpdate:
		link, err := s.store.LiveLink(ctx, channelID, ev.DedupKey)
		if errors.Is(err, ErrNoLink) {
			return nil
		}
		if err != nil {
			return err
		}
		if link.LocalState == LinkOpen || link.LocalState == LinkPending {
			return s.store.Enqueue(ctx, link.ID, channelID, OpUpdate, ev)
		}
	}
	return nil
}

// SafeMessage is what an operator may see about a failed delivery: the
// plugin's own words only when it marked them safe, because a vendor's
// response body can echo credentials.
func SafeMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, incident.ErrOperatorMessage) {
		msg := err.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return msg
	}
	return "delivery failed — see server log"
}

// channelSecrets holds what the host injects into a lifecycle call's config.
type channelSecrets struct {
	baseURL, token, secret string
}

func (s *Service) secrets(ctx context.Context, channelID string) (channelSecrets, error) {
	var tokenEnc, secretEnc, base string
	err := s.db.QueryRowContext(ctx,
		`SELECT webhook_token_enc, webhook_secret_enc, webhook_base_url FROM communication_channels WHERE id = ?`, channelID,
	).Scan(&tokenEnc, &secretEnc, &base)
	if errors.Is(err, sql.ErrNoRows) {
		return channelSecrets{}, nil
	}
	if err != nil {
		return channelSecrets{}, fmt.Errorf("itsm: reading channel webhook: %w", err)
	}
	out := channelSecrets{baseURL: base}
	if tokenEnc != "" {
		if out.token, err = s.enc.Decrypt(tokenEnc); err != nil {
			return channelSecrets{}, fmt.Errorf("itsm: decrypting webhook token: %w", err)
		}
	}
	if secretEnc != "" {
		if out.secret, err = s.enc.Decrypt(secretEnc); err != nil {
			return channelSecrets{}, fmt.Errorf("itsm: decrypting webhook secret: %w", err)
		}
	}
	return out, nil
}

// withHostKeys returns cfg plus the host-reserved keys every lifecycle call
// receives: the inbound URL and secret. Keys starting with _ are the host's.
func (s *Service) withHostKeys(ctx context.Context, channelID string, cfg map[string]string) (map[string]string, error) {
	sec, err := s.secrets(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(cfg)+2)
	for k, v := range cfg {
		if !strings.HasPrefix(k, "_") {
			out[k] = v
		}
	}
	out["_webhook_url"] = ""
	if sec.baseURL != "" && sec.token != "" {
		out["_webhook_url"] = strings.TrimRight(sec.baseURL, "/") + "/api/incident-webhooks/" + sec.token
	}
	out["_webhook_secret"] = sec.secret
	return out, nil
}

// ApplyRemote records what the ITSM says about a ticket it already has with
// McHarbor. Statuses for tickets McHarbor never opened are dropped: the ITSM
// may inform, it may never create.
func (s *Service) ApplyRemote(ctx context.Context, channelID string, st incident.RemoteStatus) (bool, error) {
	link, err := s.store.LinkByExternal(ctx, channelID, st.Ref.ExternalID)
	if errors.Is(err, ErrNoLink) && st.DedupKey != "" {
		link, err = s.store.LinkByKey(ctx, channelID, st.DedupKey)
	}
	if errors.Is(err, ErrNoLink) || st.Ref.ExternalID == "" && st.DedupKey == "" {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if link.LocalState == LinkOrphaned {
		return false, nil
	}
	if st.UpdatedAt.IsZero() {
		st.UpdatedAt = s.now()
	}
	// Vendors that reuse the dedup key across incidents (PagerDuty) can deliver a
	// late status from the previous incident; one older than this link is not ours.
	if link.OpenedAt != "" {
		if opened, err := time.Parse(time.RFC3339, link.OpenedAt); err == nil && st.UpdatedAt.Before(opened.Add(-openedSkew)) {
			return false, nil
		}
	}
	if link.LastSyncedAt != "" {
		if last, err := time.Parse(time.RFC3339, link.LastSyncedAt); err == nil && st.UpdatedAt.Before(last) {
			return false, nil
		}
	}
	firing, err := s.alertFiring(ctx, link.DedupKey)
	if err != nil {
		return false, err
	}
	if err := s.store.MarkRemote(ctx, link.ID, st, firing); err != nil {
		return false, err
	}
	if st.State == incident.RemoteAcknowledged {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE alert_states SET acknowledged_at = ?, acknowledged_by = ? WHERE state_key = ? AND active = 1`,
			s.now().UTC().Format(time.RFC3339), "itsm:"+st.Assignee, link.DedupKey); err != nil {
			return false, fmt.Errorf("itsm: recording acknowledgement: %w", err)
		}
		if hook := s.acked.Load(); hook != nil && *hook != nil {
			(*hook)(link.DedupKey)
		}
	}
	if st.State.Terminal() {
		if err := s.store.DropQueue(ctx, link.ID); err != nil {
			return false, err
		}
	}
	if fire := s.trigger.Load(); fire != nil && *fire != nil {
		(*fire)(ctx, map[string]any{
			"channelId": channelID, "channelType": link.ChannelType, "externalId": st.Ref.ExternalID,
			"number": st.Ref.Number, "url": st.Ref.URL, "dedupKey": link.DedupKey, "state": string(st.State),
			"assignee": st.Assignee, "note": st.Note,
		})
	}
	return true, nil
}

// openedSkew tolerates a vendor clock running behind McHarbor's when comparing a
// status time with the moment the link's ticket was opened.
const openedSkew = time.Minute

func (s *Service) alertFiring(ctx context.Context, stateKey string) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, `SELECT active FROM alert_states WHERE state_key = ?`, stateKey).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("itsm: reading alert state: %w", err)
	}
	return active, nil
}
