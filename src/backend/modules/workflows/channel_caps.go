// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import "github.com/therealmcsparrow/mcharbor/core/plugincaps"

// ChannelAliasResolver maps a bare stored action a loaded comm plugin claims to that
// plugin's channel type, and reports false when nothing claims it.
type ChannelAliasResolver func(action string) (string, bool)

// channelAliasFor returns the resolver in use; tests set the field, production reads
// the capability registry the plugin loader writes.
func (s *Service) channelAliasFor(action string) (string, bool) {
	if s != nil && s.channelAlias != nil {
		return s.channelAlias(action)
	}
	return plugincaps.Default.ChannelNodeAlias(action)
}

// emailChannelTypes returns the channel types loaded plugins declare as email transports.
func (s *Service) emailChannelTypes() []string {
	if s != nil && s.emailChannels != nil {
		return s.emailChannels()
	}
	return plugincaps.Default.EmailChannelTypes()
}

// applyChannelAlias rewrites a stored per-provider send action (a node the host once
// shipped for one messenger) into the generic send node bound to the channel type the
// claiming plugin owns. Only the in-memory node of one run changes, as with the host's own
// legacy rewrite; the stored canvas keeps the id it was saved with.
// A stored channel_type wins, as the host's own legacy rewrite always let it.
func (s *Service) applyChannelAlias(node *CanvasNode) {
	if node == nil {
		return
	}
	channelType, ok := s.channelAliasFor(node.Action)
	if !ok {
		return
	}
	if node.Config == nil {
		node.Config = map[string]interface{}{}
	}
	if _, present := node.Config["channel_type"]; !present {
		node.Config["channel_type"] = channelType
	}
	node.Action = plugincaps.ChannelAliasNode
}
