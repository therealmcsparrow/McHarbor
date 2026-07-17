// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

const (
	backupSecretName        = "mcharbor_backup_key"
	backupSecretRelativeDir = "secrets"
	backupSecretRelativeKey = "secrets/mcharbor_backup_key"
	backupProjectMount      = "/mcharbor-project"
	backupSecretTarget      = "/run/secrets/mcharbor_backup_key"

	backupSecretEnvContainerID = "MCHARBOR_BACKUP_SECRET_CONTAINER_ID"
	backupSecretEnvContainer   = "MCHARBOR_BACKUP_SECRET_CONTAINER"
	backupSecretEnvHostFile    = "MCHARBOR_BACKUP_SECRET_HOST_FILE"
)

var backupShortHexContainerIDRe = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

// BackupKeyInstallRequest installs the one-time key into the host-side Compose secret file.
type BackupKeyInstallRequest struct {
	Key string `json:"key"`
}

// BackupKeyInstallResponse describes the scheduled Docker secret setup.
type BackupKeyInstallResponse struct {
	Secret      string `json:"secret"`
	SecretPath  string `json:"secretPath"`
	ProjectPath string `json:"projectPath"`
	Output      string `json:"output"`
}

// BackupKeyStatusResponse describes whether the runtime backup key is active.
type BackupKeyStatusResponse struct {
	Configured bool   `json:"configured"`
	Readable   bool   `json:"readable"`
	KeyID      string `json:"keyId,omitempty"`
	Path       string `json:"path"`
}

// BackupKeyStatus checks the runtime backup encryption key without returning key material.
func (s *Service) BackupKeyStatus(keyPath string) BackupKeyStatusResponse {
	keyPath = strings.TrimSpace(keyPath)
	status := BackupKeyStatusResponse{Path: keyPath}
	if keyPath == "" {
		return status
	}

	if _, err := os.Stat(keyPath); err != nil {
		return status
	}
	status.Configured = true

	cryptoSvc, err := backupcrypto.NewFromKeyFile(keyPath)
	if err != nil {
		return status
	}
	status.Readable = true
	status.KeyID = cryptoSvc.KeyID()
	return status
}

// InstallBackupKey writes the host-side Compose secret file and schedules a Compose restart.
func (s *Service) InstallBackupKey(ctx context.Context, key string) (BackupKeyInstallResponse, error) {
	key = strings.TrimSpace(key)
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return BackupKeyInstallResponse{}, &ErrValidation{
			Message: "backup key must be a base64 encoded 32-byte value",
			Code:    i18n.ErrSettingsBackupKeyInvalid,
		}
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return BackupKeyInstallResponse{}, fmt.Errorf("creating local docker client: %w", err)
	}
	defer cli.Close()

	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	current, err := inspectCurrentMcHarborContainer(opCtx, cli)
	if err != nil {
		return BackupKeyInstallResponse{}, fmt.Errorf("inspecting current mcharbor container: %w", err)
	}

	labels := map[string]string{}
	if current.Config != nil {
		labels = current.Config.Labels
	}
	projectPath := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
	if projectPath == "" {
		return BackupKeyInstallResponse{}, &ErrValidation{
			Message: "mcharbor compose project path was not found",
			Code:    i18n.ErrSettingsComposeProjectMissing,
		}
	}

	socketMount, ok := helperMountForDestination(current.Mounts, "/var/run/docker.sock")
	if !ok {
		return BackupKeyInstallResponse{}, fmt.Errorf("docker socket mount not found")
	}

	helperName := "mcharbor-backup-secret-helper-" + xid.New().String()
	containerName := strings.TrimPrefix(current.Name, "/")
	resp, err := cli.ContainerCreate(opCtx, &container.Config{
		Image:      current.Config.Image,
		Entrypoint: []string{"./mcharbor"},
		Cmd:        []string{"backup-secret-helper"},
		WorkingDir: "/app",
		Env: helperEnv(os.Environ(), map[string]string{
			backupSecretEnvContainerID: current.ID,
			backupSecretEnvContainer:   containerName,
			backupSecretEnvHostFile:    filepath.Join(projectPath, backupSecretRelativeKey),
		}),
		Labels: map[string]string{
			"com.mcharbor.helper": "backup-secret",
		},
	}, &container.HostConfig{
		AutoRemove: true,
		Mounts: []mount.Mount{
			socketMount,
			{
				Type:     mount.TypeBind,
				Source:   projectPath,
				Target:   backupProjectMount,
				ReadOnly: false,
			},
		},
	}, nil, nil, helperName)
	if err != nil {
		return BackupKeyInstallResponse{}, fmt.Errorf("creating backup secret helper container: %w", err)
	}

	if err := copyBackupKeyToHelper(opCtx, cli, resp.ID, key); err != nil {
		removeHelperContainer(context.WithoutCancel(ctx), cli, resp.ID)
		return BackupKeyInstallResponse{}, err
	}

	if err := cli.ContainerStart(opCtx, resp.ID, container.StartOptions{}); err != nil {
		removeHelperContainer(context.WithoutCancel(ctx), cli, resp.ID)
		return BackupKeyInstallResponse{}, fmt.Errorf("starting backup secret helper container: %w", err)
	}

	return BackupKeyInstallResponse{
		Secret:      backupSecretName,
		SecretPath:  backupSecretRelativeKey,
		ProjectPath: projectPath,
		Output:      "scheduled backup secret setup and McHarbor restart",
	}, nil
}

