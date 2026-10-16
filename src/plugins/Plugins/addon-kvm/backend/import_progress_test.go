// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// errFakeAgentStopped ends the fake conversion once the test has seen what it needs.
var errFakeAgentStopped = errors.New("fake agent stopped")

// convertingHost is a host whose conversion reports progress through the relayed
// source and holds the call open until that progress has reached the import row.
type convertingHost struct {
	*fakeGuestRPC
	db      *sql.DB
	disk    int
	percent int
	seen    int
}

func (h *convertingHost) Call(ctx context.Context, envID, messageType string, body []byte, onProgress func(string, []byte)) ([]byte, error) {
	if messageType != msgVMImportConvert {
		return h.fakeGuestRPC.Call(ctx, envID, messageType, body, onProgress)
	}
	h.emit(envID, msgVMImportProgress, fmt.Sprintf(`{"operation":"vm_import_convert","percent":%d,"import":{"importId":"imp1"},"importResult":{"importId":"imp1","disks":[{"index":%d}]}}`, h.percent, h.disk))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := h.db.QueryRow(`SELECT progress FROM plugin_addon_kvm_imports WHERE id = 'imp1'`).Scan(&h.seen); err == nil && h.seen > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, errFakeAgentStopped
}

func insertInspectedImport(t *testing.T, db *sql.DB, disks []ImportDisk) {
	t.Helper()
	inspection, err := json.Marshal(ImportInspection{Kind: "vmdk", Name: "legacy box", Disks: disks})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO plugin_addon_kvm_imports
			(id, file_name, source_kind, staged_path, size_bytes, state, environment_id, inspection, host_path, created_at, updated_at)
		VALUES ('imp1', 'disk.vmdk', 'vmdk', '', 4, ?, 'env1', ?, '/var/lib/mcharbor/imports/imp1/disk.vmdk', ?, ?)`,
		ImportInspected, string(inspection), now, now); err != nil {
		t.Fatal(err)
	}
}

func fastProgressPoll(t *testing.T) {
	t.Helper()
	previous := importProgressPoll
	importProgressPoll = 5 * time.Millisecond
	t.Cleanup(func() { importProgressPoll = previous })
}

var convertRequest = ImportConvertRequest{
	EnvironmentID: "env1",
	PoolID:        "pool1", PoolBackend: "dir", PoolTarget: "/var/lib/mcharbor/pools/pool1",
	NetworkID: "net1", Bridge: "br0",
}

// TestConversionRecordsTheRelayedPercentage is the 99% bug: while the agent
// converts, the row must carry the conversion's own progress, spread across the
// disks, rather than the pull's last byte count.
func TestConversionRecordsTheRelayedPercentage(t *testing.T) {
	fastProgressPoll(t)
	svc, rpc, db := newIntentTestService(t)
	insertInspectedImport(t, db, []ImportDisk{{Index: 0}, {Index: 1}})
	host := &convertingHost{fakeGuestRPC: rpc, db: db, disk: 1, percent: 60}
	svc.rpc = newAgentBridge(host)
	svc.rpc.Subscribe(nil)

	if _, err := svc.ConvertImport(context.Background(), "imp1", convertRequest); !errors.Is(err, errFakeAgentStopped) {
		t.Fatalf("the fake conversion should end with its own error, got %v", err)
	}
	// The second of two disks at 60% is 80% of the whole conversion.
	if host.seen != 80 {
		t.Fatalf("during the conversion the row read %d%%, want 80%%", host.seen)
	}
}

// TestImportRunsWithoutAProgressFrame guards the fallback: an agent that has sent no
// progress yet leaves only the pull byte count, and the import must run as before.
func TestImportRunsWithoutAProgressFrame(t *testing.T) {
	fastProgressPoll(t)
	svc, rpc, db := newIntentTestService(t)
	insertInspectedImport(t, db, []ImportDisk{{Index: 0}})
	rpc.fail = errFakeAgentStopped
	svc.rpc = newAgentBridge(rpc)

	if _, err := svc.ConvertImport(context.Background(), "imp1", convertRequest); !errors.Is(err, errFakeAgentStopped) {
		t.Fatalf("the conversion should reach the host and fail with its error, got %v", err)
	}
	if stop := svc.watchImportProgress("imp1", "env1", nil); stop == nil {
		t.Fatal("watchImportProgress must return a usable stop func before any frame arrived")
	} else {
		stop()
		stop()
	}
}

// TestImportProgressNeverMovesBackwards guards the race between the pull byte count
// and the agent's reports, and the per-disk restart of a conversion.
func TestImportProgressNeverMovesBackwards(t *testing.T) {
	job := &importJob{}
	steps := []struct {
		phase   string
		percent int
		want    bool
	}{
		{importPhasePull, 40, true},
		{importPhasePull, 39, false},
		{importPhaseConvert, 3, true},
		{importPhasePull, 100, false},
		{importPhaseConvert, 3, false},
		{"", 90, false},
		{importPhaseConvert, 50, true},
	}
	for i, step := range steps {
		if got := job.advance(step.phase, step.percent); got != step.want {
			t.Fatalf("step %d (%s %d%%): advance = %v, want %v", i, step.phase, step.percent, got, step.want)
		}
	}
}
