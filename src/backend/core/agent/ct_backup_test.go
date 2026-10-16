// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"encoding/json"
	"testing"
)

func TestCTBackupMessageTypesAreDistinctAndPrefixed(t *testing.T) {
	// The prefix is what the plugin route ceiling files these under (slot ct), and
	// a collision with an existing ct_* name would route a backup into the
	// lifecycle handler.
	seen := map[string]string{}
	for name, value := range map[string]string{
		"MsgCTBackupRun":      MsgCTBackupRun,
		"MsgCTBackupRestore":  MsgCTBackupRestore,
		"MsgCTBackupProgress": MsgCTBackupProgress,
		"MsgCTBackupResult":   MsgCTBackupResult,
	} {
		if previous, duplicate := seen[value]; duplicate {
			t.Fatalf("%s and %s are both %q", name, previous, value)
		}
		seen[value] = name
	}
	for _, existing := range []string{
		MsgCTCreate, MsgCTStart, MsgCTStop, MsgCTDelete, MsgCTResult,
		MsgCTSnapshotCreate, MsgCTSnapshotResult, MsgCTCloneRequest,
		MsgVMBackupRun, MsgVMBackupRestore,
	} {
		if _, clash := seen[existing]; clash {
			t.Fatalf("%q collides with an existing message type", existing)
		}
	}
}

func TestCTBackupPayloadRoundTripsThroughJSON(t *testing.T) {
	// The wire shape is the contract with addon-lxc, which declares its own
	// mirror of these structs rather than importing the shared compute spec.
	// A tag that changed here and not there fails silently — the field arrives
	// zero and the agent backs up nothing.
	original := CTPayload{
		Operation:   MsgCTBackupRun,
		ContainerID: "ct-1",
		Backup: &CTBackupSpec{
			RunID:       "run-1",
			ChainID:     "chain-1",
			Kind:        "incremental",
			RootfsPath:  "/pool/ct-1",
			PoolID:      "pool-1",
			PoolBackend: "btrfs",
			PoolTarget:  "/pool",
			Manifest:    json.RawMessage(`{"name":"web"}`),
			ListingName: "mcharbor-ct-chain-1",
			Upload: &CTBackupUploadSpec{
				URL:   "/api/system-containers/backups/internal/abc",
				Token: "token",
			},
		},
		Restore: &CTRestoreSpec{
			RunID:      "run-2",
			TargetPath: "/pool/ct-2",
			Artifacts: []CTRestoreArtifactRef{
				{Part: "rootfs", Kind: "full", URL: "/d/1", Token: "t1", Sequence: 0, SizeBytes: 11},
				{Part: "rootfs", Kind: "incremental", URL: "/d/2", Token: "t2", Sequence: 1},
			},
		},
		BackupResult: &CTBackupResult{
			Kind: "full", BytesWritten: 4096,
			ListingName: "mcharbor-ct-chain-1", ListingPresent: true,
			FellBackToFull: true, FallbackReason: "the listing is gone",
			Consistency: "frozen",
		},
		BackupBytes: 4096,
		Progress:    map[string]string{"runId": "run-1", "stage": "archiving", "bytes": "2048"},
		Success:     true,
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	var decoded CTPayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if decoded.Backup == nil || decoded.Backup.ListingName != "mcharbor-ct-chain-1" {
		t.Fatalf("the backup spec did not survive the round trip: %+v", decoded.Backup)
	}
	if decoded.Backup.PoolBackend != "btrfs" || decoded.Backup.RootfsPath != "/pool/ct-1" {
		t.Fatalf("the quiesce inputs did not survive: %+v", decoded.Backup)
	}
	if string(decoded.Backup.Manifest) != `{"name":"web"}` {
		t.Fatalf("the manifest did not survive: %s", decoded.Backup.Manifest)
	}
	if decoded.Restore == nil || len(decoded.Restore.Artifacts) != 2 {
		t.Fatalf("the restore chain did not survive: %+v", decoded.Restore)
	}
	if decoded.Restore.Artifacts[1].Sequence != 1 {
		t.Fatalf("the chain order did not survive: %+v", decoded.Restore.Artifacts)
	}
	if decoded.BackupResult == nil || !decoded.BackupResult.FellBackToFull {
		t.Fatalf("the downgrade verdict did not survive: %+v", decoded.BackupResult)
	}
	if decoded.BackupResult.Consistency != "frozen" {
		t.Fatalf("the consistency did not survive: %+v", decoded.BackupResult)
	}
	if decoded.Progress["bytes"] != "2048" {
		t.Fatalf("the progress detail did not survive: %+v", decoded.Progress)
	}

	// The keys, not only the round trip. A round trip through one struct is
	// symmetric and would survive any rename; what addon-lxc mirrors is the
	// spelling on the wire, and a field renamed here arrives zero there with
	// nothing failing.
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("decoding the envelope: %v", err)
	}
	assertKeys(t, "ct payload", wire,
		"operation", "containerId", "backup", "restore", "backupResult",
		"backupBytes", "progress", "success")

	var backup map[string]json.RawMessage
	if err := json.Unmarshal(wire["backup"], &backup); err != nil {
		t.Fatalf("decoding the backup spec: %v", err)
	}
	assertKeys(t, "backup spec", backup,
		"runId", "chainId", "kind", "rootfsPath", "poolId", "poolBackend",
		"poolTarget", "manifest", "listingName", "upload")

	var result map[string]json.RawMessage
	if err := json.Unmarshal(wire["backupResult"], &result); err != nil {
		t.Fatalf("decoding the backup result: %v", err)
	}
	assertKeys(t, "backup result", result,
		"kind", "bytesWritten", "listingName", "listingPresent",
		"fellBackToFull", "fallbackReason", "consistency")

	var restore map[string]json.RawMessage
	if err := json.Unmarshal(wire["restore"], &restore); err != nil {
		t.Fatalf("decoding the restore spec: %v", err)
	}
	assertKeys(t, "restore spec", restore, "runId", "targetPath", "artifacts")
}

