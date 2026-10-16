// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package communications stores encrypted communication channels and bridges plugin sends to the notification adapters that own each channel type.
package communications

import (
	"context"
	"fmt"

	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// HostBridge is the host side of plugin.CommsHandle.
//
// A plugin names a channel by id; this resolves the row, and the
// dispatcher decrypts its config and hands it to the adapter that owns
// the channel type. No credential crosses the plugin boundary — the
// same discipline ai_providers.HostBridge follows for provider keys.
type HostBridge struct {
	app        *router.AppDeps
	service    *Service
	dispatcher *corenotify.Dispatcher
}

// NewHostBridge builds the bridge the plugin loader hands to plugins.
//
// The dispatcher's plugin runtime is bound lazily on first Send rather
// than here: the bridge is constructed before pluginruntime.Boot (the
// loader needs it to build each HostContext), so app.PluginAdapters is
// still nil at this point and binding now would leave every dispatch
// without an adapter source.
func NewHostBridge(app *router.AppDeps) *HostBridge {
	return &HostBridge{
		app:        app,
		service:    NewService(app.DB, app.Encryption),
		dispatcher: corenotify.NewDispatcher(app.DB, app.Encryption),
	}
}

// Send delivers a plugin's message to one channel.
//
// Files are passed through to the dispatcher, which drops them when the
// resolved adapter did not declare SupportsFileAttachments. That check
// stays in the dispatcher rather than here so every caller — plugin,
// workflow, alert — degrades identically.
func (b *HostBridge) Send(
	ctx context.Context,
	pluginSlug, channelID string,
	msg pluginruntime.CommsDispatchMessage,
) error {
	if b.dispatcher == nil {
		return fmt.Errorf("communications: no dispatcher configured")
	}
	// Bind the adapter source now that the runtime exists. Idempotent
	// and cheap — a plain field assignment — so re-binding per send is
	// simpler than tracking whether boot has happened.
	if b.app != nil && b.app.PluginAdapters != nil {
		b.dispatcher.SetPluginRuntime(b.app.PluginAdapters)
	}
	if b.service != nil && b.app != nil && b.app.PluginAdapters != nil {
		b.service.SetPluginRuntime(b.app.PluginAdapters)
	}

	files := make([]plugin.NotificationFile, 0, len(msg.Files))
	for _, f := range msg.Files {
		files = append(files, plugin.NotificationFile{
			Filename:    f.Filename,
			ContentType: f.ContentType,
			Data:        f.Data,
		})
	}

	// Recipient is handed to the dispatcher rather than resolved to a
	// config key here: the dispatcher is the one place that has
	// already resolved the channel's adapter spec, so it can refuse a
	// channel that declared no overridable destination instead of
	// this bridge guessing a column name per channel type.
	_, err := b.dispatcher.SendChannel(ctx, corenotify.ChannelRequest{
		ChannelID: channelID,
		Title:     msg.Title,
		Message:   msg.Message,
		Files:     files,
		Recipient: msg.Recipient,
	})

	// Both outcomes are recorded. A send that never arrived is the one
	// an operator opens the audit log to find; logging only the
	// successes made the log agree with the happy path and go silent
	// exactly when it mattered.
	//
	// pluginSlug is host-supplied by the loader's commsAdapter, so a
	// plugin cannot attribute a send to a different plugin.
	if b.app != nil && b.app.AuditLog != nil {
		fields := map[string]any{
			"channel_id":   channelID,
			"files":        len(files),
			"message_size": len(msg.Message),
			"outcome":      "success",
		}
		severity := "success"
		if err != nil {
			severity = "error"
			fields["outcome"] = "failure"
			fields["error"] = pluginruntime.RedactAuditError(err)
		}
		if auditErr := b.app.AuditLog.EmitPluginSeverityContext(ctx, pluginSlug, "plugin.communication.sent", severity, fields); auditErr != nil && b.app.Logger != nil {
			b.app.Logger.Warn("communications bridge: recording send audit", "plugin", pluginSlug, "error", auditErr)
		}
	}
	return err
}

// Channels lists what a plugin may send to.
//
// SupportsFileAttachments comes from plugin_notification_adapters,
// which the loader fills from each adapter's declared capabilities, so
// a plugin can tell in advance whether a document will actually arrive
// or whether it should put a link in the message body instead.
func (b *HostBridge) Channels(ctx context.Context) []pluginruntime.CommsChannelInfo {
	channels, err := b.service.List(ctx)
	if err != nil {
		if b.app != nil && b.app.Logger != nil {
			b.app.Logger.Error("communications bridge: listing channels", "error", err)
		}
		return nil
	}

	caps := map[string]channelCapability{}
	if list, capErr := b.service.AdapterCapabilities(ctx); capErr == nil {
		for _, c := range list {
			caps[c.ChannelType] = channelCapability{
				SupportsFileAttachments: c.SupportsFileAttachments,
				Recipient: pluginruntime.NotificationRecipientDecl{
					Key:           c.Recipient.Key(),
					Kind:          c.Recipient.Kind,
					LabelEN:       c.Recipient.Label,
					PlaceholderEN: c.Recipient.Placeholder,
					Multiple:      c.Recipient.Multiple,
				},
			}
		}
	} else if b.app != nil && b.app.Logger != nil {
		// Not fatal: without capabilities every channel simply reports
		// false, which is the safe direction — a plugin then sends a
		// link rather than assuming an attachment will land.
		b.app.Logger.Warn("communications bridge: reading adapter capabilities", "error", capErr)
	}

	out := make([]pluginruntime.CommsChannelInfo, 0, len(channels))
	for _, c := range channels {
		if !c.Enabled {
			continue
		}
		out = append(out, pluginruntime.CommsChannelInfo{
			ID:                      c.ID,
			Name:                    c.Name,
			ChannelType:             c.ChannelType,
			IsDefault:               c.IsDefault,
			SupportsFileAttachments: caps[c.ChannelType].SupportsFileAttachments,
			Recipient:               caps[c.ChannelType].Recipient,
		})
	}
	return out
}

// channelCapability is the per-channel-type answer Channels needs, kept
// local because it is two fields of AdapterCapability and nothing else.
// A channel type with no row reads as the zero value, which is the safe
// direction on both counts: no attachment promised, no override offered.
type channelCapability struct {
	SupportsFileAttachments bool
	Recipient               pluginruntime.NotificationRecipientDecl
}
