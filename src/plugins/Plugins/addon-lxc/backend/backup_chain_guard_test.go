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

// seedChainRun records one finished backup run on chain, started at, with its
// rootfs object at one destination.
func seedChainRun(t *testing.T, svc *Service, ct Container, chain BackupChain, kind, consistency string, at time.Time) BackupRun {
	t.Helper()
	ctx := context.Background()
	run, err := svc.createBackupRun(ctx, ct, BackupPlan{}, kind, chain)
	if err != nil {
		t.Fatal(err)
	}
	destID := svc.beginDestination(ctx, run.ID, backupDestination{ID: "loc-1", Name: "Local"}, BackupPartRootfs)
	svc.completeDestination(ctx, destID, "env-a/ct/web/"+run.ID+"-rootfs-"+kind+".tar.gz", "", 100)
	svc.finishBackupRun(ctx, run.ID, kind, false, "", consistency, 100)
	if _, err := svc.db.Exec(ctx, `UPDATE plugin_addon_lxc_backup_runs SET started_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339), run.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.BackupRunByID(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestCheckChainRestorableRefusesIncrementalsReadFromADirCopy(t *testing.T) {
	at := time.Date(2026, 9, 27, 17, 0, 0, 0, time.UTC)
	full := func(consistency string) BackupRun {
		return BackupRun{ID: "full1", Kind: BackupKindFull, Consistency: consistency, StartedAt: at}
	}
	incr := func(consistency string) BackupRun {
		return BackupRun{ID: "incr1", Kind: BackupKindIncremental, Consistency: consistency, StartedAt: at.Add(time.Hour)}
	}
	cases := []struct {
		name    string
		chain   []BackupRun
		backend string
		refused bool
	}{
		{"a live-copy full on its own", []BackupRun{full(BackupConsistencyLiveCopy)}, "dir", false},
		{"live-copy full and incremental", []BackupRun{full(BackupConsistencyLiveCopy), incr(BackupConsistencyLiveCopy)}, "", true},
		{"at-rest full, live-copy incremental", []BackupRun{full(BackupConsistencyFilesystem), incr(BackupConsistencyLiveCopy)}, "dir", true},
		{"live-copy full, at-rest incremental", []BackupRun{full(BackupConsistencyLiveCopy), incr(BackupConsistencyFilesystem)}, "dir", true},
		{"operator-frozen dir copies", []BackupRun{full(BackupConsistencySnapshot), incr(BackupConsistencySnapshot)}, "dir", true},
		{"btrfs snapshots", []BackupRun{full(BackupConsistencySnapshot), incr(BackupConsistencySnapshot)}, "btrfs", false},
		{"snapshots, backend unknown", []BackupRun{full(BackupConsistencySnapshot), incr(BackupConsistencySnapshot)}, "", false},
		{"dir pool at rest and frozen", []BackupRun{full(BackupConsistencyFilesystem), incr(BackupConsistencyFrozen)}, "dir", false},
	}
	for _, c := range cases {
		err := checkChainRestorable(c.chain, c.backend)
		if c.refused != errors.Is(err, ErrChainUnrestorable) {
			t.Errorf("%s: err = %v, refused want %v", c.name, err, c.refused)
			continue
		}
		if c.refused && !strings.Contains(err.Error(), "run full1") {
			t.Errorf("%s: the refusal does not name the full to restore instead: %v", c.name, err)
		}
	}
}

// legacyDirChain is the fifth pass's chain: a live-copy full and a live-copy
// incremental of a container on a dir pool.
func legacyDirChain(t *testing.T, svc *Service, rpc *fakeRPC) (Container, BackupRun, BackupRun) {
	t.Helper()
	ct, err := svc.Create(context.Background(), createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	chain := BackupChain{ID: "chain-legacy", ContainerID: ct.ID, ListingName: listingPrefix + "chain-legacy"}
	at := time.Date(2026, 9, 27, 17, 0, 27, 0, time.UTC)
	full := seedChainRun(t, svc, ct, chain, BackupKindFull, BackupConsistencyLiveCopy, at)
	incr := seedChainRun(t, svc, ct, chain, BackupKindIncremental, BackupConsistencyLiveCopy, at.Add(19*time.Second))
	return ct, full, incr
}

func countRows(t *testing.T, svc *Service, query string, args ...any) int {
	t.Helper()
	rows, err := svc.db.Query(context.Background(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

func TestRestoringALegacyDirPoolIncrementalIsRefusedBeforeAnythingMoves(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	svc.storage = newFakeStorage("local")
	ct, full, incr := legacyDirChain(t, svc, rpc)

	_, err := svc.StartRestore(context.Background(), incr.ID, RestoreBackupInput{ConfirmName: ct.Name})
	if !errors.Is(err, ErrChainUnrestorable) {
		t.Fatalf("StartRestore of a dir-pool incremental = %v, want ErrChainUnrestorable", err)
	}
	if !strings.Contains(err.Error(), full.ID) {
		t.Fatalf("the refusal does not name the full backup %s: %v", full.ID, err)
	}
	if _, sent := rpc.last(msgCTBackupRestore); sent {
		t.Fatal("the agent was asked to restore a chain that cannot be applied")
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM plugin_addon_lxc_backup_runs WHERE operation = 'restore'`); n != 0 {
		t.Fatalf("%d restore runs were recorded for a refused restore", n)
	}

	if _, err := svc.StartRestore(context.Background(), full.ID, RestoreBackupInput{ConfirmName: ct.Name}); errors.Is(err, ErrChainUnrestorable) {
		t.Fatalf("the full backup the refusal points at is refused too: %v", err)
	}
}

