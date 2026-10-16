// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/therealmcsparrow/mcharbor/core/apppaths"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

const (
	selfUpdateEnvContainerID = "MCHARBOR_SELF_UPDATE_CONTAINER_ID"
	selfUpdateEnvContainer   = "MCHARBOR_SELF_UPDATE_CONTAINER"
	selfUpdateEnvImage       = "MCHARBOR_SELF_UPDATE_IMAGE"
	selfUpdateEnvLog         = "MCHARBOR_SELF_UPDATE_LOG"
	selfUpdateEnvOperation   = "MCHARBOR_SELF_UPDATE_OPERATION"
	selfUpdateEnvOverrides   = "MCHARBOR_SELF_UPDATE_ENV_OVERRIDES_JSON"
	selfUpdateEnvRemove      = "MCHARBOR_SELF_UPDATE_ENV_REMOVE_JSON"
)

var shortHexContainerIDRe = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

// RunSelfUpdateHelper recreates the running McHarbor container from outside the
// API process. It intentionally does not read arbitrary commands from input.
func RunSelfUpdateHelper(ctx context.Context) error {
	logFile, closeLog, err := openSelfUpdateLog(os.Getenv(selfUpdateEnvLog))
	if err != nil {
		return err
	}
	defer closeLog()

	logger := log.New(logFile, "", log.LstdFlags|log.LUTC)
	logger.Println("McHarbor self-update helper started")

	containerID := strings.TrimSpace(os.Getenv(selfUpdateEnvContainerID))
	containerName := strings.Trim(strings.TrimSpace(os.Getenv(selfUpdateEnvContainer)), "/")
	targetImage := strings.TrimSpace(os.Getenv(selfUpdateEnvImage))
	operation := strings.TrimSpace(os.Getenv(selfUpdateEnvOperation))
	if containerID == "" {
		return fmt.Errorf("missing %s", selfUpdateEnvContainerID)
	}
	if targetImage == "" {
		return fmt.Errorf("missing %s", selfUpdateEnvImage)
	}
	if operation == "" {
		operation = "reinstall"
	}

	cli, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("creating Docker client: %w", err)
	}
	defer func() { closeutil.Report(cli.Close(), "cli") }()

	opCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	currentRes, err := cli.ContainerInspect(opCtx, containerID, client.ContainerInspectOptions{})
	current := currentRes.Container
	if err != nil {
		return fmt.Errorf("inspecting current container: %w", err)
	}
	if containerName == "" {
		containerName = strings.Trim(current.Name, "/")
	}
	if containerName == "" {
		return fmt.Errorf("current container name is empty")
	}

	originalImage := current.Config.Image
	if operation == "update" {
		logger.Printf("pulling target image %s", targetImage)
		reader, err := cli.ImagePull(opCtx, targetImage, client.ImagePullOptions{})
		if err != nil {
			return fmt.Errorf("pulling target image: %w", err)
		}
		if _, copyErr := io.Copy(logFile, reader); copyErr != nil {
			closeutil.Report(reader.Close(), "reader")
			return fmt.Errorf("reading image pull output: %w", copyErr)
		}
		if err := reader.Close(); err != nil {
			return fmt.Errorf("closing image pull output: %w", err)
		}
	}

	if operation == "factory_reset" {
		logger.Println("factory_reset requested — wiping /app/data")
		if err := wipeDataDir(logger); err != nil {
			return fmt.Errorf("wiping data dir: %w", err)
		}
	}

	cfg, hostCfg, netCfg := cloneSelfContainerConfig(current, targetImage)
	if err := applySelfUpdateEnvOverrides(cfg); err != nil {
		return err
	}
	timeout := 10
	if _, err := cli.ContainerStop(opCtx, current.ID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		logger.Printf("stopping current container returned: %v", err)
	}
	if _, err := cli.ContainerRemove(opCtx, current.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: false}); err != nil {
		return fmt.Errorf("removing current container: %w", err)
	}

	if err := createAndStartSelfContainer(opCtx, cli, logger, containerName, cfg, hostCfg, netCfg); err != nil {
		logger.Printf("target recreate failed: %v", err)
		cfg.Image = originalImage
		if rollbackErr := createAndStartSelfContainer(opCtx, cli, logger, containerName, cfg, hostCfg, netCfg); rollbackErr != nil {
			return fmt.Errorf("recreating target container: %w", errors.Join(err, fmt.Errorf("rollback failed: %w", rollbackErr)))
		}
		return fmt.Errorf("recreating target container: %w; rolled back to %s", err, originalImage)
	}

	logger.Println("McHarbor self-update helper completed")
	return nil
}

