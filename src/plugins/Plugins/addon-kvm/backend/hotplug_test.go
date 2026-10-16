// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedRPC answers each message type from a script, and records what was
// sent, in order, with the hot-plug action where there is one.
type scriptedRPC struct {
	mu      sync.Mutex
	calls   []string
	hotplug bool
	answer  map[string]func(payload agentPayload) (string, error)
}

func (f *scriptedRPC) Available(context.Context, string) (bool, string) { return true, "" }

func (f *scriptedRPC) Subscribe(func(envID, msgType string, frame []byte)) {}

func (f *scriptedRPC) Call(_ context.Context, _, msgType string, payload []byte, _ func(string, []byte)) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req agentPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
	}
	label := msgType
	if req.Hotplug != nil {
		label += ":" + req.Hotplug.Action
	}
	if msgType == msgVMQEMUCapability {
		if f.hotplug {
			return []byte(`{"qemuCapability":{"features":{"hotplug":true}}}`), nil
		}
		return []byte(`{"qemuCapability":{"features":{}}}`), nil
	}
	f.calls = append(f.calls, label)
	if answer, ok := f.answer[label]; ok {
		reply, err := answer(req)
		if err != nil {
			return nil, err
		}
		return []byte(reply), nil
	}
	if answer, ok := f.answer[msgType]; ok {
		reply, err := answer(req)
		if err != nil {
			return nil, err
		}
		return []byte(reply), nil
	}
	return []byte(`{"success":true}`), nil
}

func (f *scriptedRPC) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func reply(body string) func(agentPayload) (string, error) {
	return func(agentPayload) (string, error) { return body, nil }
}

func failWith(err error) func(agentPayload) (string, error) {
	return func(agentPayload) (string, error) { return "", err }
}

func newHotplugTestService(t *testing.T, state string, hotplug bool) (*Service, *scriptedRPC, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kvm.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	files, err := filepath.Glob(filepath.Join("..", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}

	rpc := &scriptedRPC{hotplug: hotplug, answer: map[string]func(agentPayload) (string, error){
		msgVMDiskCreate: reply(`{"success":true,"disk":{"volumePath":"/pool/vm1-disk.qcow2","sizeBytes":10737418240}}`),
	}}
	svc := NewService(sqliteHandle{db}, rpc, nil, nil, nil, nil, nil, t.TempDir(), quietLogger{})
	ctx := context.Background()
	for _, ensure := range []func(context.Context) error{
		svc.ensureScheduleSchema, svc.ensureImportSchema, svc.ensureNICSchema,
		svc.ensureSnapshotTreeSchema, svc.ensureIncidentSchema, svc.ensureGuestHealthSchema,
		svc.ensureDisplayProfileSchema,
	} {
		if err := ensure(ctx); err != nil {
			t.Fatal(err)
		}
	}
	insertIntentTestVM(t, db, "vm1", state)
	insertTestDisk(t, db, "boot", 0, "/pool/boot.qcow2", true, false, 0)
	insertTestDisk(t, db, "data", 1, "/pool/data.qcow2", false, false, 0)
	return svc, rpc, db
}

func insertTestDisk(t *testing.T, db *sql.DB, id string, slot int, path string, boot, cdrom bool, bootIndex int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO plugin_addon_kvm_disks
			(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
			 format, size_bytes, cache_mode, discard, readonly, is_boot, is_cdrom, boot_index,
			 created_at, updated_at)
		VALUES (?, 'vm1', 'pool', 'dir', '/pool', ?, ?, ?, 'qcow2', 10737418240, 'none', 1, ?, ?, ?, ?, ?, ?)`,
		id, slot, map[bool]string{true: "sata", false: "virtio-blk"}[cdrom], path,
		boolToInt(cdrom), boolToInt(boot), boolToInt(cdrom), nullIfZero(bootIndex), now, now); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func contains(calls []string, want string) bool { return slices.Contains(calls, want) }

func TestAddDiskWhileStoppedIsUnchanged(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateStopped, true)
	if _, err := svc.AddDisk(context.Background(), "vm1", DiskRequest{PoolID: "pool", SizeGB: 10}); err != nil {
		t.Fatal(err)
	}
	if calls := rpc.sent(); !slices.Equal(calls, []string{msgVMDiskCreate}) {
		t.Fatalf("a stopped guest sent %v, want only the volume allocation", calls)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE vm_id = 'vm1'`); n != 3 {
		t.Fatalf("disk rows = %d, want 3", n)
	}
}

