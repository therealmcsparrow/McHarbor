// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"encoding/json"
	"testing"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

var testNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func TestVMPowerStateMachine(t *testing.T) {
	steps := []struct {
		op        string
		wantState string
		wantEvent string
		wantErr   bool
	}{
		{coreagent.MsgVMStart, stateRunning, "", false},
		{coreagent.MsgVMStart, stateRunning, "", false},
		{coreagent.MsgVMPause, statePaused, "stop", false},
		{coreagent.MsgVMStart, statePaused, "", true},
		{coreagent.MsgVMPause, statePaused, "", false},
		{coreagent.MsgVMResume, stateRunning, "resume", false},
		{coreagent.MsgVMResume, stateRunning, "", false},
		{coreagent.MsgVMReset, stateRunning, "reset", false},
		{coreagent.MsgVMShutdown, stateStopped, "powerdown", false},
		{coreagent.MsgVMShutdown, stateStopped, "", false},
		{coreagent.MsgVMReset, stateStopped, "", true},
		{coreagent.MsgVMPause, stateStopped, "", true},
		{coreagent.MsgVMResume, stateStopped, "", true},
		{coreagent.MsgVMStart, stateRunning, "", false},
		{coreagent.MsgVMStop, stateStopped, "shutdown", false},
	}
	g := &vm{ID: "vm1", State: stateStopped}
	for i, step := range steps {
		event, err := vmTransition(g, step.op, testNow.Add(time.Duration(i)*time.Minute))
		if (err != nil) != step.wantErr {
			t.Fatalf("step %d %s: err = %v, want error %v", i, step.op, err, step.wantErr)
		}
		if g.State != step.wantState || event != step.wantEvent {
			t.Fatalf("step %d %s: state %s event %q, want %s %q", i, step.op, g.State, event, step.wantState, step.wantEvent)
		}
		if g.State == stateStopped && !g.StartedAt.IsZero() {
			t.Fatalf("step %d: a stopped guest kept its start time", i)
		}
	}

	crashed := &vm{ID: "vm2", State: stateCrashed, LastError: "panic", RunState: "guest-panicked"}
	if _, err := vmTransition(crashed, coreagent.MsgVMStart, testNow); err != nil || crashed.State != stateRunning || crashed.LastError != "" {
		t.Fatalf("starting a crashed guest = %+v, %v; want running with the error cleared", crashed, err)
	}
}

func TestCTLifecycleStateMachine(t *testing.T) {
	steps := []struct {
		op        string
		wantState string
		wantErr   bool
	}{
		{coreagent.MsgCTFreeze, stateDefined, true},
		{coreagent.MsgCTStart, stateRunning, false},
		{coreagent.MsgCTFreeze, stateFrozen, false},
		{coreagent.MsgCTStart, stateFrozen, true},
		{coreagent.MsgCTRestart, stateFrozen, true},
		{coreagent.MsgCTUnfreeze, stateRunning, false},
		{coreagent.MsgCTUnfreeze, stateRunning, false},
		{coreagent.MsgCTRestart, stateRunning, false},
		{coreagent.MsgCTShutdown, stateStopped, false},
		{coreagent.MsgCTUnfreeze, stateStopped, true},
		{coreagent.MsgCTRestart, stateRunning, false},
		{coreagent.MsgCTStop, stateStopped, false},
	}
	c := &ct{ID: "ct1", State: stateDefined}
	for i, step := range steps {
		err := ctTransition(c, step.op, testNow.Add(time.Duration(i)*time.Minute))
		if (err != nil) != step.wantErr {
			t.Fatalf("step %d %s: err = %v, want error %v", i, step.op, err, step.wantErr)
		}
		if c.State != step.wantState {
			t.Fatalf("step %d %s: state %s, want %s", i, step.op, c.State, step.wantState)
		}
		if (c.State == stateRunning || c.State == stateFrozen) != (c.InitPID != 0) {
			t.Fatalf("step %d: state %s with init pid %d", i, c.State, c.InitPID)
		}
	}
	if c.ExitReason != "killed" {
		t.Fatalf("exit reason after ct_stop = %q, want killed", c.ExitReason)
	}
}

