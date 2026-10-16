// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/compute/imagefetch"
	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"github.com/vishvananda/netlink"
)

func TestDeleteCTRefusesRootfsRemovalWithoutSpec(t *testing.T) {
	rt := &computeRuntime{}
	err := rt.deleteCT(context.Background(), &CTPayload{ContainerID: "ct1", DeleteRootfs: true})
	if !errors.Is(err, errCTDeleteNoSpec) {
		t.Fatalf("deleteCT without a spec = %v, want errCTDeleteNoSpec", err)
	}
}

func TestDeleteCTNeverRemovesThePoolRoot(t *testing.T) {
	pool := t.TempDir()
	marker := filepath.Join(pool, "vm-disk.qcow2")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt := &computeRuntime{}
	for _, rootfs := range []string{pool, "/", filepath.Dir(pool)} {
		err := rt.deleteCT(context.Background(), &CTPayload{
			ContainerID:  "ct1",
			DeleteRootfs: true,
			Spec:         &CTSpec{Name: "ct1", RootfsPath: rootfs, PoolTarget: pool, PoolBackend: "dir"},
		})
		if !errors.Is(err, lxc.ErrRootfsGuard) {
			t.Fatalf("deleteCT of %q = %v, want ErrRootfsGuard", rootfs, err)
		}
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the pool's contents were touched: %v", err)
	}
}

func TestDeleteCTRemovesRootfsAndNICs(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	const ctID = "d0testct0000000000ab"
	hostName, _ := netconf.VethNames(ctID, 0)
	if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: hostName}}); err != nil {
		t.Skipf("cannot create a test link (needs CAP_NET_ADMIN): %v", err)
	}
	t.Cleanup(func() {
		if link, err := netlink.LinkByName(hostName); err == nil {
			errlog.Log(netlink.LinkDel(link))
		}
	})

	pool := t.TempDir()
	rootfs := filepath.Join(pool, "containers", ctID, "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "etc", "hostname"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &computeRuntime{
		cts:   lxc.NewManager(nil),
		pools: storage.NewRegistry(storage.NewDirDriver()),
	}
	err := rt.deleteCT(context.Background(), &CTPayload{
		ContainerID:  ctID,
		DeleteRootfs: true,
		Spec: &CTSpec{
			Name: "e2e", RootfsPath: rootfs, PoolTarget: pool, PoolBackend: "dir",
			NICs: []CTNICSpec{{Slot: 0, MAC: "52:54:00:00:00:01"}},
		},
	})
	if err != nil {
		t.Fatalf("deleteCT: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(pool, "containers", ctID)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the rootfs or its container directory survived the delete: %v", err)
	}
	if _, err := os.Stat(pool); err != nil {
		t.Fatalf("the pool root went with the container: %v", err)
	}
	if _, err := netlink.LinkByName(hostName); err == nil {
		t.Fatalf("the container's host link %s survived the delete", hostName)
	}
}

func rootfsImage(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	writer := tar.NewWriter(gz)
	body := "ID=alpine\n"
	if err := writer.WriteHeader(&tar.Header{Name: "etc/os-release", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPopulateCTRootfsFromCachedImage(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	cache := t.TempDir()
	images, err := imagefetch.New(cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "img-alpine-rootfs.gz"), rootfsImage(t), 0o600); err != nil {
		t.Fatal(err)
	}
	rt := &computeRuntime{images: images}
	pool := t.TempDir()
	spec := &CTSpec{
		Name: "ct", RootfsPath: filepath.Join(pool, "containers", "ct1", "rootfs"), PoolTarget: pool,
		UIDMapStart: 100000, UIDMapCount: 65536, GIDMapStart: 100000, GIDMapCount: 65536,
	}
	req := &CTPayload{ContainerID: "ct1", ImageID: "img-alpine-rootfs", Spec: spec}

	populated, err := rt.populateCTRootfs(context.Background(), req, toCTSpec("ct1", spec))
	if err != nil || !populated {
		t.Fatalf("populateCTRootfs = %v, %v", populated, err)
	}
	if body, err := os.ReadFile(filepath.Join(spec.RootfsPath, "etc", "os-release")); err != nil || string(body) != "ID=alpine\n" {
		t.Fatalf("the image was not unpacked: %q %v", body, err)
	}

	req.ImageID = "img-not-cached"
	spec.RootfsPath = filepath.Join(pool, "containers", "ct2", "rootfs")
	if _, err := rt.populateCTRootfs(context.Background(), req, toCTSpec("ct2", spec)); err == nil ||
		!strings.Contains(err.Error(), "not in this host's image cache") {
		t.Fatalf("an uncached image = %v, want a message naming the cache", err)
	}

	req.ImageID = "img-alpine-rootfs"
	spec.RootfsPath = "/etc/evil/rootfs"
	if _, err := rt.populateCTRootfs(context.Background(), req, toCTSpec("ct3", spec)); !errors.Is(err, lxc.ErrRootfsGuard) {
		t.Fatalf("an image unpacked outside its pool = %v, want ErrRootfsGuard", err)
	}
}

func TestRemoveEmptyContainerDirPrunesContainersOnlyWhenEmpty(t *testing.T) {
	pool := t.TempDir()
	vmDisk := filepath.Join(pool, "vm-disk.qcow2")
	if err := os.WriteFile(vmDisk, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ct1", "ct2"} {
		if err := os.MkdirAll(filepath.Join(pool, "containers", id), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	removeEmptyContainerDir("ct1", filepath.Join(pool, "containers", "ct1", "rootfs"), pool)
	if _, err := os.Stat(filepath.Join(pool, "containers", "ct1")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the empty per-container dir survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pool, "containers", "ct2")); err != nil {
		t.Fatalf("another container's dir was touched: %v", err)
	}

	removeEmptyContainerDir("ct2", filepath.Join(pool, "containers", "ct2", "rootfs"), pool)
	if _, err := os.Stat(filepath.Join(pool, "containers")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the empty containers/ dir survived the last delete: %v", err)
	}
	if _, err := os.Stat(vmDisk); err != nil {
		t.Fatalf("the pool root's contents were touched: %v", err)
	}
	if _, err := os.Stat(pool); err != nil {
		t.Fatalf("the pool root was removed: %v", err)
	}
}

func TestRemoveEmptyContainerDirNeverRemovesAnEmptyPoolRoot(t *testing.T) {
	parent := t.TempDir()
	pool := filepath.Join(parent, "pool")
	if err := os.MkdirAll(filepath.Join(pool, "containers", "ct1"), 0o755); err != nil {
		t.Fatal(err)
	}
	removeEmptyContainerDir("ct1", filepath.Join(pool, "containers", "ct1", "rootfs"), pool)
	if _, err := os.Stat(pool); err != nil {
		t.Fatalf("an empty pool root was removed: %v", err)
	}
	if entries := errlog.Value(os.ReadDir(pool)); len(entries) != 0 {
		t.Fatalf("pool root left with %d entries, want none", len(entries))
	}
}
