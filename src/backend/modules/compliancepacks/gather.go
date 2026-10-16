// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	containertypes "github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Measurement bounds. Every Docker SDK call is individually bounded because the
// agent transport has no deadline of its own, and a sweep is many calls: an
// inspect costs ~2s through the agent, so they run concurrently rather than in a
// sequential loop that would take minutes on a busy host.
const (
	dockerCallTimeout = 30 * time.Second
	inspectWorkers    = 8
	maxContainers     = 1000
)

// iccOption is Docker's own option key on the default bridge network. It is
// absent when inter-container communication is at its default of enabled.
const iccOption = "com.docker.network.bridge.enable_icc"

// dockerSocketPaths are the bind sources CIS 5.31 objects to. Both spellings ship
// on real hosts, and a container holding either has host root.
var dockerSocketPaths = map[string]bool{"/var/run/docker.sock": true, "/run/docker.sock": true}

// Gatherer measures one environment. It holds the pool rather than all of
// AppDeps so a test can drive it with a fake client.
type Gatherer struct {
	pool   *docker.ClientPool
	db     *sql.DB
	logger *slog.Logger
}

// NewGatherer returns a Gatherer reading environments through pool; db supplies
// the configured connection type, which is the only evidence for CIS 2.6.
func NewGatherer(pool *docker.ClientPool, database *sql.DB, logger *slog.Logger) *Gatherer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gatherer{pool: pool, db: database, logger: logger}
}

// Gather measures envID. A source that cannot be read leaves its Known flag false
// rather than returning an error, so one unreachable half still produces a run
// with honest unknowns in place of the checks it could not answer.
func (g *Gatherer) Gather(ctx context.Context, envID string) Inputs {
	var in Inputs
	cli, err := g.pool.GetContext(ctx, envID)
	if err != nil {
		g.logger.Warn("compliance pack: docker client unavailable", "environment", envID, "error", err)
		return in
	}
	in.DaemonKnown, in.Daemon = g.gatherDaemon(ctx, envID, cli)
	in.ContainersKnown, in.Containers, in.InspectFailures, in.ContainersCapped = g.gatherContainers(ctx, envID, cli)
	return in
}

func (g *Gatherer) gatherDaemon(ctx context.Context, envID string, cli *dockerclient.Client) (bool, Daemon) {
	infoCtx, cancel := context.WithTimeout(ctx, dockerCallTimeout)
	defer cancel()
	infoRes, err := cli.Info(infoCtx, dockerclient.InfoOptions{})
	info := infoRes.Info
	if err != nil {
		g.logger.Warn("compliance pack: docker info failed", "environment", envID, "error", err)
		return false, Daemon{}
	}
	d := Daemon{
		ServerVersion:   info.ServerVersion,
		StorageDriver:   info.Driver,
		Debug:           info.Debug,
		LoggingDriver:   info.LoggingDriver,
		LiveRestore:     info.LiveRestoreEnabled,
		Experimental:    info.ExperimentalBuild,
		SecurityOptions: info.SecurityOptions,
		AuthzPlugins:    info.Plugins.Authorization,
		SwarmState:      string(info.Swarm.LocalNodeState),
		ConnectionType:  g.connectionType(ctx, envID),
	}
	netCtx, netCancel := context.WithTimeout(ctx, dockerCallTimeout)
	defer netCancel()
	if bridge, err := cli.NetworkInspect(netCtx, "bridge", dockerclient.NetworkInspectOptions{}); err == nil {
		d.BridgeICCKnown = true
		// Absent means enabled: Docker only writes the option when it was set.
		d.BridgeICC = true
		if v, ok := bridge.Network.Options[iccOption]; ok {
			d.BridgeICC = v != "false" && v != "0"
		}
	} else {
		g.logger.Warn("compliance pack: bridge network inspect failed", "environment", envID, "error", err)
	}
	return true, d
}

// connectionType reads the environment's configured transport. An agent
// environment reports "agent" whatever its row says, because the agent connects
// outbound and the daemon socket is never exposed on the network.
func (g *Gatherer) connectionType(ctx context.Context, envID string) string {
	if g.pool != nil && g.pool.IsAgentEnvContext(ctx, envID) {
		return "agent"
	}
	if g.db == nil {
		return ""
	}
	resolved := envID
	if g.pool != nil {
		if id, err := g.pool.ResolveEnvironmentIDContext(ctx, envID); err == nil {
			resolved = id
		}
	}
	var kind string
	if err := g.db.QueryRowContext(ctx,
		`SELECT connection_type FROM environments WHERE id = ?`, resolved).Scan(&kind); err != nil {
		if err != sql.ErrNoRows {
			g.logger.Warn("compliance pack: environment connection type unreadable", "environment", envID, "error", err)
		}
		return ""
	}
	return strings.TrimSpace(kind)
}