// assertKeys fails when a wire object is missing a key the plugin mirrors.
func assertKeys(t *testing.T, what string, object map[string]json.RawMessage, wanted ...string) {
	t.Helper()
	for _, key := range wanted {
		if _, present := object[key]; !present {
			t.Errorf("%s is missing the %q key; addon-lxc mirrors that spelling", what, key)
		}
	}
}

func TestSupportsCTBackupRejectsEverythingBelowTheFloor(t *testing.T) {
	// The historical 3.x floors are deliberate and untouched; this gate is the
	// calendar version of the release that introduced the messages, and the build
	// is part of it because one calendar version covers a whole month.
	cases := []struct {
		name    string
		version string
		build   string
		want    bool
	}{
		{"an agent from the 3.x line", "3.8.0", "202609271200", false},
		{"the floor version, an earlier build", CTBackupMinVersion, "202609010000", false},
		{"the floor version, unstamped", CTBackupMinVersion, "dev", false},
		{"the floor version, no build at all", CTBackupMinVersion, "", false},
		{"the floor version and build exactly", CTBackupMinVersion, CTBackupMinBuild, true},
		{"the floor version, a later build", CTBackupMinVersion, "202612311200", true},
		{"a later version, whatever the build", "2026.10.01", "dev", true},
	}
	for _, testCase := range cases {
		if got := SupportsCTBackup(testCase.version, testCase.build); got != testCase.want {
			t.Errorf("%s (%s build %s): want %v, got %v",
				testCase.name, testCase.version, testCase.build, testCase.want, got)
		}
	}
}

func TestCTBackupFloorDoesNotMoveTheHistoricalGates(t *testing.T) {
	// A new gate takes the calendar version of its own release; the four 3.x
	// floors are the record of when each of those features landed and rewriting
	// them would pass every gate for every agent.
	for name, value := range map[string]string{
		"ComputeMinVersion":         ComputeMinVersion,
		"ImageLibraryMinVersion":    ImageLibraryMinVersion,
		"GuestNetworkingMinVersion": GuestNetworkingMinVersion,
		"FreeVolumeMinVersion":      FreeVolumeMinVersion,
	} {
		if value == CTBackupMinVersion {
			t.Fatalf("%s has been rewritten to the calendar version; it is a historical floor", name)
		}
	}
}
