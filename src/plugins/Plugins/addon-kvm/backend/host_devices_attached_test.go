// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func insertTestDevice(t *testing.T, db *sql.DB, id, vmID, kind, address string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO plugin_addon_kvm_devices (id, vm_id, kind, host_address, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, vmID, kind, address, now, now); err != nil {
		t.Fatal(err)
	}
}

func TestHostDevicesNamesTheGuestsThatHaveThem(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", "running")
	insertIntentTestVM(t, db, "vm2", "stopped")
	insertIntentTestVM(t, db, "vm3", "stopped")
	if _, err := db.Exec(`UPDATE plugin_addon_kvm_vms SET environment_id = 'env2' WHERE id = 'vm3'`); err != nil {
		t.Fatal(err)
	}
	insertTestDevice(t, db, "d1", "vm1", "usb", "1-2")
	insertTestDevice(t, db, "d2", "vm2", "usb", "1-2")
	insertTestDevice(t, db, "d3", "vm3", "pci", "0000:01:00.0")
	insertTestDevice(t, db, "d4", "vm1", "pci", "1-2")
	rpc.reply = []byte(`{"hostDevices":[` +
		`{"kind":"usb","address":"1-2","assignable":true},` +
		`{"kind":"pci","address":"0000:01:00.0","assignable":true}]}`)

	devices, err := svc.HostDevices(context.Background(), "env1")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices", len(devices))
	}
	usb := devices[0].AttachedTo
	if len(usb) != 2 || usb[0].VMID != "vm1" || usb[0].Name != "guest-vm1" || usb[1].VMID != "vm2" {
		t.Fatalf("usb 1-2 attachedTo = %+v, want vm1 and vm2", usb)
	}
	if pci := devices[1].AttachedTo; len(pci) != 0 {
		t.Fatalf("pci on another environment leaked in: %+v", pci)
	}
}