func (g *Gatherer) gatherContainers(ctx context.Context, envID string, cli *dockerclient.Client) (bool, []Container, int, bool) {
	listCtx, cancel := context.WithTimeout(ctx, dockerCallTimeout)
	defer cancel()
	listRes, err := cli.ContainerList(listCtx, dockerclient.ContainerListOptions{All: true})
	list := listRes.Items
	if err != nil {
		g.logger.Warn("compliance pack: container list failed", "environment", envID, "error", err)
		return false, nil, 0, false
	}
	capped := false
	if len(list) > maxContainers {
		list, capped = list[:maxContainers], true
	}
	out := make([]Container, len(list))
	ok := make([]bool, len(list))
	var failures int
	var mu sync.Mutex
	var wg sync.WaitGroup
	work := make(chan int)
	for w := 0; w < inspectWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				inspectCtx, inspectCancel := context.WithTimeout(ctx, dockerCallTimeout)
				infoRes, err := cli.ContainerInspect(inspectCtx, list[i].ID, dockerclient.ContainerInspectOptions{})
				info := infoRes.Container
				inspectCancel()
				if err != nil {
					mu.Lock()
					failures++
					mu.Unlock()
					continue
				}
				out[i], ok[i] = reduceContainer(info), true
			}
		}()
	}
	for i := range list {
		select {
		case work <- i:
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return false, nil, failures, capped
		}
	}
	close(work)
	wg.Wait()
	kept := make([]Container, 0, len(out))
	for i := range out {
		if ok[i] {
			kept = append(kept, out[i])
		}
	}
	if failures > 0 {
		g.logger.Warn("compliance pack: some container inspects failed", "environment", envID, "failures", failures)
	}
	return true, kept, failures, capped
}

// reduceContainer keeps only the runtime settings a CIS control names. Command
// lines, environment variables and labels are dropped here rather than filtered
// later, so a secret in a container's environment can never reach a stored result.
func reduceContainer(info containertypes.InspectResponse) Container {
	c := Container{Name: strings.TrimPrefix(info.Name, "/")}
	if info.Config != nil {
		c.User = info.Config.User
		c.HasHealthcheck = info.Config.Healthcheck != nil && len(info.Config.Healthcheck.Test) > 0 &&
			info.Config.Healthcheck.Test[0] != "NONE"
	}
	if hc := info.HostConfig; hc != nil {
		c.CapAdd = hc.CapAdd
		c.Privileged = hc.Privileged
		c.NetworkMode = string(hc.NetworkMode)
		c.MemoryLimit = hc.Memory
		c.CPUShares = hc.CPUShares
		c.NanoCPUs = hc.NanoCPUs
		c.ReadonlyRootfs = hc.ReadonlyRootfs
		c.RestartPolicy = string(hc.RestartPolicy.Name)
		c.RestartMaxRetry = hc.RestartPolicy.MaximumRetryCount
		c.PIDMode = string(hc.PidMode)
		c.IPCMode = string(hc.IpcMode)
		c.UTSMode = string(hc.UTSMode)
		c.DeviceCount = len(hc.Devices)
		c.SecurityOpt = hc.SecurityOpt
		if hc.PidsLimit != nil {
			c.PIDsLimit = *hc.PidsLimit
		}
	}
	for _, m := range info.Mounts {
		if dockerSocketPaths[m.Source] {
			c.DockerSocketBind = true
		}
		switch string(m.Propagation) {
		case "shared", "rshared":
			c.SharedMounts = append(c.SharedMounts, m.Destination)
		}
	}
	if info.NetworkSettings != nil {
		for port, bindings := range info.NetworkSettings.Ports {
			for _, b := range bindings {
				n, err := strconv.Atoi(b.HostPort)
				if err != nil || n <= 0 || n >= 1024 {
					continue
				}
				c.PrivilegedPorts = append(c.PrivilegedPorts, b.HostPort+"->"+port.String())
			}
		}
	}
	return c
}
