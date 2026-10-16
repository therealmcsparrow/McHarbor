// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

func TestAStartTheAgentRefusesIsRecordedOnTheRow(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	reason := "lxc: start web: error preparing rootfs: remount-private: permission denied"
	rpc.reply = func(messageType string, _ agentPayload) ([]byte, error) {
		if messageType == msgCTStart {
			return nil, fmt.Errorf("agent failed ct_start: %s", reason)
		}
		return []byte(`{"success":true}`), nil
	}
	if err := svc.Start(ctx, ct.ID); err == nil {
		t.Fatal("Start of a refused container returned no error")
	}
	got, err := svc.Get(ctx, ct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateCrashed || got.LastError != reason {
		t.Fatalf("row = state %q, lastError %q; want crashed with the agent's reason", got.State, got.LastError)
	}
}

func TestAStartTheHostNeverRanLeavesTheRowAlone(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, err := svc.Get(ctx, ct.ID)
	if err != nil {
		t.Fatal(err)
	}

	rpc.reply = func(messageType string, _ agentPayload) ([]byte, error) {
		return nil, errors.New("compute: timeout waiting for ct_start")
	}
	if err := svc.Start(ctx, ct.ID); err == nil {
		t.Fatal("Start returned no error")
	}
	after, err := svc.Get(ctx, ct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != before.State || after.LastError != "" {
		t.Fatalf("a transport failure rewrote the row: %q/%q -> %q/%q",
			before.State, before.LastError, after.State, after.LastError)
	}
}

func TestRestartCarriesTheSpecSoTheAgentCanStartAgain(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Restart(ctx, ct.ID); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	sent, ok := rpc.last(msgCTRestart)
	if !ok {
		t.Fatal("no ct_restart was sent")
	}
	if sent.Spec == nil || sent.Spec.RootfsPath == "" {
		t.Fatalf("ct_restart spec = %+v; without it the agent stops the container and never starts it", sent.Spec)
	}
}

// seedRun records a backup run and its successful destinations the way a real
// run does, and returns the run.
func seedRun(t *testing.T, svc *Service, ct Container, locationID string, keys ...string) BackupRun {
	t.Helper()
	ctx := context.Background()
	run, err := svc.createBackupRun(ctx, ct, BackupPlan{}, BackupKindFull, BackupChain{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		destID := svc.beginDestination(ctx, run.ID, backupDestination{ID: locationID, Name: "Local"}, BackupPartRootfs)
		svc.completeDestination(ctx, destID, key, "", 100)
	}
	svc.finishBackupRun(ctx, run.ID, BackupKindFull, false, "", BackupConsistencyFilesystem, 100)
	return run
}

func runRow(t *testing.T, svc *Service, runID string) (int64, string) {
	t.Helper()
	rows, err := svc.db.Query(context.Background(),
		`SELECT bytes_total, COALESCE(progress_message, '') FROM plugin_addon_lxc_backup_runs WHERE id = ?`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("run %s not found", runID)
	}
	var total int64
	var message string
	if err := rows.Scan(&total, &message); err != nil {
		t.Fatal(err)
	}
	return total, message
}

func TestAFinishedBackupClearsItsLastProgressMessage(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	ctx := context.Background()
	run, err := svc.createBackupRun(ctx, Container{ID: "ct1", Name: "web", EnvironmentID: "env-a"},
		BackupPlan{}, BackupKindFull, BackupChain{})
	if err != nil {
		t.Fatal(err)
	}
	svc.recordBackupProgress(map[string]string{
		"runId": run.ID, "stage": "archiving", "bytes": "524700", "total": "3542177"})
	if _, message := runRow(t, svc, run.ID); !strings.Contains(message, "%") {
		t.Fatalf("progress frame not recorded: %q", message)
	}

	svc.finishBackupRun(ctx, run.ID, BackupKindFull, false, "", BackupConsistencyFilesystem, 3542177)
	total, message := runRow(t, svc, run.ID)
	if message != "" {
		t.Fatalf("a finished backup still says %q", message)
	}
	if total != 3542177 {
		t.Fatalf("bytes_total = %d", total)
	}
}

func TestARestoreRecordsTheBytesItMoved(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()
	target := Container{ID: "ct1", Name: "web", EnvironmentID: "env-a", RootfsPath: "/pool/containers/ct1/rootfs"}
	source := BackupRun{ID: "src1", ContainerID: "ct1", ContainerName: "web", Kind: BackupKindIncremental}
	run, err := svc.createRestoreRun(ctx, target, source)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := []agentRestoreArtifact{
		{Part: BackupPartRootfs, Kind: BackupKindFull, Sequence: 0, SizeBytes: 3542177},
		{Part: BackupPartRootfs, Kind: BackupKindIncremental, Sequence: 1, SizeBytes: 81920},
	}

	sameRange := idMapOf(target)
	svc.executeRestore(ctx, target, run, artifacts, nil, &sameRange, restoreFollowUp{})

	total, _ := runRow(t, svc, run.ID)
	if total != 3542177+81920 {
		t.Fatalf("restore bytes_total = %d, want %d", total, 3542177+81920)
	}
}

func TestDeletingABackupAnswersABackupMessage(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	store := newFakeStorage("local")
	svc.storage = store
	run := seedRun(t, svc, Container{ID: "ct1", Name: "web", EnvironmentID: "env-a"}, "loc-1",
		store.put("env-a/ct/web/run1-rootfs-full.tar.gz"))

	h := NewHandler(svc, nil, nil, nil, quietLogger{})
	req := httptest.NewRequest(http.MethodDelete, "/api/system-containers/backups/"+run.ID, nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("runId", run.ID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.User{ID: "user-1"})
	rec := httptest.NewRecorder()
	h.HandleDeleteBackupRun(rec, req.WithContext(ctx))

	var envelope apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if !envelope.Success || envelope.Code != "msg.ct.backup_deleted" {
		t.Fatalf("envelope = %+v, want msg.ct.backup_deleted", envelope)
	}
}

func TestDeletingTheLastRunRemovesItsEmptyFolders(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	store := newFakeStorage("local")
	svc.storage = store
	ctx := context.Background()
	ct := Container{ID: "ct1", Name: "web", EnvironmentID: "env-a"}
	store.put("/mnt/backup/homelab/ct/db/other-rootfs-full.tar.gz")
	first := seedRun(t, svc, ct, "loc-1",
		store.put("/mnt/backup/homelab/ct/web/run1-manifest-full.json"),
		store.put("/mnt/backup/homelab/ct/web/run1-rootfs-full.tar.gz"))
	last := seedRun(t, svc, ct, "loc-1",
		store.put("/mnt/backup/homelab/ct/web/run2-rootfs-full.tar.gz"))

	if err := svc.DeleteBackupRun(ctx, first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if !store.exists("/mnt/backup/homelab/ct/web") {
		t.Fatal("a folder that still holds a run was removed")
	}

	if err := svc.DeleteBackupRun(ctx, last.ID); err != nil {
		t.Fatalf("delete last: %v", err)
	}
	if store.exists("/mnt/backup/homelab/ct/web") {
		t.Fatal("the last run's empty <env>/ct/<name>/ folder survived")
	}
	if !store.exists("/mnt/backup/homelab/ct/db/other-rootfs-full.tar.gz") {
		t.Fatal("another container's backup was touched")
	}
	for _, key := range store.deletedKeys() {
		if key == "/mnt/backup" || key == "/mnt" || key == "/" {
			t.Fatalf("the cleanup reached above the backup layout: %q", key)
		}
	}
}

func TestFoldersOnARecursiveDeleteAdapterAreLeftAlone(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	store := newFakeStorage("nextcloud")
	svc.storage = store
	run := seedRun(t, svc, Container{ID: "ct1", Name: "web", EnvironmentID: "env-a"}, "loc-1",
		store.put("env-a/ct/web/run1-rootfs-full.tar.gz"))

	if err := svc.DeleteBackupRun(context.Background(), run.ID); err != nil {
		t.Fatalf("DeleteBackupRun: %v", err)
	}
	for _, key := range store.deletedKeys() {
		if !strings.HasSuffix(key, ".tar.gz") {
			t.Fatalf("a folder %q was deleted through an adapter whose delete is recursive", key)
		}
	}
}

func TestEmptyFolderCandidatesOnlyClimbTheBackupLayout(t *testing.T) {
	cases := []struct {
		remote string
		want   []string
	}{
		{"/mnt/backup/homelab/ct/web/run-rootfs-full.tar.gz",
			[]string{"/mnt/backup/homelab/ct/web", "/mnt/backup/homelab/ct", "/mnt/backup/homelab"}},
		{"homelab/ct/web/run-rootfs-full.tar.gz", []string{"homelab/ct/web", "homelab/ct", "homelab"}},
		{"ct/web/run.tar.gz", nil},
		{"/mnt/backup/homelab/vm/web/run.tar.gz", nil},
		{"/mnt/backup/../ct/web/run.tar.gz", nil},
	}
	for _, c := range cases {
		got := emptyFolderCandidates(c.remote)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q: got %v, want %v", c.remote, got, c.want)
		}
	}
}

// fakeStorage is a destination with rmdir semantics: deleting a folder that
// still holds an object fails, deleting a missing key succeeds.
type fakeStorage struct {
	locationType string

	mu      sync.Mutex
	objects map[string]bool
	dirs    map[string]bool
	deleted []string
}

func newFakeStorage(locationType string) *fakeStorage {
	return &fakeStorage{locationType: locationType, objects: map[string]bool{}, dirs: map[string]bool{}}
}

func (f *fakeStorage) put(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = true
	for dir := path.Dir(key); dir != "." && dir != "/"; dir = path.Dir(dir) {
		f.dirs[dir] = true
	}
	return key
}

func (f *fakeStorage) exists(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.objects[key] || f.dirs[key]
}

func (f *fakeStorage) deletedKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

func (f *fakeStorage) Upload(context.Context, string, string, []byte) error { return nil }

func (f *fakeStorage) Locations() []plugin.StorageLocationRef {
	return []plugin.StorageLocationRef{{ID: "loc-1", Name: "Local", LocationType: f.locationType}}
}

func (f *fakeStorage) Available() bool { return true }

func (f *fakeStorage) UploadStream(context.Context, plugin.StorageStreamRequest) (plugin.StorageStreamResult, error) {
	return plugin.StorageStreamResult{}, errors.New("not used")
}

func (f *fakeStorage) DownloadStream(context.Context, plugin.StorageDownloadRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}

func (f *fakeStorage) DeleteObject(_ context.Context, _ string, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, key)
	if f.objects[key] {
		delete(f.objects, key)
		return nil
	}
	if !f.dirs[key] {
		return nil
	}
	prefix := key + "/"
	for other := range f.objects {
		if strings.HasPrefix(other, prefix) {
			return fmt.Errorf("remove %s: directory not empty", key)
		}
	}
	for other := range f.dirs {
		if strings.HasPrefix(other, prefix) {
			return fmt.Errorf("remove %s: directory not empty", key)
		}
	}
	delete(f.dirs, key)
	return nil
}

func TestDeletingASnapshotAnswersASnapshotMessage(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_snapshots
		    (id, container_id, name, mechanism, handle, created_at, updated_at)
		VALUES ('snap1', ?, 'before-upgrade', 'zfs', 'pool/ct@before-upgrade', 'now', 'now')`,
		ct.ID); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(svc, nil, nil, nil, quietLogger{})
	req := httptest.NewRequest(http.MethodDelete, "/api/system-containers/"+ct.ID+"/snapshots/snap1", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", ct.ID)
	routeCtx.URLParams.Add("snapId", "snap1")
	reqCtx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	reqCtx = context.WithValue(reqCtx, auth.UserContextKey, &auth.User{ID: "user-1"})
	rec := httptest.NewRecorder()
	h.HandleDeleteSnapshot(rec, req.WithContext(reqCtx))

	var envelope apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if !envelope.Success || envelope.Code != "msg.ct.snapshot_deleted" {
		t.Fatalf("envelope = %+v, want msg.ct.snapshot_deleted", envelope)
	}
	if _, sent := rpc.last(msgCTSnapshotDelete); !sent {
		t.Fatal("the agent was never asked to delete the snapshot")
	}
}
