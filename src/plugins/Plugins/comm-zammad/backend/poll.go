// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// pollConcurrency caps parallel ticket reads, because Zammad documents no rate limit and a
// hosted instance should not see a burst the size of every open link.
const pollConcurrency = 4

// Poll reads each ticket with expand=true; a deleted ticket is omitted, and one failing read
// does not discard the others, because the host drops the whole batch on a returned error.
func (c *Client) Poll(ctx context.Context, raw map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	type result struct {
		st  incident.RemoteStatus
		ok  bool
		err error
	}
	results := make([]result, len(refs))
	sem := make(chan struct{}, pollConcurrency)
	var wg sync.WaitGroup
	for i, ref := range refs {
		wg.Add(1)
		go func(i int, ref incident.Ref) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i].err = ctx.Err()
				return
			}
			defer func() { <-sem }()
			id, err := ticketID(ref)
			if err != nil {
				results[i].err = err
				return
			}
			var t ticket
			err = c.do(ctx, cfg, http.MethodGet, "/api/v1/tickets/"+strconv.FormatInt(id, 10)+"?expand=true", nil, &t)
			switch {
			case errors.Is(err, incident.ErrNotFound):
				return
			case err != nil:
				results[i].err = err
				return
			}
			if t.ID == "" {
				t.ID = flexString(strconv.FormatInt(id, 10))
			}
			results[i] = result{st: c.status(cfg, cfg.base, t), ok: true}
		}(i, ref)
	}
	wg.Wait()
	var out []incident.RemoteStatus
	var firstErr error
	failed := 0
	for _, r := range results {
		switch {
		case r.ok:
			out = append(out, r.st)
		case r.err != nil:
			failed++
			if firstErr == nil {
				firstErr = r.err
			}
		}
	}
	if failed > 0 && failed == len(refs) {
		return nil, firstErr
	}
	return out, nil
}
