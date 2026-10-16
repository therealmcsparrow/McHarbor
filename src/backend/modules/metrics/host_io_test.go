// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const hostNetDev = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 9000 10 0 0 0 0 0 0 9000 10 0 0 0 0 0 0
  eth0: 1000 10 0 0 0 0 0 0 2000 10 0 0 0 0 0 0
docker0: 5000 50 0 0 0 0 0 0 6000 60 0 0 0 0 0 0
vethab12: 7000 70 0 0 0 0 0 0 8000 80 0 0 0 0 0 0
`

const hostRoute = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0100A8C0	0003	0	0	0	00000000	0	0	0
eth0	0000A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`

const hostDiskstats = `   8       0 sda 100 0 10 0 50 0 20 0 0 0 0
   8       1 sda1 90 0 9 0 40 0 18 0 0 0 0
   7       0 loop0 1 0 100 0 1 0 100 0 0 0 0
`

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestReadLocalHostIOCountsTheUplinkAndPhysicalDisks guards the double count a
// naive sum gives: docker0 and every veth carry bytes the uplink already did.
func TestReadLocalHostIOCountsTheUplinkAndPhysicalDisks(t *testing.T) {
	root := t.TempDir()
	hostProc := filepath.Join(root, "host", "proc")
	writeFile(t, filepath.Join(hostProc, "1", "net", "dev"), hostNetDev)
	writeFile(t, filepath.Join(hostProc, "1", "net", "route"), hostRoute)
	writeFile(t, filepath.Join(root, "proc", "diskstats"), hostDiskstats)
	for _, disk := range []string{"sda", "loop0"} {
		if err := os.MkdirAll(filepath.Join(root, "sys", "block", disk), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "sys", "block", "sda", "device"), 0o750); err != nil {
		t.Fatal(err)
	}

	got := readLocalHostIO(hostProc, filepath.Join(root, "proc"), filepath.Join(root, "sys"))
	if got == nil {
		t.Fatal("readLocalHostIO = nil with the host /proc mounted")
	}
	want := HostIO{NetRx: 1000, NetTx: 2000, DiskRead: 10 * 512, DiskWrite: 20 * 512}
	if *got != want {
		t.Errorf("readLocalHostIO = %+v, want %+v", *got, want)
	}
}

// TestReadLocalHostIONilWithoutTheHostProcMount keeps an install without the
// compose mount on the container sums rather than charting this container alone.
func TestReadLocalHostIONilWithoutTheHostProcMount(t *testing.T) {
	root := t.TempDir()
	if got := readLocalHostIO(filepath.Join(root, "missing"), root, root); got != nil {
		t.Errorf("readLocalHostIO = %+v, want nil", got)
	}
}

func TestCollectorTakesIOFromTheHostWhenItReportsIt(t *testing.T) {
	db := openPushSinkDB(t)
	reading := &HostReading{CPUPercent: 5, MemUsed: 4_000, MemTotal: 8_000,
		IO: &HostIO{NetRx: 900, NetTx: 800, DiskRead: 700, DiskWrite: 600}}
	c := newHostTestCollector(t, db, twoContainers, reading)
	tick(t, c, false, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))

	rows := readHostRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("host rows = %d, want 1", len(rows))
	}
	if rows[0].netRx.Int64 != 900 || rows[0].blockRead.Int64 != 700 {
		t.Fatalf("I/O = %d/%d, want the host counters 900/700, not the container sums 12/5",
			rows[0].netRx.Int64, rows[0].blockRead.Int64)
	}

	idle := openPushSinkDB(t)
	tick(t, newHostTestCollector(t, idle, nil, reading), false, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	if got := readHostRows(t, idle); len(got) != 1 || !got[0].netRx.Valid || got[0].netRx.Int64 != 900 {
		t.Fatalf("rows = %+v, want host I/O recorded on a host running no containers", got)
	}
}

// TestCompactHostIgnoresMissingCounters guards the switch from container sums
// to host counters: a NULL row folded to 0 would record the whole host counter
// as one bucket's traffic.
func TestCompactHostIgnoresMissingCounters(t *testing.T) {
	db := openPushSinkDB(t)
	insert := func(at string, rx any) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO host_metrics (id, environment_id, cpu_percent, memory_percent,
			memory_used, memory_total, net_rx, net_tx, block_read, block_write, timestamp)
			VALUES (?, 'env', 1, 1, 1, 1, ?, ?, ?, ?, ?)`, at, rx, rx, rx, rx, at); err != nil {
			t.Fatal(err)
		}
	}
	insert("2026-10-01 10:00:00", nil)
	insert("2026-10-01 10:00:30", int64(5_000_000))
	insert("2026-10-01 10:01:00", int64(5_000_600))

	c := NewCollector(db, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := c.compactHost(t.Context(), "2026-10-01 11:00:00"); err != nil {
		t.Fatal(err)
	}
	var delta int64
	if err := db.QueryRow(`SELECT SUM(net_rx_delta) FROM host_metrics_rollup WHERE environment_id = 'env'`).Scan(&delta); err != nil {
		t.Fatal(err)
	}
	if delta != 600 {
		t.Fatalf("rolled-up net_rx_delta = %d, want 600 (the NULL row is not a zero reading)", delta)
	}
}
