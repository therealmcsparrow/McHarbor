// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"html"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	statusSolved = 5
	statusClosed = 6
)

var (
	_ plugin.NotificationAdapterClient = (*Client)(nil)
	_ incident.LifecycleClient         = (*Client)(nil)
	_ incident.WebhookParser           = (*Client)(nil)
	_ incident.Poller                  = (*Client)(nil)
)

// with runs fn inside one backend and always closes it, so a v1 session is killed on every
// error path. strict reports a close failure only when fn succeeded and a retry is harmless.
func (c *Client) with(ctx context.Context, cfg glpiConfig, strict bool, fn func(glpiBackend) error) (err error) {
	b, err := c.open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		cerr := b.close()
		if strict && err == nil && cerr != nil {
			err = cerr
		}
	}()
	return fn(b)
}

func ticketID(ref incident.Ref) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(ref.ExternalID))
	if err != nil || id <= 0 {
		return 0, operatorError("ticket reference %q is not a GLPI ticket id", sanitize(ref.ExternalID))
	}
	return id, nil
}

// Test proves the URL, the credentials and the entity before any ticket depends on them; it
// only reads, so pressing it never creates a ticket.
func (c *Client) Test(ctx context.Context, config map[string]string) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	return c.with(ctx, cfg, true, func(b glpiBackend) error {
		switch be := b.(type) {
		case *v1:
			ids, err := be.activeEntities(ctx)
			if err != nil {
				return err
			}
			if cfg.EntityID > 0 && !slices.Contains(ids, cfg.EntityID) {
				return operatorError("entity %d is not active for the API user", cfg.EntityID)
			}
			return nil
		case *v2:
			_, err := be.call(ctx, http.MethodGet, "/Assistance/Ticket?limit=1", nil, nil)
			if errors.Is(err, incident.ErrNotFound) {
				return operatorError("GLPI answered 404 on %s; check the HL API version path", cfg.V2Path)
			}
			return err
		}
		return nil
	})
}

// Send is the plain-notification fallback: it opens a ticket with no externalid, so the incident
// lifecycle never mistakes it for one of its own.
func (c *Client) Send(ctx context.Context, config map[string]string, msg plugin.NotificationMessage) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	sev := incident.SeverityInfo
	switch strings.ToLower(msg.Level) {
	case "error", "critical":
		sev = incident.SeverityCritical
	case "warning":
		sev = incident.SeverityWarning
	}
	title := strings.TrimSpace(msg.Title)
	if title == "" {
		title = "McHarbor notification"
	}
	f := cfg.Severity[sev]
	in := ticketInput{
		Name:       truncateRunes(title, maxNameRunes),
		Content:    "<p><strong>" + html.EscapeString(title) + "</strong></p>" + paragraph(msg.Message),
		Urgency:    f.Urgency,
		Impact:     f.Impact,
		Priority:   f.Priority,
		EntityID:   cfg.EntityID,
		CategoryID: cfg.CategoryID,
		GroupID:    cfg.GroupID,
	}
	return c.with(ctx, cfg, false, func(b glpiBackend) error {
		_, err := b.create(ctx, in)
		return err
	})
}

// Open returns the live ticket already carrying ev's externalid, or creates one, so a retried
// Open after a lost response never creates a second ticket.
func (c *Client) Open(ctx context.Context, config map[string]string, ev incident.Event) (incident.Ref, error) {
	if err := ev.Validate(); err != nil {
		return incident.Ref{}, err
	}
	cfg, err := parseConfig(config)
	if err != nil {
		return incident.Ref{}, err
	}
	ext := externalID(ev)
	var ref incident.Ref
	err = c.with(ctx, cfg, true, func(b glpiBackend) error {
		existing, ok, err := b.findOpenByExternalID(ctx, ext)
		if err != nil {
			return err
		}
		if ok {
			ref = refFor(cfg.BaseURL, existing.ID)
			return nil
		}
		f := cfg.Severity[ev.Severity]
		id, err := b.create(ctx, ticketInput{
			Name:       truncateRunes(ev.Title, maxNameRunes),
			Content:    describe(ev),
			Urgency:    f.Urgency,
			Impact:     f.Impact,
			Priority:   f.Priority,
			ExternalID: ext,
			EntityID:   cfg.EntityID,
			CategoryID: cfg.CategoryID,
			GroupID:    cfg.GroupID,
		})
		if err != nil {
			return err
		}
		ref = refFor(cfg.BaseURL, id)
		return nil
	})
	if err != nil {
		return incident.Ref{}, err
	}
	return ref, nil
}

// Update adds one private follow-up; a close failure is not reported because the outbox would
// retry and post the follow-up twice.
func (c *Client) Update(ctx context.Context, config map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	id, err := ticketID(ref)
	if err != nil {
		return err
	}
	return c.with(ctx, cfg, false, func(b glpiBackend) error {
		return b.followup(ctx, id, describe(ev))
	})
}

// Resolve adds a solution (GLPI moves the ticket to Solved) and closes it when configured. A
// refused solution on a ticket already Solved or Closed counts as done, so a retry is harmless.
func (c *Client) Resolve(ctx context.Context, config map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	id, err := ticketID(ref)
	if err != nil {
		return err
	}
	return c.with(ctx, cfg, true, func(b glpiBackend) error {
		status := statusSolved
		if err := b.solve(ctx, id, describe(ev)); err != nil {
			if !errors.Is(err, incident.ErrOperatorMessage) {
				return err
			}
			t, gerr := b.get(ctx, id)
			if gerr != nil || t.Status < statusSolved {
				return err
			}
			status = t.Status
		}
		if cfg.CloseOnResolve && status != statusClosed {
			return b.setStatus(ctx, id, statusClosed)
		}
		return nil
	})
}