func TestAddDiskWhileRunningPlugsItInBeforeRecordingIt(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateRunning, true)
	disk, err := svc.AddDisk(context.Background(), "vm1", DiskRequest{PoolID: "pool", SizeGB: 10, Bus: "virtio-blk"})
	if err != nil {
		t.Fatal(err)
	}
	if calls := rpc.sent(); !slices.Equal(calls, []string{msgVMDiskCreate, msgVMDiskHotplug + ":attach"}) {
		t.Fatalf("sent %v, want allocate then attach", calls)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE id = ?`, disk.ID); n != 1 {
		t.Fatal("the hot-plugged disk was not recorded")
	}
}

func TestAddDiskWhileRunningRollsBackWhenTheAgentFails(t *testing.T) {
	cases := map[string]func(agentPayload) (string, error){
		"agent error":      failWith(errors.New("agent failed vm_disk_hotplug: device_add: boom")),
		"restart required": reply(`{"success":true,"errorCode":"hotplug_restart_required"}`),
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			svc, rpc, db := newHotplugTestService(t, StateRunning, true)
			rpc.answer[msgVMDiskHotplug+":attach"] = answer
			_, err := svc.AddDisk(context.Background(), "vm1", DiskRequest{PoolID: "pool", SizeGB: 10, Bus: "virtio-blk"})
			if err == nil {
				t.Fatal("a refused hot-plug was reported as success")
			}
			if name == "restart required" && !errors.Is(err, ErrHotplugRestart) {
				t.Fatalf("err = %v, want ErrHotplugRestart", err)
			}
			if calls := rpc.sent(); !contains(calls, msgVMDiskDelete) {
				t.Fatalf("the allocated volume was left behind: %v", calls)
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE vm_id = 'vm1'`); n != 2 {
				t.Fatalf("disk rows = %d, want the original 2", n)
			}
		})
	}
}

func TestHardwareChangesWhileRunningNeedTheFeature(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateRunning, false)
	if _, err := svc.AddDisk(context.Background(), "vm1", DiskRequest{PoolID: "pool", SizeGB: 10}); !errors.Is(err, ErrHotplugUnavailable) {
		t.Fatalf("err = %v, want ErrHotplugUnavailable", err)
	}
	if err := svc.DeleteDisk(context.Background(), "vm1", "data"); !errors.Is(err, ErrHotplugUnavailable) {
		t.Fatalf("err = %v, want ErrHotplugUnavailable", err)
	}
	if calls := rpc.sent(); len(calls) != 0 {
		t.Fatalf("a host without hot-plug was sent %v", calls)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE vm_id = 'vm1'`); n != 2 {
		t.Fatalf("disk rows = %d, want 2", n)
	}
}

func TestBootDiskIsRefusedWhileRunning(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateRunning, true)
	// A data disk the operator moved to the front of the boot order is the
	// boot disk too.
	insertTestDisk(t, db, "first", 2, "/pool/first.qcow2", false, false, 1)
	if _, err := db.Exec(`UPDATE plugin_addon_kvm_disks SET is_boot = 0 WHERE id = 'boot'`); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteDisk(context.Background(), "vm1", "first"); !errors.Is(err, ErrBootDiskRunning) {
		t.Fatalf("delete: err = %v, want ErrBootDiskRunning", err)
	}
	if _, err := svc.ResizeDisk(context.Background(), "vm1", "first", 20); !errors.Is(err, ErrBootDiskRunning) {
		t.Fatalf("resize: err = %v, want ErrBootDiskRunning", err)
	}
	if calls := rpc.sent(); len(calls) != 0 {
		t.Fatalf("the boot disk reached the host: %v", calls)
	}

	// The data disk is not the boot disk and changes live.
	if _, err := svc.ResizeDisk(context.Background(), "vm1", "data", 20); err != nil {
		t.Fatalf("resizing a data disk while running: %v", err)
	}
	if calls := rpc.sent(); !slices.Equal(calls, []string{msgVMDiskHotplug + ":resize"}) {
		t.Fatalf("sent %v, want a live resize", calls)
	}
}

func TestBootDiskResizeWhileStoppedIsUnchanged(t *testing.T) {
	svc, rpc, _ := newHotplugTestService(t, StateStopped, true)
	if _, err := svc.ResizeDisk(context.Background(), "vm1", "boot", 20); err != nil {
		t.Fatal(err)
	}
	if calls := rpc.sent(); !slices.Equal(calls, []string{msgVMDiskResize}) {
		t.Fatalf("sent %v, want the stopped-guest resize", calls)
	}
}

func TestDeleteDiskWhileRunning(t *testing.T) {
	t.Run("detaches, removes the volume, then the row", func(t *testing.T) {
		svc, rpc, db := newHotplugTestService(t, StateRunning, true)
		if err := svc.DeleteDisk(context.Background(), "vm1", "data"); err != nil {
			t.Fatal(err)
		}
		if calls := rpc.sent(); !slices.Equal(calls, []string{msgVMDiskHotplug + ":detach", msgVMDiskDelete}) {
			t.Fatalf("sent %v", calls)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE id = 'data'`); n != 0 {
			t.Fatal("the row outlived its disk")
		}
	})
	t.Run("a guest that keeps the disk keeps the row", func(t *testing.T) {
		svc, rpc, db := newHotplugTestService(t, StateRunning, true)
		rpc.answer[msgVMDiskHotplug+":detach"] = reply(`{"success":true,"errorCode":"hotplug_guest_refused"}`)
		if err := svc.DeleteDisk(context.Background(), "vm1", "data"); !errors.Is(err, ErrHotplugRefused) {
			t.Fatalf("err = %v, want ErrHotplugRefused", err)
		}
		if contains(rpc.sent(), msgVMDiskDelete) {
			t.Fatal("the volume of a disk the guest still holds was removed")
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE id = 'data'`); n != 1 {
			t.Fatal("the row went although the guest kept the disk")
		}
	})
	t.Run("a volume that will not go is plugged back in", func(t *testing.T) {
		svc, rpc, db := newHotplugTestService(t, StateRunning, true)
		rpc.answer[msgVMDiskDelete] = failWith(errors.New("agent failed vm_disk_delete: busy"))
		if err := svc.DeleteDisk(context.Background(), "vm1", "data"); err == nil {
			t.Fatal("a failed volume delete was reported as success")
		}
		want := []string{msgVMDiskHotplug + ":detach", msgVMDiskDelete, msgVMDiskHotplug + ":attach"}
		if calls := rpc.sent(); !slices.Equal(calls, want) {
			t.Fatalf("sent %v, want %v", calls, want)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE id = 'data'`); n != 1 {
			t.Fatal("the row went although the volume is still there")
		}
	})
}

