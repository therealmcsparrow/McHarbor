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
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const floorRefusal = "compute: unsupported: %s needs agent 2026.09.01 build 202609271429 or newer"

func createRequest() CreateRequest {
	return CreateRequest{
		EnvironmentID: "env-a",
		Name:          "web",
		ImageID:       "img-alpine-321-rootfs-amd64",
		PoolID:        "pool-a",
		PoolBackend:   "dir",
		PoolTarget:    "/mcharbor/compute",
		InitSystem:    InitOpenRC,
		NICs:          []CreateNICRequest{{NetworkID: "net-a", Bridge: "br0"}},
	}
}

func TestCreateFromImageSendsTheImageAndThePool(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)

	ct, err := svc.Create(context.Background(), createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sent, ok := rpc.last(msgCTCreate)
	if !ok {
		t.Fatal("no ct_create was sent")
	}
	if sent.ImageID != "img-alpine-321-rootfs-amd64" {
		t.Fatalf("ct_create image = %q", sent.ImageID)
	}
	if sent.Spec == nil || sent.Spec.PoolTarget != "/mcharbor/compute" ||
		sent.Spec.RootfsPath != "/mcharbor/compute/containers/"+ct.ID+"/rootfs" {
		t.Fatalf("ct_create spec = %+v, want the pool target and a rootfs inside it", sent.Spec)
	}
	if sent.Spec.TemplatePath != "" {
		t.Fatalf("an image create also named template %q", sent.Spec.TemplatePath)
	}
}

