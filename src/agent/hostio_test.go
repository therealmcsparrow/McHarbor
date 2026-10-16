// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

const fixtureNetDev = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 9000 10 0 0 0 0 0 0 9000 10 0 0 0 0 0 0
enp1s0: 1000 10 0 0 0 0 0 0 2000 10 0 0 0 0 0 0
  eno2: 300 3 0 0 0 0 0 0 400 4 0 0 0 0 0 0
   br0: 5000 50 0 0 0 0 0 0 6000 60 0 0 0 0 0 0
mchtabcd12340: 7000 70 0 0 0 0 0 0 8000 80 0 0 0 0 0 0
`

const fixtureDiskstats = `   8       0 sda 100 0 10 0 50 0 20 0 0 0 0
   8       1 sda1 90 0 9 0 40 0 18 0 0 0 0
 253       0 dm-0 80 0 8 0 30 0 16 0 0 0 0
 259       0 nvme0n1 10 0 4 0 5 0 2 0 0 0 0
   7       0 loop0 1 0 100 0 1 0 100 0 0 0 0
`

// writeIOFixture lays out a proc/sys pair in which only enp1s0, eno2, sda and
// nvme0n1 are physical.
func writeIOFixture(t *testing.T) (procRoot, sysRoot string) {
	t.Helper()
	root := t.TempDir()
	procRoot = filepath.Join(root, "proc")
	sysRoot = filepath.Join(root, "sys")
	mustWrite(t, filepath.Join(procRoot, "net", "dev"), fixtureNetDev)
	mustWrite(t, filepath.Join(procRoot, "diskstats"), fixtureDiskstats)
	for _, nic := range []string{"lo", "enp1s0", "eno2", "br0", "mchtabcd12340"} {
		mustMkdir(t, filepath.Join(sysRoot, "class", "net", nic))
	}
	for _, nic := range []string{"enp1s0", "eno2"} {
		mustMkdir(t, filepath.Join(sysRoot, "class", "net", nic, "device"))
	}
	for _, disk := range []string{"sda", "dm-0", "nvme0n1", "loop0"} {
		mustMkdir(t, filepath.Join(sysRoot, "block", disk))
	}
	for _, disk := range []string{"sda", "nvme0n1"} {
		mustMkdir(t, filepath.Join(sysRoot, "block", disk, "device"))
	}
	return procRoot, sysRoot
}

func TestReadHostIOCountsPhysicalDevicesOnce(t *testing.T) {
	procRoot, sysRoot := writeIOFixture(t)
	got := readHostIO(procRoot, sysRoot)
	if got == nil {
		t.Fatal("readHostIO returned nil for a host with physical devices")
	}
	// Bridge, tap and loopback traffic is the same bytes the uplinks already
	// carried, so only enp1s0 + eno2 count.
	if got.NetRxBytes != 1300 || got.NetTxBytes != 2400 {
		t.Errorf("net = %d/%d, want 1300/2400", got.NetRxBytes, got.NetTxBytes)
	}
	// sda + nvme0n1 only: the partition, the device-mapper volume and the
	// loop device sit on top of them.
	if got.DiskReadBytes != (10+4)*512 || got.DiskWriteBytes != (20+2)*512 {
		t.Errorf("disk = %d/%d, want %d/%d", got.DiskReadBytes, got.DiskWriteBytes, 14*512, 22*512)
	}
}

func TestReadHostIONilWithoutPhysicalDevices(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "proc", "net", "dev"), fixtureNetDev)
	if got := readHostIO(filepath.Join(root, "proc"), filepath.Join(root, "sys")); got != nil {
		t.Errorf("readHostIO = %+v, want nil when no device is physical", got)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
}
