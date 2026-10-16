// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
)

// tapCounters sums the host-side statistics of a guest's interfaces: its taps
// (bridged and NAT NICs) and its macvtaps (direct NICs on an uplink).
//
// The links are found by name rather than from saved state: netconf.TapName and
// netconf.DirectName are deterministic, so the prefix can be rebuilt from the id
// alone and the agent reports correct counters even after a restart that lost
// every spec it held.
//
// A tap's counters are swapped on purpose: its rx is traffic the host received
// *from* the guest, which is the guest's transmit. A macvtap is not swapped,
// because it counts frames from the uplink toward the guest as received and
// frames the guest sends out as transmitted, the same way the guest sees them.
func tapCounters(vmID string) (rx int64, tx int64) {
	tapRx, tapTx := linkCounters(netconf.TapName(vmID, 0), netconf.TapName(vmID, 1))
	directRx, directTx := linkCounters(netconf.DirectName(vmID, 0), netconf.DirectName(vmID, 1))
	return tapTx + directRx, tapRx + directTx
}

// linkCounters sums rx_bytes and tx_bytes, as the kernel reports them, over
// every link sharing the prefix of two slot names.
func linkCounters(slot0, slot1 string) (rx int64, tx int64) {
	// Derived from two slots rather than by trimming one, because the name is
	// truncated to the kernel's 15-character limit: where the slot digit ends
	// up — or whether it survived at all — is not something to assume.
	prefix := commonPrefix(slot0, slot1)
	if prefix == "" {
		return 0, 0
	}

	entries, err := os.ReadDir(sysClassNet)
	if err != nil {
		return 0, 0
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		base := filepath.Join(sysClassNet, entry.Name(), "statistics")
		rx += readCounter(filepath.Join(base, "rx_bytes"))
		tx += readCounter(filepath.Join(base, "tx_bytes"))
	}
	return rx, tx
}

// sysClassNet is where the link counters are read; a variable so a test can
// point it at a fixture tree.
var sysClassNet = "/sys/class/net"

// commonPrefix returns the leading run of bytes a and b share.
func commonPrefix(a, b string) string {
	limit := min(len(a), len(b))
	i := 0
	for i < limit && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// readCounter reads one sysfs counter, reporting 0 for anything unreadable.
func readCounter(path string) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0
	}
	return value
}
