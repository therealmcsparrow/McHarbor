// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pluginNotificationTestClient is the slice of the plugin's
// NotificationAdapterClient the test dispatcher needs. The host
// holds the plugin runtime as `any` (to avoid the import cycle
// between modules/communications and the plugin runtime), so the
// cast happens here.
type pluginNotificationTestClient interface {
	Test(ctx context.Context, config map[string]string) error
}

// notificationAdapterSource is a local alias to
// plugin.NotificationAdapterSource so the runtime-cast code reads
// naturally. See core/plugin/adapters.go for the canonical
// definition.
type notificationAdapterSource = plugin.NotificationAdapterSource

// lookupNotificationAdapter asks the plugin runtime (when non-nil)
// whether a NotificationAdapterClient is registered for the given
// channel_type. Returns the client and a boolean reporting whether
// the lookup succeeded; the test dispatcher then calls Test on it.
func (s *Service) lookupNotificationAdapter(channelType string) (pluginNotificationTestClient, bool) {
	if s.pluginRuntime == nil {
		return nil, false
	}
	source, ok := s.pluginRuntime.(notificationAdapterSource)
	if !ok {
		return nil, false
	}
	raw, _, ok := source.NotificationAdapter(channelType)
	if !ok || raw == nil {
		return nil, false
	}
	client, ok := raw.(pluginNotificationTestClient)
	if !ok {
		return nil, false
	}
	return client, true
}

// HasNotificationAdapter reports whether a plugin registered channelType.
func (s *Service) HasNotificationAdapter(channelType string) bool {
	if s.pluginRuntime == nil {
		return false
	}
	source, ok := s.pluginRuntime.(notificationAdapterSource)
	if !ok {
		return false
	}
	_, _, ok = source.NotificationAdapter(channelType)
	return ok
}