// RunBackupSecretHelper restarts McHarbor with the generated secret mounted.
func RunBackupSecretHelper(ctx context.Context) error {
	copiedSecretPath := filepath.Join(backupProjectMount, backupSecretRelativeKey)
	if _, err := os.Stat(copiedSecretPath); err != nil {
		return fmt.Errorf("checking backup secret file: %w", err)
	}

	containerID := strings.TrimSpace(os.Getenv(backupSecretEnvContainerID))
	containerName := strings.Trim(strings.TrimSpace(os.Getenv(backupSecretEnvContainer)), "/")
	hostSecretFile := strings.TrimSpace(os.Getenv(backupSecretEnvHostFile))
	if containerID == "" {
		return fmt.Errorf("missing %s", backupSecretEnvContainerID)
	}
	if containerName == "" {
		return fmt.Errorf("missing %s", backupSecretEnvContainer)
	}
	if hostSecretFile == "" {
		return fmt.Errorf("missing %s", backupSecretEnvHostFile)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating Docker client: %w", err)
	}
	defer cli.Close()

	opCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	current, err := cli.ContainerInspect(opCtx, containerID)
	if err != nil {
		return fmt.Errorf("inspecting current container: %w", err)
	}
	cfg, hostCfg, netCfg := cloneBackupSecretContainerConfig(current, hostSecretFile)

	timeout := 10
	if err := cli.ContainerStop(opCtx, current.ID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("stopping current container: %w", err)
	}
	if err := cli.ContainerRemove(opCtx, current.ID, container.RemoveOptions{Force: true, RemoveVolumes: false}); err != nil {
		return fmt.Errorf("removing current container: %w", err)
	}
	created, err := cli.ContainerCreate(opCtx, cfg, hostCfg, netCfg, nil, containerName)
	if err != nil {
		return fmt.Errorf("creating replacement container: %w", err)
	}
	if err := cli.ContainerStart(opCtx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting replacement container: %w", err)
	}
	return waitForBackupSecretContainerRunning(opCtx, cli, created.ID, 3*time.Second)
}

func inspectCurrentMcHarborContainer(ctx context.Context, cli *client.Client) (types.ContainerJSON, error) {
	return coredocker.CurrentMcHarborContainer(ctx, cli)
}

func currentContainerInspectCandidates() []string {
	return coredocker.CurrentMcHarborContainerCandidates()
}

