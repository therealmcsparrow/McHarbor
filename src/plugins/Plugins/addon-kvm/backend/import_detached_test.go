// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// cancelOnCall ends the caller's request the moment the host is asked to work, which
// is what a closed tab or an HTTP deadline does to a multi-minute import run.
type cancelOnCall struct {
	*fakeGuestRPC
	cancel context.CancelFunc
}

func (c *cancelOnCall) Call(ctx context.Context, envID, messageType string, body []byte, onProgress func(string, []byte)) ([]byte, error) {
	c.cancel()
	return c.fakeGuestRPC.Call(ctx, envID, messageType, body, onProgress)
}

// TestStagedImportIsRecordedAfterTheRequestIsGone guards the import outcome: the run is
// detached from the request, so recording its result must not fail on a cancelled one.
func TestStagedImportIsRecordedAfterTheRequestIsGone(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	rpc.reply = []byte(`{"importResult":{"stagedPath":"/var/lib/mcharbor/imports/disk.vmdk"}}`)

	staged := filepath.Join(t.TempDir(), "disk.vmdk")
	if err := os.WriteFile(staged, []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO plugin_addon_kvm_imports
			(id, file_name, source_kind, staged_path, size_bytes, state, environment_id, created_at, updated_at)
		VALUES ('imp1', 'disk.vmdk', 'vmdk', ?, 4, ?, 'env1', ?, ?)`, staged, ImportUploaded, now, now); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.rpc = newAgentBridge(&cancelOnCall{fakeGuestRPC: rpc, cancel: cancel})

	if _, err := svc.StageImportOnHost(ctx, "imp1", "env1"); err != nil {
		t.Fatalf("a finished staging must be recorded even though the request ended: %v", err)
	}
	var hostPath string
	if err := db.QueryRow(`SELECT COALESCE(host_path, '') FROM plugin_addon_kvm_imports WHERE id = 'imp1'`).Scan(&hostPath); err != nil {
		t.Fatal(err)
	}
	if hostPath != "/var/lib/mcharbor/imports/disk.vmdk" {
		t.Fatalf("host path not recorded, got %q", hostPath)
	}
}
