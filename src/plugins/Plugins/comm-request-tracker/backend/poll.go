// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const pollBatch = 50

// Poll reads every open ticket in one TicketSQL search per 50 ids; ids are validated as decimal
// before they reach the query, and a ticket RT no longer returns is left for the outbox to find.
func (c *Client) Poll(ctx context.Context, raw map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		if id, err := ticketID(r.ExternalID); err == nil {
			ids = append(ids, id)
		}
	}
	var out []incident.RemoteStatus
	for start := 0; start < len(ids); start += pollBatch {
		end := min(start+pollBatch, len(ids))
		clauses := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			clauses = append(clauses, "id = "+id)
		}
		path := "/tickets?" + url.Values{
			"query": {strings.Join(clauses, " OR ")}, "fields": {"Status,Owner,LastUpdated"}, "per_page": {"100"},
		}.Encode()
		var res collection
		if _, err := c.do(ctx, cfg, http.MethodGet, path, nil, &res); err != nil {
			return out, err
		}
		for _, it := range res.Items {
			id, err := ticketID(string(it.ID))
			if err != nil {
				continue
			}
			st, ok := cfg.state(it.Status)
			if !ok {
				continue
			}
			out = append(out, incident.RemoteStatus{
				Ref:       incident.Ref{ExternalID: id, Number: id, URL: cfg.ticketURL(id)},
				State:     st,
				Assignee:  sanitize(it.Owner.assignee()),
				UpdatedAt: parseRTTime(it.LastUpdated),
			})
		}
	}
	return out, nil
}
