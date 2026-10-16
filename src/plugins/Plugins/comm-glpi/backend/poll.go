// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// pollWorkers bounds concurrent ticket reads so a large batch does not flood a small GLPI.
const pollWorkers = 4

// Poll reads each ticket inside one session (v1) or token (v2). A deleted ticket (404) is
// omitted, and one unreadable ticket does not hide the others; only a batch where every read
// failed returns an error, because the host discards the whole batch on error.
func (c *Client) Poll(ctx context.Context, config map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(refs))
	for _, ref := range refs {
		if id, err := ticketID(ref); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var (
		mu       sync.Mutex
		out      []incident.RemoteStatus
		failed   int
		firstErr error
	)
	err = c.with(ctx, cfg, false, func(b glpiBackend) error {
		sem := make(chan struct{}, pollWorkers)
		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				t, err := b.get(ctx, id)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if !errors.Is(err, incident.ErrNotFound) {
						failed++
						if firstErr == nil {
							firstErr = err
						}
					}
					return
				}
				if state, ok := remoteState(t.Status, t.Deleted); ok {
					out = append(out, incident.RemoteStatus{Ref: refFor(cfg.BaseURL, id), State: state, UpdatedAt: c.now()})
				}
			}(id)
		}
		wg.Wait()
		if failed > 0 && len(out) == 0 {
			return firstErr
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
