// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package snapshotdrivers holds the snapshot drivers McHarbor ships
// itself, as opposed to the ones a plugin contributes.
//
// The split is not arbitrary. A plugin runs inside the McHarbor
// process, so it can call a vendor's REST API (Synology, TrueNAS) but
// it can never see whether `zfs` is installed on a remote agent's
// host. Only the agent can answer that, and the agent speaks the
// WebSocket protocol, not the plugin ABI. So the filesystem-level
// drivers (zfs, btrfs, lvm, vss) live here in core, where they can be
// dispatched over the agent transport, and vendor drivers stay in
// plugins.
//
// A plugin that registers one of these driver names wins: the host
// resolves the plugin first and only falls back to the builtin. That
// keeps an installed snapshot-zfs plugin working exactly as before.
package snapshotdrivers

import (
	"context"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot"
	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// EnvIDKey is the config-map key the host uses to tell a driver which
// environment a call belongs to. Plugin-owned snapshot adapters read
// the same key, so builtin and plugin drivers take an identical map.
const EnvIDKey = "_env_id"

// Driver names. These are the values stored in
// container_backup_snapshot_configs.driver.
const (
	DriverZFS   = "zfs"
	DriverBtrfs = "btrfs"
	DriverLVM   = "lvm"
	DriverVSS   = "vss"
)

// AgentPool is the slice of the Docker client pool this package needs:
// enough to tell an agent environment from a local one and to reach
// the agent's transport. Declared here rather than imported so the
// package stays testable and free of an import cycle.
type AgentPool interface {
	IsAgentEnv(envID string) bool
	AgentConnection(envID string) (*coreagent.AgentConnection, bool)
}

// Status is one driver's availability on one environment, as shown in
// the Snapshots settings tab. A driver that is missing is reported,
// not hidden: an operator who cannot see that btrfs exists has no way
// to learn that installing btrfs-progs would unlock it.
type Status struct {
	Driver      string                    `json:"driver"`
	DisplayName string                    `json:"displayName"`
	Available   bool                      `json:"available"`
	Origin      string                    `json:"origin"` // agent | local | plugin
	ToolPath    string                    `json:"toolPath,omitempty"`
	Version     string                    `json:"version,omitempty"`
	Error       string                    `json:"error,omitempty"`
	Targets     []snapshot.SnapshotTarget `json:"targets,omitempty"`
	PauseFree   bool                      `json:"pauseFree"`
}

// Service resolves and runs the builtin drivers for an environment.
type Service struct {
	pool AgentPool
}

// New creates the builtin snapshot driver service.
func New(pool AgentPool) *Service {
	return &Service{pool: pool}
}

// Specs returns the builtin adapter specs, keyed by driver, in the
// same shape the plugin registry produces so the settings UI can merge
// the two lists without caring where a driver came from.
func Specs() map[string]snapshot.SnapshotAdapterSpec {
	return map[string]snapshot.SnapshotAdapterSpec{
		DriverZFS: {
			Driver:      DriverZFS,
			DisplayName: "ZFS",
			Slug:        "mcharbor",
			Description: "Filesystem-level ZFS snapshots. Reads the backup from the dataset's frozen .zfs/snapshot view, so the container is never paused.",
			Capabilities: snapshot.SnapshotAdapterCapabilities{
				SupportsMount:        true,
				SupportsRemoteTarget: true,
				RequiresAgent:        false,
			},
			ConfigSchema: pathFilterSchema("tank", "tank/data"),
		},
		DriverBtrfs: {
			Driver:      DriverBtrfs,
			DisplayName: "Btrfs",
			Slug:        "mcharbor",
			Description: "Read-only btrfs subvolume snapshots. The backup reads the snapshot, so the container keeps running.",
			Capabilities: snapshot.SnapshotAdapterCapabilities{
				SupportsMount:        true,
				SupportsRemoteTarget: true,
				RequiresAgent:        false,
			},
			ConfigSchema: pathFilterSchema("", "/var/lib/docker/volumes"),
		},
		DriverLVM: {
			Driver:      DriverLVM,
			DisplayName: "LVM",
			Slug:        "mcharbor",
			Description: "LVM copy-on-write snapshots, mounted read-only for the duration of the backup. Needs free space in the volume group.",
			Capabilities: snapshot.SnapshotAdapterCapabilities{
				SupportsMount:        true,
				SupportsRemoteTarget: true,
				RequiresAgent:        true,
			},
			ConfigSchema: pathFilterSchema("", "/var/lib/docker"),
		},
		DriverVSS: {
			Driver:      DriverVSS,
			DisplayName: "Windows VSS",
			Slug:        "mcharbor",
			Description: "Volume Shadow Copy on a Windows host. Produces a restore point; the container is still paused briefly because the shadow is not exposed at a readable path.",
			Capabilities: snapshot.SnapshotAdapterCapabilities{
				SupportsMount:        false,
				SupportsRemoteTarget: false,
				RequiresAgent:        true,
			},
			// Empty, not nil: a nil slice marshals to JSON null and the
			// driver picker reads .length off this field. VSS genuinely
			// takes no pool, prefix or SSH target.
			ConfigSchema: []types.ConfigFieldDecl{},
		},
	}
}

// pathFilterSchema is the config form shared by the three Linux
// drivers: which pool and which path prefix to restrict targets to.
func pathFilterSchema(poolPlaceholder, prefixPlaceholder string) []types.ConfigFieldDecl {
	fields := []types.ConfigFieldDecl{{
		Key:         "dataset_prefix",
		Type:        "string",
		Label:       "Path prefix",
		Required:    false,
		Help:        "Restrict snapshots to targets under this path. Leave blank to let the plan's own mounts decide.",
		Placeholder: prefixPlaceholder,
	}}
	if poolPlaceholder != "" {
		fields = append([]types.ConfigFieldDecl{{
			Key:         "pool_name",
			Type:        "string",
			Label:       "Pool name",
			Required:    false,
			Help:        "Restrict the target list to one pool. Leave blank to allow every pool the host can see.",
			Placeholder: poolPlaceholder,
		}}, fields...)
	}
	return fields
}

// Detect reports every builtin driver's availability on one
// environment. Agent environments are asked over the transport;
// everything else is probed in the McHarbor process itself.
func (s *Service) Detect(ctx context.Context, envID string) ([]Status, error) {
	specs := Specs()
	if s.pool != nil && s.pool.IsAgentEnv(envID) {
		return s.detectViaAgent(ctx, envID, specs)
	}
	return detectLocal(ctx, specs), nil
}

func (s *Service) detectViaAgent(ctx context.Context, envID string, specs map[string]snapshot.SnapshotAdapterSpec) ([]Status, error) {
	transport, err := s.transport(envID)
	if err != nil {
		return nil, err
	}
	detected, err := transport.SnapshotDetect(ctx, envID)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(detected))
	for _, row := range detected {
		spec := specs[row.Driver]
		out = append(out, Status{
			Driver:      row.Driver,
			DisplayName: displayName(spec, row.Driver),
			Available:   row.Available,
			Origin:      "agent",
			ToolPath:    row.ToolPath,
			Version:     row.Version,
			Error:       row.Error,
			Targets:     convertTargets(row.Targets),
			PauseFree:   spec.Capabilities.SupportsMount,
		})
	}
	return out, nil
}

