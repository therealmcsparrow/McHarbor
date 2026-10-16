// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "github.com/therealmcsparrow/mcharbor/core/plugin"

func intp(v int) *int { return &v }

// rtConfigSchema drops the foundation's allow_insecure_http: RT sends the auth token on every
// call, so the plugin talks to RT over https:// only.
func rtConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "config.name.label", Required: true},
		{Key: "instance_url", Type: "string", Label: "RT base URL", LabelKey: "config.instance_url.label", Required: true, Help: "Web root of RT, without /REST/2.0.", HelpKey: "config.instance_url.help", Placeholder: "https://rt.example.edu/rt"},
		{Key: "auth_token", Type: "secret", Label: "Auth token", LabelKey: "config.auth_token.label", Required: true, Secret: true},
		{Key: "queue", Type: "string", Label: "Queue", LabelKey: "config.queue.label", Required: true, Placeholder: "Operations"},
		{Key: "dedup_cf_name", Type: "string", Label: "Dedup custom field", LabelKey: "config.dedup_cf_name.label", Default: defaultCFName},
		{Key: "resolve_status", Type: "string", Label: "Status on recovery", LabelKey: "config.resolve_status.label", Default: "resolved"},
		{Key: "requestor", Type: "string", Label: "Requestor email", LabelKey: "config.requestor.label", Help: "Optional; leave blank so no correspondence is mailed.", HelpKey: "config.requestor.help"},
		{Key: "status_map", Type: "string", Label: "Status mapping (JSON)", LabelKey: "config.status_map.label"},
		{Key: "update_on_repeat", Type: "bool", Label: "Comment on repeat", LabelKey: "config.update_on_repeat.label", Default: true},
		{Key: "reopen_window_minutes", Type: "int", Label: "Reopen window (minutes)", LabelKey: "config.reopen_window_minutes.label", Default: 0, Min: intp(0), Max: intp(1440)},
		{Key: "poll_interval_minutes", Type: "int", Label: "Poll interval (minutes)", LabelKey: "config.poll_interval_minutes.label", Default: 5, Min: intp(0), Max: intp(60)},
		{Key: "default_assignment_group", Type: "string", Label: "Owner", LabelKey: "config.default_assignment_group.label", Help: "Optional RT user name set as owner on create.", HelpKey: "config.default_assignment_group.help"},
		{Key: "severity_map", Type: "string", Label: "Severity mapping (JSON)", LabelKey: "config.severity_map.label"},
	}
}
