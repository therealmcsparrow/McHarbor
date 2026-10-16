// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
)

func TestHostLinkStoreRoundTrip(t *testing.T) {
	store := newHostLinkStore(t.TempDir())
	rec := hostLinkRecord{
		Owner: HostLinkOwnerDocker, OwnerID: "3f2a9c", Name: "lan0", Parent: "eth0",
		IPv4CIDR: "192.168.1.5/24", IPv6Enabled: true, IPv6CIDR: "fd00::5/64",
	}
	if err := store.save(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.save(hostLinkRecord{Owner: HostLinkOwnerCompute, OwnerID: "net1", Parent: "eth1", VLANTag: 100}); err != nil {
		t.Fatal(err)
	}

	got, err := store.load(HostLinkOwnerDocker, "3f2a9c")
	if err != nil || got == nil {
		t.Fatalf("load = %v, %v", got, err)
	}
	if got.Name != "lan0" || got.IPv6CIDR != "fd00::5/64" || !got.IPv6Enabled || got.SavedAt.IsZero() {
		t.Fatalf("record did not round-trip: %+v", got)
	}
	if stale := got.staleCIDRs(); len(stale) != 2 {
		t.Fatalf("stale = %v, want both applied addresses", stale)
	}

	all, err := store.list()
	if err != nil || len(all) != 2 {
		t.Fatalf("list = %v, %v", all, err)
	}
	if all[0].Owner != HostLinkOwnerCompute || all[0].VLANTag != 100 {
		t.Fatalf("list order or VLAN tag lost: %+v", all)
	}

	if err := store.forget(HostLinkOwnerDocker, "3f2a9c"); err != nil {
		t.Fatal(err)
	}
	if err := store.forget(HostLinkOwnerDocker, "3f2a9c"); err != nil {
		t.Fatalf("forgetting twice must not fail: %v", err)
	}
	if got, err := store.load(HostLinkOwnerDocker, "3f2a9c"); got != nil || err != nil {
		t.Fatalf("forgotten record still loads: %v, %v", got, err)
	}
}

func TestHostLinkStoreRefusesPathsOutsideItsDir(t *testing.T) {
	store := newHostLinkStore(t.TempDir())
	for _, id := range []string{"../escape", "a/b", `a\b`, "..", "", "x:y"} {
		if err := store.save(hostLinkRecord{Owner: HostLinkOwnerDocker, OwnerID: id}); !errors.Is(err, netconf.ErrHostLinkInvalid) {
			t.Errorf("owner id %q: save = %v, want ErrHostLinkInvalid", id, err)
		}
	}
	if _, err := store.path("k8s", "abc"); !errors.Is(err, netconf.ErrHostLinkInvalid) {
		t.Errorf("unknown owner accepted: %v", err)
	}
}

func TestHostLinkStoreSkipsUnreadableRecords(t *testing.T) {
	dir := t.TempDir()
	store := newHostLinkStore(dir)
	if err := store.save(hostLinkRecord{Owner: HostLinkOwnerDocker, OwnerID: "good", Parent: "eth0"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, hostLinkDirName, "docker-bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	all, err := store.list()
	if len(all) != 1 || err == nil {
		t.Fatalf("list = %v, %v; want the good record and an error naming the bad one", all, err)
	}
}

func TestHostLinkErrorCode(t *testing.T) {
	cases := map[error]string{
		nil: "",
		fmt.Errorf("wrap: %w", netconf.ErrHostLinkNameTaken): HostLinkErrNameTaken,
		fmt.Errorf("wrap: %w", netconf.ErrParentNotFound):    HostLinkErrParentNotFound,
		fmt.Errorf("wrap: %w", netconf.ErrHostLinkInvalid):   HostLinkErrInvalid,
		netconf.ErrNotLinux:                      HostLinkErrNotLinux,
		fmt.Errorf("netlink: %w", syscall.EPERM): HostLinkErrPermissionDenied,
		errors.New("something else"):             "",
	}
	for err, want := range cases {
		if got := hostLinkErrorCode(err); got != want {
			t.Errorf("hostLinkErrorCode(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestHostLinkRetryDelayBacksOffToTheCeiling(t *testing.T) {
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second}
	for attempt, w := range want {
		if got := hostLinkRetryDelay(attempt); got != w {
			t.Errorf("attempt %d: delay %s, want %s", attempt, got, w)
		}
	}
}

func TestHostLinkReplayRetriesOnlyAMissingParent(t *testing.T) {
	store := newHostLinkStore(t.TempDir())
	for _, id := range []string{"late", "broken", "fine", "gone"} {
		if err := store.save(hostLinkRecord{Owner: HostLinkOwnerDocker, OwnerID: id, Parent: "eth0"}); err != nil {
			t.Fatal(err)
		}
	}
	records, err := store.list()
	if err != nil {
		t.Fatal(err)
	}
	// Removed after startup read the store: the replay must not resurrect it.
	if err := store.forget(HostLinkOwnerDocker, "gone"); err != nil {
		t.Fatal(err)
	}

	calls := map[string]int{}
	replayer := hostLinkReplayer{
		store:  store,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		apply: func(rec hostLinkRecord) error {
			calls[rec.OwnerID]++
			switch rec.OwnerID {
			case "late":
				if calls["late"] < 3 {
					return fmt.Errorf("%w: eth0", netconf.ErrParentNotFound)
				}
			case "broken":
				return netconf.ErrHostLinkNameTaken
			}
			return nil
		},
		delay:  func(int) time.Duration { return time.Millisecond },
		window: time.Minute,
	}
	replayer.run(context.Background(), records)

	if calls["late"] != 3 {
		t.Errorf("late parent applied %d times, want 3 (two misses, then success)", calls["late"])
	}
	if calls["broken"] != 1 || calls["fine"] != 1 {
		t.Errorf("non-parent failures must not be retried: %v", calls)
	}
	if calls["gone"] != 0 {
		t.Errorf("a record removed after startup was replayed")
	}
}

func TestHostLinkReplayGivesUpAfterTheWindow(t *testing.T) {
	store := newHostLinkStore(t.TempDir())
	if err := store.save(hostLinkRecord{Owner: HostLinkOwnerCompute, OwnerID: "n1", Parent: "eth9"}); err != nil {
		t.Fatal(err)
	}
	records, err := store.list()
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	hostLinkReplayer{
		store:  store,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		apply: func(hostLinkRecord) error {
			attempts++
			return netconf.ErrParentNotFound
		},
		delay:  func(int) time.Duration { return time.Hour },
		window: time.Minute,
	}.run(context.Background(), records)
	if attempts != 1 {
		t.Fatalf("attempts = %d; a wait past the window must end the replay, not block", attempts)
	}
}
