// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ctsnap

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	answers map[string]string
	fails   map[string]bool
	calls   []string
}

func (f *fakeRunner) run(_ context.Context, _ time.Duration, name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	if f.fails[key] {
		return f.answers[key], errors.New("exit status 1")
	}
	answer, ok := f.answers[key]
	if !ok {
		return "", errors.New("unexpected command: " + key)
	}
	return answer, nil
}

const mountinfo = `22 1 0:21 / / rw,relatime shared:1 - zfs rpool/ROOT/debian rw,xattr,posixacl
40 22 0:40 / /tank/ct rw,relatime shared:20 - zfs tank/ct rw,xattr,posixacl
41 22 253:3 / /var/lib/mcharbor/compute/lvm/data rw,relatime shared:21 - ext4 /dev/mapper/vg--one-ct--data rw
42 22 253:4 / /srv/x\040fs rw,relatime shared:22 - xfs /dev/vg0/xlv rw
43 22 253:5 /sub /bound rw,relatime shared:23 - ext4 /dev/mapper/vg0-bindme rw
44 22 0:50 / /tank/ct rw,relatime shared:24 - zfs tank/ct-shadow rw
bad line
`

func TestContainingMountFindsTheLongestAndLatestMount(t *testing.T) {
	mounts := ParseMountInfo(mountinfo)
	if len(mounts) != 6 {
		t.Fatalf("parsed %d mounts, want 6: %+v", len(mounts), mounts)
	}

	entry, ok := ContainingMount(mounts, "/tank/ct/containers/a/rootfs")
	if !ok || entry.Source != "tank/ct-shadow" {
		t.Fatalf("a later mount at the same point shadows the earlier one, got %+v", entry)
	}
	entry, _ = ContainingMount(mounts, "/tank/ctx/rootfs")
	if entry.Source != "rpool/ROOT/debian" {
		t.Fatalf("a sibling sharing a prefix is not inside the mount, got %+v", entry)
	}
	entry, _ = ContainingMount(mounts, "/srv/x fs/containers/b/rootfs")
	if entry.Source != "/dev/vg0/xlv" || entry.FSType != "xfs" {
		t.Fatalf("an octal-escaped space is unescaped, got %+v", entry)
	}
	entry, _ = ContainingMount(mounts, "/bound/containers/c/rootfs")
	if got := FSRelative(entry, "/bound/containers/c/rootfs"); got != "/sub/containers/c/rootfs" {
		t.Fatalf("a bind mount's root is part of the path inside the filesystem, got %q", got)
	}
}

func TestZFSSnapshotTreeHonoursSnapdirAndMountpoint(t *testing.T) {
	good := ZFSProps{Mountpoint: "/tank/ct", Snapdir: "hidden", Mounted: "yes"}
	tree, err := ZFSSnapshotTree(good, "/containers/a/rootfs", "mcharbor-ctbk-r1")
	if err != nil || tree != "/tank/ct/.zfs/snapshot/mcharbor-ctbk-r1/containers/a/rootfs" {
		t.Fatalf("snapdir=hidden still resolves by path: got %q, %v", tree, err)
	}
	visible := good
	visible.Snapdir = "visible"
	if _, err := ZFSSnapshotTree(visible, "/", "s"); err != nil {
		t.Fatalf("snapdir=visible is readable: %v", err)
	}

	for name, props := range map[string]ZFSProps{
		"legacy":    {Mountpoint: "legacy", Snapdir: "hidden", Mounted: "yes"},
		"none":      {Mountpoint: "none", Snapdir: "hidden", Mounted: "no"},
		"relative":  {Mountpoint: "tank/ct", Snapdir: "hidden", Mounted: "yes"},
		"unmounted": {Mountpoint: "/tank/ct", Snapdir: "hidden", Mounted: "no"},
		"disabled":  {Mountpoint: "/tank/ct", Snapdir: "disabled", Mounted: "yes"},
	} {
		if _, err := ZFSSnapshotTree(props, "/", "s"); !errors.Is(err, ErrNotReadable) {
			t.Errorf("%s: expected ErrNotReadable so the backup freezes instead, got %v", name, err)
		}
	}
}

func TestPlanZFSDerivesTheTreeFromTheDatasetMountpoint(t *testing.T) {
	runner := &fakeRunner{answers: map[string]string{
		"zfs get -H -o property,value mountpoint,snapdir,mounted tank/ct-shadow": "mounted\tyes\nmountpoint\t/tank/ct\nsnapdir\thidden\n",
	}}
	plan, err := PlanZFS(context.Background(), runner.run, ParseMountInfo(mountinfo),
		"/tank/ct/containers/a/rootfs", "r1")
	if err != nil {
		t.Fatal(err)
	}
	want := ZFSPlan{
		Dataset:  "tank/ct-shadow",
		Snapshot: "tank/ct-shadow@mcharbor-ctbk-r1",
		Tree:     "/tank/ct/.zfs/snapshot/mcharbor-ctbk-r1/containers/a/rootfs",
	}
	if plan != want {
		t.Fatalf("plan = %+v, want %+v", plan, want)
	}

	if _, err := PlanZFS(context.Background(), runner.run, ParseMountInfo(mountinfo),
		"/var/lib/mcharbor/compute/lvm/data/rootfs", "r1"); !errors.Is(err, ErrNotReadable) {
		t.Fatalf("a rootfs on ext4 is not a zfs snapshot candidate, got %v", err)
	}
}

