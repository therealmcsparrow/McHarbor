// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// diskSectorBytes is the unit /proc/diskstats counts in, whatever the device's
// real sector size: the kernel always reports 512-byte sectors there.
const diskSectorBytes = 512

// readHostIO sums the host's network and disk counters over physical devices.
//
// Physical is the point: a guest's packet crosses its tap, a bridge and the
// uplink, and a container's crosses a veth and docker0 too, so summing every
// interface would count one transfer three times. A device with a `device` link
// in sysfs is backed by hardware (or a paravirtual disk/NIC), and that set sees
// every byte that enters or leaves the machine exactly once.
func readHostIO(procRoot, sysRoot string) *HostIOCounters {
	out := &HostIOCounters{}
	found := false

	if f, err := os.Open(filepath.Join(procRoot, "net", "dev")); err == nil {
		counters := parseNetDev(f)
		errlog.Close(f)
		for name, c := range counters {
			if hasDeviceLink(filepath.Join(sysRoot, "class", "net", name)) {
				out.NetRxBytes += c.rx
				out.NetTxBytes += c.tx
				found = true
			}
		}
	}

	if f, err := os.Open(filepath.Join(procRoot, "diskstats")); err == nil {
		counters := parseDiskstats(f)
		errlog.Close(f)
		for name, c := range counters {
			// Partitions are not under /sys/block, and dm/md/loop devices
			// have no device link, so neither is counted twice.
			if hasDeviceLink(filepath.Join(sysRoot, "block", name)) {
				out.DiskReadBytes += c.rx
				out.DiskWriteBytes += c.tx
				found = true
			}
		}
	}

	if !found {
		return nil
	}
	return out
}

// ioPair is one device's cumulative inbound/outbound (or read/write) bytes.
type ioPair struct{ rx, tx int64 }

// parseNetDev reads /proc/net/dev: two header lines, then
// "iface: rx_bytes rx_packets … (8 rx fields) tx_bytes …".
func parseNetDev(r io.Reader) map[string]ioPair {
	out := map[string]ioPair{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		name, rest, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, errRx := strconv.ParseInt(fields[0], 10, 64)
		tx, errTx := strconv.ParseInt(fields[8], 10, 64)
		if errRx != nil || errTx != nil {
			continue
		}
		out[strings.TrimSpace(name)] = ioPair{rx: rx, tx: tx}
	}
	return out
}

// parseDiskstats reads /proc/diskstats: "major minor name reads merged
// sectors_read ms writes merged sectors_written …", converted to bytes.
func parseDiskstats(r io.Reader) map[string]ioPair {
	out := map[string]ioPair{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		read, errRead := strconv.ParseInt(fields[5], 10, 64)
		written, errWrite := strconv.ParseInt(fields[9], 10, 64)
		if errRead != nil || errWrite != nil {
			continue
		}
		out[fields[2]] = ioPair{rx: read * diskSectorBytes, tx: written * diskSectorBytes}
	}
	return out
}

// hasDeviceLink reports whether a sysfs node is backed by a device.
func hasDeviceLink(node string) bool {
	_, err := os.Stat(filepath.Join(node, "device"))
	return err == nil
}
