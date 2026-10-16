// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
)

// fakeQMP is a guest's device model as QMP shows it. ports maps a root port to
// the device behind it ("" when empty); rootDevices sit on pcie.0.
type fakeQMP struct {
	// mu lets a test change the guest's layout while a claim check is reading it.
	mu          sync.Mutex
	ports       map[string]string
	rootDevices map[string]bool
	others      map[string]bool
	media       map[string]string
	fail        map[string]error
	// keepOnDelete makes device_del a request the guest ignores.
	keepOnDelete bool
	// regions gives a device the BARs query-pci reports for it; none means no regions.
	regions map[string][]map[string]interface{}
	calls   []string
}

func newFakeQMP(withPorts bool) *fakeQMP {
	f := &fakeQMP{ports: map[string]string{}, rootDevices: map[string]bool{"disk0": true},
		others: map[string]bool{}, media: map[string]string{}, fail: map[string]error{}}
	if withPorts {
		for i := 0; i < NICPorts; i++ {
			f.ports[NICPortID(i)] = ""
		}
		for i := 0; i < DiskPorts; i++ {
			f.ports[DiskPortID(i)] = ""
		}
	}
	return f
}

func (f *fakeQMP) has(id string) bool {
	if f.rootDevices[id] || f.others[id] {
		return true
	}
	if _, ok := f.ports[id]; ok {
		return true
	}
	for _, child := range f.ports {
		if child == id {
			return true
		}
	}
	return false
}

func (f *fakeQMP) Run(_ context.Context, execute string, args interface{}) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, _ := args.(map[string]interface{})
	label := execute
	if id, ok := a["id"].(string); ok {
		label += " " + id
	} else if node, ok := a["node-name"].(string); ok {
		label += " " + node
	}
	if execute != "qom-list" && execute != "query-pci" && execute != "query-block" {
		f.calls = append(f.calls, label)
	}
	if err := f.fail[execute]; err != nil {
		return nil, err
	}
	switch execute {
	case "qom-list":
		var out []map[string]string
		add := func(name string) { out = append(out, map[string]string{"name": name, "type": "child<x>"}) }
		for id := range f.rootDevices {
			add(id)
		}
		for id := range f.others {
			add(id)
		}
		for port, child := range f.ports {
			add(port)
			if child != "" {
				add(child)
			}
		}
		return json.Marshal(out)
	case "query-pci":
		var devices []map[string]interface{}
		for id := range f.rootDevices {
			devices = append(devices, map[string]interface{}{"qdev_id": id})
		}
		for port, child := range f.ports {
			var children []map[string]interface{}
			if child != "" {
				entry := map[string]interface{}{"qdev_id": child}
				if regions, ok := f.regions[child]; ok {
					entry["regions"] = regions
				}
				children = append(children, entry)
			}
			devices = append(devices, map[string]interface{}{"qdev_id": port,
				"pci_bridge": map[string]interface{}{"devices": children}})
		}
		return json.Marshal([]map[string]interface{}{{"bus": 0, "devices": devices}})
	case "query-block":
		var out []map[string]interface{}
		for id, file := range f.media {
			entry := map[string]interface{}{"qdev": id}
			if file != "" {
				entry["inserted"] = map[string]string{"file": file}
			}
			out = append(out, entry)
		}
		return json.Marshal(out)
	case "device_add":
		id, _ := a["id"].(string)
		if bus, ok := a["bus"].(string); ok {
			if _, isPort := f.ports[bus]; isPort {
				f.ports[bus] = id
				return nil, nil
			}
		}
		f.others[id] = true
	case "device_del":
		id, _ := a["id"].(string)
		if !f.has(id) {
			return nil, &qmp.Error{Class: "DeviceNotFound", Desc: "Device '" + id + "' not found"}
		}
		if f.keepOnDelete {
			return nil, nil
		}
		for port, child := range f.ports {
			if child == id {
				f.ports[port] = ""
			}
		}
		delete(f.others, id)
		delete(f.rootDevices, id)
	case "blockdev-change-medium":
		f.media[a["id"].(string)] = a["filename"].(string)
	case "eject":
		f.media[a["id"].(string)] = ""
	}
	return nil, nil
}

func newHotplug(f *fakeQMP) *Hotplug {
	return &Hotplug{Mon: f, MachineType: "pc-q35-8.2", ReleaseWait: 200 * time.Millisecond, Poll: 10 * time.Millisecond}
}

func refusalCode(err error) string {
	var refusal *HotplugError
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return ""
}

