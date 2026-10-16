// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"fmt"
	"strings"
)

// ChannelAliasNode is the host's generic send node a channel-type alias resolves to.
// It is the host's own node, so naming it here couples the host to nothing a plugin ships.
const ChannelAliasNode = "send-communication"

// manifestForm is one contributes.forms slot. A slot on the channel resource is keyed by
// the channel type the plugin registers, which is the only channel type it may speak for.
type manifestForm struct {
	Resource string `yaml:"resource"`
	Kind     string `yaml:"kind"`
	Key      string `yaml:"key"`
}

// ownChannelType holds a channel type to one the same manifest mounts a channel form for,
// so a plugin can never claim, or alias to, another plugin's channel.
func ownChannelType(doc *manifestDoc, channelType string) error {
	if err := token("channel_type", channelType); err != nil {
		return err
	}
	for _, f := range doc.Contributes.Forms {
		if f.Resource == "channel" && f.Key == channelType {
			return nil
		}
	}
	return fmt.Errorf("channel_type %q is not the key of a contributes.forms entry with resource channel", channelType)
}

// ChannelNodeAlias resolves a bare stored action a loaded plugin claims for one of its own
// channel types; the caller runs it as ChannelAliasNode with that channel_type.
func (r *Registry) ChannelNodeAlias(action string) (string, bool) {
	if action == "" || strings.HasPrefix(action, "plugin:") {
		return "", false
	}
	c, ok := r.Provider(KindWorkflowNodeAlias, action)
	if !ok || c.ChannelType == "" {
		return "", false
	}
	return c.ChannelType, true
}

// EmailChannelTypes returns the channel types loaded plugins declare as email transports,
// in registry order. Empty means no installed plugin can send email.
func (r *Registry) EmailChannelTypes() []string {
	out := []string{}
	for _, c := range r.OfKind(KindEmailChannel) {
		out = append(out, c.ChannelType)
	}
	return out
}

// IsEmailChannel reports whether a loaded plugin declares channelType an email transport.
func (r *Registry) IsEmailChannel(channelType string) bool {
	_, ok := r.Provider(KindEmailChannel, channelType)
	return ok
}