func displayName(spec snapshot.SnapshotAdapterSpec, driver string) string {
	if spec.DisplayName != "" {
		return spec.DisplayName
	}
	return driver
}

func convertTargets(rows []coreagent.SnapshotTargetOut) []snapshot.SnapshotTarget {
	out := make([]snapshot.SnapshotTarget, 0, len(rows))
	for _, row := range rows {
		out = append(out, snapshot.SnapshotTarget{
			Name:       row.Name,
			Mountpoint: row.Mountpoint,
			UsedBytes:  row.UsedBytes,
			Driver:     row.Driver,
		})
	}
	return out
}

func (s *Service) transport(envID string) (*coreagent.AgentTransport, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("no docker pool wired for snapshot dispatch")
	}
	conn, ok := s.pool.AgentConnection(envID)
	if !ok || conn == nil || conn.Transport == nil {
		return nil, fmt.Errorf("agent not connected for environment %s", envID)
	}
	return conn.Transport, nil
}

// Client returns the SnapshotAdapterClient for a builtin driver, or
// ok=false when the name is not one of ours.
func (s *Service) Client(driver string) (snapshot.SnapshotAdapterClient, snapshot.SnapshotAdapterSpec, bool) {
	spec, ok := Specs()[driver]
	if !ok {
		return nil, snapshot.SnapshotAdapterSpec{}, false
	}
	return &builtinClient{service: s, driver: driver}, spec, true
}

// builtinClient implements the same interface a plugin's snapshot
// adapter does, so container_backups runs builtin and plugin drivers
// through one code path.
type builtinClient struct {
	service *Service
	driver  string
}

