// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dockerapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	cerrdefs "github.com/containerd/errdefs"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

func sdk(t *testing.T, hostKey string) (*client.Client, *estate.Host) {
	t.Helper()
	host := estate.Build(estate.DefaultSeed, time.Now()).Host(hostKey)
	srv := httptest.NewServer(New(host))
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://" + strings.TrimPrefix(srv.URL, "http://")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cli.Close(); err != nil {
			t.Log(err)
		}
	})
	return cli, host
}

func TestSDKReadsTheSimulatedDaemon(t *testing.T) {
	cli, host := sdk(t, "prod-eu-1")
	ctx := context.Background()

	ping, err := cli.Ping(ctx, client.PingOptions{})
	if err != nil || ping.APIVersion != APIVersion {
		t.Fatalf("ping = %+v, %v", ping, err)
	}
	infoRes, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info := infoRes.Info
	if info.Containers != len(host.Containers()) || info.NCPU != host.CPUs || info.OperatingSystem == "" {
		t.Fatalf("info = containers %d cpus %d os %q", info.Containers, info.NCPU, info.OperatingSystem)
	}
	if _, err := cli.ServerVersion(ctx, client.ServerVersionOptions{}); err != nil {
		t.Fatal(err)
	}

	allRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	all := allRes.Items
	if len(all) != len(host.Containers()) {
		t.Fatalf("list all = %d, want %d", len(all), len(host.Containers()))
	}
	runningRes, err := cli.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	running := runningRes.Items
	if len(running) == 0 || len(running) >= len(all) {
		t.Fatalf("running = %d of %d; the default list must hide stopped containers", len(running), len(all))
	}
	project := all[0].Labels["com.docker.compose.project"]
	byLabelRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("label", "com.docker.compose.project="+project)})
	if err != nil {
		t.Fatal(err)
	}
	byLabel := byLabelRes.Items
	for _, c := range byLabel {
		if c.Labels["com.docker.compose.project"] != project {
			t.Fatalf("label filter leaked %s", c.Names[0])
		}
	}

	inspectRes, err := cli.ContainerInspect(ctx, all[0].ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	inspect := inspectRes.Container
	if inspect.Name != all[0].Names[0] || inspect.Config == nil || inspect.State == nil {
		t.Fatalf("inspect = %+v", inspect)
	}

	imgsRes, err := cli.ImageList(ctx, client.ImageListOptions{All: true})
	imgs := imgsRes.Items
	if err != nil || len(imgs) == 0 {
		t.Fatalf("images = %d, %v", len(imgs), err)
	}
	if _, err := cli.ImageInspect(ctx, imgs[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ImageHistory(ctx, imgs[0].ID); err != nil {
		t.Fatal(err)
	}
	volsRes, err := cli.VolumeList(ctx, client.VolumeListOptions{})
	vols := volsRes.Items
	if err != nil || len(vols) == 0 {
		t.Fatalf("volumes = %v, %v", vols, err)
	}
	netsRes, err := cli.NetworkList(ctx, client.NetworkListOptions{})
	nets := netsRes.Items
	if err != nil || len(nets) < 3 {
		t.Fatalf("networks = %d, %v", len(nets), err)
	}
	if _, err := cli.NetworkInspect(ctx, nets[len(nets)-1].ID, client.NetworkInspectOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.DiskUsage(ctx, client.DiskUsageOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestSDKLifecycleChangesState(t *testing.T) {
	cli, host := sdk(t, "dev")
	ctx := context.Background()
	var target estate.Container
	for _, c := range host.Containers() {
		if c.State == estate.StateRunning {
			target = c
			break
		}
	}
	if _, err := cli.ContainerStop(ctx, target.ID, client.ContainerStopOptions{}); err != nil {
		t.Fatal(err)
	}
	gotRes, err := cli.ContainerInspect(ctx, target.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := gotRes.Container
	if got.State.Running {
		t.Fatal("container still running after stop")
	}
	if _, err := cli.ContainerRemove(ctx, target.ID, client.ContainerRemoveOptions{}); err != nil {
		t.Fatalf("removing a stopped container: %v", err)
	}
	if _, err := cli.ContainerInspect(ctx, target.ID, client.ContainerInspectOptions{}); !cerrdefs.IsNotFound(err) {
		t.Fatalf("inspect after remove = %v, want not found", err)
	}

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: "postgres:16.4"}, Name: "demo-probe"})
	if err != nil {
		t.Fatalf("create from an estate image: %v", err)
	}
	if _, err := cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	if c, err := host.Container("demo-probe"); err != nil || c.State != estate.StateRunning {
		t.Fatalf("created container = %+v, %v", c, err)
	}
}

func TestSDKStreamsLogsStatsAndEvents(t *testing.T) {
	cli, host := sdk(t, "homelab")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var target estate.Container
	for _, c := range host.Containers() {
		if c.State == estate.StateRunning {
			target = c
			break
		}
	}

	rc, err := cli.ContainerLogs(ctx, target.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "25", Timestamps: true})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	if _, err := stdcopy.StdCopy(&stdout, &stderr, rc); err != nil {
		t.Fatalf("demultiplexing logs: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(stdout.String()+stderr.String(), "\n"); lines != 25 {
		t.Fatalf("tail=25 returned %d lines", lines)
	}

	stats, err := cli.ContainerStats(ctx, target.ID, client.ContainerStatsOptions{Stream: false})
	if err != nil {
		t.Fatal(err)
	}
	var doc container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if err := stats.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if doc.MemoryStats.Usage == 0 || doc.CPUStats.CPUUsage.TotalUsage <= doc.PreCPUStats.CPUUsage.TotalUsage {
		t.Fatalf("stats give the UI nothing to chart: %+v", doc.CPUStats)
	}

	stream := cli.Events(ctx, client.EventsListOptions{})
	time.Sleep(50 * time.Millisecond)
	if _, err := host.RestartContainer(target.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-stream.Messages:
		if m.Actor.ID != target.ID {
			t.Fatalf("event for %s, want %s", m.Actor.ID, target.ID)
		}
	case err := <-stream.Err:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("no event after a restart")
	}

	pull, err := cli.ImagePull(ctx, "alpine:3.20", client.ImagePullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(pull)
	n := 0
	for sc.Scan() {
		n++
	}
	if err := pull.Close(); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("pull printed no progress")
	}
	if _, err := host.Image("alpine:3.20"); err != nil {
		t.Fatalf("pulled image missing: %v", err)
	}
}