// RunSelfStartWatchdog is a short-lived production safety net. It waits for the
// old McHarbor container to be replaced, then repeatedly starts the replacement
// if Docker leaves it created or exited.
func RunSelfStartWatchdog(ctx context.Context) error {
	logFile, closeLog, err := openSelfUpdateLog(os.Getenv(selfUpdateEnvLog))
	if err != nil {
		return err
	}
	defer closeLog()

	logger := log.New(logFile, "", log.LstdFlags|log.LUTC)
	logger.Println("McHarbor self-start watchdog started")

	oldContainerID := strings.TrimSpace(os.Getenv(selfUpdateEnvContainerID))
	containerName := strings.Trim(strings.TrimSpace(os.Getenv(selfUpdateEnvContainer)), "/")
	if containerName == "" {
		return fmt.Errorf("missing %s", selfUpdateEnvContainer)
	}

	cli, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("creating Docker client: %w", err)
	}
	defer func() { closeutil.Report(cli.Close(), "cli") }()

	watchCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		inspectRes, err := cli.ContainerInspect(watchCtx, containerName, client.ContainerInspectOptions{})
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				logger.Printf("waiting for replacement container %s to be created", containerName)
			} else {
				logger.Printf("inspecting %s failed: %v", containerName, err)
			}
		}
		inspect := inspectRes.Container
		switch {
		case err != nil:
		case containerIDMatches(inspect.ID, oldContainerID):
			logger.Printf("old container %s is still present; waiting for replacement", oldContainerID)
		case inspect.State != nil && inspect.State.Running:
			logger.Printf("replacement container %s is running", inspect.ID)
			logger.Println("McHarbor self-start watchdog completed")
			return nil
		default:
			status, exitCode, stateErr := selfContainerState(inspect)
			logger.Printf("replacement container %s is not running: status=%s exitCode=%d error=%s", inspect.ID, status, exitCode, stateErr)
			if _, err := cli.ContainerStart(watchCtx, inspect.ID, client.ContainerStartOptions{}); err != nil {
				logger.Printf("watchdog start attempt failed for %s: %v", inspect.ID, err)
			} else {
				logger.Printf("watchdog start attempt sent for %s", inspect.ID)
			}
		}

		select {
		case <-watchCtx.Done():
			return fmt.Errorf("watching replacement container %s: %w", containerName, watchCtx.Err())
		case <-ticker.C:
		}
	}
}

func applySelfUpdateEnvOverrides(cfg *container.Config) error {
	if cfg == nil {
		return fmt.Errorf("container config is nil")
	}
	if raw := strings.TrimSpace(os.Getenv(selfUpdateEnvOverrides)); raw != "" {
		var overrides map[string]string
		if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
			return fmt.Errorf("decoding self-update env overrides: %w", err)
		}
		cfg.Env = mergeContainerEnv(cfg.Env, overrides)
	}
	if raw := strings.TrimSpace(os.Getenv(selfUpdateEnvRemove)); raw != "" {
		var removals []string
		if err := json.Unmarshal([]byte(raw), &removals); err != nil {
			return fmt.Errorf("decoding self-update env removals: %w", err)
		}
		cfg.Env = removeContainerEnv(cfg.Env, removals)
	}
	return nil
}

// removeContainerEnv drops every KEY=VALUE entry whose key is listed. Entries
// with no "=" pass through untouched, matching mergeContainerEnv.
func removeContainerEnv(base []string, removals []string) []string {
	if len(removals) == 0 {
		return base
	}
	drop := make(map[string]struct{}, len(removals))
	for _, key := range removals {
		drop[key] = struct{}{}
	}
	out := make([]string, 0, len(base))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, remove := drop[key]; remove {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func mergeContainerEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(overrides))
	seen := make(map[string]struct{}, len(overrides))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			out = append(out, item)
			continue
		}
		if value, replace := overrides[key]; replace {
			out = append(out, key+"="+value)
			seen[key] = struct{}{}
			continue
		}
		out = append(out, item)
	}
	for key, value := range overrides {
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

func openSelfUpdateLog(logPath string) (*os.File, func(), error) {
	if strings.TrimSpace(logPath) == "" {
		logPath = "/app/data/self-update/self-update.log"
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, nil, fmt.Errorf("creating self-update log directory: %w", err)
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("opening self-update log: %w", err)
	}
	return file, func() { closeutil.Report(file.Close(), "file") }, nil
}

func cloneSelfContainerConfig(current container.InspectResponse, targetImage string) (*container.Config, *container.HostConfig, *network.NetworkingConfig) {
	cfg := *current.Config
	cfg.Image = targetImage
	if isGeneratedSelfHostname(cfg.Hostname) {
		cfg.Hostname = ""
	}

	hostCfg := *current.HostConfig
	hostCfg.AutoRemove = false
	hostCfg.ContainerIDFile = ""

	netCfg := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}
	if current.NetworkSettings != nil {
		for name, endpoint := range current.NetworkSettings.Networks {
			if endpoint == nil {
				continue
			}
			netCfg.EndpointsConfig[name] = &network.EndpointSettings{
				IPAMConfig: endpoint.IPAMConfig,
				Links:      endpoint.Links,
				Aliases:    filterSelfNetworkAliases(endpoint.Aliases),
				DriverOpts: endpoint.DriverOpts,
				GwPriority: endpoint.GwPriority,
			}
		}
	}

	return &cfg, &hostCfg, netCfg
}

