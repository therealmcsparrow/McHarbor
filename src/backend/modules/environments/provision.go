// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// AgentContainerName is the name the deployed agent container takes.
//
// Fixed rather than generated, because it is also what the install script and
// the SSH deploy path use: an operator must not end up with two agents on one
// host because two code paths named them differently.
const AgentContainerName = "mcharbor-agent"

// DefaultAgentImage is the image an auto-deployed agent runs.
const DefaultAgentImage = "ghcr.io/therealmcsparrow/mcharbor-agent:latest"

// agentPullTimeout bounds the image pull. A first pull on a slow link is
// minutes, not seconds, and failing it early would leave a migration armed
// with no agent to satisfy it.
const agentPullTimeout = 10 * time.Minute

// agentProvisionTimeout bounds the create and start, which are local calls.
const agentProvisionTimeout = 60 * time.Second

// agentTransferPort is the port the agent listens on for pushed file transfers.
const agentTransferPort = "8788"

// AgentDeployment describes what a migration did about the agent itself.
//
// Error is populated rather than returned when deployment fails: the token is
// already issued and the migration is already armed, so the operator needs the
// reason plus the manual fallback, not a rolled-back request.
type AgentDeployment struct {
	Attempted   bool   `json:"attempted"`
	Deployed    bool   `json:"deployed"`
	ContainerID string `json:"containerId,omitempty"`
	Image       string `json:"image,omitempty"`
	Error       string `json:"error,omitempty"`
}

// agentProvisionOptions is what the agent container needs to dial home.
type agentProvisionOptions struct {
	ServerURL   string
	Token       string
	Image       string
	ForceTLS    bool
	InsecureTLS bool
}

// provisionAgentContainer deploys the agent onto the environment's own host.
//
// It runs through the connection the environment already has — that is the
// whole reason this is possible without asking for SSH credentials: a socket,
// TCP or SSH environment is, by definition, a Docker daemon McHarbor can
// already create containers on.
func (s *Service) provisionAgentContainer(
	ctx context.Context,
	env *Environment,
	opts agentProvisionOptions,
) AgentDeployment {
	result := AgentDeployment{Attempted: true, Image: opts.Image}
	if result.Image == "" {
		result.Image = DefaultAgentImage
	}

	cli, err := s.dockerPool.GetContext(ctx, env.ID)
	if err != nil {
		result.Error = fmt.Sprintf("connecting to the environment: %v", err)
		return result
	}

	pullCtx, cancelPull := context.WithTimeout(ctx, agentPullTimeout)
	defer cancelPull()
	if err := pullAgentImage(pullCtx, cli, result.Image); err != nil {
		result.Error = err.Error()
		return result
	}

	runCtx, cancelRun := context.WithTimeout(ctx, agentProvisionTimeout)
	defer cancelRun()

	// An agent container already on the host is ours and holds a token that is
	// about to stop working. Replaced rather than left alongside, because two
	// agents on one daemon fight for the same pool slot.
	if _, err := cli.ContainerRemove(runCtx, AgentContainerName, client.ContainerRemoveOptions{
		Force: true,
	}); err != nil && !cerrdefs.IsNotFound(err) {
		result.Error = fmt.Sprintf("removing the previous agent container: %v", err)
		return result
	}

	config, hostConfig := agentContainerSpec(env, result.Image, opts)

	created, err := cli.ContainerCreate(runCtx, client.ContainerCreateOptions{Config: config, HostConfig: hostConfig, Name: AgentContainerName})
	if err != nil {
		result.Error = fmt.Sprintf("creating the agent container: %v", err)
		return result
	}
	result.ContainerID = created.ID

	if _, err := cli.ContainerStart(runCtx, created.ID, client.ContainerStartOptions{}); err != nil {
		if _, rmErr := cli.ContainerRemove(runCtx, created.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
			slog.Warn("environments: removing agent container after failed start",
				"env", env.ID, "container", created.ID, "error", rmErr)
		}
		result.Error = fmt.Sprintf("starting the agent container: %v", err)
		return result
	}

	result.Deployed = true
	return result
}