func TestAttachDiskSendsBlockdevThenDeviceOnItsOwnPort(t *testing.T) {
	f := newFakeQMP(true)
	disk := Disk{Slot: 3, Path: "/pool/vm-3.qcow2", Format: "qcow2", Discard: true}
	if err := newHotplug(f).AttachDisk(context.Background(), disk); err != nil {
		t.Fatal(err)
	}
	want := []string{"blockdev-del blk3", "blockdev-del blk3-file", "blockdev-add blk3-file", "blockdev-add blk3", "device_add disk3"}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("commands = %v, want %v", f.calls, want)
	}
	if f.ports[DiskPortID(3)] != "disk3" {
		t.Fatalf("disk3 landed behind %v, want %s", f.ports, DiskPortID(3))
	}

	// Attaching what is already attached sends nothing: a retry after a lost
	// reply is safe.
	f.calls = nil
	if err := newHotplug(f).AttachDisk(context.Background(), disk); err != nil || len(f.calls) != 0 {
		t.Fatalf("second attach: err %v, commands %v", err, f.calls)
	}
}

func TestAttachDiskRollsBackTheNodesWhenDeviceAddFails(t *testing.T) {
	f := newFakeQMP(true)
	f.fail["device_add"] = &qmp.Error{Class: "GenericError", Desc: "boom"}
	err := newHotplug(f).AttachDisk(context.Background(), Disk{Slot: 2, Path: "/pool/vm-2.qcow2"})
	if err == nil {
		t.Fatal("a failed device_add was reported as success")
	}
	tail := f.calls[len(f.calls)-3:]
	want := []string{"device_add disk2", "blockdev-del blk2", "blockdev-del blk2-file"}
	if !slices.Equal(tail, want) {
		t.Fatalf("rollback = %v, want %v", tail, want)
	}
}

func TestAttachNICRollsBackTheNetdevWhenDeviceAddFails(t *testing.T) {
	f := newFakeQMP(true)
	f.fail["device_add"] = &qmp.Error{Class: "GenericError", Desc: "boom"}
	err := newHotplug(f).AttachNIC(context.Background(), NIC{Slot: 1, MAC: "52:54:00:00:00:01"},
		map[string]interface{}{"type": "tap", "ifname": "tap1"})
	if err == nil {
		t.Fatal("a failed device_add was reported as success")
	}
	want := []string{"netdev_del net1", "netdev_add net1", "device_add nic1", "netdev_del net1"}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("commands = %v, want %v", f.calls, want)
	}
}

func TestDetachDiskWaitsThenClosesTheNodes(t *testing.T) {
	f := newFakeQMP(true)
	f.ports[DiskPortID(4)] = "disk4"
	if err := newHotplug(f).DetachDisk(context.Background(), Disk{Slot: 4}); err != nil {
		t.Fatal(err)
	}
	want := []string{"device_del disk4", "blockdev-del blk4", "blockdev-del blk4-file"}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("commands = %v, want %v", f.calls, want)
	}
}

func TestDetachDiskReturnsOnDeviceDeletedEvent(t *testing.T) {
	f := newFakeQMP(true)
	f.ports[DiskPortID(4)] = "disk4"
	f.keepOnDelete = true
	h := newHotplug(f)
	h.ReleaseWait = 5 * time.Second
	h.Poll = time.Hour
	h.Deleted = func(id string) (<-chan struct{}, func()) {
		ch := make(chan struct{})
		if id == "disk4" {
			close(ch)
		}
		return ch, func() {}
	}
	if err := h.DetachDisk(context.Background(), Disk{Slot: 4}); err != nil {
		t.Fatalf("DEVICE_DELETED did not end the wait: %v", err)
	}
}

func TestDetachDiskKeepsTheNodesWhenTheGuestRefuses(t *testing.T) {
	f := newFakeQMP(true)
	f.ports[DiskPortID(4)] = "disk4"
	f.keepOnDelete = true
	err := newHotplug(f).DetachDisk(context.Background(), Disk{Slot: 4})
	if refusalCode(err) != HotplugGuestRefused {
		t.Fatalf("err = %v, want %s", err, HotplugGuestRefused)
	}
	for _, call := range f.calls {
		if strings.HasPrefix(call, "blockdev-del") {
			t.Fatalf("closed a node the guest still uses: %v", f.calls)
		}
	}
}

func TestBootDiskIsRefusedBeforeAnyCommand(t *testing.T) {
	for name, op := range map[string]func(*Hotplug) error{
		"detach": func(h *Hotplug) error { return h.DetachDisk(context.Background(), Disk{Slot: 0, IsBoot: true}) },
		"resize": func(h *Hotplug) error {
			return h.ResizeDisk(context.Background(), Disk{Slot: 0, IsBoot: true}, 1<<34)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeQMP(true)
			if code := refusalCode(op(newHotplug(f))); code != HotplugBootDisk {
				t.Fatalf("code = %q, want %s", code, HotplugBootDisk)
			}
			if len(f.calls) != 0 {
				t.Fatalf("commands reached QEMU for the boot disk: %v", f.calls)
			}
		})
	}
}