func TestVMSnapshotBookkeeping(t *testing.T) {
	g := &vm{ID: "vm1", State: stateRunning}
	a, err := vmSnapshotCreate(g, "", "a", false, "", testNow)
	if err != nil {
		t.Fatal(err)
	}
	b, err := vmSnapshotCreate(g, "", "b", true, "filesystem", testNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if b.ParentID != a.ID || g.Current != b.ID {
		t.Fatalf("b parent %q current %q, want parent %q current %q", b.ParentID, g.Current, a.ID, b.ID)
	}
	if _, err := vmSnapshotCreate(g, "", "a", false, "", testNow); err == nil {
		t.Fatal("a second snapshot with the same handle was accepted")
	}

	g.State = stateStopped
	vmSnapshotRevert(g, "", "a", testNow)
	if g.Current != a.ID || g.State != stateStopped {
		t.Fatalf("after revert to a: current %q state %s", g.Current, g.State)
	}
	c, err := vmSnapshotCreate(g, "", "c", false, "", testNow.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if c.ParentID != a.ID {
		t.Fatalf("c branched from %q, want a (%q)", c.ParentID, a.ID)
	}

	vmSnapshotRevert(g, b.ID, "", testNow)
	if g.State != stateRunning {
		t.Fatalf("reverting to a memory snapshot of a running guest left it %s", g.State)
	}

	vmSnapshotDelete(g, a.ID, "")
	if len(g.Snapshots) != 2 || b.ParentID != "" || c.ParentID != "" {
		t.Fatalf("after deleting a: %d snapshots, b parent %q, c parent %q", len(g.Snapshots), b.ParentID, c.ParentID)
	}
	vmSnapshotDelete(g, "", "b")
	if g.Current != "" || len(g.Snapshots) != 1 {
		t.Fatalf("deleting the current snapshot left current %q and %d snapshots", g.Current, len(g.Snapshots))
	}
	vmSnapshotDelete(g, "missing", "missing")
	if len(g.Snapshots) != 1 {
		t.Fatal("deleting an unknown snapshot removed one")
	}
}

func TestCTSnapshotRevertPrunesLaterSnapshots(t *testing.T) {
	c := &ct{ID: "ct1", State: stateStopped, RootfsPath: "/tank/vms/ct-ct1"}
	for i, name := range []string{"one", "two", "three"} {
		if _, err := ctSnapshotCreate(c, "id-"+name, name, testNow.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if c.Snapshots[1].Handle != "/tank/vms/ct-ct1@two" || c.Snapshots[2].ParentID != "id-two" {
		t.Fatalf("snapshot two = %+v, three parent %q", c.Snapshots[1], c.Snapshots[2].ParentID)
	}
	c.State = stateRunning
	if err := ctSnapshotRevert(c, "id-one"); err == nil {
		t.Fatal("reverting a running container was accepted")
	}
	c.State = stateStopped
	if err := ctSnapshotRevert(c, "id-one"); err != nil {
		t.Fatal(err)
	}
	if len(c.Snapshots) != 1 || c.Snapshots[0].ID != "id-one" {
		t.Fatalf("after revert to one: %d snapshots", len(c.Snapshots))
	}
	ctSnapshotDelete(c, "id-one")
	if len(c.Snapshots) != 0 {
		t.Fatal("delete kept the snapshot")
	}
}

func TestEstateIsDeterministicAndSized(t *testing.T) {
	e := estate.Build(estate.DefaultSeed, testNow)
	a := New(e, 42, testNow)
	b := New(estate.Build(estate.DefaultSeed, testNow), 42, testNow)

	vms, cts, pools, networks := a.Counts()
	if vms < 110 || vms > 130 || cts < 70 || cts > 90 {
		t.Fatalf("estate has %d VMs and %d CTs, want about 120 and 80", vms, cts)
	}
	if pools < 2*len(a.order) || networks < 2*len(a.order) {
		t.Fatalf("%d pools and %d networks over %d hosts", pools, networks, len(a.order))
	}

	var crashed, snapped, templates, running int
	for _, key := range a.order {
		ha, hb := a.hosts[key], b.hosts[key]
		if len(ha.pools) < 2 || len(ha.pools) > 4 || len(ha.networks) < 2 || len(ha.networks) > 3 {
			t.Fatalf("%s has %d pools and %d networks", key, len(ha.pools), len(ha.networks))
		}
		for i, id := range ha.vmOrder {
			if hb.vmOrder[i] != id {
				t.Fatalf("%s VM %d differs between two builds from one seed", key, i)
			}
			g := ha.vms[id]
			switch g.State {
			case stateCrashed:
				crashed++
				if g.LastError == "" {
					t.Fatalf("crashed %s carries no error", g.Name)
				}
			case stateRunning:
				running++
			}
			if len(g.Snapshots) > 0 {
				snapped++
				if g.Current != g.Snapshots[len(g.Snapshots)-1].ID {
					t.Fatalf("%s current snapshot is not the tip", g.Name)
				}
			}
			if g.IsTemplate {
				templates++
			}
		}
	}
	if crashed < 1 || crashed > 2 || templates < 3 {
		t.Fatalf("%d crashed VMs and %d templates", crashed, templates)
	}
	if running < vms/2 {
		t.Fatalf("only %d of %d VMs run", running, vms)
	}
	if ratio := float64(snapped) / float64(vms); ratio < 0.18 || ratio > 0.45 {
		t.Fatalf("%.0f%% of VMs have snapshots, want about 30%%", ratio*100)
	}
	if other := New(e, 43, testNow); other.hosts["homelab"].vmOrder[0] == a.hosts["homelab"].vmOrder[0] {
		t.Fatal("a different seed produced the same ids")
	}
}

func TestRFBEncodesInTheClientPixelFormat(t *testing.T) {
	bgr := pixelFormat{bpp: 32, depth: 24, trueColour: true, rMax: 255, gMax: 255, bMax: 255, rShift: 0, gShift: 8, bShift: 16}
	got := bgr.encode(nil, 0x112233)
	if len(got) != 4 || got[0] != 0x11 || got[1] != 0x22 || got[2] != 0x33 {
		t.Fatalf("encoded %x, want 112233 in little-endian red-first order", got)
	}
	rgb565 := pixelFormat{bpp: 16, depth: 16, trueColour: true, rMax: 31, gMax: 63, bMax: 31, rShift: 11, gShift: 5, bShift: 0}
	if got := rgb565.encode(nil, 0xffffff); got[0] != 0xff || got[1] != 0xff {
		t.Fatalf("white in rgb565 = %x", got)
	}
	if parsePixelFormat(serverPixelFormat.bytes()) != serverPixelFormat {
		t.Fatal("pixel format does not round-trip")
	}
}

func TestHotplugChangesARunningGuestAndRefusesTheBootDisk(t *testing.T) {
	g := &vm{ID: "vm1", State: stateRunning, Disks: []*disk{{ID: "d0", Slot: 0, IsBoot: true}}}
	attach := &coreagent.VMPayload{Hotplug: &coreagent.VMHotplugSpec{Action: "attach",
		Disk: &coreagent.VMDiskSpec{DiskID: "d2", Slot: 2, SizeBytes: 1 << 30, VolumePath: "/p/d2"}}}
	reply := &coreagent.VMPayload{}
	if err := applyHotplug(g, coreagent.MsgVMDiskHotplug, attach, reply); err != nil || len(g.Disks) != 2 {
		t.Fatalf("attach: err %v, disks %d", err, len(g.Disks))
	}
	if err := applyHotplug(g, coreagent.MsgVMDiskHotplug, attach, reply); err != nil || len(g.Disks) != 2 {
		t.Fatalf("a repeated attach added a second disk: err %v, disks %d", err, len(g.Disks))
	}

	boot := &coreagent.VMPayload{Hotplug: &coreagent.VMHotplugSpec{Action: "detach",
		Disk: &coreagent.VMDiskSpec{DiskID: "d0", Slot: 0, IsBoot: true}}}
	reply = &coreagent.VMPayload{}
	if err := applyHotplug(g, coreagent.MsgVMDiskHotplug, boot, reply); err != nil || reply.ErrorCode != "hotplug_boot_disk" || len(g.Disks) != 2 {
		t.Fatalf("boot disk detach: err %v, code %q, disks %d", err, reply.ErrorCode, len(g.Disks))
	}

	nicAttach := &coreagent.VMPayload{Hotplug: &coreagent.VMHotplugSpec{Action: "attach",
		NIC: &coreagent.VMNICSpec{NICID: "n1", Slot: 1, MAC: "52:54:00:00:00:01"}}}
	if err := applyHotplug(g, coreagent.MsgVMNICHotplug, nicAttach, &coreagent.VMPayload{}); err != nil || len(g.NICs) != 1 {
		t.Fatalf("nic attach: err %v, nics %d", err, len(g.NICs))
	}

	g.State = stateStopped
	if err := applyHotplug(g, coreagent.MsgVMDiskHotplug, attach, &coreagent.VMPayload{}); err == nil {
		t.Fatal("a stopped guest took a hot-plug")
	}
}

func TestEveryHotplugMessageIsSimulated(t *testing.T) {
	for _, msgType := range []string{coreagent.MsgVMDiskHotplug, coreagent.MsgVMMediaChange, coreagent.MsgVMNICHotplug} {
		if !vmTypes[msgType] {
			t.Errorf("%s is not routed to the vm simulator", msgType)
		}
	}
}

// TestVMSpecCarriesSpiceOptions checks the demo accepts a define whose spec
// carries a display profile, as McHarbor sends it on the wire.
func TestVMSpecCarriesSpiceOptions(t *testing.T) {
	var payload coreagent.VMPayload
	wire := `{"vmId":"vm1","spec":{"name":"web","vcpus":2,"memoryMb":2048,"machineType":"pc-q35-8.2",` +
		`"spiceOptions":{"imageCompression":"off","streamingVideo":"filter","playbackCompression":"on"}}}`
	if err := json.Unmarshal([]byte(wire), &payload); err != nil {
		t.Fatal(err)
	}
	g := &vm{ID: "vm1"}
	applyVMSpec(g, payload.Spec)
	if g.SpiceOptions == nil || g.SpiceOptions.ImageCompression != "off" ||
		g.SpiceOptions.StreamingVideo != "filter" || g.SpiceOptions.PlaybackCompression != "on" {
		t.Fatalf("spice options = %+v, want the profile the spec carried", g.SpiceOptions)
	}
	payload.Spec.SpiceOptions.ImageCompression = "lz"
	if g.SpiceOptions.ImageCompression != "off" {
		t.Fatal("the simulator aliases the caller's options instead of copying them")
	}
}