// pullAgentImage fetches the agent image, so a stale local `latest` is not
// silently reused on a host that has had an agent before.
func pullAgentImage(ctx context.Context, cli *client.Client, ref string) error {
	resp, err := cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling %s: %w", ref, err)
	}
	// The pull is only complete once the stream is drained; closing early
	// leaves the layer download unfinished and the image absent.
	_, copyErr := io.Copy(io.Discard, resp)
	closeErr := resp.Close()
	if copyErr != nil {
		logCloseErr(closeErr)
		return fmt.Errorf("pulling %s: %w", ref, copyErr)
	}
	return closeErr
}

// agentContainerSpec builds the container the agent runs as.
//
// How the agent reaches its own Docker daemon depends on how McHarbor reaches
// it: a socket or SSH environment is a daemon with a socket on that host, so
// the socket is bind-mounted; a TCP environment has no socket to mount and the
// agent is pointed at the same address McHarbor uses.
func agentContainerSpec(
	env *Environment,
	ref string,
	opts agentProvisionOptions,
) (*container.Config, *container.HostConfig) {
	dataDir := "/var/lib/mcharbor-agent"
	envVars := []string{
		"MCHARBOR_URL=" + opts.ServerURL,
		"MCHARBOR_AGENT_TOKEN=" + opts.Token,
		"MCHARBOR_DATA_DIR=" + dataDir,
		"MCHARBOR_COMPOSE_DIR=" + dataDir + "/compose",
		"MCHARBOR_TRANSFER_LISTEN=0.0.0.0:" + agentTransferPort,
		"MCHARBOR_OFFLINE_BUFFER=true",
		"LOG_LEVEL=info",
	}
	if opts.ForceTLS {
		envVars = append(envVars, "MCHARBOR_FORCE_TLS=1")
	}
	if opts.InsecureTLS {
		envVars = append(envVars, "MCHARBOR_INSECURE=true")
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: "mcharbor-agent-data",
			Target: dataDir,
		}},
		PortBindings: network.PortMap{
			network.MustParsePort(agentTransferPort + "/tcp"): []network.PortBinding{{HostPort: agentTransferPort}},
		},
	}

	if socket := agentSocketPath(env); socket != "" {
		envVars = append(envVars, "DOCKER_HOST=unix:///var/run/docker.sock")
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: socket,
			Target: "/var/run/docker.sock",
		})
	} else if host := agentTCPHost(env); host != "" {
		envVars = append(envVars, "DOCKER_HOST="+host)
	}

	config := &container.Config{
		Image: ref,
		Env:   envVars,
		Labels: map[string]string{
			"com.mcharbor.agent":             "true",
			"com.mcharbor.agent.environment": env.ID,
		},
		ExposedPorts: network.PortSet{network.MustParsePort(agentTransferPort + "/tcp"): struct{}{}},
	}
	return config, hostConfig
}

// agentSocketPath names the daemon socket to hand the agent, or empty when the
// environment is not reached through one.
func agentSocketPath(env *Environment) string {
	switch env.ConnectionType {
	case ConnSocket:
		if env.SocketPath != nil && strings.TrimSpace(*env.SocketPath) != "" {
			return strings.TrimSpace(*env.SocketPath)
		}
		return "/var/run/docker.sock"
	case ConnSSH:
		// SSH reaches a daemon whose socket lives on that host at the standard
		// path; the agent runs on the same host and uses it directly.
		return "/var/run/docker.sock"
	}
	return ""
}

// agentTCPHost builds a DOCKER_HOST for a TCP environment.
func agentTCPHost(env *Environment) string {
	if env.Host == nil || strings.TrimSpace(*env.Host) == "" || env.Port == nil {
		return ""
	}
	return fmt.Sprintf("tcp://%s:%d", strings.TrimSpace(*env.Host), *env.Port)
}
