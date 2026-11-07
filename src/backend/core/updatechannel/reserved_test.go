// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updatechannel

import "testing"

// TestReservedFoldsKeyAndCategory: MySQL and MSSQL match settings.key and category under a
// case-insensitive, trailing-blank-insensitive collation, so a differently spelled key reaches
// the stored row and must be refused exactly like the canonical spelling.
func TestReservedFoldsKeyAndCategory(t *testing.T) {
	cases := []struct{ key, cat string }{
		{"update_channels_mcharbor_auto", "update_channels"},
		{"UPDATE_CHANNELS_MCHARBOR_AUTO", "Update_Channels"},
		{"UPDATE_CHANNELS_MCHARBOR_AUTO", "general"},
		{"update_channels_mcharbor_auto ", "general"},
		{" Update_Channels_Plugin_Overrides", ""},
		{"AGENT_PINNED_ENV_IDS", "x"},
		{"Agent_Return_State\t", "x"},
		{"anything", "AGENT_PINS"},
		{"anything", "Update_Channels "},
		{"self_update_auto_install", "self_update"},
		{"SELF_UPDATE_AUTO_INSTALL", "general"},
		{"Self_Update_Agent_Excluded_Env_Ids ", "general"},
		{"self_update_agent_excluded_env_ids", ""},
		{"agent_release_settings", "general"},
		{"Agent_Release_Settings", "x"},
		{"update_channéls_mcharbor", "general"},
		{"general_ünicode", "general"},
	}
	for _, c := range cases {
		if !Reserved(c.key, c.cat) {
			t.Errorf("Reserved(%q, %q) = false, want true", c.key, c.cat)
		}
	}
}

// TestReservedLeavesOrdinaryRowsAlone: the refusal must not spread to the other self_update
// rows, whose writers are the generic routes' neighbours, or to unrelated keys.
func TestReservedLeavesOrdinaryRowsAlone(t *testing.T) {
	cases := []struct{ key, cat string }{
		{"theme", "ui"},
		{"security.mfa_required", "security"},
		{"self_update_interval_hours", "self_update"},
		{"agent_release", "general"},
	}
	for _, c := range cases {
		if Reserved(c.key, c.cat) {
			t.Errorf("Reserved(%q, %q) = true, want false", c.key, c.cat)
		}
	}
}