func (c *builtinClient) Capability(ctx context.Context, config map[string]string) (snapshot.SnapshotCapability, error) {
	envID := config[EnvIDKey]
	if c.service.pool != nil && c.service.pool.IsAgentEnv(envID) {
		transport, err := c.service.transport(envID)
		if err != nil {
			return snapshot.SnapshotCapability{Driver: c.driver, Available: false, Error: err.Error()}, nil //nolint:nilerr // a failed capability probe is reported in-band as SnapshotCapability.Error
		}
		reply, err := transport.SnapshotCapability(ctx, envID, c.driver)
		if err != nil {
			return snapshot.SnapshotCapability{Driver: c.driver, Available: false, Error: err.Error()}, nil //nolint:nilerr // a failed capability probe is reported in-band as SnapshotCapability.Error
		}
		return snapshot.SnapshotCapability{
			Driver:    c.driver,
			ToolPath:  reply.ToolPath,
			Version:   reply.Version,
			Available: reply.Available,
			Targets:   filterTargets(convertTargets(reply.TargetsOut), config),
			Error:     reply.Error,
		}, nil
	}
	capability := localCapability(ctx, c.driver)
	capability.Targets = filterTargets(capability.Targets, config)
	return capability, nil
}

func (c *builtinClient) Create(ctx context.Context, config map[string]string, req snapshot.SnapshotCreateRequest) (snapshot.SnapshotRef, error) {
	ref, _, err := c.service.CreateWithPaths(ctx, config[EnvIDKey], c.driver, req)
	return ref, err
}

func (c *builtinClient) Remove(ctx context.Context, config map[string]string, ref snapshot.SnapshotRef) error {
	if ref.SnapshotName == "" {
		return nil
	}
	envID := config[EnvIDKey]
	if c.service.pool != nil && c.service.pool.IsAgentEnv(envID) {
		transport, err := c.service.transport(envID)
		if err != nil {
			return err
		}
		return transport.SnapshotRemove(ctx, envID, coreagent.SnapshotPayload{
			Driver:       c.driver,
			SnapshotName: ref.SnapshotName,
			Targets:      ref.Targets,
		})
	}
	return localRemove(ctx, c.driver, ref)
}

// CreateWithPaths freezes the targets and also returns the frozen path
// of each one, which is what lets the backup read a consistent copy
// instead of pausing the container. The plugin-facing Create cannot
// carry the map because SnapshotRef has no field for it, so the host
// calls this directly for builtin drivers and accepts a mount-path-only
// answer from plugin-owned ones.
func (s *Service) CreateWithPaths(ctx context.Context, envID, driver string, req snapshot.SnapshotCreateRequest) (snapshot.SnapshotRef, map[string]string, error) {
	if len(req.Targets) == 0 {
		return snapshot.SnapshotRef{}, nil, fmt.Errorf("snapshot %s: no targets to freeze", driver)
	}
	if s.pool == nil || !s.pool.IsAgentEnv(envID) {
		ref, err := localCreate(ctx, driver, req)
		if err != nil {
			return snapshot.SnapshotRef{}, nil, err
		}
		return ref, localMountPaths(ref, req), nil
	}
	transport, err := s.transport(envID)
	if err != nil {
		return snapshot.SnapshotRef{}, nil, err
	}
	reply, err := transport.SnapshotCreate(ctx, envID, coreagent.SnapshotPayload{
		Driver:     driver,
		Targets:    req.Targets,
		SnapshotID: req.SnapshotID,
		ReadOnly:   req.ReadOnly,
		TimeoutSec: req.Timeout,
	})
	if err != nil {
		return snapshot.SnapshotRef{}, nil, err
	}
	ref := snapshot.SnapshotRef{
		SnapshotName: reply.SnapshotName,
		Targets:      req.Targets,
		MountPath:    reply.MountPath,
		CreatedAt:    parseTime(reply.CreatedAt),
	}
	return ref, reply.MountPaths, nil
}

// IsBuiltin reports whether the driver is one McHarbor ships itself.
func IsBuiltin(driver string) bool {
	_, ok := Specs()[driver]
	return ok
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// filterTargets applies the config's pool and prefix filters. An
// operator who scoped a config to one pool must not have a backup
// silently snapshot a different one.
func filterTargets(targets []snapshot.SnapshotTarget, config map[string]string) []snapshot.SnapshotTarget {
	pool := strings.TrimSpace(config["pool_name"])
	prefix := strings.TrimSpace(config["dataset_prefix"])
	if pool == "" && prefix == "" {
		return targets
	}
	out := make([]snapshot.SnapshotTarget, 0, len(targets))
	for _, target := range targets {
		if pool != "" && target.Name != pool && !strings.HasPrefix(target.Name, pool+"/") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(target.Name, prefix) && !strings.HasPrefix(target.Mountpoint, prefix) {
			continue
		}
		out = append(out, target)
	}
	return out
}
