// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import "testing"

// TestConfiguredEmailGoesToTheNodesRecipients guards the send-email node's "To": it must
// retarget the send, or the mail silently reaches the channel's stored recipients instead.
func TestConfiguredEmailGoesToTheNodesRecipients(t *testing.T) {
	req := configuredEmailRequest("mail-1", nil, "ops@example.com,oncall@example.com", "Disk full", "hello")
	if req.Recipient != "ops@example.com,oncall@example.com" {
		t.Fatalf("the node's recipients must reach the channel, got Recipient %q", req.Recipient)
	}
	if req.ChannelID != "mail-1" || req.Title != "Disk full" || req.Message != "hello" {
		t.Fatalf("channel, subject and body must pass through unchanged, got %+v", req)
	}
}
