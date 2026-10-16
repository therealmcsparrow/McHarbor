// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "github.com/therealmcsparrow/mcharbor/core/plugin"

func intp(v int) *int { return &v }

// itopConfigSchema is the channel's field list; team, agent and service are required because
// iTop refuses to resolve a ticket without all three.
func itopConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "config.name.label", Required: true},
		{Key: "instance_url", Type: "string", Label: "iTop URL", LabelKey: "config.instance_url.label", Required: true, Placeholder: "https://itop.example.fr", Help: "Base URL of iTop, https:// only.", HelpKey: "config.instance_url.help"},
		{Key: "auth_mode", Type: "string", Label: "Authentication", LabelKey: "config.auth_mode.label", Default: "token", Help: "token (iTop 3.1+) or password.", HelpKey: "config.auth_mode.help"},
		{Key: "auth_token", Type: "secret", Label: "Application token", LabelKey: "config.auth_token.label", Secret: true},
		{Key: "auth_user", Type: "string", Label: "REST user login", LabelKey: "config.auth_user.label"},
		{Key: "auth_pwd", Type: "secret", Label: "REST user password", LabelKey: "config.auth_pwd.label", Secret: true},
		{Key: "ticket_class", Type: "string", Label: "Ticket class", LabelKey: "config.ticket_class.label", Default: "Incident", Help: "Incident or UserRequest.", HelpKey: "config.ticket_class.help"},
		{Key: "org_name", Type: "string", Label: "Organization name", LabelKey: "config.org_name.label", Required: true},
		{Key: "caller_email", Type: "string", Label: "Caller email", LabelKey: "config.caller_email.label", Help: "Person used as caller_id; optional.", HelpKey: "config.caller_email.help"},
		{Key: "origin", Type: "string", Label: "Origin", LabelKey: "config.origin.label", Default: "monitoring"},
		{Key: "team_id", Type: "int", Label: "Team ID for auto-assign", LabelKey: "config.team_id.label", Required: true, Min: intp(1), Help: "Used when a ticket must be assigned before it can be resolved.", HelpKey: "config.team_id.help"},
		{Key: "agent_id", Type: "int", Label: "Agent ID for auto-assign", LabelKey: "config.agent_id.label", Required: true, Min: intp(1)},
		{Key: "service_id", Type: "int", Label: "Service ID on resolve", LabelKey: "config.service_id.label", Required: true, Min: intp(1)},
		{Key: "resolution_code", Type: "string", Label: "Resolution code", LabelKey: "config.resolution_code.label", Default: "other"},
		{Key: "close_on_resolve", Type: "bool", Label: "Close after resolving", LabelKey: "config.close_on_resolve.label", Default: false},
		{Key: "update_on_repeat", Type: "bool", Label: "Log repeats in the private log", LabelKey: "config.update_on_repeat.label", Default: true},
		{Key: "reopen_window_minutes", Type: "int", Label: "Reopen window (minutes)", LabelKey: "config.reopen_window_minutes.label", Default: 0, Min: intp(0), Max: intp(1440)},
		{Key: "poll_interval_minutes", Type: "int", Label: "Poll interval (minutes)", LabelKey: "config.poll_interval_minutes.label", Default: 5, Min: intp(0), Max: intp(60)},
		{Key: "default_assignment_group", Type: "int", Label: "Team ID on create", LabelKey: "config.default_assignment_group.label", Default: 0, Min: intp(0), Help: "Optional team_id set at creation (the ticket stays new).", HelpKey: "config.default_assignment_group.help"},
		{Key: "severity_map", Type: "string", Label: "Severity mapping (JSON)", LabelKey: "config.severity_map.label", Placeholder: `{"critical":{"impact":"1","urgency":"1"}}`},
	}
}
