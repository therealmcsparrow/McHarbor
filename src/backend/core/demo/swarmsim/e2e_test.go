// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"bytes"
	"context"
	cerrdefs "github.com/containerd/errdefs"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/demo/dockerapi"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

func sdkFor(t *testing.T, e *estate.Estate, s *Sim, key string) *client.Client {
	t.Helper()
	srv := dockerapi.New(e.Host(key))
	if b := s.Backend(key); b != nil {
		srv.SetSwarm(b)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	cli, err := client.New(client.WithHost("tcp://" + strings.TrimPrefix(ts.URL, "http://")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cli.Close(); err != nil {
			t.Log(err)
		}
	})
	return cli
}

func runningOn(t *testing.T, cli *client.Client, key, value string) []swarm.Task {
	t.Helper()
	tasksRes, err := cli.TaskList(context.Background(), client.TaskListOptions{Filters: make(client.Filters).Add(key, value).Add("desired-state", "running")})
	if err != nil {
		t.Fatal(err)
	}
	tasks := tasksRes.Items
	return tasks
}

func TestSDKDrivesTheSwarm(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	e := estate.Build(estate.DefaultSeed, now)
	s := New(e, estate.DefaultSeed, now, DefaultConfig())
	clk := &clock{t: now}
	s.SetClock(clk.now)
	cli := sdkFor(t, e, s, "swarm-production")
	ctx := context.Background()

	infoRes, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info := infoRes.Info
	if info.Swarm.LocalNodeState != swarm.LocalNodeStateActive || !info.Swarm.ControlAvailable || info.Swarm.NodeID == "" || info.Swarm.Cluster == nil || info.Swarm.Managers != 3 {
		t.Fatalf("info.Swarm = %+v", info.Swarm)
	}
	swRes, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{})
	sw := swRes.Swarm
	if err != nil || sw.ID != info.Swarm.Cluster.ID || !strings.HasPrefix(sw.JoinTokens.Worker, "SWMTKN-1-") {
		t.Fatalf("swarm inspect = %+v, %v", sw, err)
	}
	if key, err := cli.SwarmGetUnlockKey(ctx); err != nil || !strings.HasPrefix(key.Key, "SWMKEY-1-") {
		t.Fatalf("unlock key = %+v, %v", key, err)
	}
	history := int64(8)
	sw.Spec.Orchestration.TaskHistoryRetentionLimit = &history
	if _, err := cli.SwarmUpdate(ctx, client.SwarmUpdateOptions{Version: sw.Version, Spec: sw.Spec, RotateWorkerToken: true}); err != nil {
		t.Fatal(err)
	}
	if againRes, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{}); err != nil || againRes.Swarm.Version.Index <= sw.Version.Index || againRes.Swarm.JoinTokens.Worker == sw.JoinTokens.Worker || *againRes.Swarm.Spec.Orchestration.TaskHistoryRetentionLimit != 8 {
		t.Fatalf("swarm after update = %+v, %v", againRes.Swarm, err)
	}
	if _, err := cli.SwarmInit(ctx, client.SwarmInitOptions{ListenAddr: "0.0.0.0:2377"}); err == nil || !strings.Contains(err.Error(), "demo mode") {
		t.Fatalf("swarm init = %v", err)
	}

	nodesRes, err := cli.NodeList(ctx, client.NodeListOptions{})
	nodes := nodesRes.Items
	if err != nil || len(nodes) != 6 {
		t.Fatalf("nodes = %d, %v", len(nodes), err)
	}
	managersRes, err := cli.NodeList(ctx, client.NodeListOptions{Filters: make(client.Filters).Add("role", "manager")})
	managers := managersRes.Items
	if err != nil || len(managers) != 3 {
		t.Fatalf("managers = %d, %v", len(managers), err)
	}

	allRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Status: true})
	all := allRes.Items
	if err != nil || len(all) != 45 || all[0].ServiceStatus == nil {
		t.Fatalf("services = %d, %v", len(all), err)
	}
	shopRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Filters: make(client.Filters).Add("label", "com.docker.stack.namespace=shop")})
	shop := shopRes.Items
	if err != nil || len(shop) != 5 {
		t.Fatalf("shop stack = %d, %v", len(shop), err)
	}
	globalRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Filters: make(client.Filters).Add("mode", "global")})
	global := globalRes.Items
	if err != nil || len(global) == 0 {
		t.Fatalf("global = %d, %v", len(global), err)
	}
	for _, g := range global {
		if g.Spec.Mode.Global == nil {
			t.Fatalf("%s is not global", g.Spec.Name)
		}
	}
	namedRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Filters: make(client.Filters).Add("name", "shop_api")})
	named := namedRes.Items
	if err != nil || len(named) != 1 {
		t.Fatalf("name filter = %d, %v", len(named), err)
	}

	apiRes, err := cli.ServiceInspect(ctx, "shop_api", client.ServiceInspectOptions{InsertDefaults: true})
	if err != nil {
		t.Fatal(err)
	}
	api := apiRes.Service
	five := uint64(5)
	api.Spec.Mode.Replicated.Replicas = &five
	if _, err := cli.ServiceUpdate(ctx, api.ID, client.ServiceUpdateOptions{Version: api.Version, Spec: api.Spec}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ServiceUpdate(ctx, api.ID, client.ServiceUpdateOptions{Version: api.Version, Spec: api.Spec}); err == nil || !strings.Contains(err.Error(), "update out of sequence") {
		t.Fatalf("stale update = %v", err)
	}
	clk.step(10 * time.Second)
	tasks := runningOn(t, cli, "service", api.ID)
	if len(tasks) != 5 {
		t.Fatalf("shop_api tasks = %d", len(tasks))
	}
	for _, tk := range tasks {
		if tk.Status.State != swarm.TaskStateRunning {
			t.Fatalf("task %s is %s after convergence", tk.ID, tk.Status.State)
		}
	}
	node := tasks[0].NodeID
	for _, tk := range runningOn(t, cli, "node", node) {
		if tk.NodeID != node {
			t.Fatalf("node filter returned a task on %s", tk.NodeID)
		}
	}
	if tkRes, err := cli.TaskInspect(ctx, tasks[0].ID, client.TaskInspectOptions{}); err != nil || tkRes.Task.ServiceID != api.ID {
		t.Fatalf("task inspect = %+v, %v", tkRes.Task, err)
	}

	for _, read := range []func() (io.ReadCloser, error){
		func() (io.ReadCloser, error) {
			return cli.ServiceLogs(ctx, api.ID, client.ServiceLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "10", Timestamps: true})
		},
		func() (io.ReadCloser, error) {
			return cli.TaskLogs(ctx, tasks[0].ID, client.TaskLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "5"})
		},
	} {
		rc, err := read()
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if _, err := stdcopy.StdCopy(&out, &errOut, rc); err != nil {
			t.Fatal(err)
		}
		if err := rc.Close(); err != nil {
			t.Fatal(err)
		}
		lines := strings.Count(out.String()+errOut.String(), "\n")
		if lines == 0 || lines > 10 {
			t.Fatalf("log lines = %d", lines)
		}
	}

	secretsRes, err := cli.SecretList(ctx, client.SecretListOptions{})
	secrets := secretsRes.Items
	if err != nil || len(secrets) == 0 || len(secrets[0].Spec.Data) != 0 {
		t.Fatalf("secrets = %d, %v", len(secrets), err)
	}
	configsRes, err := cli.ConfigList(ctx, client.ConfigListOptions{})
	configs := configsRes.Items
	if err != nil || len(configs) == 0 {
		t.Fatalf("configs = %d, %v", len(configs), err)
	}
	if cfgRes, err := cli.ConfigInspect(ctx, configs[0].ID, client.ConfigInspectOptions{}); err != nil || len(cfgRes.Config.Spec.Data) == 0 {
		t.Fatalf("config inspect = %v", err)
	}
	if _, err := cli.SecretRemove(ctx, secrets[len(secrets)-1].ID, client.SecretRemoveOptions{}); err != nil && !strings.Contains(err.Error(), "in use") {
		t.Fatalf("secret remove = %v", err)
	}

	overlaysRes, err := cli.NetworkList(ctx, client.NetworkListOptions{Filters: make(client.Filters).Add("scope", "swarm")})
	overlays := overlaysRes.Items
	if err != nil || len(overlays) < 3 {
		t.Fatalf("overlays = %d, %v", len(overlays), err)
	}
	everythingRes, err := cli.NetworkList(ctx, client.NetworkListOptions{})
	everything := everythingRes.Items
	if err != nil || len(everything) <= len(overlays) {
		t.Fatalf("networks = %d, %v", len(everything), err)
	}

	created, err := cli.NetworkCreate(ctx, "demo_default", client.NetworkCreateOptions{Driver: "overlay", Labels: map[string]string{stackLabel: "demo"}})
	if err != nil {
		t.Fatal(err)
	}
	sec, err := cli.SecretCreate(ctx, client.SecretCreateOptions{Spec: swarm.SecretSpec{Annotations: swarm.Annotations{Name: "demo_token"}, Data: []byte("s3cret")}})
	if err != nil {
		t.Fatal(err)
	}
	three := uint64(3)
	resp, err := cli.ServiceCreate(ctx, client.ServiceCreateOptions{Spec: swarm.ServiceSpec{
		Annotations: swarm.Annotations{Name: "demo_web", Labels: map[string]string{stackLabel: "demo"}},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{Image: "nginx:1.27-alpine", Secrets: []*swarm.SecretReference{{SecretName: "demo_token"}}},
			Networks:      []swarm.NetworkAttachmentConfig{{Target: "demo_default"}},
		},
		Mode: swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &three}},
	}, QueryRegistry: true})
	if err != nil {
		t.Fatal(err)
	}
	clk.step(5 * time.Second)
	if n := len(runningOn(t, cli, "service", resp.ID)); n != 3 {
		t.Fatalf("created service tasks = %d", n)
	}
	webRes, err := cli.ServiceInspect(ctx, resp.ID, client.ServiceInspectOptions{})
	web := webRes.Service
	if err != nil || web.Spec.TaskTemplate.Networks[0].Target != created.ID || web.Spec.TaskTemplate.ContainerSpec.Secrets[0].SecretID != sec.ID {
		t.Fatalf("created service = %+v, %v", web.Spec.TaskTemplate, err)
	}

	var worker swarm.Node
	for _, n := range nodes {
		if n.Description.Hostname == "prod-wrk-02" {
			worker = n
		}
	}
	worker.Spec.Availability = swarm.NodeAvailabilityDrain
	if _, err := cli.NodeUpdate(ctx, worker.ID, client.NodeUpdateOptions{Version: worker.Version, Spec: worker.Spec}); err != nil {
		t.Fatal(err)
	}
	clk.step(10 * time.Second)
	if left := runningOn(t, cli, "node", worker.ID); len(left) != 0 {
		t.Fatalf("%d tasks still desired on the drained node", len(left))
	}

	if _, err := cli.ServiceRemove(ctx, resp.ID, client.ServiceRemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ServiceInspect(ctx, resp.ID, client.ServiceInspectOptions{}); !cerrdefs.IsNotFound(err) {
		t.Fatalf("inspect after remove = %v", err)
	}
	if _, err := cli.NetworkRemove(ctx, created.ID, client.NetworkRemoveOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestPlainHostIsNotASwarm(t *testing.T) {
	now := time.Now()
	e := estate.Build(estate.DefaultSeed, now)
	s := New(e, estate.DefaultSeed, now, DefaultConfig())
	cli := sdkFor(t, e, s, "prod-eu-1")
	ctx := context.Background()
	infoRes, err := cli.Info(ctx, client.InfoOptions{})
	info := infoRes.Info
	if err != nil || info.Swarm.LocalNodeState != swarm.LocalNodeStateInactive {
		t.Fatalf("info.Swarm = %+v, %v", info.Swarm, err)
	}
	if _, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{}); err == nil || !strings.Contains(err.Error(), "not a swarm manager") {
		t.Fatalf("swarm inspect on a plain host = %v", err)
	}
	if _, err := cli.ServiceList(ctx, client.ServiceListOptions{}); err == nil {
		t.Fatal("service list on a plain host must fail")
	}
}
