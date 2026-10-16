// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "github.com/therealmcsparrow/mcharbor/core/plugin"

func intp(v int) *int { return &v }

// glpiConfigSchema declares both API surfaces in one schema; the frontend shows only the
// credential fields of the selected api_version, and parseConfig enforces which are required.
func glpiConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "config.name.label", Required: true},
		{Key: "instance_url", Type: "string", Label: "GLPI URL", LabelKey: "config.instance_url.label", Required: true, Help: "Base URL without /apirest.php or /api.php.", HelpKey: "config.instance_url.help", Placeholder: "https://glpi.example.org"},
		{Key: "api_version", Type: "string", Label: "API", LabelKey: "config.api_version.label", Default: "v1", Help: "v1 (apirest.php, GLPI 9.5-11) or v2 (api.php high-level API, GLPI 11+).", HelpKey: "config.api_version.help"},
		{Key: "app_token", Type: "secret", Label: "App-Token (v1)", LabelKey: "config.app_token.label", Secret: true, Help: "Optional; required when the API client defines one.", HelpKey: "config.app_token.help"},
		{Key: "user_token", Type: "secret", Label: "User API token (v1)", LabelKey: "config.user_token.label", Secret: true},
		{Key: "v2_path", Type: "string", Label: "HL API version path (v2)", LabelKey: "config.v2_path.label", Default: "v2.0"},
		{Key: "oauth_client_id", Type: "string", Label: "OAuth client ID (v2)", LabelKey: "config.oauth_client_id.label"},
		{Key: "oauth_client_secret", Type: "secret", Label: "OAuth client secret (v2)", LabelKey: "config.oauth_client_secret.label", Secret: true},
		{Key: "oauth_username", Type: "string", Label: "API user login (v2)", LabelKey: "config.oauth_username.label"},
		{Key: "oauth_password", Type: "secret", Label: "API user password (v2)", LabelKey: "config.oauth_password.label", Secret: true},
		{Key: "entities_id", Type: "int", Label: "Entity ID", LabelKey: "config.entities_id.label", Default: 0, Min: intp(0), Help: "0 creates tickets in the API user's active entity.", HelpKey: "config.entities_id.help"},
		{Key: "itilcategories_id", Type: "int", Label: "ITIL category ID", LabelKey: "config.itilcategories_id.label", Default: 0, Min: intp(0)},
		{Key: "close_on_resolve", Type: "bool", Label: "Close after solving", LabelKey: "config.close_on_resolve.label", Default: false, Help: "Moves the ticket from Solved to Closed immediately.", HelpKey: "config.close_on_resolve.help"},
		{Key: "webhook_signing_secret", Type: "secret", Label: "GLPI webhook secret", LabelKey: "config.webhook_signing_secret.label", Secret: true, Help: "The secret shown in GLPI's webhook form; overrides the channel's generated secret.", HelpKey: "config.webhook_signing_secret.help"},
		{Key: "update_on_repeat", Type: "bool", Label: "Add a follow-up on repeat", LabelKey: "config.update_on_repeat.label", Default: true},
		{Key: "reopen_window_minutes", Type: "int", Label: "Reopen window (minutes)", LabelKey: "config.reopen_window_minutes.label", Default: 0, Min: intp(0), Max: intp(1440)},
		{Key: "poll_interval_minutes", Type: "int", Label: "Poll interval (minutes)", LabelKey: "config.poll_interval_minutes.label", Default: 5, Min: intp(0), Max: intp(60)},
		{Key: "default_assignment_group", Type: "int", Label: "Assigned group ID", LabelKey: "config.default_assignment_group.label", Default: 0, Min: intp(0)},
		{Key: "severity_map", Type: "string", Label: "Severity mapping (JSON)", LabelKey: "config.severity_map.label", Placeholder: `{"critical":{"urgency":5,"impact":5,"priority":6}}`, Help: "Optional override of urgency, impact and priority per McHarbor severity.", HelpKey: "config.severity_map.help"},
	}
}
