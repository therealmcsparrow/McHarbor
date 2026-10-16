// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const pollBatch = 50

// Poll reads tickets in comma-separated batches of 50; a batch rejected for one deleted ticket is
// retried per ID, so the deleted one is omitted and the rest still report their state.
func (c *Client) Poll(ctx context.Context, m map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	cfg, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	var out []incident.RemoteStatus
	for start := 0; start < len(refs); start += pollBatch {
		batch := refs[start:min(start+pollBatch, len(refs))]
		got, err := c.pollBatch(ctx, cfg, batch)
		if errors.Is(err, incident.ErrNotFound) && len(batch) > 1 {
			got, err = nil, nil
			for _, ref := range batch {
				one, oneErr := c.pollBatch(ctx, cfg, []incident.Ref{ref})
				if errors.Is(oneErr, incident.ErrNotFound) {
					continue
				}
				if oneErr != nil {
					return out, oneErr
				}
				got = append(got, one...)
			}
		} else if errors.Is(err, incident.ErrNotFound) {
			continue
		}
		if err != nil {
			return out, err
		}
		out = append(out, got...)
	}
	return out, nil
}

func (c *Client) pollBatch(ctx context.Context, cfg config, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	byID := make(map[string]incident.Ref, len(refs))
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		if r.ExternalID == "" {
			continue
		}
		byID[r.ExternalID] = r
		ids = append(ids, r.ExternalID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var resp getResp
	if err := c.call(ctx, cfg, "/TicketGet", "POST", map[string]any{"TicketID": strings.Join(ids, ","), "DynamicFields": 1}, &resp); err != nil {
		return nil, err
	}
	var out []incident.RemoteStatus
	for _, t := range resp.tickets() {
		id := str(t["TicketID"])
		ref, ok := byID[id]
		if !ok {
			continue
		}
		st, ok := cfg.status(id, firstNonEmpty(str(t["TicketNumber"]), ref.Number), t)
		if !ok {
			continue
		}
		out = append(out, st)
	}
	return out, nil
}