func TestAGuestWithoutPortsNeedsARestart(t *testing.T) {
	cases := map[string]func(*Hotplug, *fakeQMP) error{
		"disk attach": func(h *Hotplug, _ *fakeQMP) error {
			return h.AttachDisk(context.Background(), Disk{Slot: 2, Path: "/pool/a.qcow2"})
		},
		"disk detach": func(h *Hotplug, f *fakeQMP) error {
			f.rootDevices["disk2"] = true
			return h.DetachDisk(context.Background(), Disk{Slot: 2})
		},
		"nic attach": func(h *Hotplug, _ *fakeQMP) error {
			return h.AttachNIC(context.Background(), NIC{Slot: 0, MAC: "52:54:00:00:00:02"}, nil)
		},
		"nic detach": func(h *Hotplug, f *fakeQMP) error {
			f.rootDevices["nic0"] = true
			return h.DetachNIC(context.Background(), NIC{Slot: 0})
		},
	}
	for name, op := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeQMP(false)
			if code := refusalCode(op(newHotplug(f), f)); code != HotplugRestartRequired {
				t.Fatalf("code = %q, want %s", code, HotplugRestartRequired)
			}
			for _, call := range f.calls {
				if strings.HasPrefix(call, "device_") || strings.HasPrefix(call, "blockdev-add") {
					t.Fatalf("a guest with no ports was sent %v", f.calls)
				}
			}
		})
	}
}

func TestAPinnedNICIsFixed(t *testing.T) {
	f := newFakeQMP(true)
	f.rootDevices["nic0"] = true
	err := newHotplug(f).DetachNIC(context.Background(), NIC{Slot: 0, PCIAddr: "0x04"})
	if refusalCode(err) != HotplugFixedDevice {
		t.Fatalf("err = %v, want %s", err, HotplugFixedDevice)
	}
}

func TestSATAAndNewDrivesAreNeverPluggedLive(t *testing.T) {
	f := newFakeQMP(true)
	h := newHotplug(f)
	if code := refusalCode(h.AttachDisk(context.Background(), Disk{Slot: 5, Bus: "sata", Path: "/p/a"})); code != HotplugBusUnsupported {
		t.Fatalf("sata attach code = %q", code)
	}
	if code := refusalCode(h.AttachDisk(context.Background(), Disk{Slot: 5, IsCDROM: true, Path: "/p/a.iso"})); code != HotplugBusUnsupported {
		t.Fatalf("drive attach code = %q", code)
	}
	if code := refusalCode(h.InsertMedia(context.Background(), Disk{Slot: 7, Path: "/p/a.iso"})); code != HotplugNoDrive {
		t.Fatalf("insert without a drive code = %q", code)
	}
}

func TestMediaChangeAndEjectAreIdempotent(t *testing.T) {
	f := newFakeQMP(true)
	f.others["disk6"] = true
	f.media["disk6"] = "/iso/old.iso"
	h := newHotplug(f)
	if err := h.InsertMedia(context.Background(), Disk{Slot: 6, Path: "/iso/new.iso"}); err != nil {
		t.Fatal(err)
	}
	if f.media["disk6"] != "/iso/new.iso" || f.calls[0] != "blockdev-change-medium disk6" {
		t.Fatalf("media %v after %v", f.media, f.calls)
	}
	f.calls = nil
	if err := h.InsertMedia(context.Background(), Disk{Slot: 6, Path: "/iso/new.iso"}); err != nil || len(f.calls) != 0 {
		t.Fatalf("re-insert: err %v, commands %v", err, f.calls)
	}
	if err := h.EjectMedia(context.Background(), Disk{Slot: 6}); err != nil {
		t.Fatal(err)
	}
	if f.media["disk6"] != "" || f.calls[0] != "eject disk6" {
		t.Fatalf("media %v after %v", f.media, f.calls)
	}
	f.calls = nil
	if err := h.EjectMedia(context.Background(), Disk{Slot: 6}); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.calls {
		if strings.HasPrefix(call, "eject") {
			t.Fatalf("an empty drive was ejected again: %v", f.calls)
		}
	}
}

func TestSetLinkAndDetachNIC(t *testing.T) {
	f := newFakeQMP(true)
	f.ports[NICPortID(2)] = "nic2"
	h := newHotplug(f)
	if err := h.SetLink(context.Background(), NIC{Slot: 2}, false); err != nil {
		t.Fatal(err)
	}
	if err := h.DetachNIC(context.Background(), NIC{Slot: 2}); err != nil {
		t.Fatal(err)
	}
	want := []string{"set_link", "device_del nic2", "netdev_del net2"}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("commands = %v, want %v", f.calls, want)
	}
	if err := h.SetLink(context.Background(), NIC{Slot: 2}, true); err == nil {
		t.Fatal("set_link on a NIC that is gone was reported as success")
	}
}