func createAndStartSelfContainer(ctx context.Context, cli *client.Client, logger *log.Logger, containerName string, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig) error {
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: cfg, HostConfig: hostCfg, NetworkingConfig: netCfg, Name: containerName})
	if err != nil {
		return fmt.Errorf("creating replacement container: %w", err)
	}

	const attempts = 12
	for attempt := 1; attempt <= attempts; attempt++ {
		if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
			logger.Printf("starting replacement container attempt %d/%d failed: %v", attempt, attempts, err)
		} else if err := waitForSelfContainerRunning(ctx, cli, created.ID, 3*time.Second); err != nil {
			logger.Printf("replacement container did not stay running after attempt %d/%d: %v", attempt, attempts, err)
		} else {
			logger.Printf("replacement container %s started", created.ID)
			return nil
		}

		if attempt < attempts {
			if err := sleepSelfUpdate(ctx, 2*time.Second); err != nil {
				break
			}
		}
	}

	inspectRes, inspectErr := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	inspect := inspectRes.Container
	if inspectErr == nil && inspect.State != nil {
		status, exitCode, stateErr := selfContainerState(inspect)
		logger.Printf("replacement container final state: status=%s exitCode=%d error=%s", status, exitCode, stateErr)
	}
	if _, err := cli.ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: false}); err != nil {
		logger.Printf("removing failed replacement container %s: %v", created.ID, err)
	}
	return fmt.Errorf("starting replacement container after retries")
}

func waitForSelfContainerRunning(ctx context.Context, cli *client.Client, containerID string, stableFor time.Duration) error {
	deadline := time.NewTimer(stableFor)
	defer deadline.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		inspectRes, err := cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
		if err != nil {
			return fmt.Errorf("inspecting replacement container: %w", err)
		}
		inspect := inspectRes.Container
		if inspect.State == nil {
			return fmt.Errorf("replacement container has no state")
		}
		if !inspect.State.Running {
			return fmt.Errorf("replacement container status=%s exitCode=%d error=%s", inspect.State.Status, inspect.State.ExitCode, inspect.State.Error)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return nil
		case <-ticker.C:
		}
	}
}

func sleepSelfUpdate(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func selfContainerState(inspect container.InspectResponse) (string, int, string) {
	if inspect.State == nil {
		return "unknown", 0, "missing state"
	}
	return string(inspect.State.Status), inspect.State.ExitCode, inspect.State.Error
}

func isGeneratedSelfHostname(hostname string) bool {
	return shortHexContainerIDRe.MatchString(strings.ToLower(strings.TrimSpace(hostname)))
}

func filterSelfNetworkAliases(aliases []string) []string {
	filtered := make([]string, 0, len(aliases))
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || isGeneratedSelfHostname(alias) {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		filtered = append(filtered, alias)
	}
	return filtered
}

// wipeDataDir removes every entry under the McHarbor data directory
// (default /app/data) except the data directory itself — that
// directory is the mount point of a bind-mounted volume and must
// stay present. Used by the factory_reset operation to wipe the DB,
// encryption key, catalog, plugin installs, secrets, backups and
// logs so McHarbor starts from a fresh state on next launch.
func wipeDataDir(logger *log.Logger) error {
	dataDir := strings.TrimSpace(os.Getenv("MCHARBOR_DATA_DIR"))
	if dataDir == "" {
		dataDir = apppaths.DefaultDataDir
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Printf("data dir %s does not exist; nothing to wipe", dataDir)
			return nil
		}
		return err
	}

	for _, entry := range entries {
		target := filepath.Join(dataDir, entry.Name())
		logger.Printf("removing %s", target)
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("removing %s: %w", target, err)
		}
	}

	logger.Printf("data dir %s wiped", dataDir)
	return nil
}
