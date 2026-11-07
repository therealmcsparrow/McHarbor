// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"net/http"
	"strings"
	"testing"
)

// TestGenericWritesRefuseTheUpdateChannelAndPinRows: the prerelease switches, the plugin
// overrides and the agent pins have dedicated, gated and audited writers, so a holder of
// settings.manage alone must not reach them through the key/value routes.
func TestGenericWritesRefuseTheUpdateChannelAndPinRows(t *testing.T) {
	database, _, r := newSensitiveKeyFixture(t)
	reserved := []struct{ key, category string }{
		{"update_channels_mcharbor", "update_channels"},
		{"update_channels_agents", "update_channels"},
		{"update_channels_plugins", "update_channels"},
		{"update_channels_mcharbor_auto", "update_channels"},
		{"update_channels_agents_auto", "update_channels"},
		{"update_channels_plugin_overrides", "update_channels"},
		{"agent_pinned_env_ids", "agent_pins"},
		{"agent_return_state", "agent_pins"},
		{"update_channels_mcharbor_auto", "self_update"},
		{"agent_pinned_env_ids", ""},
		{"anything_new", "update_channels"},
		{"anything_new", "agent_pins"},
		{"UPDATE_CHANNELS_MCHARBOR_AUTO", "Update_Channels"},
		{"UPDATE_CHANNELS_MCHARBOR_AUTO", "general"},
		{"Update_Channels_Plugin_Overrides", "general"},
		{"AGENT_PINNED_ENV_IDS", "general"},
		{"self_update_auto_install", "self_update"},
		{"SELF_UPDATE_AUTO_INSTALL", "Self_Update"},
		{"Self_Update_Agent_Excluded_Env_Ids", "general"},
		{"self_update_agent_excluded_env_ids", "self_update"},
		{"agent_release_settings", "general"},
		{"AGENT_RELEASE_SETTINGS", "general"},
		{"TLS_KEY", "general"},
		{"Webpush_Vapid_Private", "general"},
	}
	for _, c := range reserved {
		name := c.key + "/" + c.category
		single := send(r, http.MethodPut, "/settings/"+c.key, `{"value":"true","category":"`+c.category+`"}`)
		if single.Code != http.StatusBadRequest || !strings.Contains(single.Body.String(), "err.protected_target") {
			t.Errorf("PUT /settings/%s = %d %s, want 400 err.protected_target", name, single.Code, single.Body.String())
		}
		bulk := send(r, http.MethodPut, "/settings", `{"settings":[{"key":"theme","value":"dark","category":"ui"},{"key":"`+c.key+`","value":"true","category":"`+c.category+`"}]}`)
		if bulk.Code != http.StatusBadRequest || !strings.Contains(bulk.Body.String(), "err.protected_target") {
			t.Errorf("bulk update with %s = %d %s, want 400 err.protected_target", name, bulk.Code, bulk.Body.String())
		}
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM settings WHERE key LIKE 'update_channels%' OR key LIKE 'agent_%' OR key = 'anything_new' OR (key = 'theme' AND value = 'dark')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a refused request still wrote %d setting rows", n)
	}

	ok := send(r, http.MethodPut, "/settings/theme", `{"value":"dark","category":"ui"}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("an ordinary key was refused: %d %s", ok.Code, ok.Body.String())
	}
}

// TestReservedRowsStayReadable: refusing the writes must not hide the rows from a read, which
// the settings pages and the update cards rely on.
func TestReservedRowsStayReadable(t *testing.T) {
	database, h, _ := newSensitiveKeyFixture(t)
	if _, err := database.Exec(`INSERT INTO settings (id, key, value, category, updated_at) VALUES ('x1', 'update_channels_mcharbor', 'true', 'update_channels', '2026-11-07T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	list, err := h.service.List(t.Context(), "update_channels")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v; want the reserved row still returned", list, err)
	}
	if isSensitiveKey("update_channels_mcharbor") {
		t.Fatal("a reserved key joined sensitiveKeys, which would hide it from reads")
	}
}