func TestAddNICWhileRunningDropsTheRowTheGuestRefused(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateRunning, true)
	rpc.answer[msgVMNICHotplug+":attach"] = reply(`{"success":true,"errorCode":"hotplug_no_free_port"}`)
	if _, err := svc.AddNIC(context.Background(), "vm1", NICRequest{NetworkID: "n1", Bridge: "mch-br0"}); !errors.Is(err, ErrHotplugNoPort) {
		t.Fatalf("err = %v, want ErrHotplugNoPort", err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_nics WHERE vm_id = 'vm1'`); n != 0 {
		t.Fatal("a NIC the guest refused was left recorded")
	}

	delete(rpc.answer, msgVMNICHotplug+":attach")
	nic, err := svc.AddNIC(context.Background(), "vm1", NICRequest{NetworkID: "n1", Bridge: "mch-br0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNIC(context.Background(), "vm1", nic.ID, NICUpdateRequest{LinkUp: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_nics WHERE id = ? AND link_down = 1`, nic.ID); n != 1 {
		t.Fatal("a live link change was not persisted")
	}
	if _, err := svc.UpdateNIC(context.Background(), "vm1", nic.ID, NICUpdateRequest{Model: "e1000e"}); !errors.Is(err, ErrRunning) {
		t.Fatalf("model change while running: err = %v, want ErrRunning", err)
	}
	if err := svc.DeleteNIC(context.Background(), "vm1", nic.ID); err != nil {
		t.Fatal(err)
	}
	want := []string{msgVMNICHotplug + ":attach", msgVMNICHotplug + ":attach", msgVMNICHotplug + ":link", msgVMNICHotplug + ":detach"}
	if calls := rpc.sent(); !slices.Equal(calls, want) {
		t.Fatalf("sent %v, want %v", calls, want)
	}
}

func ptr[T any](v T) *T { return &v }

