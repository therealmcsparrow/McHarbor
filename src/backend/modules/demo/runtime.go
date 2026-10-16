// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/computesim"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
	"github.com/therealmcsparrow/mcharbor/core/demo/kubeapi"
	"github.com/therealmcsparrow/mcharbor/core/demo/swarmsim"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/version"
	modagent "github.com/therealmcsparrow/mcharbor/modules/agent"
)

// minAgentBuild is the build the simulated agents report at least, so every
// build-gated agent feature is available to the demo whatever BUILD says.
const minAgentBuild = "202609271429"

// Runtime is the running simulation: the estate, its fakes, and the listeners
// the demo agents and the fake Kubernetes API are served on.
type Runtime struct {
	estate  *estate.Estate
	compute *computesim.Sim
	kube    *kubeapi.Sim
	swarm   *swarmsim.Sim

	mu     sync.Mutex
	counts Counts
}

// Counts sizes the simulation for the status endpoint.
func (rt *Runtime) Counts() *Counts {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	c := rt.counts
	c.Containers = rt.estate.ContainerCount()
	return &c
}

// Start seeds demo.db for the simulated estate and starts its agents and the
// fake Kubernetes API. It runs after the plugin runtime has booted, because
// the compute plugins add columns at Init that the seed writes.
func Start(ctx context.Context, app *router.AppDeps, logger *slog.Logger) (*Runtime, error) {
	m := coredemo.Current()
	if m == nil {
		return nil, nil
	}
	now := time.Now()
	cfg := m.Effective()
	rt := &Runtime{
		estate: estate.BuildWith(estate.DefaultSeed, now, estate.Options{Environments: cfg.Environments, Containers: cfg.Containers}),
		kube:   kubeapi.NewSized(estate.DefaultSeed, now, kubeapi.Size{Clusters: cfg.KubernetesClusters, Deployments: cfg.KubernetesDeployments}),
	}
	rt.swarm = swarmsim.New(rt.estate, estate.DefaultSeed, now, swarmsim.Config{Clusters: cfg.SwarmClusters, Nodes: cfg.SwarmNodes, Services: cfg.SwarmServices})
	rt.compute = computesim.NewSized(rt.estate, estate.DefaultSeed, now, computesim.Size{VMs: cfg.VirtualMachines, SystemContainers: cfg.SystemContainers})

	// The preview host joins after the compute and swarm sims are built, so it
	// carries neither VMs nor a swarm: it is a throw-away PR environment.
	previewKeys, err := addPreviewHosts(ctx, app.DB, rt.estate, now)
	if err != nil {
		logger.Warn("demo: preview environments not added", "error", err)
	}
	envIDs, err := seedEnvironments(ctx, app.DB, app.Encryption, rt.estate, m)
	if err != nil {
		return nil, err
	}
	stacks, err := seedStacks(ctx, app.DB, app.Config.DataDir, rt.estate, envIDs)
	if err != nil {
		return nil, err
	}
	if seeded, err := seedGitOps(ctx, app.DB, app.Encryption, app.Config.DataDir, rt.estate, envIDs, previewKeys, now); err != nil {
		logger.Warn("demo: git and gitops not seeded", "error", err)
	} else if seeded.Repos > 0 {
		logger.Info("demo: git and gitops seeded", "repos", seeded.Repos, "pipelines", seeded.Pipelines,
			"promotions", seeded.Promotions, "approvals", seeded.Approvals, "previews", seeded.Previews)
	}
	gitStacks, err := standUpGitOpsStacks(ctx, app.DB, rt.estate, envIDs)
	if err != nil {
		logger.Warn("demo: gitops stacks not stood up", "error", err)
	}
	historyEnvIDs := make(map[string]string, len(envIDs))
	for key, id := range envIDs {
		if !containsKey(previewKeys, key) {
			historyEnvIDs[key] = id
		}
	}
	if err := rt.compute.SeedDB(ctx, app.DB, envIDs); err != nil {
		logger.Warn("demo: compute plugins not seeded", "error", err)
	}
	if err := rt.swarm.SeedDB(ctx, app.DB, envIDs); err != nil {
		logger.Warn("demo: swarm not seeded", "error", err)
	}
	// The protection seed reads the VM and system-container rows the compute sim
	// just wrote, so it has to run after SeedDB rather than beside it.
	if seeded, err := seedBackups(ctx, app.DB, rt.estate, envIDs, now); err != nil {
		logger.Warn("demo: backups and snapshot schedules not seeded", "error", err)
	} else if seeded.ContainerPlans+seeded.VMPlans+seeded.CTPlans > 0 {
		logger.Info("demo: backups and snapshot schedules seeded",
			"locations", seeded.Locations,
			"containerPlans", seeded.ContainerPlans, "containerRuns", seeded.ContainerRuns,
			"vmPlans", seeded.VMPlans, "vmRuns", seeded.VMRuns, "vmSnapshotSchedules", seeded.VMSchedules,
			"ctPlans", seeded.CTPlans, "ctRuns", seeded.CTRuns)
	}
	if seeded, err := seedRunbooks(ctx, app.DB, now); err != nil {
		logger.Warn("demo: runbooks not seeded", "error", err)
	} else if seeded.Runbooks > 0 {
		logger.Info("demo: runbooks seeded", "runbooks", seeded.Runbooks, "steps", seeded.Steps, "runs", seeded.Runs)
	}
	// After seedGitOps, because two watches read their intent from the demo repo.
	if seeded, err := seedReconciler(ctx, app.DB, rt.estate, envIDs, now); err != nil {
		logger.Warn("demo: reconciler watches not seeded", "error", err)
	} else if seeded.Watches > 0 {
		logger.Info("demo: reconciler watches seeded",
			"sources", seeded.Sources, "watches", seeded.Watches, "drifted", seeded.Drifted)
	}
	if err := rt.startKube(ctx, app, m, logger); err != nil {
		logger.Warn("demo: kubernetes plugin not seeded", "error", err)
	}
	if err := rt.startAgents(ctx, app, m, logger); err != nil {
		return nil, err
	}
	if err := seedHistoryFor(ctx, app.DB, rt.estate, historyEnvIDs, now, cfg.HistoryDays, instanceSecret(app)); err != nil {
		logger.Warn("demo: history not seeded", "error", err)
	}
	// After the history seed, because the demo team it creates owns the secrets.
	if seeded, err := seedVault(ctx, app.DB, app.Encryption, rt.estate, historyEnvIDs, m, now); err != nil {
		logger.Warn("demo: vault secrets not seeded", "error", err)
	} else if seeded.Secrets > 0 {
		logger.Info("demo: vault secrets seeded",
			"secrets", seeded.Secrets, "versions", seeded.Versions, "references", seeded.References)
	}

	vms, cts, _, _ := rt.compute.Counts()
	_, _, _, pods := rt.kube.Counts()
	swarmClusters, swarmNodes, swarmServices, _ := rt.swarm.Counts()
	rt.counts = Counts{
		Environments:     len(rt.estate.Hosts) - len(previewKeys),
		Stacks:           stacks + gitStacks,
		VirtualMachines:  vms,
		SystemContainers: cts,
		Clusters:         len(rt.kube.Clusters()),
		Pods:             pods,
		HistoryDays:      cfg.HistoryDays,
		SwarmClusters:    swarmClusters,
		SwarmNodes:       swarmNodes,
		SwarmServices:    swarmServices,
	}
	logger.Info("demo mode running", "environments", rt.counts.Environments, "containers", rt.estate.ContainerCount(), "vms", vms, "systemContainers", cts, "clusters", rt.counts.Clusters, "pods", pods, "swarmClusters", swarmClusters, "swarmServices", swarmServices)
	return rt, nil
}

