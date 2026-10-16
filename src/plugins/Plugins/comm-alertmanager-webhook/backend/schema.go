// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "github.com/therealmcsparrow/mcharbor/core/plugin"

// amConfigSchema names the URL webhook_url because the host redacts secrets by key name, and
// drops reopen, poll and assignment-group fields, which a one-way protocol has no use for.
func amConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "config.name.label", Required: true, Placeholder: "Grafana IRM — Platform"},
		{Key: "webhook_url", Type: "secret", Secret: true, Label: "Receiver URL", LabelKey: "config.webhook_url.label", Required: true, Help: "Alertmanager webhook URL from the receiver. Often contains a token, so it is stored encrypted.", HelpKey: "config.webhook_url.help"},
		{Key: "profile", Type: "string", Label: "Receiver profile", LabelKey: "config.profile.label", Default: "generic", Help: "generic, grafana_irm, squadcast, zenduty or keep.", HelpKey: "config.profile.help"},
		{Key: "receiver_name", Type: "string", Label: "Receiver name", LabelKey: "config.receiver_name.label", Default: "mcharbor", Help: "Sent as the payload's receiver field.", HelpKey: "config.receiver_name.help"},
		{Key: "auth_mode", Type: "string", Label: "Authentication", LabelKey: "config.auth_mode.label", Default: "none", Help: "none, bearer or basic.", HelpKey: "config.auth_mode.help"},
		{Key: "bearer_token", Type: "secret", Secret: true, Label: "Bearer token", LabelKey: "config.bearer_token.label"},
		{Key: "basic_username", Type: "string", Label: "Basic auth username", LabelKey: "config.basic_username.label"},
		{Key: "basic_password", Type: "secret", Secret: true, Label: "Basic auth password", LabelKey: "config.basic_password.label"},
		{Key: "hmac_secret", Type: "secret", Secret: true, Label: "HMAC signing secret", LabelKey: "config.hmac_secret.label", Help: "Optional. Adds X-McHarbor-Signature: t=<unix>,v1=<hex> for custom gateways.", HelpKey: "config.hmac_secret.help"},
		{Key: "allow_http", Type: "bool", Label: "Allow plain http", LabelKey: "config.allow_http.label", Default: false, Help: "Only for a receiver on a trusted private network, such as self-hosted Keep.", HelpKey: "config.allow_http.help"},
		{Key: "update_on_repeat", Type: "bool", Label: "Re-send firing on repeat", LabelKey: "config.update_on_repeat.label", Default: true},
		{Key: "test_sends_alert", Type: "bool", Label: "Test connection sends a test alert", LabelKey: "config.test_sends_alert.label", Default: false, Help: "When on, Test fires and immediately resolves an info alert named McHarborConnectionTest.", HelpKey: "config.test_sends_alert.help"},
		{Key: "severity_map", Type: "string", Label: "Severity map (JSON)", LabelKey: "config.severity_map.label", Placeholder: `{"critical":"P1"}`},
	}
}
