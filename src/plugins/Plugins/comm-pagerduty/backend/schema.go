// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "github.com/therealmcsparrow/mcharbor/core/plugin"

func intPtr(v int) *int { return &v }

// pagerdutyConfigSchema drops instance_url, default_assignment_group and reopen_window_minutes
// from the foundation's fields: the region picks the host, the routing key picks the service.
func pagerdutyConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "config.name.label", Required: true, Placeholder: "PagerDuty — Platform"},
		{Key: "region", Type: "string", Label: "Service region", LabelKey: "config.region.label", Default: "us", Required: true, Help: "us or eu. Must match the region your PagerDuty account lives in.", HelpKey: "config.region.help"},
		{Key: "routing_key", Type: "secret", Secret: true, Label: "Integration (routing) key", LabelKey: "config.routing_key.label", Required: true, Help: "32-character key from the service's Events API V2 integration.", HelpKey: "config.routing_key.help"},
		{Key: "rest_api_key", Type: "secret", Secret: true, Label: "REST API key (read-only)", LabelKey: "config.rest_api_key.label", Help: "Optional. Enables status polling and incident links.", HelpKey: "config.rest_api_key.help"},
		{Key: "webhook_signing_secret", Type: "secret", Secret: true, Label: "Webhook signing secret", LabelKey: "config.webhook_signing_secret.label", Help: "Shown once when the v3 webhook subscription is created.", HelpKey: "config.webhook_signing_secret.help"},
		{Key: "update_on_repeat", Type: "bool", Label: "Re-trigger on repeat firing", LabelKey: "config.update_on_repeat.label", Default: true},
		{Key: "poll_interval_minutes", Type: "int", Label: "Poll interval (minutes)", LabelKey: "config.poll_interval_minutes.label", Default: 5, Min: intPtr(0), Max: intPtr(60), Help: "0 disables polling. Needs a REST key.", HelpKey: "config.poll_interval_minutes.help"},
		{Key: "severity_map", Type: "string", Label: "Severity map (JSON)", LabelKey: "config.severity_map.label", Placeholder: `{"warning":"error"}`},
	}
}
