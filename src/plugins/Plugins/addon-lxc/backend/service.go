// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Service holds every query and every agent call.
type Service struct {
	db     plugin.DBHandle
	rpc    agentRPC
	logger pluginLogger

	// backupLog puts this plugin's backup and restore events in the host's
	// Backups log, beside the machine and Docker ones. nil on a host that wired
	// none.
	backupLog plugin.BackupLogHandle
	// environments names the host's environments, which this plugin's own tables
	// cannot: a destination path reading "env-d1k…/ct/web" helps nobody.
	environments plugin.EnvironmentHandle
	// storage is the host's storage bridge, used for backup destinations. nil on
	// a host with none configured, which the backup path refuses by name.
	storage plugin.StorageHandle

	// backupMu guards the two in-flight transfer maps below. A run's upload
	// target and a restore's download grants live only for the length of the
	// operation, so they are memory rather than rows — a restart invalidates
	// both, and reapOrphanedBackupRuns is what turns that into an answer.
	backupMu        sync.RWMutex
	backupUploads   map[string]backupUploadTarget
	backupDownloads map[string]backupDownload
	backupGrants    *backupGrants
}

// Errors the handlers map onto stable codes. The text never reaches a client.
var (
	ErrNotFound         = errors.New("not found")
	ErrNameTaken        = errors.New("name already in use")
	ErrRunning          = errors.New("this container is running")
	ErrNotRunning       = errors.New("this container is not running")
	ErrHostUnavailable  = errors.New("this container's host cannot be reached")
	ErrChainTooDeep     = errors.New("this container's snapshot chain is too deep")
	ErrTemplateInUse    = errors.New("this template still has linked clones")
	ErrTemplateRunning  = errors.New("a template cannot be started")
	ErrNoCapability     = errors.New("this host cannot run system containers")
	ErrBadMount         = errors.New("this mount is not allowed")
	ErrIDRangeExhausted = errors.New("no free subordinate id range is left on this host")
	// ErrAgentTooOld means the container's host runs an agent below the backup
	// floor. Distinct from ErrHostUnavailable because the operator's next step is
	// rebuilding the agent, not bringing the host back.
	ErrAgentTooOld = errors.New("this container's host runs an agent too old to back up a container")
	// ErrAgentTooOldRootfs means the host's agent predates building a rootfs from
	// an image or template and removing it on delete, so the request was refused
	// before anything was changed on the host.
	ErrAgentTooOldRootfs = errors.New("this container's host runs an agent too old to build or remove a container's rootfs")
	// ErrImageNotCached means a create named an image the host has not
	// downloaded. Checked before the row is written, so a refusal leaves nothing
	// behind; the wrapped message carries the image id the operator must fetch.
	ErrImageNotCached = errors.New("image not in this host's image cache")
	// ErrStatsUnavailable means the host answered a stats request without a sample,
	// which is what an agent from before container stats does for a running container.
	ErrStatsUnavailable = errors.New("the host returned no statistics for this container")
)

// maxRows bounds every unpaginated select.
const maxRows = 1000

// maxChainDepth caps CoW snapshot chains. Past this the read amplification is
// bad enough that the container becomes mysteriously slow and nobody connects
// it to the snapshot they took last month.
const maxChainDepth = 16

// defaultGraceSeconds is how long init gets to exit before the cgroup is
// killed. Ten seconds rather than systemd's ninety: a system container that has
// not stopped in ten is almost always one whose stop signal never reached the
// right process, and waiting out ninety teaches the operator that stopping
// containers is slow.
const defaultGraceSeconds = 10

// agentFloorMarker is how a version refusal is told apart from an offline host.
//
// The bridge phrases a version refusal as "<message> needs agent <version> build
// <build> or newer" and tags it agent_outdated; either is enough here, and the
// sentence is what lets this plugin disable its backup controls and name the build
// the operator has to reach, rather than reporting a generic command failure.
const agentFloorMarker = "needs agent "

// NewService builds the service.
func NewService(
	db plugin.DBHandle,
	rpc any,
	backupLog plugin.BackupLogHandle,
	environments plugin.EnvironmentHandle,
	storage plugin.StorageHandle,
	logger pluginLogger,
) *Service {
	return &Service{
		db: db, rpc: newAgentRPC(rpc),
		backupLog: backupLog, environments: environments, storage: storage,
		logger:       logger,
		backupGrants: newBackupGrants(),
	}
}

// call sends one ct-family message and decodes the reply.
//
// Nothing here is queued for an offline agent. A start command that arrives an
// hour late, after the operator has given up and started the container by hand,
// is worse than one that failed immediately and said why.
func (s *Service) call(
	ctx context.Context,
	envID, messageType string,
	payload *agentPayload,
) (*agentPayload, error) {
	if s.rpc == nil {
		return nil, ErrHostUnavailable
	}
	if available, reason := s.rpc.Available(ctx, envID); !available {
		return nil, fmt.Errorf("%w: %s", ErrHostUnavailable, reason)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", messageType, err)
	}

	raw, err := s.rpc.Call(ctx, envID, messageType, body, nil)
	if err != nil {
		if agentRPCCode(err) == rpcAgentOutdated || strings.Contains(err.Error(), agentFloorMarker) {
			return nil, fmt.Errorf("%w: %s", floorError(messageType), err.Error())
		}
		return nil, err
	}

	var reply agentPayload
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("decoding %s reply: %w", messageType, err)
	}
	return &reply, nil
}

// floorError names which agent floor a refusal came from, so the operator is told
// what the update unlocks: backups, or building and removing a rootfs.
func floorError(messageType string) error {
	if messageType == msgCTBackupRun || messageType == msgCTBackupRestore {
		return ErrAgentTooOld
	}
	return ErrAgentTooOldRootfs
}

// --- Reads ----------------------------------------------------------------

const containerColumns = `
	id, environment_id, name, COALESCE(description, ''), state,
	COALESCE(os_family, ''), COALESCE(os_version, ''),
	pool_id, pool_backend, pool_target, rootfs_path, COALESCE(template_path, ''),
	init_system, COALESCE(init_argv, ''),
	privileged, nesting,
	COALESCE(uid_map_start, 0), COALESCE(uid_map_count, 0),
	COALESCE(gid_map_start, 0), COALESCE(gid_map_count, 0), idmap_mechanism,
	COALESCE(cpu_max, ''), COALESCE(cpu_weight, 0), COALESCE(cpuset, ''),
	COALESCE(memory_max_bytes, 0), COALESCE(memory_high_bytes, 0),
	swap_max_bytes, COALESCE(pids_max, 0), COALESCE(io_max, ''),
	COALESCE(disk_quota_bytes, 0),
	COALESCE(applied_limits, ''), COALESCE(limits_applied_at, ''),
	autostart, autostart_order, autostart_delay_s,
	is_template, COALESCE(template_source_id, ''),
	COALESCE(observed_ips, ''), COALESCE(unit_name, ''), COALESCE(init_pid, 0),
	COALESCE(last_started_at, ''), COALESCE(exit_reason, ''),
	COALESCE(last_error, ''), created_at, updated_at`