func TestCreateFromAnUncachedImageIsRefusedBeforeTheRowIsWritten(t *testing.T) {
	rpc := &fakeRPC{canRun: true, cached: []string{"img-debian-12-rootfs-amd64"}}
	svc, db := newTestService(t, rpc)

	_, err := svc.Create(context.Background(), createRequest())
	if !errors.Is(err, ErrImageNotCached) {
		t.Fatalf("Create = %v, want ErrImageNotCached", err)
	}
	if !strings.Contains(err.Error(), "img-alpine-321-rootfs-amd64") {
		t.Fatalf("the refusal does not name the image to download: %q", err.Error())
	}
	if _, sent := rpc.last(msgCTCreate); sent {
		t.Fatal("ct_create reached the host for an image it does not have")
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plugin_addon_lxc_containers`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("%d container row(s) left behind by a refused create", rows)
	}
}

func TestUncachedImageRefusalReachesTheClientWithItsCode(t *testing.T) {
	rec := httptest.NewRecorder()
	h := &Handler{logger: quietLogger{}}
	h.respondErr(rec, httptest.NewRequest(http.MethodGet, "/", nil), "create", fmt.Errorf("%w: %s", ErrImageNotCached, "img-alpine-321-rootfs-amd64"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, codeImageNotCached) || !strings.Contains(body, "img-alpine-321-rootfs-amd64") {
		t.Fatalf("body = %s, want the image_not_cached code and the image id", body)
	}
}

func TestDeleteSendsTheFullSpec(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()

	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, ct.ID, DeleteRequest{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	sent, ok := rpc.last(msgCTDelete)
	if !ok {
		t.Fatal("no ct_delete was sent")
	}
	if !sent.DeleteRootfs {
		t.Fatal("ct_delete did not ask for the rootfs to go")
	}
	if sent.Spec == nil {
		t.Fatal("ct_delete carried no spec, so the agent cannot find the rootfs or the NICs")
	}
	if sent.Spec.RootfsPath != ct.RootfsPath || sent.Spec.PoolTarget != "/mcharbor/compute" ||
		sent.Spec.PoolBackend != "dir" {
		t.Fatalf("ct_delete spec = %+v", sent.Spec)
	}
	if len(sent.Spec.NICs) != 1 || sent.Spec.NICs[0].VethHost == "" {
		t.Fatalf("ct_delete NICs = %+v, want the host end by name", sent.Spec.NICs)
	}
	if _, err := svc.Get(ctx, ct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the row survived a successful delete: %v", err)
	}
}

func TestAgentFloorRefusals(t *testing.T) {
	ctx := context.Background()

	t.Run("create leaves no row", func(t *testing.T) {
		rpc := &fakeRPC{canRun: true, reply: func(messageType string, _ agentPayload) ([]byte, error) {
			if messageType == msgCTCreate {
				return nil, errors.New(strings.Replace(floorRefusal, "%s", messageType, 1))
			}
			return []byte(`{"success":true}`), nil
		}}
		svc, _ := newTestService(t, rpc)
		if _, err := svc.Create(ctx, createRequest()); !errors.Is(err, ErrAgentTooOldRootfs) {
			t.Fatalf("Create = %v, want ErrAgentTooOldRootfs", err)
		}
		list, err := svc.List(ctx, "", false)
		if err != nil || len(list) != 0 {
			t.Fatalf("a refused create left %d rows (%v)", len(list), err)
		}
	})

	t.Run("delete keeps the row", func(t *testing.T) {
		rpc := &fakeRPC{canRun: true}
		svc, _ := newTestService(t, rpc)
		ct, err := svc.Create(ctx, createRequest())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		rpc.reply = func(messageType string, _ agentPayload) ([]byte, error) {
			return nil, errors.New(strings.Replace(floorRefusal, "%s", messageType, 1))
		}
		if err := svc.Delete(ctx, ct.ID, DeleteRequest{}); !errors.Is(err, ErrAgentTooOldRootfs) {
			t.Fatalf("Delete = %v, want ErrAgentTooOldRootfs", err)
		}
		if _, err := svc.Get(ctx, ct.ID); err != nil {
			t.Fatalf("a refused delete removed the row: %v", err)
		}
	})

	t.Run("backups keep their own error", func(t *testing.T) {
		if !errors.Is(floorError(msgCTBackupRun), ErrAgentTooOld) || !errors.Is(floorError(msgCTDelete), ErrAgentTooOldRootfs) {
			t.Fatal("floorError maps the wrong message types")
		}
	})
}

func TestEmptyContainerListIsAnArray(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	handler := &Handler{svc: svc, logger: quietLogger{}}

	recorder := httptest.NewRecorder()
	handler.HandleList(recorder, httptest.NewRequest(http.MethodGet, "/api/system-containers", nil))

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", recorder.Body.String(), err)
	}
	if string(body.Data) != "[]" {
		t.Fatalf("empty list data = %s, want []", body.Data)
	}
}

type keylessStorage struct {
	plugin.StorageHandle
}

func (keylessStorage) Locations() []plugin.StorageLocationRef {
	return []plugin.StorageLocationRef{{ID: "default-local-backup", Name: "Default local backup"}}
}

func (keylessStorage) UploadStream(_ context.Context, req plugin.StorageStreamRequest) (plugin.StorageStreamResult, error) {
	_, _ = io.Copy(io.Discard, req.Body)
	return plugin.StorageStreamResult{}, errors.New("storage stream: no backup encryption key is installed on this host")
}

func (keylessStorage) DownloadStream(context.Context, plugin.StorageDownloadRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}

func (keylessStorage) DeleteObject(context.Context, string, string) error { return nil }

func TestBackupRunErrorCarriesTheDestinationReason(t *testing.T) {
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
		t.Fatalf("resolveDestinations: %v", err)
	}
	chain, err := svc.chainFor(ctx, ct.ID, "")
	if err != nil {
		t.Fatalf("chainFor: %v", err)
	}
	run, err := svc.createBackupRun(ctx, ct, plan, BackupKindFull, chain)
	if err != nil {
		t.Fatalf("createBackupRun: %v", err)
	}

	rpc.reply = func(messageType string, payload agentPayload) ([]byte, error) {
		if messageType != msgCTBackupRun {
			return []byte(`{"success":true}`), nil
		}
		target, live := svc.backupUploadFor(payload.Backup.RunID)
		if !live {
			t.Error("the run registered no upload target")
		} else {
			_ = svc.fanOutBackup(ctx, target, "manifest", BackupKindFull, strings.NewReader("{}"), 2)
		}
		return []byte(`{"success":false,"error":"compute: McHarbor refused the backup upload with 502 Bad Gateway"}`), nil
	}
	svc.executeBackupRun(ctx, ct, plan, run, chain, destinations)

	stored, err := svc.BackupRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("BackupRunByID: %v", err)
	}
	if stored.Status != "failure" {
		t.Fatalf("run status = %q", stored.Status)
	}
	if !strings.Contains(stored.Error, "no backup encryption key is installed") ||
		!strings.Contains(stored.Error, "Default local backup") {
		t.Fatalf("run error = %q, want the destination's reason", stored.Error)
	}
	if stored.ErrorHint != errorHintBackupKeyMissing {
		t.Fatalf("run errorHint = %q, want %q", stored.ErrorHint, errorHintBackupKeyMissing)
	}
}
