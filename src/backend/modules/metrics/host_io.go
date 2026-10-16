// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// localHostProc is where docker-compose.yml mounts the host's /proc read-only.
// Its pid 1 is the host's init, so /host/proc/1/net is the host's own network
// namespace rather than this container's.
const localHostProc = "/host/proc"

// diskSectorBytes is the unit /proc/diskstats counts in, whatever the device's
// real sector size: the kernel always reports 512-byte sectors there.
const diskSectorBytes = 512

// readLocalHostIO reads the McHarbor host's own network and disk counters, or
// nil when the host /proc is not mounted and only this container is visible.
//
// Network counts the interfaces that carry a default route in the host's
// namespace. This container's sysfs belongs to its own namespace, so the
// physical-device test the agent uses is not available here; the uplink is the
// one interface every container, guest and host byte leaves or arrives through.
// Disk is not namespaced, so it uses the same physical-device rule as the agent:
// whole disks with a sysfs device link, never a partition, dm or loop device.
func readLocalHostIO(hostProc, procRoot, sysRoot string) *HostIO {
	netDev, err := os.ReadFile(filepath.Join(hostProc, "1", "net", "dev"))
	if err != nil {
		return nil
	}
	route, err := os.ReadFile(filepath.Join(hostProc, "1", "net", "route"))
	if err != nil {
		return nil
	}
	uplinks := defaultRouteInterfaces(strings.NewReader(string(route)))
	if len(uplinks) == 0 {
		return nil
	}

	counters := parseNetDev(strings.NewReader(string(netDev)))
	out := &HostIO{}
	for name := range uplinks {
		out.NetRx += counters[name].rx
		out.NetTx += counters[name].tx
	}

	if stats, err := os.ReadFile(filepath.Join(procRoot, "diskstats")); err == nil {
		for name, c := range parseDiskstats(strings.NewReader(string(stats))) {
			if _, statErr := os.Stat(filepath.Join(sysRoot, "block", name, "device")); statErr == nil {
				out.DiskRead += c.rx
				out.DiskWrite += c.tx
			}
		}
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

// defaultRouteInterfaces reads /proc/net/route and returns the interfaces whose
// destination is 0.0.0.0 — the uplinks.
func defaultRouteInterfaces(r io.Reader) map[string]struct{} {
	out := map[string]struct{}{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] == "Iface" {
			continue
		}
		if fields[1] == "00000000" {
			out[fields[0]] = struct{}{}
		}
	}
	return out
}
