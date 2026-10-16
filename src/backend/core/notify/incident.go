// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notify

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// IncidentClient returns the channel type's plugin client when it can open,
// update and resolve tickets; a chat-only plugin reports false.
func (d *Dispatcher) IncidentClient(channelType string) (incident.LifecycleClient, bool) {
	client, _, ok := d.lookupNotificationAdapter(channelType)
	if !ok {
		return nil, false
	}
	lc, ok := client.(incident.LifecycleClient)
	return lc, ok
}

// WebhookParser returns the channel type's inbound webhook parser, when it has one.
func (d *Dispatcher) WebhookParser(channelType string) (incident.WebhookParser, bool) {
	client, _, ok := d.lookupNotificationAdapter(channelType)
	if !ok {
		return nil, false
	}
	wp, ok := client.(incident.WebhookParser)
	return wp, ok
}

// Poller returns the channel type's status poller, when it has one.
func (d *Dispatcher) Poller(channelType string) (incident.Poller, bool) {
	client, _, ok := d.lookupNotificationAdapter(channelType)
	if !ok {
		return nil, false
	}
	p, ok := client.(incident.Poller)
	return p, ok
}

// ChannelConfig resolves an enabled channel's type and decrypted config through
// the same path SendChannel uses, so ticket calls see exactly what sends see.
func (d *Dispatcher) ChannelConfig(ctx context.Context, channelID string) (string, map[string]string, error) {
	target, err := d.resolveChannelTarget(ctx, channelID, "")
	if err != nil {
		return "", nil, err
	}
	cfg := make(map[string]string, len(target.Config))
	for k, v := range target.Config {
		cfg[k] = v
	}
	return target.ChannelType, cfg, nil
}