func TestDeviceLVDecodesDeviceMapperNames(t *testing.T) {
	cases := []struct {
		source, vg, lv string
		ok             bool
	}{
		{"/dev/mapper/vg--one-ct--data", "vg-one", "ct-data", true},
		{"/dev/mapper/vg0-root", "vg0", "root", true},
		{"/dev/vg0/xlv", "vg0", "xlv", true},
		{"/dev/mapper/nodash", "", "", false},
		{"/dev/dm-3", "", "", false},
		{"/dev/sda1", "", "", false},
		{"/dev/disk/by-id", "", "", false},
		{"tank/ct", "", "", false},
	}
	for _, c := range cases {
		vg, lv, ok := DeviceLV(c.source)
		if vg != c.vg || lv != c.lv || ok != c.ok {
			t.Errorf("DeviceLV(%q) = %q, %q, %v; want %q, %q, %v", c.source, vg, lv, ok, c.vg, c.lv, c.ok)
		}
	}
}

func TestMountOptionsAreReadOnlyAndXFSNeedsNoUUID(t *testing.T) {
	xfs, ok := MountOptions("xfs")
	if !ok || !strings.Contains(xfs, "nouuid") || !strings.HasPrefix(xfs, "ro,") {
		t.Fatalf("xfs options = %q, %v; a second mount of one UUID needs nouuid and must be read-only", xfs, ok)
	}
	ext4, ok := MountOptions("ext4")
	if !ok || !strings.HasPrefix(ext4, "ro,") || strings.Contains(ext4, "nouuid") {
		t.Fatalf("ext4 options = %q, %v", ext4, ok)
	}
	if _, ok := MountOptions("btrfs"); ok {
		t.Fatal("a filesystem this does not know how to mount safely is refused")
	}
}

func TestPlanLVMNeedsAThinOrigin(t *testing.T) {
	rootfs := "/var/lib/mcharbor/compute/lvm/data/containers/a/rootfs"
	thin := &fakeRunner{answers: map[string]string{
		"lvs --reportformat json -o segtype vg-one/ct-data": `{"report":[{"lv":[{"segtype":"thin"}]}]}`,
	}}
	plan, err := PlanLVM(context.Background(), thin.run, ParseMountInfo(mountinfo), rootfs, "r2")
	if err != nil {
		t.Fatal(err)
	}
	want := LVMPlan{
		VG:       "vg-one",
		Origin:   "ct-data",
		Snapshot: "mcharbor-ctbk-r2",
		Device:   "/dev/vg-one/mcharbor-ctbk-r2",
		FSType:   "ext4",
		Options:  "ro,nodev,nosuid,noexec",
		MountDir: "/run/mcharbor-agent/ct-snap/mcharbor-ctbk-r2",
		Tree:     "/run/mcharbor-agent/ct-snap/mcharbor-ctbk-r2/containers/a/rootfs",
	}
	if plan != want {
		t.Fatalf("plan = %+v\nwant  %+v", plan, want)
	}

	thick := &fakeRunner{answers: map[string]string{
		"lvs --reportformat json -o segtype vg-one/ct-data": `{"report":[{"lv":[{"segtype":"linear"}]}]}`,
	}}
	if _, err := PlanLVM(context.Background(), thick.run, ParseMountInfo(mountinfo), rootfs, "r2"); !errors.Is(err, ErrNotReadable) {
		t.Fatalf("a thick origin falls back to the freezer, got %v", err)
	}
}

func TestSnapshotNamesRoundTripAndRefuseHostileRunIDs(t *testing.T) {
	name, err := SnapshotName("d2b6k3c72upg6lg0abcd")
	if err != nil || name != "mcharbor-ctbk-d2b6k3c72upg6lg0abcd" {
		t.Fatalf("SnapshotName = %q, %v", name, err)
	}
	if id, ok := RunIDFromName(name); !ok || id != "d2b6k3c72upg6lg0abcd" {
		t.Fatalf("RunIDFromName(%q) = %q, %v", name, id, ok)
	}
	for _, bad := range []string{"", "../etc", "a b", "-rf", "x@y", strings.Repeat("a", 65)} {
		if _, err := SnapshotName(bad); err == nil {
			t.Errorf("SnapshotName(%q) accepted a run id that would reach an argv or a path", bad)
		}
	}
	for _, foreign := range []string{"mcharbor-backup-r1", "ct-data", "mcharbor-ctbk-", "mcharbor-ctbk-a/b"} {
		if _, ok := RunIDFromName(foreign); ok {
			t.Errorf("RunIDFromName(%q) claimed a name this package never produces", foreign)
		}
	}
}

