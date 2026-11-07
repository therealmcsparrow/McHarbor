// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updatechannel

import (
	"strings"
	"unicode"
)

// Settings rows owned by the update-channel stores. Each has a dedicated writer that is
// gated by its own permission and audited, so the generic key/value routes refuse them.
const (
	// PluginOverridesKey holds every per-plugin prerelease override as one JSON list.
	PluginOverridesKey = "update_channels_plugin_overrides"
	// PinsCategory is the settings category of the agent pin rows.
	PinsCategory = "agent_pins"
	// PinListKey holds the environment IDs of pinned agents.
	PinListKey = "agent_pinned_env_ids"
	// PinStateKey holds the version each pinned agent was told to return to.
	PinStateKey = "agent_return_state"
	// SelfUpdateAutoInstallKey is the stable auto-install switch, written only by the updates route.
	SelfUpdateAutoInstallKey = "self_update_auto_install"
	// SelfUpdateAgentExcludedKey lists the environments left out of agent sweeps; pins lean on it.
	SelfUpdateAgentExcludedKey = "self_update_agent_excluded_env_ids"
	// AgentReleaseSettingsKey holds the agent release sources and image repository.
	AgentReleaseSettingsKey = "agent_release_settings"
)

// Category is the settings category of the prerelease switches and plugin overrides.
const Category = category

var reservedKeys = func() map[string]struct{} {
	keys := map[string]struct{}{}
	for _, k := range []string{PluginOverridesKey, PinListKey, PinStateKey, SelfUpdateAutoInstallKey, SelfUpdateAgentExcludedKey, AgentReleaseSettingsKey} {
		keys[Fold(k)] = struct{}{}
	}
	for _, f := range fields {
		keys[Fold(f.key)] = struct{}{}
	}
	return keys
}()

// Fold returns s lowercased and without surrounding blanks. MySQL, MariaDB and MSSQL match
// settings.key and category case-insensitively and ignore trailing blanks, so a comparison
// done in Go must fold the same way or a differently spelled key reaches the stored row.
func Fold(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// hasNonASCII reports whether s holds a byte above 0x7f: accent-insensitive collations such as
// utf8mb4_0900_ai_ci equate "é" with "e", which no Go-side fold can follow.
func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return true
		}
	}
	return false
}

// Reserved reports whether a write of key under category would touch a row the channel
// and pin stores own. It matches on the folded key or category, so a new key under either
// category is covered, and a switch cannot be moved, recased or accented to dodge it.
func Reserved(key, cat string) bool {
	if hasNonASCII(key) || hasNonASCII(cat) {
		return true
	}
	if _, ok := reservedKeys[Fold(key)]; ok {
		return true
	}
	fc := Fold(cat)
	return fc == category || fc == PinsCategory
}
