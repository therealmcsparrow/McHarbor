// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the logging-siem plugin. It exports
// a single symbol — `Plugin` — that the host loads via `plugin.Open`
// and dispatches to.
//
// Scope: this plugin owns a single ForwarderType ("siem"). The
// siemClient implements plugin.LogForwarderAdapterClient (Test +
// ForwardPending + DeliveryHistory) and POSTs signed JSON documents
// to an HTTP collector.
//
// Why logging (not notification): a SIEM collector ingests
// machine-readable audit records for retention and correlation, not
// human-readable alerts. The contract is correspondingly different —
// there is no Title/Message, and delivery is a durable, retried,
// deduplicated pipeline rather than fire-and-forget.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// siemForwarderType is the ForwarderType this plugin owns. It matches
// the `forwarder_type` value the host's audit_log_forwarders row
// carries, and the value migration 143 imports the old
// audit_siem_config row under.
const siemForwarderType = "siem"

// LoggingSIEMPlugin is the singleton the host dispatches to.
type LoggingSIEMPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	client   *siemClient
	// reportErr holds a failed report-provider registration until Init
	// can log it; RegisterAdapters runs before the plugin has a logger.
	reportErr error
}

// Manifest returns the parsed plugin.yaml.
func (p *LoggingSIEMPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — logging-siem registers no HTTP routes. The host's
// /api/audit/logging subtree is the single admin surface for every
// forwarder, built-in or plugin.
func (p *LoggingSIEMPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op: this plugin is configured from Settings >
// Integrations > Logging, so a sidebar entry would just duplicate a
// surface the host already owns.
func (p *LoggingSIEMPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets — no dashboard widgets.
func (p *LoggingSIEMPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes — no workflow nodes.
func (p *LoggingSIEMPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions — this plugin declares no permission keys. The
// host's settings.manage already gates the tab that configures it.
func (p *LoggingSIEMPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the SIEM client for its single
// ForwarderType.
//
// SupportsDeliveryHistory is true: an HTTP collector returns a status
// code, so every attempt has a real, recordable outcome worth showing
// an operator. RequiresLeaderElection is true because an N-node
// cluster posting the same event N times would inflate the
// collector's event count and break any per-event correlation rule
// downstream.
//
// The delivery report is registered after the forwarder and its failure
// is kept rather than returned: a report shelf collision must never take
// audit forwarding offline with it.
func (p *LoggingSIEMPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Logging().Register(
		plugin.LogForwarderAdapterSpec{
			ForwarderType: siemForwarderType,
			Slug:          p.manifest.Slug,
			DisplayName:   "SIEM (HTTP)",
			Icon:          "plugin.svg",
			Description:   "Signed JSON over HTTPS to Splunk HEC, Sentinel, Elastic, or any JSON collector.",
			Capabilities: plugin.LogForwarderAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsDeliveryHistory: true,
				RequiresLeaderElection:  true,
			},
			ConfigSchema: siemConfigSchema(),
		},
		p.client,
	); err != nil {
		return err
	}
	p.reportErr = reg.Reporting().Register(siemReportSpec(), &reportProvider{plugin: p})
	return nil
}

// siemConfigSchema returns the form fields the Logging tab renders.
//
// The keys match the JSON object migration 143 builds when it imports
// the old audit_siem_config row, so an operator who already had SIEM
// forwarding configured sees their endpoint and secret intact after
// the upgrade. Renaming a key here without a matching migration
// change silently blanks that field on existing installs.
func siemConfigSchema() []plugin.ConfigFieldDecl {
	maxRetriesMin, maxRetriesMax := 1, 10
	timeoutMin, timeoutMax := 1, 60
	return []plugin.ConfigFieldDecl{
		{
			Key:         "endpoint_url",
			Type:        "string",
			Label:       "Endpoint URL",
			Required:    true,
			Help:        "HTTPS endpoint that accepts a JSON POST body. Splunk HEC, Microsoft Sentinel, Elastic, or any collector that returns 2xx on success.",
			Placeholder: "https://siem.example.com/services/collector/event",
		},
		{
			Key:      "signing_secret",
			Type:     "secret",
			Label:    "Signing Secret",
			Required: true,
			Help:     "Shared secret used to compute the X-McHarbor-Signature HMAC-SHA256 header, so the collector can verify each event came from this McHarbor. Encrypted at rest. Leave blank when saving to keep the stored value.",
		},
		{
			Key:     "max_retries",
			Type:    "int",
			Label:   "Max Retries",
			Default: 3,
			Min:     &maxRetriesMin,
			Max:     &maxRetriesMax,
			Help:    "How many times one event is retried before it is given up on. Delivery history keeps the failed attempts either way.",
		},
		{
			Key:     "timeout_seconds",
			Type:    "int",
			Label:   "Timeout (seconds)",
			Default: 10,
			Min:     &timeoutMin,
			Max:     &timeoutMax,
			Help:    "Per-request timeout. A collector slower than this counts the attempt as failed and retries on the next cycle.",
		},
	}
}

// Init stashes the host context so the client can reach the database
// and the logger.
func (p *LoggingSIEMPlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host
	p.client.attach(host)
	// Bring an install that predates multi-instance forwarding up to
	// the current ledger shape. A failure here is logged rather than
	// fatal: refusing to load would take the whole forwarder offline
	// over a schema detail the operator cannot act on.
	if err := p.client.ensureSchema(ctx); err != nil && host.Logger != nil {
		host.Logger.Error("preparing siem delivery history schema failed", "error", err)
	}
	if p.reportErr != nil && host.Logger != nil {
		host.Logger.Error("registering siem delivery report failed", "error", p.reportErr)
	}
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.logging-siem.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *LoggingSIEMPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.logging-siem.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*LoggingSIEMPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be a
// variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &LoggingSIEMPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "logging-siem",
		Name:    "SIEM",
		Version: "2026.10.01",
	},
	client: newSIEMClient(),
}