// List returns the containers in an environment, or in all of them.
func (s *Service) List(ctx context.Context, envID string, templates bool) ([]Container, error) {
	query := `SELECT ` + containerColumns + ` FROM plugin_addon_lxc_containers WHERE is_template = ?`
	args := []any{boolToInt(templates)}
	if envID != "" {
		query += ` AND environment_id = ?`
		args = append(args, envID)
	}
	query += ` ORDER BY name LIMIT ?`
	args = append(args, maxRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Availability is per environment, not per container, and asking the
	// transport once per row would turn a 200-container list into 200 lookups
	// of the same answer.
	availability := map[string]bool{}
	reasons := map[string]string{}

	out := []Container{}
	for rows.Next() {
		ct, err := scanContainer(rows)
		if err != nil {
			return nil, err
		}
		if _, seen := availability[ct.EnvironmentID]; !seen {
			ok, reason := s.availability(ctx, ct.EnvironmentID)
			availability[ct.EnvironmentID] = ok
			reasons[ct.EnvironmentID] = reason
		}
		applyAvailability(&ct, availability[ct.EnvironmentID], reasons[ct.EnvironmentID])
		out = append(out, ct)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	return out, nil
}

func (s *Service) availability(ctx context.Context, envID string) (bool, string) {
	if s.rpc == nil {
		return false, "agent_bridge_missing"
	}
	return s.rpc.Available(ctx, envID)
}

// applyAvailability marks a container whose host cannot be reached.
//
// The state is forced to unknown rather than left at whatever was last stored.
// Showing "running" for a container nobody has heard from in an hour is the
// specific lie this exists to prevent.
func applyAvailability(ct *Container, available bool, reason string) {
	ct.HostAvailable = available
	ct.UnavailableReason = reason
	if !available {
		ct.State = StateUnknown
	}
}

// Get reads one container with its NICs, mounts and snapshots.
func (s *Service) Get(ctx context.Context, id string) (Container, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+containerColumns+` FROM plugin_addon_lxc_containers WHERE id = ?`, id)
	if err != nil {
		return Container{}, fmt.Errorf("reading container: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Container{}, fmt.Errorf("reading container: %w", err)
		}
		return Container{}, ErrNotFound
	}
	ct, err := scanContainer(rows)
	if err != nil {
		return Container{}, err
	}
	if err := rows.Close(); err != nil {
		return Container{}, fmt.Errorf("reading container: %w", err)
	}

	available, reason := s.availability(ctx, ct.EnvironmentID)
	applyAvailability(&ct, available, reason)

	if ct.NICs, err = s.NICs(ctx, ct.ID); err != nil {
		return Container{}, err
	}
	if ct.Mounts, err = s.Mounts(ctx, ct.ID); err != nil {
		return Container{}, err
	}
	if ct.Snapshots, err = s.Snapshots(ctx, ct.ID); err != nil {
		return Container{}, err
	}
	return ct, nil
}

// NICs returns one container's interfaces.
func (s *Service) NICs(ctx context.Context, containerID string) ([]NIC, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, container_id, network_id, bridge, COALESCE(vlan_tag, 0),
		       COALESCE(attach_kind, ''), COALESCE(parent_link, ''), COALESCE(link_mode, ''),
		       slot, guest_name, mac, COALESCE(veth_host, ''),
		       address_mode, COALESCE(static_ip, ''), COALESCE(static_gw, ''),
		       COALESCE(static_ipv6, ''), COALESCE(static_gw6, ''),
		       COALESCE(static_dns, ''), created_at, updated_at
		FROM plugin_addon_lxc_nics WHERE container_id = ? ORDER BY slot LIMIT ?`,
		containerID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading interfaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []NIC{}
	for rows.Next() {
		nic, err := scanNIC(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, nic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading interfaces: %w", err)
	}
	return out, nil
}

// Mounts returns one container's mounts.
func (s *Service) Mounts(ctx context.Context, containerID string) ([]Mount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, container_id, kind, source, destination, readonly, shift_ids,
		       created_at, updated_at
		FROM plugin_addon_lxc_mounts WHERE container_id = ?
		ORDER BY destination LIMIT ?`, containerID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading mounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Mount{}
	for rows.Next() {
		mount, err := scanMount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, mount)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading mounts: %w", err)
	}
	return out, nil
}

// Snapshots returns one container's snapshots, newest first.
func (s *Service) Snapshots(ctx context.Context, containerID string) ([]Snapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, container_id, COALESCE(parent_id, ''), name,
		       COALESCE(description, ''), mechanism, was_frozen,
		       COALESCE(size_bytes, 0), chain_depth, handle, created_at, updated_at
		FROM plugin_addon_lxc_snapshots WHERE container_id = ?
		ORDER BY created_at DESC LIMIT ?`, containerID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Snapshot{}
	for rows.Next() {
		snap, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading snapshots: %w", err)
	}
	return out, nil
}

// Provisioning reads the first-boot setup for a container.
func (s *Service) Provisioning(ctx context.Context, containerID string) (Provisioning, error) {
	rows, err := s.db.Query(ctx, `
		SELECT container_id, COALESCE(hostname, ''), COALESCE(timezone, ''),
		       COALESCE(ssh_keys, ''), COALESCE(password_hash, ''),
		       net_config_format, COALESCE(applied_at, ''), created_at, updated_at
		FROM plugin_addon_lxc_provisioning WHERE container_id = ?`, containerID)
	if err != nil {
		return Provisioning{}, fmt.Errorf("reading provisioning: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Provisioning{}, fmt.Errorf("reading provisioning: %w", err)
		}
		return Provisioning{}, ErrNotFound
	}
	return scanProvisioning(rows)
}

// passwordHash reads the stored crypt(3) hash. Separate from Provisioning
// because that type deliberately does not carry it — this is the one caller
// that legitimately needs it, on its way to the agent.
func (s *Service) passwordHash(ctx context.Context, containerID string) (string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT COALESCE(password_hash, '') FROM plugin_addon_lxc_provisioning WHERE container_id = ?`,
		containerID)
	if err != nil {
		return "", fmt.Errorf("reading provisioning: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("reading provisioning: %w", err)
		}
		return "", nil
	}
	var hash string
	if err := rows.Scan(&hash); err != nil {
		return "", fmt.Errorf("reading provisioning: %w", err)
	}
	return hash, nil
}

// Stats samples one running container's cgroup.
func (s *Service) Stats(ctx context.Context, id string) (Stats, error) {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return Stats{}, err
	}
	if ct.State != StateRunning && ct.State != StateFrozen {
		return Stats{}, ErrNotRunning
	}

	reply, err := s.call(ctx, ct.EnvironmentID, msgCTQuery, &agentPayload{
		Operation:   "stats",
		ContainerID: ct.ID,
	})
	if err != nil {
		return Stats{}, err
	}
	if reply.Stats == nil {
		// Not "not running": the row says it is, and an agent that predates
		// container stats answers the query as a reconcile with no sample.
		return Stats{}, ErrStatsUnavailable
	}
	return statsFromAgent(*reply.Stats), nil
}

// Processes lists what is running inside a container.
//
// Straight through to the agent with nothing cached: a process list is stale
// the moment it is taken, and a cached one is worse than none because it looks
// current.
func (s *Service) Processes(ctx context.Context, id string) ([]Process, error) {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if ct.State != StateRunning && ct.State != StateFrozen {
		return nil, ErrNotRunning
	}

	reply, err := s.call(ctx, ct.EnvironmentID, msgCTProcesses, &agentPayload{
		ContainerID: ct.ID,
	})
	if err != nil {
		return nil, err
	}

	out := make([]Process, 0, len(reply.Processes))
	for _, proc := range reply.Processes {
		out = append(out, Process(proc))
	}
	return out, nil
}

// --- Create ---------------------------------------------------------------

// Create materializes a container: the row first, then the rootfs on the host.
//
// The order matters and is the opposite of the intuitive one. Writing the row
// first means a create that dies halfway leaves a visible container in a failed
// state with its last error attached, which the operator can inspect and
// delete. Calling the agent first and the database second means a rootfs on
// disk that McHarbor has never heard of — an orphan only the reconcile pass
// will ever find.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Container, error) {
	ct, err := s.defineContainer(ctx, req, createOrigin{})
	if err != nil {
		return Container{}, err
	}
	if err := s.materialize(ctx, ct, req.ImageID); err != nil {
		if errors.Is(err, ErrAgentTooOldRootfs) {
			// The host refused before the agent saw anything, so there is nothing
			// on disk to inspect and a crashed row would only be clutter.
			s.discardDefinition(ctx, ct.ID)
			return Container{}, err
		}
		return Container{}, s.failCreate(ctx, ct.ID, err)
	}
	if ct.OSFamily == "" {
		s.recordOSRelease(ctx, ct)
	}

	if req.StartAfterCreate {
		if err := s.Start(ctx, ct.ID); err != nil {
			// Not a create failure: the container exists and is correct, it
			// just did not come up. The operator gets it in the list with the
			// error attached rather than losing the work.
			s.logger.Warn("system containers: created but did not start",
				"container", ct.ID, "error", err)
		}
	}

	return s.Get(ctx, ct.ID)
}

// createOrigin carries what a create learns from somewhere other than its body:
// the distribution a template or a backup manifest says the rootfs holds.
type createOrigin struct {
	OSFamily  string
	OSVersion string
}

// failCreate records a create that died after its row was written, and returns
// the cause so the caller can hand it straight back.
func (s *Service) failCreate(ctx context.Context, id string, cause error) error {
	s.markCreateFailed(ctx, id, cause)
	return cause
}

// discardDefinition removes a row whose create never reached the host; the child
// rows go with it through ON DELETE CASCADE.
func (s *Service) discardDefinition(ctx context.Context, id string) {
	if _, err := s.db.Exec(ctx, `DELETE FROM plugin_addon_lxc_containers WHERE id = ?`, id); err != nil {
		s.logger.Error("system containers: discarding a refused create",
			"container", id, "error", err)
	}
}