func TestStaleSelectionSkipsRunsInFlightAndForeignNames(t *testing.T) {
	active := NewActive()
	done := active.Add("live")

	mounts := []MountEntry{
		{MountPoint: MountRoot + "/mcharbor-ctbk-dead"},
		{MountPoint: MountRoot + "/mcharbor-ctbk-dead/nested"},
		{MountPoint: MountRoot + "/mcharbor-ctbk-live"},
		{MountPoint: MountRoot + "/operator-thing"},
		{MountPoint: MountRoot},
		{MountPoint: "/run/mcharbor-agent/ct-snapshot/mcharbor-ctbk-dead"},
	}
	if got, want := StaleMounts(mounts, MountRoot, active),
		[]string{MountRoot + "/mcharbor-ctbk-dead/nested", MountRoot + "/mcharbor-ctbk-dead"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("StaleMounts = %v, want %v (deepest first, live and foreign untouched)", got, want)
	}

	if got := StaleDirs([]string{"mcharbor-ctbk-dead", "mcharbor-ctbk-live", "lost+found"}, active); !reflect.DeepEqual(got, []string{"mcharbor-ctbk-dead"}) {
		t.Fatalf("StaleDirs = %v", got)
	}

	lvs := []LV{
		{VG: "vg0", Name: "mcharbor-ctbk-dead", Origin: "ct-data"},
		{VG: "vg0", Name: "mcharbor-ctbk-live", Origin: "ct-data"},
		{VG: "vg0", Name: "mcharbor-ctbk-origin-less", Origin: ""},
		{VG: "vg0", Name: "ct-data_snap_nightly", Origin: "ct-data"},
	}
	if got := StaleLVs(lvs, active); !reflect.DeepEqual(got, []string{"vg0/mcharbor-ctbk-dead"}) {
		t.Fatalf("StaleLVs = %v (only an origin-bearing snapshot of ours whose run is gone)", got)
	}

	snaps := []string{"tank/ct@mcharbor-ctbk-dead", "tank/ct@mcharbor-ctbk-live", "tank/ct@nightly", "tank/ct", "-x@mcharbor-ctbk-y"}
	if got := StaleZFSSnapshots(snaps, active); !reflect.DeepEqual(got, []string{"tank/ct@mcharbor-ctbk-dead"}) {
		t.Fatalf("StaleZFSSnapshots = %v", got)
	}

	siblings := []string{"rootfs", "rootfs.snap.mcharbor-ctbk-dead", "rootfs.snap.mcharbor-ctbk-live", "rootfs.snap.nightly", "other.snap.mcharbor-ctbk-dead"}
	if got := StaleSiblings("/pool/containers/a/rootfs", siblings, active); !reflect.DeepEqual(got, []string{"/pool/containers/a/rootfs.snap.mcharbor-ctbk-dead"}) {
		t.Fatalf("StaleSiblings = %v", got)
	}

	done()
	if got := StaleLVs(lvs, active); len(got) != 2 {
		t.Fatalf("a finished run's volume is stale once it leaves the active set, got %v", got)
	}
}

func TestListingsParseWhatTheToolsPrint(t *testing.T) {
	runner := &fakeRunner{answers: map[string]string{
		"lvs --reportformat json -o vg_name,lv_name,origin": `{"report":[{"lv":[{"vg_name":"vg0","lv_name":"mcharbor-ctbk-dead","origin":"ct"},{"vg_name":"vg0","lv_name":"ct","origin":""}]}]}`,
		"zfs list -H -t snapshot -o name -d 1 tank/ct":      "tank/ct@a\ntank/ct@mcharbor-ctbk-dead\n\n",
		"zfs list -H -t snapshot -o name":                   "tank/ct@a\n",
	}}
	rows, err := ListLVs(context.Background(), runner.run)
	if err != nil || len(rows) != 2 || rows[0].Origin != "ct" || rows[1].Name != "ct" {
		t.Fatalf("ListLVs = %+v, %v", rows, err)
	}
	names, err := ListZFSSnapshots(context.Background(), runner.run, "tank/ct")
	if err != nil || !reflect.DeepEqual(names, []string{"tank/ct@a", "tank/ct@mcharbor-ctbk-dead"}) {
		t.Fatalf("scoped ListZFSSnapshots = %v, %v", names, err)
	}
	if _, err := ListZFSSnapshots(context.Background(), runner.run, ""); err != nil {
		t.Fatalf("host-wide ListZFSSnapshots: %v", err)
	}
}
