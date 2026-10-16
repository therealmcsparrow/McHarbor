// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the logging-syslog plugin. It exports
// a single symbol — `Plugin` — that the host loads via `plugin.Open`
// and dispatches to.
//
// Scope: this plugin owns a single ForwarderType ("syslog"). The
// syslogClient implements plugin.LogForwarderAdapterClient (Test +
// ForwardPending + DeliveryHistory) and emits RFC5424 messages over
// UDP, TCP, or TCP+TLS.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// syslogForwarderType is the ForwarderType this plugin owns. It
// matches the `forwarder_type` value migration 143 imports the old
// audit_syslog_config row under.
const syslogForwarderType = "syslog"

// LoggingSyslogPlugin is the singleton the host dispatches to.
type LoggingSyslogPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	client   *syslogClient
	// reportErr holds a failed report-provider registration until Init
	// can log it; RegisterAdapters runs before the plugin has a logger.
	reportErr error
}

// Manifest returns the parsed plugin.yaml.
func (p *LoggingSyslogPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — no HTTP routes; the host's /api/audit/logging
// subtree is the single admin surface for every forwarder.
func (p *LoggingSyslogPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op: configured from Settings > Integrations >
// Logging.
func (p *LoggingSyslogPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets — no dashboard widgets.
func (p *LoggingSyslogPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes — no workflow nodes.
func (p *LoggingSyslogPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions — declares no permission keys; the host's
// settings.manage gates the tab that configures this plugin.
func (p *LoggingSyslogPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the syslog client for its single
// ForwarderType.
//
// SupportsDeliveryHistory is true even though UDP cannot confirm
// receipt: the history then records "the datagram left this host",
// which is genuinely useful (it distinguishes a misconfigured
// forwarder from a collector-side drop) as long as nobody reads it as
// proof of delivery. The Protocol field on each row is what keeps
// that honest — a "success" tagged `udp` is visibly weaker evidence
// than one tagged `tls`.
//
// RequiresLeaderElection is true: an N-node cluster would otherwise
// emit N copies of each event and inflate the collector's counts.
//
// The delivery report is registered after the forwarder and its failure
// is kept rather than returned: a report shelf collision must never take
// audit forwarding offline with it.
func (p *LoggingSyslogPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Logging().Register(
		plugin.LogForwarderAdapterSpec{
			ForwarderType: syslogForwarderType,
			Slug:          p.manifest.Slug,
			DisplayName:   "Syslog (RFC5424)",
			Icon:          "plugin.svg",
			Description:   "RFC5424 over UDP, TCP, or TLS to rsyslog, syslog-ng, QRadar, or ArcSight.",
			Capabilities: plugin.LogForwarderAdapterCapabilities{
				EncryptedCredentials:    false,
				SupportsDeliveryHistory: true,
				RequiresLeaderElection:  true,
			},
			ConfigSchema: syslogConfigSchema(),
		},
		p.client,
	); err != nil {
		return err
	}
	p.reportErr = reg.Reporting().Register(syslogReportSpec(), &reportProvider{plugin: p})
	return nil
}

// syslogConfigSchema returns the form fields the Logging tab renders.
//
// Keys and defaults match the JSON object migration 143 builds when it
// imports the old audit_syslog_config row — host, port 6514,
// protocol tls, facility local0, tag mcharbor, 3 retries, 5s timeout —
// so an operator who already had syslog forwarding configured sees
// their settings intact after the upgrade.
func syslogConfigSchema() []plugin.ConfigFieldDecl {
	portMin, portMax := 1, 65535
	retriesMin, retriesMax := 1, 10
	timeoutMin, timeoutMax := 1, 60
	return []plugin.ConfigFieldDecl{
		{
			Key:         "host",
			Type:        "string",
			Label:       "Host",
			Required:    true,
			Help:        "Hostname or IP of the syslog collector.",
			Placeholder: "syslog.example.com",
		},
		{
			Key:      "port",
			Type:     "int",
			Label:    "Port",
			Required: true,
			Default:  6514,
			Min:      &portMin,
			Max:      &portMax,
			Help:     "6514 is the IANA port for syslog over TLS; 514 is the traditional plaintext UDP/TCP port.",
		},
		{
			Key:     "protocol",
			Type:    "string",
			Label:   "Protocol",
			Default: "tls",
			Help:    "One of tls, tcp, udp. TLS is the only option that both encrypts the audit trail in transit and confirms the collector accepted it — prefer it unless your collector cannot terminate TLS.",
		},
		{
			Key:     "facility",
			Type:    "string",
			Label:   "Facility",
			Default: "local0",
			Help:    "Syslog facility: local0-local7, auth, authpriv, daemon, syslog, or user. Determines which file or ruleset the collector routes these events into.",
		},
		{
			Key:         "tag",
			Type:        "string",
			Label:       "Tag",
			Default:     "mcharbor",
			Help:        "RFC5424 APP-NAME. Identifies McHarbor in the collector's stream; keep it stable so existing filters keep matching.",
			Placeholder: "mcharbor",
		},
		{
			Key:     "max_retries",
			Type:    "int",
			Label:   "Max Retries",
			Default: 3,
			Min:     &retriesMin,
			Max:     &retriesMax,
			Help:    "How many times one event is retried before it is given up on. Delivery history keeps the failed attempts either way.",
		},
		{
			Key:     "timeout_seconds",
			Type:    "int",
			Label:   "Timeout (seconds)",
			Default: 5,
			Min:     &timeoutMin,
			Max:     &timeoutMax,
			Help:    "Connect and write timeout. A collector slower than this counts the attempt as failed and retries on the next cycle.",
		},
	}
}

// Init stashes the host context so the client can reach the database
// and the logger.
func (p *LoggingSyslogPlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host
	p.client.attach(host)
	// Bring an install that predates multi-instance forwarding up to
	// the current ledger shape. A failure here is logged rather than
	// fatal: refusing to load would take the whole forwarder offline
	// over a schema detail the operator cannot act on.
	if err := p.client.ensureSchema(ctx); err != nil && host.Logger != nil {
		host.Logger.Error("preparing syslog delivery history schema failed", "error", err)
	}
	if p.reportErr != nil && host.Logger != nil {
		host.Logger.Error("registering syslog delivery report failed", "error", p.reportErr)
	}
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.logging-syslog.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *LoggingSyslogPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.logging-syslog.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*LoggingSyslogPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be a
// variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &LoggingSyslogPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "logging-syslog",
		Name:    "Syslog",
		Version: "2026.10.01",
	},
	client: newSyslogClient(),
}