// markCreateFailed leaves a half-built container visible as crashed with its
// cause attached, rather than deleting a row the operator may want to inspect.
func (s *Service) markCreateFailed(ctx context.Context, id string, cause error) {
	if err := s.setState(ctx, id, StateCrashed, cause.Error()); err != nil {
		s.logger.Error("system containers: recording a failed create",
			"container", id, "error", err)
	}
}

// defineContainer writes a new container's row, provisioning, interfaces and
// mounts, without asking the host to build anything yet.
//
// Split from Create so a recreate from a backup can write the definition, lay the
// restored rootfs down, and only then materialize — an image unpacked first would
// be thrown away by the restore a moment later.
func (s *Service) defineContainer(ctx context.Context, req CreateRequest, origin createOrigin) (Container, error) {
	if s.rpc == nil {
		return Container{}, ErrHostUnavailable
	}

	capability, err := s.probe(ctx, req.EnvironmentID)
	if err != nil {
		return Container{}, err
	}
	if !capability.CanRunCTs {
		return Container{}, fmt.Errorf("%w: %s", ErrNoCapability, capability.UnsupportedWhy)
	}

	taken, err := s.nameTaken(ctx, req.EnvironmentID, req.Name, "")
	if err != nil {
		return Container{}, err
	}
	if taken {
		return Container{}, ErrNameTaken
	}
	if req.ImageID != "" {
		if err := s.requireCachedImage(ctx, req.EnvironmentID, req.ImageID); err != nil {
			return Container{}, err
		}
	}

	poolBackend, poolTarget, err := s.resolvePool(ctx, req.EnvironmentID, req.PoolID, req.PoolBackend, req.PoolTarget)
	if err != nil {
		return Container{}, err
	}

	now := time.Now().UTC()
	id := xid.New().String()

	initSystem := req.InitSystem
	if initSystem == "" {
		initSystem = InitSystemd
	}

	limits := withLimitDefaults(req.Limits)

	ct := Container{
		ID:              id,
		EnvironmentID:   req.EnvironmentID,
		Name:            req.Name,
		Description:     req.Description,
		State:           StateDefined,
		PoolID:          req.PoolID,
		PoolBackend:     poolBackend,
		PoolTarget:      poolTarget,
		RootfsPath:      rootfsPath(poolTarget, id),
		InitSystem:      initSystem,
		Privileged:      req.Privileged,
		Nesting:         req.Nesting,
		Limits:          limits,
		Autostart:       req.Autostart,
		AutostartOrder:  req.AutostartOrder,
		AutostartDelayS: req.AutostartDelayS,
		UnitName:        unitName(id),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// An unprivileged container needs a subordinate id range nothing else on
	// this host is using. A privileged one has no user namespace at all, so it
	// gets none — and that is exactly what makes it dangerous.
	if !req.Privileged {
		ct.UIDMapStart, ct.UIDMapCount, err = s.allocateIDRange(ctx, req.EnvironmentID)
		if err != nil {
			return Container{}, err
		}
		// The same range for both. Separate uid and gid ranges are possible and
		// buy nothing here: a file's owner and group both have to be shifted,
		// and two ranges means two ways to get the arithmetic wrong.
		ct.GIDMapStart, ct.GIDMapCount = ct.UIDMapStart, ct.UIDMapCount
		ct.IDMapMechanism = idmapMechanism(capability)
	}

	if req.TemplateID != "" {
		source, err := s.Get(ctx, req.TemplateID)
		if err != nil {
			return Container{}, err
		}
		ct.TemplatePath = source.RootfsPath
		ct.TemplateSourceID = source.ID
		ct.OSFamily, ct.OSVersion = source.OSFamily, source.OSVersion
	}
	if origin.OSFamily != "" {
		ct.OSFamily, ct.OSVersion = origin.OSFamily, origin.OSVersion
	}

	if err := s.insertContainer(ctx, ct); err != nil {
		return Container{}, err
	}

	// From here on a failure rolls the row back to a visible failed state
	// rather than deleting it. See the note above Create.
	fail := func(cause error) (Container, error) {
		return Container{}, s.failCreate(ctx, id, cause)
	}

	hash := ""
	if req.Password != "" {
		if hash, err = hashPassword(req.Password); err != nil {
			return fail(fmt.Errorf("hashing the container password: %w", err))
		}
	}
	if err := s.writeProvisioning(ctx, ct, req, hash); err != nil {
		return fail(err)
	}

	for _, nicReq := range req.NICs {
		if _, err := s.attachNIC(ctx, ct, nicReq); err != nil {
			return fail(err)
		}
	}
	for _, mountReq := range req.Mounts {
		if _, err := s.attachMount(ctx, ct, mountReq); err != nil {
			return fail(err)
		}
	}
	return ct, nil
}

// requireCachedImage refuses a create whose image the host has not downloaded.
// The agent would refuse it too, but only after the row exists, leaving a
// crashed container behind for what is really a missing download.
func (s *Service) requireCachedImage(ctx context.Context, envID, imageID string) error {
	cached, err := s.imageList(ctx, envID)
	if err != nil {
		return fmt.Errorf("listing the host's image cache: %w", err)
	}
	for _, image := range cached {
		if image.ImageID == imageID {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrImageNotCached, imageID)
}

// materialize asks the host to build a defined container: its rootfs from an
// image or template, and its libcontainer state.
//
// An empty imageID with no template means the rootfs is already in place, which
// is the recreate path: the restore laid it down first.
func (s *Service) materialize(ctx context.Context, ct Container, imageID string) error {
	spec, err := s.buildSpec(ctx, ct)
	if err != nil {
		return err
	}
	if imageID != "" {
		spec.TemplatePath = "" // the agent unpacks from the image instead
	}

	reply, err := s.call(ctx, ct.EnvironmentID, msgCTCreate, &agentPayload{
		ContainerID: ct.ID,
		ImageID:     imageID,
		PoolID:      ct.PoolID,
		Spec:        spec,
	})
	if err != nil {
		return err
	}
	if !reply.Success {
		return agentError(reply)
	}
	return nil
}

// buildSpec assembles what the agent needs to materialize one container.
//
// Built field by field from the row rather than by copying a struct, for the
// same reason buildDiagnosisFacts is: a new column added to the containers
// table should not silently start being sent to a root process on the host.
func (s *Service) buildSpec(ctx context.Context, ct Container) (*agentSpec, error) {
	nics, err := s.NICs(ctx, ct.ID)
	if err != nil {
		return nil, err
	}
	mounts, err := s.Mounts(ctx, ct.ID)
	if err != nil {
		return nil, err
	}
	prov, err := s.Provisioning(ctx, ct.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	hash, err := s.passwordHash(ctx, ct.ID)
	if err != nil {
		return nil, err
	}

	spec := &agentSpec{
		Name:         ct.Name,
		RootfsPath:   ct.RootfsPath,
		PoolID:       ct.PoolID,
		PoolBackend:  ct.PoolBackend,
		PoolTarget:   ct.PoolTarget,
		TemplatePath: ct.TemplatePath,
		InitSystem:   ct.InitSystem,
		InitArgv:     ct.InitArgv,
		Privileged:   ct.Privileged,
		Nesting:      ct.Nesting,
		UIDMapStart:  ct.UIDMapStart,
		UIDMapCount:  ct.UIDMapCount,
		GIDMapStart:  ct.GIDMapStart,
		GIDMapCount:  ct.GIDMapCount,
		IDMapMech:    ct.IDMapMechanism,
		Limits:       limitsToAgent(ct.Limits),
		Hostname:     prov.Hostname,
		SSHKeys:      prov.SSHKeys,
		PasswordHash: hash,
		NetConfigFmt: prov.NetConfigFormat,
		UnitName:     ct.UnitName,
	}

	for _, nic := range nics {
		spec.NICs = append(spec.NICs, agentNIC{
			NICID:       nic.ID,
			NetworkID:   nic.NetworkID,
			Bridge:      nic.Bridge,
			VLANTag:     nic.VLANTag,
			AttachKind:  nic.AttachKind,
			ParentLink:  nic.ParentLink,
			LinkMode:    nic.LinkMode,
			Slot:        nic.Slot,
			GuestName:   nic.GuestName,
			MAC:         nic.MAC,
			VethHost:    nic.VethHost,
			AddressMode: nic.AddressMode,
			StaticIP:    nic.StaticIP,
			StaticGW:    nic.StaticGW,
			StaticIPv6:  nic.StaticIPv6,
			StaticGW6:   nic.StaticGW6,
			StaticDNS:   nic.StaticDNS,
		})
	}
	for _, mount := range mounts {
		spec.Mounts = append(spec.Mounts, agentMount{
			MountID:     mount.ID,
			Kind:        mount.Kind,
			Source:      mount.Source,
			Destination: mount.Destination,
			ReadOnly:    mount.ReadOnly,
			ShiftIDs:    mount.ShiftIDs,
		})
	}
	return spec, nil
}

// --- Lifecycle ------------------------------------------------------------

// Start boots a container.
func (s *Service) Start(ctx context.Context, id string) error {
	return s.boot(ctx, id, msgCTStart)
}

// agentFailedPrefix is how the host's transport words an operation the agent
// answered with a refusal. Matched as text, like backupKeyMissingMarker: the
// sentinel lives in a host module a plugin must not link against.
const agentFailedPrefix = "agent failed "

// boot sends a start or a restart with the container's current spec. Both carry
// the spec, because the agent builds the container afresh on every start.
func (s *Service) boot(ctx context.Context, id, messageType string) error {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ct.IsTemplate {
		return ErrTemplateRunning
	}
	if messageType == msgCTStart && ct.State == StateRunning {
		return nil
	}

	spec, err := s.buildSpec(ctx, ct)
	if err != nil {
		return err
	}

	payload := &agentPayload{ContainerID: ct.ID, Spec: spec}
	if messageType == msgCTRestart {
		payload.GraceSeconds = defaultGraceSeconds
	}
	reply, err := s.call(ctx, ct.EnvironmentID, messageType, payload)
	if cause, refused := bootRefusal(messageType, reply, err); refused {
		if err := s.setState(ctx, id, StateCrashed, cause); err != nil {
			s.logger.Error("system containers: recording a failed start",
				"container", id, "error", err)
		}
	}
	if err != nil {
		return err
	}
	if !reply.Success {
		return agentError(reply)
	}

	if err := s.applyState(ctx, id, reply.State, StateRunning); err != nil {
		return err
	}
	s.recordStart(ctx, ct, reply.State, time.Now().UTC())
	return nil
}

// bootRefusal returns the reason to record on the row when the agent itself
// refused a start, and false for anything else. The host turns a refusal into a
// call error with no reply, so without this the row kept its old state and no
// lastError. A host that was unreachable or too old never ran the start, so it
// is not the container's failure to record.
func bootRefusal(messageType string, reply *agentPayload, err error) (string, bool) {
	if err != nil {
		prefix := agentFailedPrefix + messageType + ": "
		if detail := err.Error(); strings.HasPrefix(detail, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(detail, prefix)), true
		}
		return "", false
	}
	if reply != nil && !reply.Success {
		return agentError(reply).Error(), true
	}
	return "", false
}

// Shutdown asks init to stop, escalating to a cgroup kill after the grace
// period. The escalation is reported so the audit trail can record it: a
// container whose init never sees its stop signal looks identical to a slow but
// clean shutdown unless somebody says otherwise.
func (s *Service) Shutdown(ctx context.Context, id string, graceSeconds int) (escalated bool, err error) {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if ct.State != StateRunning && ct.State != StateFrozen {
		return false, ErrNotRunning
	}
	if graceSeconds <= 0 {
		graceSeconds = defaultGraceSeconds
	}

	reply, err := s.call(ctx, ct.EnvironmentID, msgCTShutdown, &agentPayload{
		ContainerID:  ct.ID,
		GraceSeconds: graceSeconds,
	})
	if err != nil {
		return false, err
	}
	if !reply.Success {
		return false, agentError(reply)
	}

	escalated = reply.State != nil && reply.State.ExitReason == "killed"
	return escalated, s.applyState(ctx, id, reply.State, StateStopped)
}

// Stop kills the container's cgroup outright.
func (s *Service) Stop(ctx context.Context, id string) error {
	return s.simpleLifecycle(ctx, id, msgCTStop, StateStopped)
}

// Restart stops and starts in one agent round trip, so the container is never
// left stopped by a failure between two separate calls.
func (s *Service) Restart(ctx context.Context, id string) error {
	return s.boot(ctx, id, msgCTRestart)
}

// Freeze suspends every process via the cgroup freezer.
//
// Distinct from stopping: memory, open files and network connections all
// survive, and a thaw resumes mid-syscall. This is what makes a consistent
// snapshot of a running container possible.
func (s *Service) Freeze(ctx context.Context, id string) error {
	return s.simpleLifecycle(ctx, id, msgCTFreeze, StateFrozen)
}

// Unfreeze thaws a frozen container.
func (s *Service) Unfreeze(ctx context.Context, id string) error {
	return s.simpleLifecycle(ctx, id, msgCTUnfreeze, StateRunning)
}

func (s *Service) simpleLifecycle(ctx context.Context, id, messageType, expected string) error {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	reply, err := s.call(ctx, ct.EnvironmentID, messageType, &agentPayload{ContainerID: ct.ID})
	if err != nil {
		return err
	}
	if !reply.Success {
		return agentError(reply)
	}
	return s.applyState(ctx, id, reply.State, expected)
}

// applyState writes what the agent reported.
//
// The expected state is a fallback for an agent that acknowledged an operation
// without attaching state, never an override: if the agent says the container
// is crashed after a start, the row says crashed.
func (s *Service) applyState(ctx context.Context, id string, state *agentState, expected string) error {
	if state == nil {
		return s.setState(ctx, id, expected, "")
	}

	var startedAt any
	if state.StartedAt != "" {
		startedAt = state.StartedAt
	}
	ips, err := json.Marshal(state.IPs)
	if err != nil {
		return fmt.Errorf("encoding observed addresses: %w", err)
	}

	_, execErr := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_containers
		SET state = ?, unit_name = COALESCE(NULLIF(?, ''), unit_name),
		    init_pid = ?, observed_ips = ?,
		    last_started_at = COALESCE(?, last_started_at),
		    exit_reason = ?, last_error = ?, updated_at = ?
		WHERE id = ?`,
		state.State, state.UnitName, livePID(state.State, state.InitPID), string(ips),
		startedAt, state.ExitReason, state.LastError,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	if execErr != nil {
		return fmt.Errorf("recording container state: %w", execErr)
	}
	return nil
}

func (s *Service) setState(ctx context.Context, id, state, lastError string) error {
	_, execErr := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_containers
		SET state = ?, last_error = ?, updated_at = ?,
		    init_pid = CASE WHEN ? THEN init_pid ELSE 0 END
		WHERE id = ?`,
		state, lastError, time.Now().UTC().Format(time.RFC3339Nano), boolToInt(isLiveState(state)), id)
	if execErr != nil {
		return fmt.Errorf("recording container state: %w", execErr)
	}
	return nil
}

// --- Limits ---------------------------------------------------------------

// UpdateLimits changes a container's cgroup limits, live where possible.
//
// This is the method the whole plugin is shaped around. cgroup v2 applies these
// to a running container immediately — no restart, no downtime — which is a
// real advantage over a VM and worth surfacing honestly rather than hiding
// behind a generic edit form.
//
// Two things it does that a naive implementation would not:
//
//   - It stores the agent's *applied* values separately from the requested
//     ones. The kernel rounds memory to a page multiple, refuses a memory.high
//     above memory.max, and clamps pids.max to what the host can carry. Echoing
//     the request back would leave an operator believing a limit that is not in
//     force.
//   - It works on a stopped container too, storing the values for next start
//     and saying `live: false`. The alternative — refusing — would mean an
//     operator cannot prepare a container's limits before booting it.
func (s *Service) UpdateLimits(ctx context.Context, id string, requested Limits) (LimitsResponse, error) {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return LimitsResponse{}, err
	}

	requested = withLimitDefaults(requested)
	live := ct.State == StateRunning || ct.State == StateFrozen
	applied := requested

	if live {
		reply, err := s.call(ctx, ct.EnvironmentID, msgCTUpdateLimits, &agentPayload{
			ContainerID: ct.ID,
			Limits:      limitsToAgent(requested),
		})
		if err != nil {
			return LimitsResponse{}, err
		}
		if !reply.Success {
			return LimitsResponse{}, agentError(reply)
		}
		if reply.Applied != nil {
			applied = limitsFromAgent(*reply.Applied)
		}
	}

	if err := s.storeLimits(ctx, ct.ID, requested, applied, live); err != nil {
		return LimitsResponse{}, err
	}

	return LimitsResponse{
		Requested: requested,
		Applied:   applied,
		Clamped:   clampedFields(requested, applied),
		Live:      live,
	}, nil
}

