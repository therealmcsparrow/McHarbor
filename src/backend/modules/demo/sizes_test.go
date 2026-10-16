// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/computesim"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/kubeapi"
)

var sizeNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func TestDefaultConfigMatchesTheGenerators(t *testing.T) {
	def := coredemo.DefaultConfig()
	if def.Environments != estate.DefaultEnvironments() {
		t.Errorf("environments default %d, estate builds %d", def.Environments, estate.DefaultEnvironments())
	}
	if def.Containers != estate.DefaultContainers() {
		t.Errorf("containers default %d, estate targets %d", def.Containers, estate.DefaultContainers())
	}
	if def.KubernetesClusters != kubeapi.DefaultClusters() || def.KubernetesDeployments != kubeapi.DefaultDeployments() {
		t.Errorf("kubernetes default %d/%d, generator %d/%d", def.KubernetesClusters, def.KubernetesDeployments, kubeapi.DefaultClusters(), kubeapi.DefaultDeployments())
	}
	vms, cts, _, _ := computesim.New(estate.Build(estate.DefaultSeed, sizeNow), estate.DefaultSeed, sizeNow).Counts()
	if def.VirtualMachines != vms || def.SystemContainers != cts {
		t.Errorf("compute default %d/%d, generator %d/%d", def.VirtualMachines, def.SystemContainers, vms, cts)
	}
}

func TestSizesAreHonoured(t *testing.T) {
	small := estate.BuildWith(estate.DefaultSeed, sizeNow, estate.Options{Environments: 3, Containers: 90})
	if len(small.Hosts) != 3 {
		t.Fatalf("hosts = %d, want 3", len(small.Hosts))
	}
	if n := small.ContainerCount(); n < 45 || n > 180 {
		t.Fatalf("containers = %d, want about 90", n)
	}
	vms, cts, _, _ := computesim.NewSized(small, estate.DefaultSeed, sizeNow, computesim.Size{VMs: 17, SystemContainers: 5}).Counts()
	if vms != 17 || cts != 5 {
		t.Fatalf("compute = %d/%d, want exactly 17/5", vms, cts)
	}
	none, _, _, _ := computesim.NewSized(small, estate.DefaultSeed, sizeNow, computesim.Size{VMs: 0, SystemContainers: 0}).Counts()
	if none != 0 {
		t.Fatalf("zero VMs asked, %d built", none)
	}

	big := estate.BuildWith(estate.DefaultSeed, sizeNow, estate.Options{Environments: 20})
	if len(big.Hosts) != 20 {
		t.Fatalf("hosts = %d, want 20", len(big.Hosts))
	}
	seen := map[string]bool{}
	for _, h := range big.Hosts {
		if seen[h.Key] {
			t.Fatalf("duplicate host key %s when cloning specs", h.Key)
		}
		seen[h.Key] = true
	}

	k := kubeapi.NewSized(estate.DefaultSeed, sizeNow, kubeapi.Size{Clusters: 1, Deployments: 40})
	if len(k.Clusters()) != 1 {
		t.Fatalf("clusters = %d, want 1", len(k.Clusters()))
	}
	k5 := kubeapi.NewSized(estate.DefaultSeed, sizeNow, kubeapi.Size{Clusters: 5, Deployments: 0})
	if len(k5.Clusters()) != 5 {
		t.Fatalf("clusters = %d, want 5", len(k5.Clusters()))
	}
}

func TestEnterRefusesInvalidSizesAndRebuildsOnChange(t *testing.T) {
	dir := t.TempDir()
	src := sourceDB(t, dir)
	s := NewService(src, dir, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	ctx := context.Background()

	bad := coredemo.DefaultConfig()
	bad.HistoryDays = 1
	var fe *coredemo.FieldError
	if _, err := s.Enter(ctx, "u1", &bad); !errors.As(err, &fe) || fe.Field != "historyDays" {
		t.Fatalf("invalid sizes: %v", err)
	}

	cfg := coredemo.DefaultConfig()
	cfg.Environments = 4
	if _, err := s.Enter(ctx, "u1", &cfg); err != nil {
		t.Fatal(err)
	}
	m, err := coredemo.Load(dir)
	if err != nil || m == nil || m.Effective() != cfg {
		t.Fatalf("marker sizes = %+v, %v", m, err)
	}
	secret := m.Secret
	if err := coredemo.ParkMarker(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := coredemo.Load(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Enter(ctx, "u1", nil); err != nil {
		t.Fatal(err)
	}
	again, err := coredemo.Load(dir)
	if err != nil || again.Secret != secret || again.Effective() != cfg {
		t.Fatal("re-entering with the same sizes must reuse the parked demo")
	}
	if err := coredemo.ParkMarker(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := coredemo.Load(dir); err != nil {
		t.Fatal(err)
	}

	cfg.HistoryDays = 30
	if _, err := s.Enter(ctx, "u1", &cfg); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := coredemo.Load(dir)
	if err != nil || rebuilt.Secret == secret || rebuilt.Effective().HistoryDays != 30 {
		t.Fatal("changed sizes must rebuild demo.db with a fresh secret")
	}
	if _, err := coredemo.Load(filepath.Join(dir, "missing")); err != nil {
		t.Fatal(err)
	}
}
