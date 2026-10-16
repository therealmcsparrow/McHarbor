// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"testing"
)

// A linked clone's disks read through its source's, so the source cannot be
// destroyed while one exists — whether or not it was marked as a template.
func TestDeletingAGuestWithLinkedClonesIsRefused(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "src", StateStopped)
	insertIntentTestVM(t, db, "clone", StateStopped)
	if _, err := db.Exec(`UPDATE plugin_addon_kvm_vms SET template_source_id = 'src' WHERE id = 'clone'`); err != nil {
		t.Fatal(err)
	}

	err := svc.Delete(context.Background(), "src", DeleteOptions{})
	if !errors.Is(err, ErrTemplateInUse) {
		t.Fatalf("Delete of a linked clone's source = %v, want ErrTemplateInUse", err)
	}
	for _, call := range rpc.calls {
		if call == msgVMDelete {
			t.Fatal("vm_delete reached the host for a source a linked clone still reads")
		}
	}
}