func (s *Service) storeLimits(ctx context.Context, id string, requested, applied Limits, live bool) error {
	encoded, err := json.Marshal(applied)
	if err != nil {
		return fmt.Errorf("encoding applied limits: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	// A stopped container's limits have not reached a kernel, so the applied
	// timestamp stays where it was rather than claiming otherwise.
	appliedAt := any(nil)
	if live {
		appliedAt = now
	}

	_, execErr := s.db.Exec(ctx, `
		UPDATE plugin_addon_lxc_containers
		SET cpu_max = ?, cpu_weight = ?, cpuset = ?,
		    memory_max_bytes = ?, memory_high_bytes = ?, swap_max_bytes = ?,
		    pids_max = ?, io_max = ?, disk_quota_bytes = ?,
		    applied_limits = ?, limits_applied_at = COALESCE(?, limits_applied_at),
		    updated_at = ?
		WHERE id = ?`,
		nullIfEmpty(requested.CPUMax), requested.CPUWeight, nullIfEmpty(requested.CPUSet),
		requested.MemoryMaxBytes, requested.MemoryHighBytes, requested.SwapMaxBytes,
		requested.PidsMax, nullIfEmpty(requested.IOMax), requested.DiskQuotaBytes,
		string(encoded), appliedAt, now, id)
	if execErr != nil {
		return fmt.Errorf("recording limits: %w", execErr)
	}
	return nil
}

// --- Update and delete ----------------------------------------------------

// Update edits a container's metadata. Limits go through UpdateLimits.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Container, error) {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return Container{}, err
	}

	var sets []string
	var args []any

	if req.Name != nil && *req.Name != ct.Name {
		taken, err := s.nameTaken(ctx, ct.EnvironmentID, *req.Name, ct.ID)
		if err != nil {
			return Container{}, err
		}
		if taken {
			return Container{}, ErrNameTaken
		}
		sets, args = append(sets, "name = ?"), append(args, *req.Name)
	}
	if req.Description != nil {
		sets, args = append(sets, "description = ?"), append(args, *req.Description)
	}
	if req.Autostart != nil {
		sets, args = append(sets, "autostart = ?"), append(args, boolToInt(*req.Autostart))
	}
	if req.AutostartOrder != nil {
		sets, args = append(sets, "autostart_order = ?"), append(args, *req.AutostartOrder)
	}
	if req.AutostartDelayS != nil {
		sets, args = append(sets, "autostart_delay_s = ?"), append(args, *req.AutostartDelayS)
	}
	if req.Nesting != nil {
		// Nesting changes the device cgroup and the apparmor profile, neither
		// of which can be swapped under a live container.
		if ct.State == StateRunning || ct.State == StateFrozen {
			return Container{}, ErrRunning
		}
		sets, args = append(sets, "nesting = ?"), append(args, boolToInt(*req.Nesting))
	}

	if len(sets) == 0 {
		return ct, nil
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339Nano), id)

	_, execErr := s.db.Exec(ctx,
		`UPDATE plugin_addon_lxc_containers SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if execErr != nil {
		return Container{}, fmt.Errorf("updating container: %w", execErr)
	}
	return s.Get(ctx, id)
}

// Delete destroys a container and, unless asked otherwise, its rootfs.
func (s *Service) Delete(ctx context.Context, id string, req DeleteRequest) error {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ct.State == StateRunning || ct.State == StateFrozen {
		return ErrRunning
	}

	// A container whose rootfs still backs a linked clone cannot go: deleting
	// it would corrupt every clone at once, silently, at the next read. Any
	// container can be a linked clone's source, not only a marked template.
	clones, err := s.linkedClones(ctx, ct.ID)
	if err != nil {
		return err
	}
	if len(clones) > 0 {
		return fmt.Errorf("%w: %s", ErrTemplateInUse, strings.Join(clones, ", "))
	}

	// The full spec travels with the delete: the agent needs the rootfs path and
	// pool to remove the tree, and the NICs to remove their host ends by name.
	spec, err := s.buildSpec(ctx, ct)
	if err != nil {
		return err
	}
	reply, err := s.call(ctx, ct.EnvironmentID, msgCTDelete, &agentPayload{
		ContainerID:  ct.ID,
		DeleteRootfs: !req.KeepRootfs,
		Spec:         spec,
	})
	// A host that cannot be reached is the one case where the row stays: the
	// rootfs is still out there, and removing the row would turn a recoverable
	// situation into an invisible orphan.
	if err != nil {
		return err
	}
	if !reply.Success {
		return agentError(reply)
	}

	// The child rows go with it through ON DELETE CASCADE.
	_, execErr := s.db.Exec(ctx, `DELETE FROM plugin_addon_lxc_containers WHERE id = ?`, id)
	if execErr != nil {
		return fmt.Errorf("deleting container: %w", execErr)
	}
	return nil
}

func (s *Service) linkedClones(ctx context.Context, templateID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT name FROM plugin_addon_lxc_containers
		WHERE template_source_id = ? AND template_path IS NOT NULL AND template_path != ''
		LIMIT ?`, templateID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("checking linked clones: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("checking linked clones: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checking linked clones: %w", err)
	}
	return names, nil
}

