// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
)

// TestTapCountersIncludesMacvtap guards the direct-NIC case: a guest on a
// macvlan network has no tap at all, and before this its traffic read as zero.
func TestTapCountersIncludesMacvtap(t *testing.T) {
	const vmID = "daur3rpouu8c72va6o60"
	root := t.TempDir()
	previous := sysClassNet
	sysClassNet = root
	t.Cleanup(func() { sysClassNet = previous })

	writeLink(t, root, netconf.TapName(vmID, 0), 100, 200)    // tap: host rx = guest tx
	writeLink(t, root, netconf.DirectName(vmID, 0), 1000, 50) // macvtap: as the guest sees it
	writeLink(t, root, netconf.TapName("someotherguest00000", 0), 9999, 9999)

	rx, tx := tapCounters(vmID)
	if rx != 200+1000 || tx != 100+50 {
		t.Errorf("tapCounters = rx %d tx %d, want rx 1200 tx 150", rx, tx)
	}
}

func writeLink(t *testing.T, root, name string, rx, tx int64) {
	t.Helper()
	dir := filepath.Join(root, name, "statistics")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for file, value := range map[string]int64{"rx_bytes": rx, "tx_bytes": tx} {
		body := []byte(strconv.FormatInt(value, 10) + "\n")
		if err := os.WriteFile(filepath.Join(dir, file), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
