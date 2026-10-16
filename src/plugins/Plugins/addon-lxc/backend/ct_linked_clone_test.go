// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"testing"
)

// A linked clone reads through its source's rootfs, so the source must outlive
// it — whether or not anyone marked the source as a template.
func TestDeletingASourceWithLinkedClonesIsRefused(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, db := newTestService(t, rpc)
	ctx := context.Background()

	source, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}
	cloneReq := createRequest()
	cloneReq.Name = "web-clone"
	clone, err := svc.Create(ctx, cloneReq)
	if err != nil {
		t.Fatalf("Create clone: %v", err)
	}
	if _, err := db.Exec(`UPDATE plugin_addon_lxc_containers SET template_source_id = ?, template_path = ? WHERE id = ?`,
		source.ID, source.RootfsPath, clone.ID); err != nil {
		t.Fatal(err)
	}

	err = svc.Delete(ctx, source.ID, DeleteRequest{})
	if !errors.Is(err, ErrTemplateInUse) {
		t.Fatalf("Delete of a linked clone's source = %v, want ErrTemplateInUse", err)
	}
	if _, sent := rpc.last(msgCTDelete); sent {
		t.Fatal("ct_delete reached the host for a source a linked clone still reads")
	}

	// Once the clone is gone, so is the reason to refuse.
	if err := svc.Delete(ctx, clone.ID, DeleteRequest{}); err != nil {
		t.Fatalf("Delete clone: %v", err)
	}
	if err := svc.Delete(ctx, source.ID, DeleteRequest{}); err != nil {
		t.Fatalf("Delete source after its clone: %v", err)
	}
}

// A clone the host reports as a full copy is independent, so it never makes
// its source undeletable, whatever the request asked for.
func TestACloneIsRecordedAsLinkedOnlyWhenTheHostSaysSo(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()

	source, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}
	clone, err := svc.Clone(ctx, source.ID, CloneRequest{Name: "web-copy", Linked: true})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	sent, ok := rpc.last(msgCTCloneRequest)
	if !ok || sent.Spec == nil || sent.Spec.RootfsPath != source.RootfsPath {
		t.Fatalf("ct_clone_request did not carry the source's spec: %+v", sent.Spec)
	}
	if clone.TemplatePath != "" {
		t.Fatalf("a clone the host did not link was recorded as linked to %q", clone.TemplatePath)
	}
	if err := svc.Delete(ctx, source.ID, DeleteRequest{}); err != nil {
		t.Fatalf("Delete of a full clone's source: %v", err)
	}
}
