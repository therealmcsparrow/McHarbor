// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const pollConcurrency = 5

// Poll looks each ref up by incident_key; an empty result is not ErrNotFound, because an
// alert grouped into another incident is still live and must not orphan its link.
func (c *Client) Poll(ctx context.Context, config map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	if cfg.RESTKey == "" {
		return nil, nil
	}
	var (
		mu       sync.Mutex
		out      []incident.RemoteStatus
		firstErr error
		wg       sync.WaitGroup
		sem      = make(chan struct{}, pollConcurrency)
	)
	for _, ref := range refs {
		if ref.ExternalID == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(ref incident.Ref) {
			defer wg.Done()
			defer func() { <-sem }()
			st, ok, err := c.pollOne(ctx, cfg, ref)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if ok {
				out = append(out, st)
			}
		}(ref)
	}
	wg.Wait()
	return out, firstErr
}

// pollOne reports only the newest incident for a key, because PagerDuty opens a fresh incident
// on the same dedup_key after a resolve and the older one must not override the live state.
func (c *Client) pollOne(ctx context.Context, cfg pdConfig, ref incident.Ref) (incident.RemoteStatus, bool, error) {
	q := url.Values{"incident_key": {ref.ExternalID}, "time_zone": {"UTC"}, "date_range": {"all"}}
	var resp struct {
		Incidents []pdIncident `json:"incidents"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.restURL(cfg)+"/incidents?"+q.Encode(), restHeaders(cfg), nil, &resp, cfg.RESTKey, cfg.RoutingKey); err != nil {
		return incident.RemoteStatus{}, false, err
	}
	var newest *pdIncident
	for i := range resp.Incidents {
		inc := &resp.Incidents[i]
		if newest == nil || inc.CreatedAt.After(newest.CreatedAt) {
			newest = inc
		}
	}
	if newest == nil {
		return incident.RemoteStatus{}, false, nil
	}
	at := newest.LastStatusChangeAt
	if at.IsZero() {
		at = c.now()
	}
	st, ok := toRemoteStatus(*newest, at)
	if !ok {
		return incident.RemoteStatus{}, false, nil
	}
	st.Ref.ExternalID = ref.ExternalID
	st.DedupKey = ref.ExternalID
	return st, true, nil
}