func ExampleNICPortID() {
	fmt.Println(NICPortID(3), DiskPortID(12))
	// Output: rpn3 rpd12
}

// A guest that never releases the device must be refused by code before the
// caller's budget runs out; a bare context error reached McHarbor as a host
// failure and named nothing the operator could act on.
func TestAnUnreleasedDeviceIsRefusedInsideTheCallersBudget(t *testing.T) {
	f := newFakeQMP(true)
	f.ports[NICPortID(1)] = "nic1"
	f.keepOnDelete = true
	h := newHotplug(f)
	h.ReleaseWait = 0
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := h.DetachNIC(ctx, NIC{Slot: 1})
	if refusalCode(err) != HotplugGuestRefused {
		t.Fatalf("err = %v, want %s", err, HotplugGuestRefused)
	}
	if ctx.Err() != nil {
		t.Fatal("the refusal arrived only after the caller's deadline")
	}
	if !strings.Contains(err.Error(), "hot-plug driver") {
		t.Fatalf("the refusal does not say why a guest declines: %v", err)
	}
}

func TestReleaseWaitIsLongButLeavesRoomForTheCleanup(t *testing.T) {
	if got := releaseWait(context.Background(), 0); got != defaultReleaseWait || got < time.Minute {
		t.Fatalf("unbounded wait = %s, want %s and at least a minute", got, defaultReleaseWait)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if got := releaseWait(ctx, 0); got > 50*time.Second || got < 45*time.Second {
		t.Fatalf("wait inside a 60s budget = %s, want just under 50s", got)
	}
	if got := releaseWait(ctx, 5*time.Second); got != 5*time.Second {
		t.Fatalf("a configured wait = %s, want 5s", got)
	}
	short, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if got := releaseWait(short, 0); got != minReleaseWait {
		t.Fatalf("an exhausted budget waits %s, want %s", got, minReleaseWait)
	}
}

func memoryBAR(address int64) map[string]interface{} {
	return map[string]interface{}{"bar": 4, "type": "memory", "address": address, "size": 16384}
}

// TestClaimedReadsWhetherTheGuestAssignedTheDevicesBARs: a guest with hot-plug support
// gives a new device an address; one without leaves every memory BAR unmapped, which is
// what the operator has to be told because the API call itself succeeded.
func TestClaimedReadsWhetherTheGuestAssignedTheDevicesBARs(t *testing.T) {
	ctx := context.Background()
	f := newFakeQMP(true)
	f.ports[DiskPortID(3)] = "disk3"
	f.ports[NICPortID(1)] = "nic1"
	f.regions = map[string][]map[string]interface{}{
		"disk3": {{"bar": 0, "type": "io", "address": -1, "size": 64}, memoryBAR(-1), memoryBAR(0xfe000000)},
		"nic1":  {memoryBAR(-1), memoryBAR(-1)},
	}

	if claimed, err := newHotplug(f).DiskClaimed(ctx, 3, 0); err != nil || !claimed {
		t.Fatalf("disk with an assigned BAR: claimed=%v err=%v, want claimed", claimed, err)
	}
	if claimed, err := newHotplug(f).NICClaimed(ctx, 1, 0); err != nil || claimed {
		t.Fatalf("NIC with every memory BAR unmapped: claimed=%v err=%v, want unclaimed", claimed, err)
	}

	// A device without regions in the report, or one it does not list, is not guessed at.
	f.ports[DiskPortID(4)] = "disk4"
	if claimed, _ := newHotplug(f).DiskClaimed(ctx, 4, 0); !claimed {
		t.Fatal("a device with no reported regions was called unclaimed")
	}
	if claimed, _ := newHotplug(f).DiskClaimed(ctx, 9, 0); !claimed {
		t.Fatal("a device missing from the layout was called unclaimed")
	}
}

// TestClaimedWaitsForTheGuest: the first reads still show the BARs unmapped while the
// guest is enumerating; the check keeps reading until the address appears.
func TestClaimedWaitsForTheGuest(t *testing.T) {
	f := newFakeQMP(true)
	f.ports[DiskPortID(3)] = "disk3"
	f.regions = map[string][]map[string]interface{}{"disk3": {memoryBAR(-1)}}
	go func() {
		time.Sleep(2 * claimPoll / 3)
		f.mu.Lock()
		f.regions["disk3"] = []map[string]interface{}{memoryBAR(0xfe000000)}
		f.mu.Unlock()
	}()
	if claimed, err := newHotplug(f).DiskClaimed(context.Background(), 3, 5*time.Second); err != nil || !claimed {
		t.Fatalf("claimed=%v err=%v, want claimed once the guest assigned it", claimed, err)
	}
}