func TestMediaWhileRunningKeepsTheDrive(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateRunning, true)
	insertTestDisk(t, db, "cd", 3, "/iso/install.iso", false, true, 0)

	if err := svc.EjectMedia(context.Background(), "vm1", "cd"); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE id = 'cd' AND volume_path = ''`); n != 1 {
		t.Fatal("a live eject did not leave an empty drive")
	}
	disk, err := svc.AttachMedia(context.Background(), "vm1", MediaRequest{SourcePath: "/var/lib/mcharbor/compute/images/virtio.iso"})
	if err != nil {
		t.Fatal(err)
	}
	if disk.ID != "cd" {
		t.Fatalf("the disc went into %s, want the emptied drive", disk.ID)
	}
	if calls := rpc.sent(); !slices.Equal(calls, []string{msgVMMediaChange + ":eject", msgVMMediaChange + ":insert"}) {
		t.Fatalf("sent %v", calls)
	}
	if _, err := svc.AttachMedia(context.Background(), "vm1", MediaRequest{SourcePath: "/var/lib/mcharbor/compute/images/b.iso"}); !errors.Is(err, ErrMediaNoDrive) {
		t.Fatalf("a second disc with no empty drive: err = %v, want ErrMediaNoDrive", err)
	}
}

func TestMediaWhileStoppedIsUnchanged(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateStopped, true)
	insertTestDisk(t, db, "cd", 3, "/iso/install.iso", false, true, 0)
	if err := svc.EjectMedia(context.Background(), "vm1", "cd"); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE id = 'cd'`); n != 0 {
		t.Fatal("a stopped eject kept the drive")
	}
	if calls := rpc.sent(); len(calls) != 0 {
		t.Fatalf("a stopped eject reached the host: %v", calls)
	}
}

func TestHotplugRefusalsReachTheClientAsTheirOwnCodes(t *testing.T) {
	h := &Handler{logger: quietLogger{}}
	for err, code := range map[error]string{
		ErrHotplugRestart:     "err.vm.hotplug_restart_required",
		ErrBootDiskRunning:    "err.vm.boot_disk_running",
		ErrMediaNoDrive:       "err.vm.media_no_drive",
		ErrHotplugUnavailable: "err.vm.hotplug_unavailable",
	} {
		rec := httptest.NewRecorder()
		h.respondErr(rec, httptest.NewRequest(http.MethodGet, "/", nil), "test", err)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) {
			t.Errorf("%v answered %d %s, want 409 %s", err, rec.Code, rec.Body.String(), code)
		}
	}
}

func TestIsBootDisk(t *testing.T) {
	disks := []Disk{
		{ID: "a", IsBoot: true},
		{ID: "b", BootIndex: 2},
		{ID: "cd", IsCDROM: true, BootIndex: 1},
		{ID: "c"},
	}
	for id, want := range map[string]bool{"a": true, "b": false, "cd": false, "c": false} {
		for _, d := range disks {
			if d.ID == id && isBootDisk(disks, d) != want {
				t.Errorf("isBootDisk(%s) = %v, want %v", id, !want, want)
			}
		}
	}
	disks[0].IsBoot = false
	if !isBootDisk(disks, disks[1]) {
		t.Error("the first disk in the boot order is not the boot disk")
	}
}

// TestLiveAddSaysWhenTheGuestNeverPickedTheDeviceUp: the attach succeeded, so the disk
// and NIC are kept, but the operator is told the guest will not see them until it restarts.
func TestLiveAddSaysWhenTheGuestNeverPickedTheDeviceUp(t *testing.T) {
	svc, rpc, _ := newHotplugTestService(t, StateRunning, true)
	rpc.answer[msgVMDiskHotplug] = reply(`{"success":true,"notice":"` + agentHotplugUnclaimed + `"}`)
	rpc.answer[msgVMNICHotplug] = reply(`{"success":true,"notice":"` + agentHotplugUnclaimed + `"}`)

	disk, err := svc.AddDisk(context.Background(), "vm1", DiskRequest{PoolID: "pool", SizeGB: 10, Bus: "virtio-blk"})
	if err != nil {
		t.Fatalf("an unclaimed attach is still an attach: %v", err)
	}
	if disk.Notice != agentHotplugUnclaimed {
		t.Fatalf("disk notice = %q, want %q", disk.Notice, agentHotplugUnclaimed)
	}
	nic, err := svc.AddNIC(context.Background(), "vm1", NICRequest{NetworkID: "n1", Bridge: "mch-br0"})
	if err != nil || nic.Notice != agentHotplugUnclaimed {
		t.Fatalf("NIC notice = %q (%v), want %q", nic.Notice, err, agentHotplugUnclaimed)
	}

	rpc.answer[msgVMDiskHotplug] = reply(`{"success":true}`)
	claimed, err := svc.AddDisk(context.Background(), "vm1", DiskRequest{PoolID: "pool", SizeGB: 10, Bus: "virtio-blk"})
	if err != nil || claimed.Notice != "" {
		t.Fatalf("a claimed attach carried notice %q (%v)", claimed.Notice, err)
	}
}
