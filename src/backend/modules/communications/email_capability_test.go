// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// TestSupportsEmailIsWhatThePluginDeclares: the security picker's "can carry an email"
// is the plugin's own email-channel declaration, present only while the plugin is loaded.
func TestSupportsEmailIsWhatThePluginDeclares(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "comm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO plugin_notification_adapters (origin, channel_type, display_name) VALUES
		('comm-captest-mail', 'captest_mail', 'Mail'), ('comm-captest-chat', 'captest_chat', 'Chat')`); err != nil {
		t.Fatal(err)
	}
	plugincaps.Default.Set("comm-captest-mail", []plugincaps.Capability{{Kind: plugincaps.KindEmailChannel, ChannelType: "captest_mail"}})
	t.Cleanup(func() { plugincaps.Default.Remove("comm-captest-mail") })

	supports := func() map[string]bool {
		caps, err := NewService(database, nil).AdapterCapabilities(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, c := range caps {
			out[c.ChannelType] = c.SupportsEmail
		}
		return out
	}
	if got := supports(); !got["captest_mail"] || got["captest_chat"] {
		t.Fatalf("want only the declaring channel type to carry email, got %v", got)
	}
	plugincaps.Default.Remove("comm-captest-mail")
	if got := supports(); got["captest_mail"] {
		t.Fatalf("an unloaded plugin's channel must not carry email, got %v", got)
	}
}
