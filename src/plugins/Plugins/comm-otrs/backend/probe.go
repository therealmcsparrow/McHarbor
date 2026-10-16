// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Test runs read-only probes so that "Test connection" never leaves a ticket behind in a
// production queue, yet still catches a misnamed dedup field.
func (c *Client) Test(ctx context.Context, m map[string]string) error {
	cfg, err := parseConfig(m)
	if err != nil {
		return err
	}
	c.dropSession(cfg)
	if err := c.call(ctx, cfg, "/TicketSearch", "POST", map[string]any{"Queues": []string{cfg.Queue}, "Limit": 1}, nil); err != nil {
		return err
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("comm-otrs: probe nonce: %w", err)
	}
	var hits searchResp
	probe := map[string]any{
		"DynamicField_" + cfg.DedupField: map[string]any{"Equals": "mcharbor-probe-" + hex.EncodeToString(nonce)},
		"Limit":                          1,
	}
	if err := c.call(ctx, cfg, "/TicketSearch", "POST", probe, &hits); err != nil {
		return err
	}
	if len(hits.ids()) > 0 {
		return fmt.Errorf("%w: dedup dynamic field %q is not searchable; create it as a Ticket Text field", incident.ErrOperatorMessage, cfg.DedupField)
	}
	return nil
}