func helperMountForDestination(mounts []types.MountPoint, destination string) (mount.Mount, bool) {
	return coredocker.MountForDestination(mounts, destination)
}

func helperEnv(base []string, overrides map[string]string) []string {
	env := make([]string, 0, len(base)+len(overrides)+1)
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found && (key == "BACKUP_ENCRYPTION_KEY_FILE" || overrides[key] != "") {
			continue
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	env = append(env, "DOCKER_HOST=unix:///var/run/docker.sock")
	return env
}

func cloneBackupSecretContainerConfig(current container.InspectResponse, hostSecretFile string) (*container.Config, *container.HostConfig, *network.NetworkingConfig) {
	cfg := *current.Config
	cfg.Env = setEnvValue(cfg.Env, "BACKUP_ENCRYPTION_KEY_FILE", backupSecretTarget)
	if backupShortHexContainerIDRe.MatchString(strings.ToLower(strings.TrimSpace(cfg.Hostname))) {
		cfg.Hostname = ""
	}

	hostCfg := *current.HostConfig
	hostCfg.AutoRemove = false
	hostCfg.ContainerIDFile = ""
	hostCfg.Mounts = replaceBackupSecretMount(hostCfg.Mounts, hostSecretFile)

	netCfg := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}
	if current.NetworkSettings != nil {
		for name, endpoint := range current.NetworkSettings.Networks {
			if endpoint == nil {
				continue
			}
			netCfg.EndpointsConfig[name] = &network.EndpointSettings{
				IPAMConfig: endpoint.IPAMConfig,
				Links:      endpoint.Links,
				Aliases:    filterGeneratedAliases(endpoint.Aliases),
				DriverOpts: endpoint.DriverOpts,
				GwPriority: endpoint.GwPriority,
			}
		}
	}

	return &cfg, &hostCfg, netCfg
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			out = append(out, prefix+value)
			found = true
			continue
		}
		out = append(out, entry)
	}
	if !found {
		out = append(out, prefix+value)
	}
	return out
}

func replaceBackupSecretMount(mounts []mount.Mount, hostSecretFile string) []mount.Mount {
	out := make([]mount.Mount, 0, len(mounts)+1)
	for _, item := range mounts {
		if filepath.Clean(item.Target) == backupSecretTarget {
			continue
		}
		out = append(out, item)
	}
	out = append(out, mount.Mount{
		Type:     mount.TypeBind,
		Source:   hostSecretFile,
		Target:   backupSecretTarget,
		ReadOnly: true,
	})
	return out
}

func filterGeneratedAliases(aliases []string) []string {
	filtered := make([]string, 0, len(aliases))
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || backupShortHexContainerIDRe.MatchString(strings.ToLower(alias)) {
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

func waitForBackupSecretContainerRunning(ctx context.Context, cli *client.Client, containerID string, stableFor time.Duration) error {
	deadline := time.NewTimer(stableFor)
	defer deadline.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		inspect, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			return fmt.Errorf("inspecting replacement container: %w", err)
		}
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

func copyBackupKeyToHelper(ctx context.Context, cli *client.Client, containerID, key string) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:     backupSecretRelativeDir,
		Typeflag: tar.TypeDir,
		Mode:     0o700,
		ModTime:  time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("preparing backup secret directory archive: %w", err)
	}

	data := []byte(key)
	if err := tw.WriteHeader(&tar.Header{
		Name:     backupSecretRelativeKey,
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     int64(len(data)),
		ModTime:  time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("preparing backup secret archive: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("writing backup secret archive: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing backup secret archive: %w", err)
	}

	if err := cli.CopyToContainer(ctx, containerID, backupProjectMount, io.NopCloser(&buf), container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("copying backup secret into helper project mount: %w", err)
	}
	return nil
}

func removeHelperContainer(ctx context.Context, cli *client.Client, containerID string) {
	rmCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_ = cli.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true, RemoveVolumes: false})
}
