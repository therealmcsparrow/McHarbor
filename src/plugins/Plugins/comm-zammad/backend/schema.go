// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "github.com/therealmcsparrow/mcharbor/core/plugin"

func intp(v int) *int { return &v }

// zammadConfigSchema returns the channel fields; default_assignment_group is repurposed as the
// optional owner login because a Zammad group, not an assignment group, is the queue.
func zammadConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "config.name.label", Required: true},
		{Key: "instance_url", Type: "string", Label: "Zammad URL", LabelKey: "config.instance_url.label", Required: true, Placeholder: "https://support.example.de", Help: "Base URL of the Zammad instance; https:// only.", HelpKey: "config.instance_url.help"},
		{Key: "access_token", Type: "secret", Label: "Access token", LabelKey: "config.access_token.label", Required: true, Secret: true, Help: "Personal access token of a dedicated agent user with ticket.agent. Encrypted at rest.", HelpKey: "config.access_token.help"},
		{Key: "group", Type: "string", Label: "Group", LabelKey: "config.group.label", Required: true, Placeholder: "Operations"},
		{Key: "customer", Type: "string", Label: "Customer", LabelKey: "config.customer.label", Required: true, Help: "Login, email or guess:<email> of the customer user tickets are created for.", HelpKey: "config.customer.help", Placeholder: "mcharbor@example.de"},
		{Key: "on_behalf_of", Type: "string", Label: "Act on behalf of", LabelKey: "config.on_behalf_of.label", Help: "Optional From header; requires admin.user on the token owner.", HelpKey: "config.on_behalf_of.help"},
		{Key: "resolve_state", Type: "string", Label: "State on recovery", LabelKey: "config.resolve_state.label", Default: "closed"},
		{Key: "suppress_notifications", Type: "bool", Label: "Suppress agent notifications for notes", LabelKey: "config.suppress_notifications.label", Default: false},
		{Key: "status_map", Type: "string", Label: "State mapping (JSON)", LabelKey: "config.status_map.label", Placeholder: `{"waiting for vendor":"acknowledged"}`},
		{Key: "update_on_repeat", Type: "bool", Label: "Add a note on repeat", LabelKey: "config.update_on_repeat.label", Default: true},
		{Key: "reopen_window_minutes", Type: "int", Label: "Reopen window (minutes)", LabelKey: "config.reopen_window_minutes.label", Default: 0, Min: intp(0), Max: intp(1440)},
		{Key: "poll_interval_minutes", Type: "int", Label: "Poll interval (minutes)", LabelKey: "config.poll_interval_minutes.label", Default: 5, Min: intp(0), Max: intp(60)},
		{Key: "default_assignment_group", Type: "string", Label: "Owner login", LabelKey: "config.default_assignment_group.label", Help: "Optional owner set on create.", HelpKey: "config.default_assignment_group.help"},
		{Key: "severity_map", Type: "string", Label: "Severity mapping (JSON)", LabelKey: "config.severity_map.label", Placeholder: `{"critical":"3 high"}`},
	}
}
