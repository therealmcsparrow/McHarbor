// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

func rangeAt(start int64) backupIDMap {
	return backupIDMap{UIDStart: start, UIDCount: 65536, GIDStart: start, GIDCount: 65536}
}

func TestResolveSourceIDMapPicksTheMostReliableSource(t *testing.T) {
	a, b := rangeAt(1000000), rangeAt(1065536)
	row := Container{UIDMapStart: 1131072, UIDMapCount: 65536, GIDMapStart: 1131072, GIDMapCount: 65536}
	withMap := &backupManifest{IDMap: &b}
	legacyPriv := &backupManifest{Privileged: true}
	legacyUnpriv := &backupManifest{}

	cases := []struct {
		name     string
		recorded []backupIDMap
		row      *Container
		manifest *backupManifest
		want     *backupIDMap
		origin   string
		conflict bool
	}{
		{"recorded runs win over everything", []backupIDMap{a, a}, &row, withMap, &a, idMapFromRun, false},
		{"runs that disagree are refused", []backupIDMap{a, b}, &row, withMap, nil, "", true},
		{"a legacy run falls back to its container", nil, &row, withMap, &backupIDMap{UIDStart: 1131072, UIDCount: 65536, GIDStart: 1131072, GIDCount: 65536}, idMapFromContainer, false},
		{"a privileged container is the identity", nil, &Container{Privileged: true, UIDMapStart: 5}, nil, &backupIDMap{Privileged: true}, idMapFromContainer, false},
		{"then to the manifest's recorded range", nil, nil, withMap, &b, idMapFromManifest, false},
		{"a legacy privileged manifest is the identity", nil, nil, legacyPriv, &backupIDMap{Privileged: true}, idMapFromManifest, false},
		{"a legacy unprivileged manifest leaves it to the tree", nil, nil, legacyUnpriv, nil, idMapFromTree, false},
		{"nothing at all leaves it to the tree", nil, nil, nil, nil, idMapFromTree, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, origin, err := resolveSourceIDMap(tc.recorded, tc.row, tc.manifest)
			if tc.conflict {
				if !errors.Is(err, ErrIDMapConflict) {
					t.Fatalf("got %+v, %v; want ErrIDMapConflict", got, err)
				}
				return
			}
			if err != nil || origin != tc.origin {
				t.Fatalf("origin %q, err %v; want %q", origin, err, tc.origin)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRestoreNeedsTheAgentsAckOnlyWhenOwnersCanMove(t *testing.T) {
	a := rangeAt(1000000)
	if restoreNeedsAck(&a, rangeAt(1000000)) {
		t.Fatal("a restore into the same range waits for an acknowledgement an old agent never sends")
	}
	if !restoreNeedsAck(&a, rangeAt(1065536)) || !restoreNeedsAck(nil, a) ||
		!restoreNeedsAck(&backupIDMap{Privileged: true}, a) {
		t.Fatal("a restore that can move owners does not need the agent to say it did")
	}
	if ackOK(&agentPayload{Success: true}) || ackOK(&agentPayload{BackupResult: &agentBackupResult{IDShift: "skipped"}}) {
		t.Fatal("a reply that never mentions ownership counts as handled")
	}
	for _, shift := range []string{"none", "shifted"} {
		if !ackOK(&agentPayload{BackupResult: &agentBackupResult{IDShift: shift}}) {
			t.Fatalf("%q is not accepted", shift)
		}
	}
}

func TestTheManifestCarriesTheIDRangeAndOldManifestsStillRead(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	ct := Container{ID: "ct1", Name: "web", EnvironmentID: "env-a", InitSystem: InitOpenRC,
		UIDMapStart: 1065536, UIDMapCount: 65536, GIDMapStart: 1065536, GIDMapCount: 65536}
	raw, err := svc.backupManifestFor(context.Background(), ct)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := parseBackupManifest(raw)
	if err != nil {
		t.Fatalf("a new manifest does not read back: %v", err)
	}
	if manifest.IDMap == nil || *manifest.IDMap != rangeAt(1065536) {
		t.Fatalf("manifest idMap = %+v", manifest.IDMap)
	}
	if !strings.Contains(string(raw), `"uidStart": 1065536`) {
		t.Fatalf("the manifest does not name the range: %s", raw)
	}

	legacy := []byte(`{"version":1,"containerId":"ct0","name":"old","environmentId":"env-a",
		"initSystem":"openrc","privileged":false,"nesting":false,"poolBackend":"dir",
		"rootfsPath":"/p/containers/ct0/rootfs","limits":{},"takenAt":"2026-09-27T17:00:00Z"}`)
	old, err := parseBackupManifest(legacy)
	if err != nil || old.IDMap != nil || old.Name != "old" {
		t.Fatalf("a manifest written before the range was recorded: %+v, %v", old, err)
	}
}

func TestABackupRecordsItsIDRangeBeforeTheAgentIsCalled(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	svc.storage = keylessStorage{}
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	plan := BackupPlan{FullEvery: 7, DestinationIDs: []string{"default-local-backup"}}
	destinations, err := svc.resolveDestinations(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	chain, err := svc.chainFor(ctx, ct.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	run, err := svc.createBackupRun(ctx, ct, plan, BackupKindFull, chain)
	if err != nil {
		t.Fatal(err)
	}

	var recordedAtCall []backupIDMap
	var manifest backupManifest
	rpc.reply = func(messageType string, payload agentPayload) ([]byte, error) {
		if messageType == msgCTBackupRun {
			recordedAtCall, _ = svc.recordedIDMaps(ctx, []BackupRun{{ID: payload.Backup.RunID}})
			_ = json.Unmarshal(payload.Backup.Manifest, &manifest)
		}
		return []byte(`{"success":false,"error":"stop here"}`), nil
	}
	svc.executeBackupRun(ctx, ct, plan, run, chain, destinations)

	want := idMapOf(ct)
	if want.UIDStart != 1000000 {
		t.Fatalf("the first container's range starts at %d", want.UIDStart)
	}
	if len(recordedAtCall) != 1 || recordedAtCall[0] != want {
		t.Fatalf("recorded when the agent was called = %+v, want %+v", recordedAtCall, want)
	}
	if manifest.IDMap == nil || *manifest.IDMap != want {
		t.Fatalf("the manifest sent to the agent carries %+v", manifest.IDMap)
	}
}

// crossRangeFixture is two containers with different ranges and one finished
// full backup of the first, with its range recorded unless legacy is set.
func crossRangeFixture(t *testing.T, rpc *fakeRPC, legacy bool) (*Service, Container, Container, BackupRun) {
	t.Helper()
	svc, _ := newTestService(t, rpc)
	svc.storage = newFakeStorage("local")
	ctx := context.Background()
	a, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	second := createRequest()
	second.Name = "web-restored"
	b, err := svc.Create(ctx, second)
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}
	if a.UIDMapStart == b.UIDMapStart {
		t.Fatalf("both containers got range %d", a.UIDMapStart)
	}
	chain := BackupChain{ID: "chain-a", ContainerID: a.ID, ListingName: listingPrefix + "chain-a"}
	run := seedChainRun(t, svc, a, chain, BackupKindFull, BackupConsistencyFilesystem,
		time.Date(2026, 9, 27, 19, 50, 0, 0, time.UTC))
	if !legacy {
		if err := svc.recordRunIDMap(ctx, run.ID, idMapOf(a)); err != nil {
			t.Fatal(err)
		}
	}
	return svc, a, b, run
}

func waitRestore(t *testing.T, svc *Service) BackupRun {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := svc.db.Query(context.Background(),
			`SELECT id FROM plugin_addon_lxc_backup_runs WHERE operation = 'restore' AND status != 'running' LIMIT 1`)
		if err != nil {
			t.Fatal(err)
		}
		var id string
		if rows.Next() {
			_ = rows.Scan(&id)
		}
		_ = rows.Close()
		if id != "" {
			run, err := svc.BackupRunByID(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the restore never finished")
	return BackupRun{}
}

func TestARestoreIntoAnotherContainerSendsBothRangesAndNeedsTheAck(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reply  string
		status string
	}{
		{"an agent that predates re-owning", `{"success":true}`, BackupStatusFailure},
		{"an agent that re-owned", `{"success":true,"backupResult":{"kind":"","bytesWritten":0,"listingPresent":false,"idShift":"shifted"}}`, BackupStatusSuccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rpc := &fakeRPC{canRun: true}
			rpc.reply = func(messageType string, _ agentPayload) ([]byte, error) {
				if messageType == msgCTBackupRestore {
					return []byte(tc.reply), nil
				}
				return []byte(`{"success":true}`), nil
			}
			svc, a, b, run := crossRangeFixture(t, rpc, false)
			if _, err := svc.StartRestore(context.Background(), run.ID,
				RestoreBackupInput{TargetContainerID: b.ID, ConfirmName: b.Name}); err != nil {
				t.Fatalf("StartRestore: %v", err)
			}
			done := waitRestore(t, svc)
			sent, ok := rpc.last(msgCTBackupRestore)
			if !ok || sent.Restore == nil {
				t.Fatal("no restore was sent")
			}
			if sent.Restore.SourceIDMap == nil || *sent.Restore.SourceIDMap != idMapOf(a) {
				t.Fatalf("source range sent = %+v, want %+v", sent.Restore.SourceIDMap, idMapOf(a))
			}
			if sent.Restore.TargetIDMap == nil || *sent.Restore.TargetIDMap != idMapOf(b) {
				t.Fatalf("target range sent = %+v, want %+v", sent.Restore.TargetIDMap, idMapOf(b))
			}
			if done.Status != tc.status {
				t.Fatalf("restore status %q (%s), want %q", done.Status, done.Error, tc.status)
			}
			if tc.status == BackupStatusFailure && !strings.Contains(done.Error, "too old to move") {
				t.Fatalf("the failure does not say the agent could not move the owners: %q", done.Error)
			}
		})
	}
}

func TestARestoreIntoItsOwnContainerStillWorksWithAnOldAgent(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, a, _, run := crossRangeFixture(t, rpc, false)
	if _, err := svc.StartRestore(context.Background(), run.ID, RestoreBackupInput{ConfirmName: a.Name}); err != nil {
		t.Fatalf("StartRestore: %v", err)
	}
	if done := waitRestore(t, svc); done.Status != BackupStatusSuccess {
		t.Fatalf("a same-range restore through an agent that sends no acknowledgement: %q %s", done.Status, done.Error)
	}
}

func TestALegacyBackupTakesItsRangeFromTheSourceContainerThenFromTheTree(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, a, b, run := crossRangeFixture(t, rpc, true)
	ctx := context.Background()
	if _, err := svc.StartRestore(ctx, run.ID, RestoreBackupInput{TargetContainerID: b.ID, ConfirmName: b.Name}); err != nil {
		t.Fatalf("StartRestore: %v", err)
	}
	waitRestore(t, svc)
	sent, _ := rpc.last(msgCTBackupRestore)
	if sent.Restore.SourceIDMap == nil || *sent.Restore.SourceIDMap != idMapOf(a) {
		t.Fatalf("a legacy run with its container still present sent %+v", sent.Restore.SourceIDMap)
	}

	if _, err := svc.db.Exec(ctx, `DELETE FROM plugin_addon_lxc_containers WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(ctx, `DELETE FROM plugin_addon_lxc_backup_runs WHERE operation = 'restore'`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartRestore(ctx, run.ID, RestoreBackupInput{TargetContainerID: b.ID, ConfirmName: b.Name}); err != nil {
		t.Fatalf("StartRestore without the source container: %v", err)
	}
	waitRestore(t, svc)
	sent, _ = rpc.last(msgCTBackupRestore)
	if sent.Restore.SourceIDMap != nil {
		t.Fatalf("with no record, no container and no manifest the plugin guessed %+v", sent.Restore.SourceIDMap)
	}
	if sent.Restore.TargetIDMap == nil || *sent.Restore.TargetIDMap != idMapOf(b) {
		t.Fatalf("target range sent = %+v", sent.Restore.TargetIDMap)
	}
}

func TestAChainWhoseRunsDisagreeOnTheRangeIsRefusedBeforeAnythingMoves(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, a, b, full := crossRangeFixture(t, rpc, false)
	ctx := context.Background()
	chain := BackupChain{ID: "chain-a", ContainerID: a.ID, ListingName: listingPrefix + "chain-a"}
	incr := seedChainRun(t, svc, a, chain, BackupKindIncremental, BackupConsistencyFilesystem,
		full.StartedAt.Add(time.Hour))
	if err := svc.recordRunIDMap(ctx, incr.ID, idMapOf(b)); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.StartRestore(ctx, incr.ID, RestoreBackupInput{TargetContainerID: b.ID, ConfirmName: b.Name}); !errors.Is(err, ErrIDMapConflict) {
		t.Fatalf("StartRestore of a chain with two ranges = %v", err)
	}
	if _, sent := rpc.last(msgCTBackupRestore); sent {
		t.Fatal("the agent was asked to restore a chain with two ranges")
	}

	h := NewHandler(svc, nil, nil, nil, quietLogger{})
	req := httptest.NewRequest(http.MethodPost, "/api/system-containers/backups/"+incr.ID+"/restore",
		strings.NewReader(`{"confirmName":"`+b.Name+`","targetContainerId":"`+b.ID+`"}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("runId", incr.ID)
	reqCtx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	reqCtx = context.WithValue(reqCtx, auth.UserContextKey, &auth.User{ID: "user-1"})
	rec := httptest.NewRecorder()
	h.HandleRestoreBackup(rec, req.WithContext(reqCtx))
	var envelope apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if rec.Code != http.StatusConflict || envelope.Code != "err.ct.backup_idmap_conflict" {
		t.Fatalf("status %d envelope %+v, want 409 err.ct.backup_idmap_conflict", rec.Code, envelope)
	}
}

func TestDeletingABackupRunTakesItsRecordedRangeWithIt(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _, _, run := crossRangeFixture(t, rpc, false)
	ctx := context.Background()
	if _, err := svc.db.Exec(ctx, `DELETE FROM plugin_addon_lxc_backup_runs WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM plugin_addon_lxc_backup_run_idmaps WHERE id = ?`, run.ID); n != 0 {
		t.Fatalf("%d id-range rows outlived their run", n)
	}
}

func TestEnsureSchemaCreatesTheIDRangeTableOnAnInstallThatPredatesIt(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _, _, run := crossRangeFixture(t, rpc, true)
	ctx := context.Background()
	if _, err := svc.db.Exec(ctx, `DROP TABLE plugin_addon_lxc_backup_run_idmaps`); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if err := svc.EnsureSchema(ctx); err != nil {
		t.Fatalf("a second EnsureSchema: %v", err)
	}
	if err := svc.recordRunIDMap(ctx, run.ID, rangeAt(1000000)); err != nil {
		t.Fatalf("recording into the table EnsureSchema made: %v", err)
	}
	if _, err := svc.db.Exec(ctx, `DELETE FROM plugin_addon_lxc_backup_runs WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM plugin_addon_lxc_backup_run_idmaps`); n != 0 {
		t.Fatalf("%d id-range rows outlived their run in the table EnsureSchema made", n)
	}
}