// --- Interfaces -----------------------------------------------------------

// AddNIC attaches one interface.
func (s *Service) AddNIC(ctx context.Context, containerID string, req CreateNICRequest) (NIC, error) {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return NIC{}, err
	}
	// Hot-plugging a veth into a running container's netns works, but the
	// container's own network configuration will not know about it until it
	// restarts, which produces an interface that exists and does nothing. A
	// stopped container is the honest requirement.
	if ct.State == StateRunning || ct.State == StateFrozen {
		return NIC{}, ErrRunning
	}
	return s.attachNIC(ctx, ct, req)
}

func (s *Service) attachNIC(ctx context.Context, ct Container, req CreateNICRequest) (NIC, error) {
	slot, err := s.nextNICSlot(ctx, ct.ID)
	if err != nil {
		return NIC{}, err
	}

	guestName := req.GuestName
	if guestName == "" {
		guestName = fmt.Sprintf("eth%d", slot)
	}
	addressMode := req.AddressMode
	if addressMode == "" {
		addressMode = "dhcp"
	}

	if err := validateNICIPv6(req.AttachKind, req.StaticIPv6, req.StaticGW6); err != nil {
		return NIC{}, err
	}

	dns, err := json.Marshal(req.StaticDNS)
	if err != nil {
		return NIC{}, fmt.Errorf("encoding dns servers: %w", err)
	}

	mac, err := generateMAC()
	if err != nil {
		return NIC{}, err
	}

	now := time.Now().UTC()
	nic := NIC{
		ID:          xid.New().String(),
		ContainerID: ct.ID,
		NetworkID:   req.NetworkID,
		Bridge:      req.Bridge,
		VLANTag:     req.VLANTag,
		AttachKind:  req.AttachKind,
		ParentLink:  req.ParentLink,
		LinkMode:    req.LinkMode,
		Slot:        slot,
		GuestName:   guestName,
		MAC:         mac,
		VethHost:    vethName(ct.ID, slot),
		AddressMode: addressMode,
		StaticIP:    req.StaticIP,
		StaticGW:    req.StaticGW,
		StaticIPv6:  req.StaticIPv6,
		StaticGW6:   req.StaticGW6,
		StaticDNS:   req.StaticDNS,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// The MAC is globally unique, so a collision is possible and retried
	// rather than allowed — two guests with one MAC on a segment presents as
	// intermittent packet loss and names nothing.
	for attempt := 0; ; attempt++ {
		_, execErr := s.db.Exec(ctx, `
			INSERT INTO plugin_addon_lxc_nics
			(id, container_id, network_id, bridge, vlan_tag,
			 attach_kind, parent_link, link_mode, slot, guest_name, mac,
			 veth_host, address_mode, static_ip, static_gw, static_ipv6, static_gw6,
			 static_dns, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nic.ID, nic.ContainerID, nic.NetworkID, nic.Bridge, nullIfZero(nic.VLANTag),
			nullIfEmpty(nic.AttachKind), nullIfEmpty(nic.ParentLink), nullIfEmpty(nic.LinkMode),
			nic.Slot, nic.GuestName, nic.MAC, nic.VethHost, nic.AddressMode,
			nullIfEmpty(nic.StaticIP), nullIfEmpty(nic.StaticGW),
			nullIfEmpty(nic.StaticIPv6), nullIfEmpty(nic.StaticGW6), string(dns),
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if execErr == nil {
			return nic, nil
		}
		if attempt >= 8 || !isUniqueViolation(execErr) {
			return NIC{}, fmt.Errorf("attaching an interface: %w", execErr)
		}
		if nic.MAC, err = generateMAC(); err != nil {
			return NIC{}, err
		}
	}
}

func (s *Service) nextNICSlot(ctx context.Context, containerID string) (int, error) {
	rows, err := s.db.Query(ctx,
		`SELECT COALESCE(MAX(slot), -1) FROM plugin_addon_lxc_nics WHERE container_id = ?`,
		containerID)
	if err != nil {
		return 0, fmt.Errorf("allocating an interface slot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("allocating an interface slot: %w", err)
		}
		return 0, nil
	}
	var highest int
	if err := rows.Scan(&highest); err != nil {
		return 0, fmt.Errorf("allocating an interface slot: %w", err)
	}
	return highest + 1, nil
}

// DeleteNIC removes an interface.
func (s *Service) DeleteNIC(ctx context.Context, containerID, nicID string) error {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return err
	}
	if ct.State == StateRunning || ct.State == StateFrozen {
		return ErrRunning
	}

	affected, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_lxc_nics WHERE id = ? AND container_id = ?`, nicID, containerID)
	if err != nil {
		return fmt.Errorf("removing an interface: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Mounts ---------------------------------------------------------------

// AddMount attaches one mount.
//
// The permission for this was already checked in the handler, which is where
// the caller's identity lives. What happens here is the second, independent
// check on the mount itself: a volume must be inside the container's own pool.
// A caller who fakes `kind: "volume"` for a path outside the pool gets it
// rejected here, and again by the agent.
func (s *Service) AddMount(ctx context.Context, containerID string, req CreateMountRequest) (Mount, error) {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return Mount{}, err
	}
	if ct.State == StateRunning || ct.State == StateFrozen {
		return Mount{}, ErrRunning
	}
	return s.attachMount(ctx, ct, req)
}

func (s *Service) attachMount(ctx context.Context, ct Container, req CreateMountRequest) (Mount, error) {
	if err := validateMount(ct, req); err != nil {
		return Mount{}, err
	}

	shift := true
	if req.ShiftIDs != nil {
		shift = *req.ShiftIDs
	}

	now := time.Now().UTC()
	mount := Mount{
		ID:          xid.New().String(),
		ContainerID: ct.ID,
		Kind:        req.Kind,
		Source:      req.Source,
		Destination: req.Destination,
		ReadOnly:    req.ReadOnly,
		ShiftIDs:    shift,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, execErr := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_mounts
		(id, container_id, kind, source, destination, readonly, shift_ids,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mount.ID, mount.ContainerID, mount.Kind, mount.Source, mount.Destination,
		boolToInt(mount.ReadOnly), boolToInt(mount.ShiftIDs),
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if execErr != nil {
		if isUniqueViolation(execErr) {
			return Mount{}, fmt.Errorf("%w: %s is already mounted", ErrBadMount, req.Destination)
		}
		return Mount{}, fmt.Errorf("adding a mount: %w", execErr)
	}
	return mount, nil
}

// DeleteMount removes a mount.
func (s *Service) DeleteMount(ctx context.Context, containerID, mountID string) error {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return err
	}
	if ct.State == StateRunning || ct.State == StateFrozen {
		return ErrRunning
	}

	affected, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_lxc_mounts WHERE id = ? AND container_id = ?`,
		mountID, containerID)
	if err != nil {
		return fmt.Errorf("removing a mount: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Snapshots ------------------------------------------------------------

// CreateSnapshot takes a snapshot, freezing the container first when asked.
//
// Freezing is the default for a running container and costs milliseconds on a
// CoW pool. Without it the snapshot is crash-consistent: everything in page
// cache that had not reached disk is simply absent, which is the same state a
// power cut produces. The flag is recorded so the operator can tell the two
// apart later, when it matters.
func (s *Service) CreateSnapshot(ctx context.Context, containerID string, req CreateSnapshotRequest) (Snapshot, error) {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return Snapshot{}, err
	}

	depth, err := s.chainDepth(ctx, containerID)
	if err != nil {
		return Snapshot{}, err
	}
	if depth >= maxChainDepth {
		return Snapshot{}, ErrChainTooDeep
	}

	running := ct.State == StateRunning
	freeze := running
	if req.Freeze != nil {
		freeze = *req.Freeze && running
	}

	if freeze {
		if err := s.Freeze(ctx, containerID); err != nil {
			return Snapshot{}, err
		}
		// Thaw whatever happens next. A container left frozen by a failed
		// snapshot is indistinguishable from a hung one from the outside.
		defer func() {
			if err := s.Unfreeze(ctx, containerID); err != nil {
				s.logger.Error("system containers: thawing after a snapshot",
					"container", containerID, "error", err)
			}
		}()
	}

	// The spec travels with every snapshot call: the agent keeps a snapshot
	// beside the rootfs and must be told where that is, never guess.
	spec, err := s.buildSpec(ctx, ct)
	if err != nil {
		return Snapshot{}, err
	}
	id := xid.New().String()
	reply, err := s.call(ctx, ct.EnvironmentID, msgCTSnapshotCreate, &agentPayload{
		ContainerID:  ct.ID,
		SnapshotID:   id,
		SnapshotName: req.Name,
		Spec:         spec,
	})
	if err != nil {
		return Snapshot{}, err
	}
	if !reply.Success {
		return Snapshot{}, agentError(reply)
	}

	parentID, err := s.latestSnapshotID(ctx, containerID)
	if err != nil {
		return Snapshot{}, err
	}

	now := time.Now().UTC()
	snap := Snapshot{
		ID:          id,
		ContainerID: ct.ID,
		ParentID:    parentID,
		Name:        req.Name,
		Description: req.Description,
		Mechanism:   mechanismFor(ct.PoolBackend),
		WasFrozen:   freeze,
		SizeBytes:   reply.SizeBytes,
		ChainDepth:  depth + 1,
		Handle:      snapshotHandle(reply, ct, req.Name),
	}
	snap.CreatedAt, snap.UpdatedAt = now, now

	_, execErr := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_snapshots
		(id, container_id, parent_id, name, description, mechanism, was_frozen,
		 size_bytes, chain_depth, handle, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.ContainerID, nullIfEmpty(snap.ParentID), snap.Name,
		nullIfEmpty(snap.Description), snap.Mechanism, boolToInt(snap.WasFrozen),
		snap.SizeBytes, snap.ChainDepth, snap.Handle,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if execErr != nil {
		if isUniqueViolation(execErr) {
			return Snapshot{}, ErrNameTaken
		}
		return Snapshot{}, fmt.Errorf("recording a snapshot: %w", execErr)
	}
	return snap, nil
}

// RevertSnapshot rolls a container's rootfs back.
//
// Destructive to everything after the snapshot, including later snapshots,
// which is why the container must be stopped: reverting the filesystem under a
// running init produces a container whose processes reference inodes that no
// longer exist, and it does not fail cleanly.
func (s *Service) RevertSnapshot(ctx context.Context, containerID, snapshotID string) error {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return err
	}
	if ct.State == StateRunning || ct.State == StateFrozen {
		return ErrRunning
	}

	snap, err := s.snapshot(ctx, containerID, snapshotID)
	if err != nil {
		return err
	}

	spec, err := s.buildSpec(ctx, ct)
	if err != nil {
		return err
	}
	reply, err := s.call(ctx, ct.EnvironmentID, msgCTSnapshotRevert, &agentPayload{
		ContainerID: ct.ID,
		SnapshotID:  snap.ID,
		Spec:        spec,
	})
	if err != nil {
		return err
	}
	if !reply.Success {
		return agentError(reply)
	}

	// A revert discards the history after the snapshot. Each snapshot is its
	// own copy on the pool, so the later ones are removed there as well as
	// here; leaving the copies would fill the pool with snapshots nobody can
	// see or reach.
	s.dropLaterSnapshots(ctx, ct, spec, snap.CreatedAt)
	_, execErr := s.db.Exec(ctx, `
		DELETE FROM plugin_addon_lxc_snapshots
		WHERE container_id = ? AND created_at > ?`,
		containerID, snap.CreatedAt.Format(time.RFC3339Nano))
	if execErr != nil {
		return fmt.Errorf("pruning superseded snapshots: %w", execErr)
	}
	return nil
}

// DeleteSnapshot drops one snapshot.
func (s *Service) DeleteSnapshot(ctx context.Context, containerID, snapshotID string) error {
	ct, err := s.Get(ctx, containerID)
	if err != nil {
		return err
	}
	snap, err := s.snapshot(ctx, containerID, snapshotID)
	if err != nil {
		return err
	}

	spec, err := s.buildSpec(ctx, ct)
	if err != nil {
		return err
	}
	reply, err := s.call(ctx, ct.EnvironmentID, msgCTSnapshotDelete, &agentPayload{
		ContainerID: ct.ID,
		SnapshotID:  snap.ID,
		Spec:        spec,
	})
	if err != nil {
		return err
	}
	if !reply.Success {
		return agentError(reply)
	}

	_, execErr := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_lxc_snapshots WHERE id = ? AND container_id = ?`,
		snapshotID, containerID)
	if execErr != nil {
		return fmt.Errorf("deleting a snapshot: %w", execErr)
	}
	return nil
}

// dropLaterSnapshots removes from the pool every snapshot taken after a revert
// point. Best effort: a copy the host could not remove is logged, and its row
// goes anyway, because the revert already made it meaningless.
func (s *Service) dropLaterSnapshots(ctx context.Context, ct Container, spec *agentSpec, after time.Time) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM plugin_addon_lxc_snapshots
		WHERE container_id = ? AND created_at > ? LIMIT ?`,
		ct.ID, after.Format(time.RFC3339Nano), maxRows)
	if err != nil {
		s.logger.Warn("system containers: listing snapshots superseded by a revert",
			"container", ct.ID, "error", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		s.logger.Warn("system containers: reading snapshots superseded by a revert",
			"container", ct.ID, "error", err)
	}
	for _, id := range ids {
		reply, err := s.call(ctx, ct.EnvironmentID, msgCTSnapshotDelete, &agentPayload{
			ContainerID: ct.ID, SnapshotID: id, Spec: spec,
		})
		if err == nil && !reply.Success {
			err = agentError(reply)
		}
		if err != nil {
			s.logger.Warn("system containers: removing a snapshot superseded by a revert",
				"container", ct.ID, "snapshot", id, "error", err)
		}
	}
}

func (s *Service) snapshot(ctx context.Context, containerID, snapshotID string) (Snapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, container_id, COALESCE(parent_id, ''), name,
		       COALESCE(description, ''), mechanism, was_frozen,
		       COALESCE(size_bytes, 0), chain_depth, handle, created_at, updated_at
		FROM plugin_addon_lxc_snapshots WHERE id = ? AND container_id = ?`,
		snapshotID, containerID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("reading a snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Snapshot{}, fmt.Errorf("reading a snapshot: %w", err)
		}
		return Snapshot{}, ErrNotFound
	}
	return scanSnapshot(rows)
}

func (s *Service) chainDepth(ctx context.Context, containerID string) (int, error) {
	rows, err := s.db.Query(ctx,
		`SELECT COALESCE(MAX(chain_depth), 0) FROM plugin_addon_lxc_snapshots WHERE container_id = ?`,
		containerID)
	if err != nil {
		return 0, fmt.Errorf("measuring the snapshot chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("measuring the snapshot chain: %w", err)
		}
		return 0, nil
	}
	var depth int
	if err := rows.Scan(&depth); err != nil {
		return 0, fmt.Errorf("measuring the snapshot chain: %w", err)
	}
	return depth, nil
}

func (s *Service) latestSnapshotID(ctx context.Context, containerID string) (string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM plugin_addon_lxc_snapshots
		WHERE container_id = ? ORDER BY created_at DESC LIMIT 1`, containerID)
	if err != nil {
		return "", fmt.Errorf("reading the snapshot chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("reading the snapshot chain: %w", err)
		}
		return "", nil
	}
	var id string
	if err := rows.Scan(&id); err != nil {
		return "", fmt.Errorf("reading the snapshot chain: %w", err)
	}
	return id, nil
}

// --- Clone and template ---------------------------------------------------

// Clone copies a container. On a CoW pool this is a metadata operation.
func (s *Service) Clone(ctx context.Context, sourceID string, req CloneRequest) (Container, error) {
	source, err := s.Get(ctx, sourceID)
	if err != nil {
		return Container{}, err
	}
	// A running source would be cloned mid-write. Freezing it would be
	// possible but would stop a production container to copy it, which is a
	// surprise nobody wants from a "clone" button.
	if source.State == StateRunning || source.State == StateFrozen {
		return Container{}, ErrRunning
	}

	taken, err := s.nameTaken(ctx, source.EnvironmentID, req.Name, "")
	if err != nil {
		return Container{}, err
	}
	if taken {
		return Container{}, ErrNameTaken
	}

	// A clone into another pool resolves that pool like a create would; staying
	// in the source's pool takes the source's stored copy.
	poolID, poolBackend, poolTarget := source.PoolID, source.PoolBackend, source.PoolTarget
	if req.PoolID != "" {
		poolID = req.PoolID
		poolBackend, poolTarget, err = s.resolvePool(ctx, source.EnvironmentID, req.PoolID, req.PoolBackend, req.PoolTarget)
		if err != nil {
			return Container{}, err
		}
	}

	now := time.Now().UTC()
	id := xid.New().String()

	clone := source
	clone.ID = id
	clone.Name = req.Name
	clone.State = StateDefined
	clone.PoolID, clone.PoolBackend, clone.PoolTarget = poolID, poolBackend, poolTarget
	clone.RootfsPath = rootfsPath(poolTarget, id)
	clone.IsTemplate = false
	clone.TemplateSourceID = source.ID
	clone.UnitName = unitName(id)
	clone.ObservedIPs = nil
	clone.InitPID = 0
	clone.LastStartedAt = nil
	clone.ExitReason, clone.LastError = "", ""
	clone.CreatedAt, clone.UpdatedAt = now, now
	clone.NICs, clone.Mounts, clone.Snapshots = nil, nil, nil

	// A clone is recorded as linked only when the host says it made one. The
	// request can only ask: a rootfs re-owned into the clone's own id range
	// shares nothing with its source, and recording a link that is not there
	// would make the source undeletable for no reason.
	wantLinked := req.Linked && poolID == source.PoolID && supportsCoW(source.PoolBackend)
	clone.TemplatePath = ""

	// A fresh id range: reusing the source's would let the clone's root reach
	// the source's files, which is the exact failure unprivileged containers
	// exist to prevent.
	if !clone.Privileged {
		clone.UIDMapStart, clone.UIDMapCount, err = s.allocateIDRange(ctx, clone.EnvironmentID)
		if err != nil {
			return Container{}, err
		}
		clone.GIDMapStart, clone.GIDMapCount = clone.UIDMapStart, clone.UIDMapCount
	}

	if err := s.insertContainer(ctx, clone); err != nil {
		return Container{}, err
	}

	fail := func(cause error) (Container, error) {
		if err := s.setState(ctx, id, StateCrashed, cause.Error()); err != nil {
			s.logger.Error("system containers: recording a failed clone",
				"container", id, "error", err)
		}
		return Container{}, cause
	}

	// Provisioning is copied but the hostname is not: two containers answering
	// to one hostname on a segment is the classic clone footgun, and it takes a
	// while to diagnose because everything looks correct from inside each one.
	if err := s.cloneProvisioning(ctx, source.ID, clone.ID, req.Hostname, req.Name); err != nil {
		return fail(err)
	}
	// New MACs for the same reason.
	for _, nic := range source.NICs {
		if _, err := s.attachNIC(ctx, clone, CreateNICRequest{
			NetworkID:   nic.NetworkID,
			Bridge:      nic.Bridge,
			VLANTag:     nic.VLANTag,
			AttachKind:  nic.AttachKind,
			ParentLink:  nic.ParentLink,
			LinkMode:    nic.LinkMode,
			GuestName:   nic.GuestName,
			AddressMode: nic.AddressMode,
			// A static address is deliberately not copied: two containers on
			// one address is worse than a clone that comes up on DHCP.
		}); err != nil {
			return fail(err)
		}
	}
	for _, mount := range source.Mounts {
		if _, err := s.attachMount(ctx, clone, CreateMountRequest{
			Kind:        mount.Kind,
			Source:      mount.Source,
			Destination: mount.Destination,
			ReadOnly:    mount.ReadOnly,
			ShiftIDs:    &mount.ShiftIDs,
		}); err != nil {
			return fail(err)
		}
	}

	spec, err := s.buildSpec(ctx, clone)
	if err != nil {
		return fail(err)
	}

	sourceSpec, err := s.buildSpec(ctx, source)
	if err != nil {
		return fail(err)
	}
	reply, err := s.call(ctx, source.EnvironmentID, msgCTCloneRequest, &agentPayload{
		ContainerID: source.ID,
		Spec:        sourceSpec,
		Clone: &agentClone{
			TargetContainerID: clone.ID,
			Spec:              spec,
			Linked:            wantLinked,
		},
	})
	if err != nil {
		return fail(err)
	}
	if !reply.Success {
		return fail(agentError(reply))
	}
	if reply.Clone != nil && reply.Clone.Linked {
		if _, err := s.db.Exec(ctx,
			`UPDATE plugin_addon_lxc_containers SET template_path = ? WHERE id = ?`,
			source.RootfsPath, clone.ID); err != nil {
			return fail(fmt.Errorf("recording the clone's link to its source: %w", err))
		}
	}

	if req.Start {
		if err := s.Start(ctx, clone.ID); err != nil {
			s.logger.Warn("system containers: cloned but did not start",
				"container", clone.ID, "error", err)
		}
	}
	return s.Get(ctx, clone.ID)
}

// MakeTemplate marks a container as a template, or unmarks it.
func (s *Service) MakeTemplate(ctx context.Context, id string, isTemplate bool) error {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if isTemplate && (ct.State == StateRunning || ct.State == StateFrozen) {
		return ErrRunning
	}
	if !isTemplate {
		clones, err := s.linkedClones(ctx, id)
		if err != nil {
			return err
		}
		if len(clones) > 0 {
			return fmt.Errorf("%w: %s", ErrTemplateInUse, strings.Join(clones, ", "))
		}
	}

	_, execErr := s.db.Exec(ctx,
		`UPDATE plugin_addon_lxc_containers SET is_template = ?, updated_at = ? WHERE id = ?`,
		boolToInt(isTemplate), time.Now().UTC().Format(time.RFC3339Nano), id)
	if execErr != nil {
		return fmt.Errorf("marking a template: %w", execErr)
	}
	return nil
}

// --- Reconcile ------------------------------------------------------------

// Reconcile asks an agent for every container it is actually running and
// aligns the rows with the answer.
//
// Returns the orphans: units on the host with no row here. They are reported
// rather than killed — a container McHarbor does not know about might be one an
// operator created by hand, and deleting it because it is unfamiliar is not a
// decision a background pass should make.
func (s *Service) Reconcile(ctx context.Context, envID string) ([]string, error) {
	reply, err := s.call(ctx, envID, msgCTQuery, &agentPayload{
		Operation: "reconcile",
		EnvID:     envID,
	})
	if err != nil {
		return nil, err
	}

	for _, state := range reply.States {
		observed := state
		if err := s.applyState(ctx, observed.ContainerID, &observed, observed.State); err != nil {
			s.logger.Error("system containers: reconciling state",
				"container", observed.ContainerID, "error", err)
		}
	}
	return reply.Orphans, nil
}

// --- Writes ---------------------------------------------------------------

func (s *Service) insertContainer(ctx context.Context, ct Container) error {
	argv, err := json.Marshal(ct.InitArgv)
	if err != nil {
		return fmt.Errorf("encoding the init command: %w", err)
	}
	stamp := ct.CreatedAt.Format(time.RFC3339Nano)

	_, execErr := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_containers
		(id, environment_id, name, description, state, os_family, os_version,
		 pool_id, pool_backend, pool_target, rootfs_path, template_path,
		 init_system, init_argv, privileged, nesting,
		 uid_map_start, uid_map_count, gid_map_start, gid_map_count, idmap_mechanism,
		 cpu_max, cpu_weight, cpuset, memory_max_bytes, memory_high_bytes,
		 swap_max_bytes, pids_max, io_max, disk_quota_bytes,
		 autostart, autostart_order, autostart_delay_s,
		 is_template, template_source_id, unit_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ct.ID, ct.EnvironmentID, ct.Name, nullIfEmpty(ct.Description), ct.State,
		nullIfEmpty(ct.OSFamily), nullIfEmpty(ct.OSVersion),
		ct.PoolID, ct.PoolBackend, ct.PoolTarget, ct.RootfsPath, nullIfEmpty(ct.TemplatePath),
		ct.InitSystem, string(argv), boolToInt(ct.Privileged), boolToInt(ct.Nesting),
		nullIfZero64(ct.UIDMapStart), nullIfZero64(ct.UIDMapCount),
		nullIfZero64(ct.GIDMapStart), nullIfZero64(ct.GIDMapCount),
		defaultString(ct.IDMapMechanism, "idmapped"),
		nullIfEmpty(ct.Limits.CPUMax), nullIfZero(ct.Limits.CPUWeight),
		nullIfEmpty(ct.Limits.CPUSet), nullIfZero64(ct.Limits.MemoryMaxBytes),
		nullIfZero64(ct.Limits.MemoryHighBytes), ct.Limits.SwapMaxBytes,
		nullIfZero64(ct.Limits.PidsMax), nullIfEmpty(ct.Limits.IOMax),
		nullIfZero64(ct.Limits.DiskQuotaBytes),
		boolToInt(ct.Autostart), ct.AutostartOrder, ct.AutostartDelayS,
		boolToInt(ct.IsTemplate), nullIfEmpty(ct.TemplateSourceID), ct.UnitName,
		stamp, stamp)
	if execErr != nil {
		if isUniqueViolation(execErr) {
			return ErrNameTaken
		}
		return fmt.Errorf("creating container: %w", execErr)
	}
	return nil
}

func (s *Service) writeProvisioning(ctx context.Context, ct Container, req CreateRequest, hash string) error {
	keys, err := json.Marshal(req.SSHKeys)
	if err != nil {
		return fmt.Errorf("encoding ssh keys: %w", err)
	}

	hostname := req.Hostname
	if hostname == "" {
		hostname = req.Name
	}

	stamp := ct.CreatedAt.Format(time.RFC3339Nano)
	_, execErr := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_provisioning
		(container_id, hostname, timezone, ssh_keys, password_hash,
		 net_config_format, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ct.ID, hostname, nullIfEmpty(req.Timezone), string(keys), nullIfEmpty(hash),
		netConfigFormat(ct.OSFamily), stamp, stamp)
	if execErr != nil {
		return fmt.Errorf("writing provisioning: %w", execErr)
	}
	return nil
}

func (s *Service) cloneProvisioning(ctx context.Context, sourceID, targetID, hostname, fallback string) error {
	source, err := s.Provisioning(ctx, sourceID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	hash, err := s.passwordHash(ctx, sourceID)
	if err != nil {
		return err
	}

	if hostname == "" {
		hostname = fallback
	}
	keys, err := json.Marshal(source.SSHKeys)
	if err != nil {
		return fmt.Errorf("encoding ssh keys: %w", err)
	}

	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	_, execErr := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_provisioning
		(container_id, hostname, timezone, ssh_keys, password_hash,
		 net_config_format, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		targetID, hostname, nullIfEmpty(source.Timezone), string(keys),
		nullIfEmpty(hash), source.NetConfigFormat, stamp, stamp)
	if execErr != nil {
		return fmt.Errorf("copying provisioning: %w", execErr)
	}
	return nil
}

func (s *Service) nameTaken(ctx context.Context, envID, name, exceptID string) (bool, error) {
	rows, err := s.db.Query(ctx, `
		SELECT 1 FROM plugin_addon_lxc_containers
		WHERE environment_id = ? AND name = ? AND id != ? LIMIT 1`,
		envID, name, exceptID)
	if err != nil {
		return false, fmt.Errorf("checking the container name: %w", err)
	}
	defer func() { _ = rows.Close() }()

	taken := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("checking the container name: %w", err)
	}
	return taken, nil
}

// allocateIDRange picks a subordinate id range no other container on this host
// is using.
//
// Ranges are 65536 wide and start at idRangeBase, so container N gets
// [base + N*65536, +65536). Overlap would let one container's root reach
// another's files through a shared pool directory, which is precisely the
// boundary an unprivileged container is for — so the range is derived from the
// highest allocated one rather than from a count, and a gap left by a deleted
// container is not reused while anything might still own files in it.
func (s *Service) allocateIDRange(ctx context.Context, envID string) (start, count int64, err error) {
	rows, err := s.db.Query(ctx, `
		SELECT COALESCE(MAX(uid_map_start), 0) FROM plugin_addon_lxc_containers
		WHERE environment_id = ?`, envID)
	if err != nil {
		return 0, 0, fmt.Errorf("allocating an id range: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var highest int64
	if rows.Next() {
		if err := rows.Scan(&highest); err != nil {
			return 0, 0, fmt.Errorf("allocating an id range: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("allocating an id range: %w", err)
	}

	if highest == 0 {
		start = idRangeBase
	} else {
		start = highest + idRangeSize
	}
	// A 32-bit uid_t is the hard ceiling. Running out means ~65000 containers
	// have been created in one environment, which is not a situation to paper
	// over by wrapping around into somebody else's range.
	if start+idRangeSize > idRangeCeiling {
		return 0, 0, ErrIDRangeExhausted
	}
	return start, idRangeSize, nil
}