func TestRecreatingFromALegacyDirPoolIncrementalDefinesNothing(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	svc.storage = newFakeStorage("local")
	_, _, incr := legacyDirChain(t, svc, rpc)
	before := countRows(t, svc, `SELECT COUNT(*) FROM plugin_addon_lxc_containers`)

	create := createRequest()
	create.Name = "web-restored"
	create.ImageID = ""
	if _, err := svc.Recreate(context.Background(), incr.ID, create, createOrigin{}); !errors.Is(err, ErrChainUnrestorable) {
		t.Fatalf("Recreate from a dir-pool incremental = %v, want ErrChainUnrestorable", err)
	}
	if after := countRows(t, svc, `SELECT COUNT(*) FROM plugin_addon_lxc_containers`); after != before {
		t.Fatalf("a refused recreate defined a container (%d -> %d rows)", before, after)
	}
}

func TestTheRestoreRouteAnswersTheUnrestorableCode(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	svc.storage = newFakeStorage("local")
	ct, full, incr := legacyDirChain(t, svc, rpc)

	h := NewHandler(svc, nil, nil, nil, quietLogger{})
	req := httptest.NewRequest(http.MethodPost, "/api/system-containers/backups/"+incr.ID+"/restore",
		strings.NewReader(`{"confirmName":"`+ct.Name+`"}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("runId", incr.ID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.User{ID: "user-1"})
	rec := httptest.NewRecorder()
	h.HandleRestoreBackup(rec, req.WithContext(ctx))

	var envelope apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if rec.Code != http.StatusConflict || envelope.Code != "err.ct.backup_chain_unrestorable" {
		t.Fatalf("status %d envelope %+v, want 409 err.ct.backup_chain_unrestorable", rec.Code, envelope)
	}
	if !strings.Contains(envelope.Error, full.ID) {
		t.Fatalf("the detail does not name the full to restore instead: %q", envelope.Error)
	}
}

func TestAFullTakenInsteadOfAnIncrementalHeadsANewChain(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	chain, err := svc.chainFor(ctx, ct.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 27, 17, 0, 0, 0, time.UTC)
	first := seedChainRun(t, svc, ct, chain, BackupKindFull, BackupConsistencyLiveCopy, at)
	if _, err := svc.recordChainRun(ctx, chain, first.ID, BackupKindFull, false, ""); err != nil {
		t.Fatal(err)
	}

	// The plan asked for an incremental; the agent read a dir copy and took a full.
	second := seedChainRun(t, svc, ct, chain, BackupKindFull, BackupConsistencyLiveCopy, at.Add(time.Hour))
	fresh, err := svc.recordChainRun(ctx, chain, second.ID, BackupKindFull, true, "this run read a fresh copy")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == chain.ID {
		t.Fatal("a fallback did not start a new chain")
	}
	if fresh.ListingName != chain.ListingName {
		t.Fatalf("listing %q -> %q: the new chain lost the level-0 listing its head just wrote",
			chain.ListingName, fresh.ListingName)
	}
	moved, err := svc.BackupRunByID(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.ChainID != fresh.ID {
		t.Fatalf("the full that heads the new chain is still on %q, want %q", moved.ChainID, fresh.ID)
	}

	current, err := svc.chainFor(ctx, ct.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if kind, reason := svc.decideRunKind(ctx, BackupPlan{FullEvery: 7}, current, ""); kind != BackupKindIncremental {
		t.Fatalf("the run after a fallback = %s (%s), want an incremental on the new chain", kind, reason)
	}
	third := seedChainRun(t, svc, ct, current, BackupKindIncremental, BackupConsistencyFilesystem, at.Add(2*time.Hour))
	ancestry, err := svc.chainAncestors(ctx, third)
	if err != nil {
		t.Fatal(err)
	}
	if len(ancestry) != 2 || ancestry[0].ID != second.ID || ancestry[1].ID != third.ID {
		t.Fatalf("ancestry = %v, want the fallback full then the incremental", runIDs(ancestry))
	}
}

func TestAFullOnAnInvalidatedChainClearsTheInvalidation(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	chain, err := svc.chainFor(ctx, ct.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.InvalidateChain(ctx, ct.ID, "this container was restored from a backup"); err != nil {
		t.Fatal(err)
	}
	if chain, err = svc.chainFor(ctx, ct.ID, ""); err != nil || !chain.Invalidated {
		t.Fatalf("chain not invalidated: %+v, %v", chain, err)
	}
	full := seedChainRun(t, svc, ct, chain, BackupKindFull, BackupConsistencyFilesystem, time.Now())
	if _, err := svc.recordChainRun(ctx, chain, full.ID, BackupKindFull, false, ""); err != nil {
		t.Fatal(err)
	}
	current, err := svc.chainFor(ctx, ct.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if current.Invalidated {
		t.Fatal("a full copy left the chain invalidated, so every later run is a full copy too")
	}
	if kind, reason := svc.decideRunKind(ctx, BackupPlan{FullEvery: 7}, current, ""); kind != BackupKindIncremental {
		t.Fatalf("the run after a full on a fresh chain = %s (%s)", kind, reason)
	}
}

func runIDs(runs []BackupRun) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.ID)
	}
	return out
}