func (rt *Runtime) startKube(ctx context.Context, app *router.AppDeps, m *coredemo.Marker, logger *slog.Logger) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listening for the fake kubernetes api: %w", err)
	}
	tokens := make(map[string]string)
	for _, c := range rt.kube.Clusters() {
		tokens[m.ClusterToken(c.Key)] = c.Key
	}
	srv := &http.Server{Handler: rt.kube.Handler(tokens), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("demo: fake kubernetes api stopped", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		if err := srv.Close(); err != nil {
			logger.Debug("demo: closing fake kubernetes api", "error", err)
		}
	}()
	url := "http://" + ln.Addr().String()
	if err := kubeapi.SeedDB(ctx, app.DB, url, rt.kube.Clusters(), m.ClusterToken); err != nil {
		return err
	}
	if _, err := app.DB.ExecContext(ctx, "UPDATE plugin_kubernetes_clusters SET server_url = ? WHERE server_url LIKE 'http://127.0.0.1:%'", url); err != nil {
		return fmt.Errorf("pointing demo clusters at this boot's listener: %w", err)
	}
	return nil
}

func (rt *Runtime) startAgents(ctx context.Context, app *router.AppDeps, m *coredemo.Marker, logger *slog.Logger) error {
	svc, ok := app.LookupService(modagent.AgentWSServiceName)
	handler, isHandler := svc.(http.Handler)
	if !ok || !isHandler {
		return errors.New("the agent websocket handler is not registered")
	}
	pl := newPipeListener()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(pl); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, errListenerClosed) {
			logger.Warn("demo: agent listener stopped", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		if err := srv.Close(); err != nil {
			logger.Debug("demo: closing agent listener", "error", err)
		}
	}()
	build := version.Build()
	if len(build) != len(minAgentBuild) || build < minAgentBuild {
		build = minAgentBuild
	}
	for i, h := range rt.estate.Hosts {
		host := h
		remote := fmt.Sprintf("10.%d.%d.%d:%d", 10+estate.StableHash(host.Key)%40, estate.StableHash(host.Key)>>8%250, estate.StableHash(host.Key)>>16%250+2, 40000+i)
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
			NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return pl.dial(ctx, remote)
			},
		}
		agent := fakeagent.New(fakeagent.Config{
			Host:       host,
			Token:      m.AgentToken(host.Key),
			Version:    version.Current(),
			Build:      build,
			Logger:     logger,
			Extensions: []fakeagent.Extension{rt.compute},
			Swarm:      rt.swarm.Backend(host.Key),
			Dial: func(ctx context.Context, token string) (*websocket.Conn, error) {
				conn, resp, err := dialer.DialContext(ctx, "ws://demo-agents.internal/api/agent/ws?token="+token, nil)
				if resp != nil && resp.Body != nil {
					logCloseErr(resp.Body.Close())
				}
				return conn, err
			},
		})
		go func(delay time.Duration) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			agent.Run(ctx)
		}(time.Duration(i) * 150 * time.Millisecond)
	}
	return nil
}

// instanceSecret is the loaded config's shared secret, which seals the seeded audit rows;
// empty when no config is wired, which leaves them unsealed.
func instanceSecret(app *router.AppDeps) string {
	if app == nil || app.Config == nil {
		return ""
	}
	return app.Config.Secret
}
