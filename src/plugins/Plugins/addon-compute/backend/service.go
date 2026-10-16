// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Service holds every query and every agent call. Handlers do none of either —
// they parse, validate, check auth, call here, audit, and write a response.
type Service struct {
	db plugin.DBHandle
	// migrations tracks a relocation per environment so the UI can watch one
	// that outlives the request that started it.
	migrations *migrationJobs
	rpc        *agentBridge
	audit      plugin.AuditHandle
	// lifecycle puts a pool's creation and removal on the Logging timeline,
	// which before this carried Docker subjects only. nil when the host wired
	// no recorder, which every write below tolerates.
	lifecycle plugin.LifecycleHandle
	// catalogs reads the enabled vm_image catalog sources the operator
	// manages in Settings. nil when the host wired no catalog registry,
	// which leaves the mounted directory as the only source.
	catalogs plugin.CatalogHandle
	// envs enumerates the host's environments. nil when the host wired no
	// workflow bridge, which degrades the list rather than breaking it.
	envs environmentLister
	// workDir is the host's data directory. The image library lives under it
	// unless the operator pointed MCHARBOR_COMPUTE_IMAGE_LIBRARY elsewhere.
	workDir string
	logger  pluginLogger

	// grants authorizes an agent to pull an uploaded image over HTTP, for a
	// host that does not share the library directory.
	grants *libraryGrants

	// reconciledAt throttles the image-cache reconciliation below, keyed by
	// environment id.
	reconcileMu  sync.Mutex
	reconciledAt map[string]time.Time
}

// Errors the handlers map onto i18n codes. The text never reaches a client.
var (
	ErrNotFound      = errors.New("not found")
	ErrNameTaken     = errors.New("name already in use")
	ErrNotAgentEnv   = errors.New("compute requires an agent environment")
	ErrPoolInUse     = errors.New("pool still holds guest volumes")
	ErrInvalidTarget = errors.New("invalid pool target")
	// ErrModeImmutable refuses an edit that would change a network's mode, which
	// decides what it owns on the host and is therefore a different network.
	ErrModeImmutable     = errors.New("a network's mode cannot be changed")
	ErrUnverifiedRefused = errors.New("this image publishes no checksum")
	// ErrInvalidImage marks an image-create request whose fields the service refused.
	ErrInvalidImage = errors.New("invalid image request")
	// ErrBuiltinImage marks an attempt to delete a catalog entry the seed owns.
	ErrBuiltinImage = errors.New("built-in catalog entries cannot be removed")
)

// maxRows bounds every unpaginated select. A host with a runaway row count
// should degrade, not exhaust memory.
const maxRows = 1000

// NewService builds the service.
func NewService(
	db plugin.DBHandle,
	rpc *agentBridge,
	audit plugin.AuditHandle,
	lifecycle plugin.LifecycleHandle,
	envs environmentLister,
	catalogs plugin.CatalogHandle,
	workDir string,
	logger pluginLogger,
) *Service {
	return &Service{db: db, rpc: rpc, audit: audit, lifecycle: lifecycle, envs: envs, catalogs: catalogs, workDir: workDir, logger: logger, grants: newLibraryGrants(), migrations: newMigrationJobs()}
}

// recordAudit writes an audit row for work that outlives the request.
//
// Unattributed by design: a pool move finishes minutes after the handler that
// started it returned, and the row that says it began already names the actor.
func (s *Service) recordAudit(event string, fields map[string]any) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Emit(event, fields); err != nil {
		s.logger.Warn("compute: audit emit failed", "event", event, "error", err)
	}
}

// recordPoolLifecycle puts a pool's creation or removal on the Logging
// timeline, beside the guests that live on it.
func (s *Service) recordPoolLifecycle(ctx context.Context, pool Pool, action, severity string) {
	if s.lifecycle == nil {
		return
	}
	if err := s.lifecycle.Record(ctx, plugin.LifecycleEvent{
		EnvironmentID: pool.EnvironmentID,
		SubjectType:   "pool",
		SubjectID:     pool.ID,
		SubjectName:   pool.Name,
		EventType:     "storage",
		Action:        action,
		Severity:      severity,
		Metadata:      map[string]any{"backend": pool.Backend, "target": pool.Target},
	}); err != nil {
		s.logger.Warn("compute: recording a pool change on the timeline failed",
			"pool", pool.ID, "action", action, "error", err)
	}
}

// environmentLister is the slice of the host's workflow handle this plugin uses
// to enumerate environments.
//
// Borrowed from the workflow bridge because it is the only handle that exposes
// the host's environment list at all — addon-swarm reaches for the same one in
// its event watcher. It answers with ids only; names come to the frontend
// through the host's environment bridge, because a plugin cannot read the
// host's `environments` table.
type environmentLister interface {
	DockerEnvironmentIDs(ctx context.Context) ([]string, error)
}

// --- Environments ---------------------------------------------------------

// ListEnvironments returns every environment the host has, with this plugin's
// compute settings merged onto it.
//
// Driven by the host's environment list rather than by this plugin's own rows,
// which is the whole point. The settings table is written lazily — a row
// appears the first time somebody opens an environment's capability page — so
// listing only stored rows meant an empty picker on a fresh install, and one
// that could never fill: the only way to create a row was to open a page
// reachable only from the picker.
func (s *Service) ListEnvironments(ctx context.Context) ([]ComputeEnvironment, error) {
	stored, err := s.storedEnvironments(ctx)
	if err != nil {
		return nil, err
	}

	ids, err := s.hostEnvironmentIDs(ctx)
	if err != nil {
		// Falling back to the stored rows keeps the page working when the host
		// wired no workflow bridge; it is the pre-existing behaviour, and the
		// warning says why the list may be short.
		s.logger.Warn("compute: cannot list the host's environments; "+
			"showing only environments compute has already been opened on",
			"error", err)
		out := make([]ComputeEnvironment, 0, len(stored))
		for _, env := range stored {
			s.applyAvailability(ctx, &env)
			out = append(out, env)
		}
		return out, nil
	}

	out := make([]ComputeEnvironment, 0, len(ids))
	for _, id := range ids {
		env, ok := stored[id]
		if !ok {
			// Never opened here. Reported as compute-off and unprobed rather
			// than omitted, so the operator can see the host and turn it on.
			env = ComputeEnvironment{EnvironmentID: id}
		}
		s.applyAvailability(ctx, &env)
		out = append(out, env)
	}
	return out, nil
}

// applyAvailability asks the transport whether this environment can be reached
// right now. Asked fresh rather than stored: a stored answer would be a guess
// about the present, and the failure modes have different fixes.
func (s *Service) applyAvailability(ctx context.Context, env *ComputeEnvironment) {
	if s.rpc == nil {
		env.Available, env.UnavailableReason = false, "agent_bridge_missing"
		return
	}
	env.Available, env.UnavailableReason = s.rpc.Available(ctx, env.EnvironmentID)
}

// hostEnvironmentIDs enumerates the host's active environments.
func (s *Service) hostEnvironmentIDs(ctx context.Context) ([]string, error) {
	if s.envs == nil {
		return nil, errors.New("no environment lister is wired")
	}
	return s.envs.DockerEnvironmentIDs(ctx)
}

// storedEnvironments reads this plugin's own settings rows, keyed by
// environment id.
// sharedLibraryKey names one environment's shared-library flag in the plugin
// key/value store.
//
// The KV store rather than a column because a plugin's migrations run only at
// install: a column added now would be missing on every instance that already
// has this plugin, and the flag would read false forever.
func sharedLibraryKey(envID string) string { return "shared-library:" + envID }

// sharedLibrary reports whether this host's agent mounts the shared library.
func (s *Service) sharedLibrary(ctx context.Context, envID string) bool {
	value, ok, err := s.db.KV().Get(ctx, sharedLibraryKey(envID))
	if err != nil {
		s.logger.Warn("reading the shared library flag failed", "env", envID, "error", err)
		return false
	}
	return ok && value == "1"
}

// setSharedLibrary records the operator's answer, deleting rather than storing
// a false so the store holds only what is true.
func (s *Service) setSharedLibrary(ctx context.Context, envID string, shared bool) error {
	if shared {
		return s.db.KV().Set(ctx, sharedLibraryKey(envID), "1")
	}
	return s.db.KV().Delete(ctx, sharedLibraryKey(envID))
}

func (s *Service) storedEnvironments(ctx context.Context) (map[string]ComputeEnvironment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, environment_id, compute_enabled, COALESCE(capabilities, ''),
		       COALESCE(probed_at, ''),
		       COALESCE(default_pool_id, ''), COALESCE(default_network_id, ''),
		       created_at, updated_at
		  FROM plugin_addon_compute_environments
		 ORDER BY created_at
		 LIMIT ?`, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing compute environments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]ComputeEnvironment{}
	for rows.Next() {
		env, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		env.SharedLibrary = s.sharedLibrary(ctx, env.EnvironmentID)
		out[env.EnvironmentID] = env
	}
	return out, rows.Err()
}

// GetEnvironment returns one environment's settings, creating the row lazily.
//
// Lazy creation keeps the plugin from having to observe environment creation in
// the host, which it has no hook for. A host nobody has configured simply has
// compute off, which is the correct default anyway.
func (s *Service) GetEnvironment(ctx context.Context, envID string) (ComputeEnvironment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, environment_id, compute_enabled, COALESCE(capabilities, ''),
		       COALESCE(probed_at, ''),
		       COALESCE(default_pool_id, ''), COALESCE(default_network_id, ''),
		       created_at, updated_at
		  FROM plugin_addon_compute_environments
		 WHERE environment_id = ?`, envID)
	if err != nil {
		return ComputeEnvironment{}, fmt.Errorf("reading compute environment: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if rows.Next() {
		env, err := scanEnvironment(rows)
		if err != nil {
			return ComputeEnvironment{}, err
		}
		env.SharedLibrary = s.sharedLibrary(ctx, envID)
		if s.rpc != nil {
			env.Available, env.UnavailableReason = s.rpc.Available(ctx, envID)
		}
		return env, nil
	}
	if err := rows.Err(); err != nil {
		return ComputeEnvironment{}, err
	}

	env := ComputeEnvironment{
		ID:             xid.New().String(),
		EnvironmentID:  envID,
		ComputeEnabled: false,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_environments
			(id, environment_id, compute_enabled, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?)`,
		env.ID, envID, env.CreatedAt.Format(time.RFC3339), env.UpdatedAt.Format(time.RFC3339)); err != nil {
		return ComputeEnvironment{}, fmt.Errorf("creating compute environment row: %w", err)
	}
	if s.rpc != nil {
		env.Available, env.UnavailableReason = s.rpc.Available(ctx, envID)
	}
	return env, nil
}

// UpdateEnvironment turns compute on or off and sets the defaults.
//
// Enabling is refused on a non-agent environment. The reason is not a policy
// choice: the McHarbor container genuinely cannot host guests, because its own
// deploy workflow would kill them and its network namespace is behind Docker's
// NAT.
func (s *Service) UpdateEnvironment(ctx context.Context, envID string, req EnvironmentUpdateRequest) (ComputeEnvironment, error) {
	env, err := s.GetEnvironment(ctx, envID)
	if err != nil {
		return ComputeEnvironment{}, err
	}

	if req.ComputeEnabled != nil && *req.ComputeEnabled {
		if s.rpc == nil {
			return ComputeEnvironment{}, ErrNotAgentEnv
		}
		if available, reason := s.rpc.Available(ctx, envID); !available && reason == "not_agent_environment" {
			return ComputeEnvironment{}, ErrNotAgentEnv
		}
		env.ComputeEnabled = true
	} else if req.ComputeEnabled != nil {
		env.ComputeEnabled = false
	}

	if req.DefaultPoolID != "" {
		env.DefaultPoolID = req.DefaultPoolID
	}
	if req.DefaultNetworkID != "" {
		env.DefaultNetworkID = req.DefaultNetworkID
	}
	if req.SharedLibrary != nil {
		if err := s.setSharedLibrary(ctx, envID, *req.SharedLibrary); err != nil {
			return ComputeEnvironment{}, fmt.Errorf("recording the shared library flag: %w", err)
		}
		env.SharedLibrary = *req.SharedLibrary
		// Joining or leaving the shared library changes what this host can
		// already see, so its inventory is re-read now rather than up to a
		// minute later. Done inline: the operator is looking at the answer.
		s.invalidateReconcile(envID)
		s.reconcileImageCache(ctx, envID)
	}
	env.UpdatedAt = time.Now().UTC()

	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_environments
		   SET compute_enabled = ?, default_pool_id = ?, default_network_id = ?, updated_at = ?
		 WHERE environment_id = ?`,
		boolToInt(env.ComputeEnabled), nullIfEmpty(env.DefaultPoolID),
		nullIfEmpty(env.DefaultNetworkID), env.UpdatedAt.Format(time.RFC3339), envID); err != nil {
		return ComputeEnvironment{}, fmt.Errorf("updating compute environment: %w", err)
	}
	return env, nil
}

// InstallCapability has the host's agent install the packages for one missing capability item
// and stores the capability it reports afterwards. An item no package provides is refused here.
func (s *Service) InstallCapability(ctx context.Context, envID, item string) (InstallResult, error) {
	if !installableItems[item] {
		return InstallResult{}, fmt.Errorf("%w: %q is not installable", ErrInvalidTarget, item)
	}
	if s.rpc == nil {
		return InstallResult{}, ErrNotAgentEnv
	}
	result, err := s.rpc.Install(ctx, envID, item)
	if result.Capability != nil {
		if storeErr := s.storeCapability(ctx, envID, *result.Capability); storeErr != nil {
			s.logger.Warn("storing capability after install failed", "env", envID, "error", storeErr)
		}
	}
	return result, err
}

// Probe asks the agent what a host can run and stores the answer.
func (s *Service) Probe(ctx context.Context, envID string) (Capability, error) {
	if s.rpc == nil {
		return Capability{}, ErrNotAgentEnv
	}
	capability, err := s.rpc.Probe(ctx, envID)
	if err != nil {
		return Capability{}, err
	}
	return capability, s.storeCapability(ctx, envID, capability)
}

// storeCapability caches a capability report on the environment row. Only a missing row is an
// error; failing to write the cache is logged, because the report itself is still good.
func (s *Service) storeCapability(ctx context.Context, envID string, capability Capability) error {

	// Ensure the row exists before writing to it.
	if _, err := s.GetEnvironment(ctx, envID); err != nil {
		return err
	}

	payload, err := json.Marshal(capability)
	if err != nil {
		// The probe itself succeeded; failing to cache it is not worth losing
		// the answer the operator asked for.
		s.logger.Warn("storing capability report failed", "env", envID, "error", err)
		return nil
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_environments
		   SET capabilities = ?, probed_at = ?, updated_at = ?
		 WHERE environment_id = ?`,
		string(payload), time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339), envID); err != nil {
		s.logger.Warn("storing capability report failed", "env", envID, "error", err)
	}
	return nil
}

// --- Pools ----------------------------------------------------------------

// nullIfBlank stores an empty field as NULL, which is what "not set" is.
func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// DefaultPoolTarget is where a pool lands when the operator names no location.
//
// It matches the candidate the agent's dir driver proposes, so the wizard, the
// create dialog and an auto-provisioned pool all agree on one path.
const DefaultPoolTarget = "/var/lib/mcharbor/compute"

// DefaultPoolName names an auto-provisioned pool.
const DefaultPoolName = "default"

// DetectPools asks a host which storage backends it can offer.
//
// A host that cannot be asked still gets a dir entry rather than an empty list:
// a directory pool needs nothing detected to be created, and an empty list
// leaves the create dialog with no selectable backend and no way forward.
func (s *Service) DetectPools(ctx context.Context, envID string) ([]PoolBackend, error) {
	// Where this environment says its drives belong. The create dialog offers
	// it first, so a pool made here lands where the operator already said
	// disks go instead of at the agent's built-in path.
	preferred := s.preferredPoolTarget(ctx, envID)

	if s.rpc == nil {
		return []PoolBackend{dirFallback(preferred)}, nil
	}
	detected, err := s.rpc.PoolDetect(ctx, envID)
	if err != nil {
		s.logger.Warn("compute: pool detection failed; offering a directory pool only",
			"env", envID, "error", err)
		return []PoolBackend{dirFallback(preferred)}, nil
	}
	if len(detected) == 0 {
		return []PoolBackend{dirFallback(preferred)}, nil
	}
	return withDirAvailable(detected, preferred), nil
}

// preferredPoolTarget is where a new pool is suggested.
//
// The pool itself is the only thing that names a location now, so a host with
// one already gets its target offered again rather than the built-in path —
// the second pool on a host almost always belongs beside the first.
func (s *Service) preferredPoolTarget(ctx context.Context, envID string) string {
	pools, err := s.ListPools(ctx, envID)
	if err != nil {
		return DefaultPoolTarget
	}
	for _, pool := range pools {
		if pool.Backend == BackendDir && pool.Target != "" {
			return pool.Target
		}
	}
	return DefaultPoolTarget
}

// withDirAvailable forces the dir entry to selectable.
//
// An agent older than the one that stopped gating dir on qemu-img reports the
// backend unavailable, which leaves a host with no creatable pool at all. The
// agent's own detail and remediation are kept, so the operator still reads why
// qemu-img matters — they just are not blocked from making the directory.
func withDirAvailable(detected []PoolBackend, preferred string) []PoolBackend {
	out := make([]PoolBackend, 0, len(detected)+1)
	found := false
	for _, entry := range detected {
		if entry.Backend == BackendDir {
			found = true
			entry.Available = true
			entry.Candidates = withPreferredFirst(entry.Candidates, preferred)
		}
		out = append(out, entry)
	}
	if !found {
		out = append(out, dirFallback(preferred))
	}
	return out
}

// withPreferredFirst puts the configured drives location at the head of the
// list the dialog selects from, and never lists it twice.
//
// The host's own candidates are kept behind it: it still knows which mounts
// have room, and an operator who wants one of those picks it.
func withPreferredFirst(candidates []PoolCandidate, preferred string) []PoolCandidate {
	if preferred == "" {
		return candidates
	}
	out := make([]PoolCandidate, 0, len(candidates)+1)
	out = append(out, PoolCandidate{Target: preferred})
	for _, candidate := range candidates {
		if candidate.Target == preferred {
			// Keep the host's numbers for it rather than the bare path.
			out[0] = candidate
			continue
		}
		out = append(out, candidate)
	}
	return out
}

// dirFallback is the backend every host can offer without being asked.
func dirFallback(preferred string) PoolBackend {
	if preferred == "" {
		preferred = DefaultPoolTarget
	}
	return PoolBackend{
		Backend:   BackendDir,
		Available: true,
		Candidates: []PoolCandidate{
			{Target: preferred},
		},
	}
}

// EnsureDefaultPool returns the environment's pool, creating a directory pool
// at target when the host has none.
//
// This is what lets a guest be created against a plain data location: the pool
// row still exists, because every disk is addressed through one, but the
// operator never has to go and make it first.
func (s *Service) EnsureDefaultPool(ctx context.Context, req PoolEnsureRequest) (Pool, bool, error) {
	if req.EnvironmentID == "" {
		return Pool{}, false, ErrInvalidTarget
	}
	pools, err := s.ListPools(ctx, req.EnvironmentID)
	if err != nil {
		return Pool{}, false, err
	}
	for _, pool := range pools {
		if pool.IsDefault {
			return pool, false, nil
		}
	}
	if len(pools) > 0 {
		return pools[0], false, nil
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = s.preferredPoolTarget(ctx, req.EnvironmentID)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = DefaultPoolName
	}
	pool, err := s.CreatePool(ctx, PoolCreateRequest{
		EnvironmentID: req.EnvironmentID,
		Name:          name,
		Backend:       BackendDir,
		Target:        target,
		IsDefault:     true,
	})
	if err != nil {
		return Pool{}, false, err
	}
	return pool, true, nil
}

// ListPools returns pools, optionally for one environment.
func (s *Service) ListPools(ctx context.Context, envID string) ([]Pool, error) {
	query := `
		SELECT id, environment_id, name, backend, target, is_default,
		       COALESCE(capacity_bytes, 0), COALESCE(used_bytes, 0),
		       COALESCE(allocated_bytes, 0), volume_count, COALESCE(sampled_at, ''),
		       state, COALESCE(last_error, ''), created_at, updated_at
		  FROM plugin_addon_compute_pools`
	args := []any{}
	if envID != "" {
		query += ` WHERE environment_id = ?`
		args = append(args, envID)
	}
	query += ` ORDER BY environment_id, name LIMIT ?`
	args = append(args, maxRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing pools: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Pool
	for rows.Next() {
		pool, err := scanPool(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pool)
	}
	return out, rows.Err()
}

// GetPool reads one pool.
func (s *Service) GetPool(ctx context.Context, id string) (Pool, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, environment_id, name, backend, target, is_default,
		       COALESCE(capacity_bytes, 0), COALESCE(used_bytes, 0),
		       COALESCE(allocated_bytes, 0), volume_count, COALESCE(sampled_at, ''),
		       state, COALESCE(last_error, ''), created_at, updated_at
		  FROM plugin_addon_compute_pools WHERE id = ?`, id)
	if err != nil {
		return Pool{}, fmt.Errorf("reading pool: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Pool{}, err
		}
		return Pool{}, ErrNotFound
	}
	return scanPool(rows)
}

// CreatePool initializes a pool on the host and records it.
//
// The agent runs first. A row for a pool the host could not create would show
// the operator a pool that does not exist, and the first guest disk on it would
// fail with an error pointing at the disk rather than at the pool.
func (s *Service) CreatePool(ctx context.Context, req PoolCreateRequest) (Pool, error) {
	if s.rpc == nil {
		return Pool{}, ErrNotAgentEnv
	}
	if err := validatePoolRequest(req); err != nil {
		return Pool{}, err
	}

	id := xid.New().String()
	stats, err := s.rpc.PoolCreate(ctx, req.EnvironmentID, PoolRequest{
		PoolID:  id,
		Name:    req.Name,
		Backend: req.Backend,
		Target:  req.Target,
	})
	if err != nil {
		return Pool{}, err
	}

	now := time.Now().UTC()
	pool := Pool{
		ID:             id,
		EnvironmentID:  req.EnvironmentID,
		Name:           req.Name,
		Backend:        req.Backend,
		Target:         req.Target,
		IsDefault:      req.IsDefault,
		CapacityBytes:  stats.CapacityBytes,
		UsedBytes:      stats.UsedBytes,
		AllocatedBytes: stats.AllocatedBytes,
		VolumeCount:    stats.VolumeCount,
		Instant:        InstantBackends[req.Backend],
		State:          StateReady,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if !stats.SampledAt.IsZero() {
		sampled := stats.SampledAt
		pool.SampledAt = &sampled
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_pools
			(id, environment_id, name, backend, target, is_default,
			 capacity_bytes, used_bytes, allocated_bytes, volume_count,
			 sampled_at, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pool.ID, pool.EnvironmentID, pool.Name, pool.Backend, pool.Target,
		boolToInt(pool.IsDefault), pool.CapacityBytes, pool.UsedBytes,
		pool.AllocatedBytes, pool.VolumeCount, formatTimePtr(pool.SampledAt),
		pool.State, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		if isUniqueViolation(err) {
			return Pool{}, ErrNameTaken
		}
		return Pool{}, fmt.Errorf("recording pool: %w", err)
	}

	if pool.IsDefault {
		if err := s.setDefaultPool(ctx, pool.EnvironmentID, pool.ID); err != nil {
			s.logger.Warn("setting default pool failed", "pool", pool.ID, "error", err)
		}
	}
	s.recordPoolLifecycle(ctx, pool, "created", "success")
	return pool, nil
}

// setDefaultPool makes one pool the environment's default, clearing the rest.
func (s *Service) setDefaultPool(ctx context.Context, envID, poolID string) error {
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_pools SET is_default = 0 WHERE environment_id = ?`, envID); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_pools SET is_default = 1 WHERE id = ?`, poolID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_environments SET default_pool_id = ? WHERE environment_id = ?`,
		poolID, envID)
	return err
}

// RefreshPool samples a pool's capacity.
func (s *Service) RefreshPool(ctx context.Context, id string) (Pool, error) {
	pool, err := s.GetPool(ctx, id)
	if err != nil {
		return Pool{}, err
	}
	if s.rpc == nil {
		return pool, nil
	}

	stats, err := s.rpc.PoolStats(ctx, pool.EnvironmentID, PoolRef{
		ID: pool.ID, Name: pool.Name, Backend: pool.Backend, Target: pool.Target,
	})
	if err != nil {
		// A pool whose host is unreachable keeps its last known numbers and
		// records why they are stale, rather than showing zeroes that would
		// read as "empty".
		pool.State = StateError
		pool.LastError = err.Error()
		_, _ = s.db.Exec(ctx, `
			UPDATE plugin_addon_compute_pools SET state = ?, last_error = ?, updated_at = ?
			 WHERE id = ?`, StateError, err.Error(), time.Now().UTC().Format(time.RFC3339), id)
		return pool, nil
	}

	now := time.Now().UTC()
	pool.CapacityBytes = stats.CapacityBytes
	pool.UsedBytes = stats.UsedBytes
	pool.AllocatedBytes = stats.AllocatedBytes
	pool.VolumeCount = stats.VolumeCount
	pool.SampledAt = &now
	pool.State = StateReady
	pool.LastError = ""

	_, err = s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_pools
		   SET capacity_bytes = ?, used_bytes = ?, allocated_bytes = ?, volume_count = ?,
		       sampled_at = ?, state = ?, last_error = NULL, updated_at = ?
		 WHERE id = ?`,
		pool.CapacityBytes, pool.UsedBytes, pool.AllocatedBytes, pool.VolumeCount,
		now.Format(time.RFC3339), StateReady, now.Format(time.RFC3339), id)
	return pool, err
}

// DeletePool detaches a pool.
//
// force is the operator's explicit acceptance that guest volumes on it will go.
// Without it the agent refuses and names what is in the way, which is what turns
// a refusal into something actionable.
func (s *Service) DeletePool(ctx context.Context, id string, force bool) error {
	pool, err := s.GetPool(ctx, id)
	if err != nil {
		return err
	}
	if s.rpc != nil {
		if err := s.rpc.PoolRemove(ctx, pool.EnvironmentID, PoolRequest{
			PoolID: pool.ID, Name: pool.Name, Backend: pool.Backend,
			Target: pool.Target, Force: force,
		}); err != nil {
			return err
		}
	}
	if _, err = s.db.Exec(ctx, `DELETE FROM plugin_addon_compute_pools WHERE id = ?`, id); err != nil {
		return err
	}
	s.recordPoolLifecycle(ctx, pool, "removed", "warning")
	return nil
}

// --- Networks -------------------------------------------------------------

// DetectLinks enumerates a host's interfaces.
func (s *Service) DetectLinks(ctx context.Context, envID string) ([]HostLink, error) {
	if s.rpc == nil {
		return nil, ErrNotAgentEnv
	}
	return s.rpc.NetDetect(ctx, envID)
}

// ListNetworks returns networks, optionally for one environment.
func (s *Service) ListNetworks(ctx context.Context, envID string) ([]Network, error) {
	query := `
		SELECT id, environment_id, name, mode, bridge, COALESCE(vlan_tag, 0),
		       COALESCE(subnet_cidr, ''), COALESCE(gateway_ip, ''), dhcp_enabled,
		       COALESCE(dhcp_range_start, ''), COALESCE(dhcp_range_end, ''),
		       nat_enabled, COALESCE(uplink_link, ''), COALESCE(link_mode, ''),
		       host_access, COALESCE(host_access_ip, ''), ipv6_enabled,
		       COALESCE(host_access_ipv6, ''), COALESCE(host_link_name, ''), is_default, state,
		       COALESCE(last_error, ''), created_at, updated_at
		  FROM plugin_addon_compute_networks`
	args := []any{}
	if envID != "" {
		query += ` WHERE environment_id = ?`
		args = append(args, envID)
	}
	query += ` ORDER BY environment_id, name LIMIT ?`
	args = append(args, maxRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Network
	for rows.Next() {
		network, err := scanNetwork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, network)
	}
	return out, rows.Err()
}

// nextBridgeName picks the lowest free mcharbor-brN on an environment.
//
// The kernel caps an interface name at 15 characters, so a bridge cannot be
// named after the network: "mcharbor-" plus anything an operator would type
// overruns it, and truncating instead would give two networks the same bridge.
// An index is short, stable, readable, and is the name this feature's own
// protocol documentation has always claimed it uses.
func (s *Service) nextBridgeName(ctx context.Context, envID string) (string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT bridge FROM plugin_addon_compute_networks
		 WHERE environment_id = ? AND bridge LIKE 'mcharbor-br%'
		 LIMIT ?`, envID, maxRows)
	if err != nil {
		return "", fmt.Errorf("reading bridge names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	taken := make(map[int]struct{})
	for rows.Next() {
		var bridge string
		if err := rows.Scan(&bridge); err != nil {
			return "", fmt.Errorf("reading bridge names: %w", err)
		}
		if index, err := strconv.Atoi(strings.TrimPrefix(bridge, "mcharbor-br")); err == nil {
			taken[index] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("reading bridge names: %w", err)
	}

	for index := 0; index < maxRows; index++ {
		if _, used := taken[index]; !used {
			return fmt.Sprintf("mcharbor-br%d", index), nil
		}
	}
	return "", errors.New("this host already has every managed bridge name in use")
}

// GetNetwork reads one network.
func (s *Service) GetNetwork(ctx context.Context, id string) (Network, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, environment_id, name, mode, bridge, COALESCE(vlan_tag, 0),
		       COALESCE(subnet_cidr, ''), COALESCE(gateway_ip, ''), dhcp_enabled,
		       COALESCE(dhcp_range_start, ''), COALESCE(dhcp_range_end, ''),
		       nat_enabled, COALESCE(uplink_link, ''), COALESCE(link_mode, ''),
		       host_access, COALESCE(host_access_ip, ''), ipv6_enabled,
		       COALESCE(host_access_ipv6, ''), COALESCE(host_link_name, ''), is_default, state,
		       COALESCE(last_error, ''), created_at, updated_at
		  FROM plugin_addon_compute_networks WHERE id = ?`, id)
	if err != nil {
		return Network{}, fmt.Errorf("reading network: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Network{}, err
		}
		return Network{}, ErrNotFound
	}
	return scanNetwork(rows)
}

// CreateNetwork records a guest network, verifying or creating it on the host.
func (s *Service) CreateNetwork(ctx context.Context, req NetworkCreateRequest) (Network, error) {
	if s.rpc == nil {
		return Network{}, ErrNotAgentEnv
	}
	if err := validateNetworkRequest(req); err != nil {
		return Network{}, err
	}

	id := xid.New().String()
	// Bridged mode uses the operator's own bridge, managed and external get one
	// McHarbor creates and names. The direct modes get none at all: their
	// guests stack on the adapter itself, and inventing a bridge here would put
	// a device on the host that nothing would ever carry traffic over.
	if req.Mode == "managed" || req.Mode == "external" {
		bridge, err := s.nextBridgeName(ctx, req.EnvironmentID)
		if err != nil {
			return Network{}, err
		}
		req.Bridge = bridge
	}

	if isDirectMode(req.Mode) {
		if err := s.ensureDirectNetwork(ctx, req.EnvironmentID,
			directNetworkFrom(id, req)); err != nil {
			return Network{}, err
		}
	} else if err := s.rpc.NetEnsure(ctx, req.EnvironmentID, NetworkRequest{
		NetworkID:      id,
		Name:           req.Name,
		Mode:           req.Mode,
		Bridge:         req.Bridge,
		VLANTag:        req.VLANTag,
		SubnetCIDR:     req.SubnetCIDR,
		GatewayIP:      req.GatewayIP,
		DHCPEnabled:    req.DHCPEnabled,
		DHCPRangeStart: req.DHCPRangeStart,
		DHCPRangeEnd:   req.DHCPRangeEnd,
		NATEnabled:     req.NATEnabled,
		UplinkLink:     req.UplinkLink,
	}); err != nil {
		return Network{}, err
	}

	now := time.Now().UTC()
	network := Network{
		ID:             id,
		EnvironmentID:  req.EnvironmentID,
		Name:           req.Name,
		Mode:           req.Mode,
		Bridge:         req.Bridge,
		VLANTag:        req.VLANTag,
		SubnetCIDR:     req.SubnetCIDR,
		GatewayIP:      req.GatewayIP,
		DHCPEnabled:    req.DHCPEnabled,
		DHCPRangeStart: req.DHCPRangeStart,
		DHCPRangeEnd:   req.DHCPRangeEnd,
		NATEnabled:     req.NATEnabled,
		UplinkLink:     req.UplinkLink,
		LinkMode:       req.LinkMode,
		HostAccess:     req.HostAccess,
		HostAccessIP:   req.HostAccessIP,
		IPv6Enabled:    req.IPv6Enabled,
		HostAccessIPv6: req.HostAccessIPv6,
		HostLinkName:   req.HostLinkName,
		IsDefault:      req.IsDefault,
		State:          StateReady,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_networks
			(id, environment_id, name, mode, bridge, vlan_tag, subnet_cidr, gateway_ip,
			 dhcp_enabled, dhcp_range_start, dhcp_range_end, nat_enabled, uplink_link,
			 link_mode, host_access, host_access_ip, ipv6_enabled, host_access_ipv6,
			 host_link_name, is_default, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		network.ID, network.EnvironmentID, network.Name, network.Mode, network.Bridge,
		nullIfZero(network.VLANTag), nullIfEmpty(network.SubnetCIDR), nullIfEmpty(network.GatewayIP),
		boolToInt(network.DHCPEnabled), nullIfEmpty(network.DHCPRangeStart),
		nullIfEmpty(network.DHCPRangeEnd), boolToInt(network.NATEnabled),
		nullIfEmpty(network.UplinkLink), nullIfEmpty(network.LinkMode),
		boolToInt(network.HostAccess), nullIfEmpty(network.HostAccessIP),
		boolToInt(network.IPv6Enabled), nullIfEmpty(network.HostAccessIPv6),
		nullIfEmpty(network.HostLinkName),
		boolToInt(network.IsDefault), network.State,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		if isUniqueViolation(err) {
			return Network{}, ErrNameTaken
		}
		return Network{}, fmt.Errorf("recording network: %w", err)
	}
	return network, nil
}

// UpdateNetwork changes an existing network in place.
//
// The mode and the host are fixed: both decide what the network owns on that
// host, so changing either is a different network wearing an old id. Everything
// else is re-validated and re-applied to the agent.
func (s *Service) UpdateNetwork(ctx context.Context, id string, req NetworkCreateRequest) (Network, error) {
	existing, err := s.GetNetwork(ctx, id)
	if err != nil {
		return Network{}, err
	}
	if req.Mode != "" && req.Mode != existing.Mode {
		return Network{}, ErrModeImmutable
	}
	req.Mode = existing.Mode
	req.EnvironmentID = existing.EnvironmentID
	// Managed and external networks own a bridge McHarbor named at create time.
	// Re-deriving it here would hand the network a second bridge and orphan the
	// one its guests are on.
	if existing.Mode == "managed" || existing.Mode == "external" {
		req.Bridge = existing.Bridge
	}
	if err := validateNetworkRequest(req); err != nil {
		return Network{}, err
	}

	if s.rpc != nil {
		if isDirectMode(existing.Mode) {
			// The host shim is named after the network id, not after what it
			// stacks on, so an edit that moves the parent or the tag would find
			// the old shim by name, leave it on the old adapter and report
			// success. Taking the old one down first is what makes the parent
			// and VLAN editable at all.
			if directNeedsTeardown(existing, req) {
				if err := s.removeDirectNetwork(ctx, existing.EnvironmentID,
					directRemovalFrom(existing)); err != nil {
					return Network{}, err
				}
			}
			if err := s.ensureDirectNetwork(ctx, req.EnvironmentID,
				directNetworkFrom(id, req)); err != nil {
				return Network{}, err
			}
		} else if err := s.rpc.NetEnsure(ctx, req.EnvironmentID, NetworkRequest{
			NetworkID:      id,
			Name:           req.Name,
			Mode:           req.Mode,
			Bridge:         req.Bridge,
			VLANTag:        req.VLANTag,
			SubnetCIDR:     req.SubnetCIDR,
			GatewayIP:      req.GatewayIP,
			DHCPEnabled:    req.DHCPEnabled,
			DHCPRangeStart: req.DHCPRangeStart,
			DHCPRangeEnd:   req.DHCPRangeEnd,
			NATEnabled:     req.NATEnabled,
			UplinkLink:     req.UplinkLink,
		}); err != nil {
			return Network{}, err
		}
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_networks
		   SET name = ?, bridge = ?, vlan_tag = ?, subnet_cidr = ?, gateway_ip = ?,
		       dhcp_enabled = ?, dhcp_range_start = ?, dhcp_range_end = ?,
		       nat_enabled = ?, uplink_link = ?, link_mode = ?, host_access = ?,
		       host_access_ip = ?, ipv6_enabled = ?, host_access_ipv6 = ?,
		       host_link_name = ?, is_default = ?, updated_at = ?
		 WHERE id = ?`,
		req.Name, req.Bridge, nullIfZero(req.VLANTag), nullIfEmpty(req.SubnetCIDR),
		nullIfEmpty(req.GatewayIP), boolToInt(req.DHCPEnabled),
		nullIfEmpty(req.DHCPRangeStart), nullIfEmpty(req.DHCPRangeEnd),
		boolToInt(req.NATEnabled), nullIfEmpty(req.UplinkLink),
		nullIfEmpty(req.LinkMode), boolToInt(req.HostAccess),
		nullIfEmpty(req.HostAccessIP), boolToInt(req.IPv6Enabled),
		nullIfEmpty(req.HostAccessIPv6), nullIfEmpty(req.HostLinkName),
		boolToInt(req.IsDefault),
		now.Format(time.RFC3339), id); err != nil {
		if isUniqueViolation(err) {
			return Network{}, ErrNameTaken
		}
		return Network{}, fmt.Errorf("updating network: %w", err)
	}
	return s.GetNetwork(ctx, id)
}

// DeleteNetwork removes a network.
func (s *Service) DeleteNetwork(ctx context.Context, id string) error {
	network, err := s.GetNetwork(ctx, id)
	if err != nil {
		return err
	}
	if s.rpc != nil {
		if isDirectMode(network.Mode) {
			// The tag and the parent are what the agent needs to find what a
			// direct network owns: a host shim and, where McHarbor made it, a
			// VLAN sub-interface.
			if err := s.removeDirectNetwork(ctx, network.EnvironmentID,
				directRemovalFrom(network)); err != nil {
				return err
			}
		} else if err := s.rpc.NetRemove(ctx, network.EnvironmentID, NetworkRequest{
			NetworkID: network.ID, Name: network.Name,
			Mode: network.Mode, Bridge: network.Bridge,
		}); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, `DELETE FROM plugin_addon_compute_networks WHERE id = ?`, id)
	return err
}

// ListLeases returns a managed network's DHCP leases.
func (s *Service) ListLeases(ctx context.Context, networkID string) ([]Lease, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, network_id, mac, ip, COALESCE(hostname, ''), COALESCE(guest_kind, ''),
		       COALESCE(guest_id, ''), COALESCE(expires_at, ''), created_at, updated_at
		  FROM plugin_addon_compute_leases
		 WHERE network_id = ? ORDER BY ip LIMIT ?`, networkID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing leases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Lease
	for rows.Next() {
		var lease Lease
		var expires, created, updated string
		if err := rows.Scan(&lease.ID, &lease.NetworkID, &lease.MAC, &lease.IP,
			&lease.Hostname, &lease.GuestKind, &lease.GuestID,
			&expires, &created, &updated); err != nil {
			return nil, err
		}
		lease.ExpiresAt = parseTimePtr(expires)
		lease.CreatedAt = parseTime(created)
		lease.UpdatedAt = parseTime(updated)
		out = append(out, lease)
	}
	return out, rows.Err()
}

// RecordLease upserts a lease reported by an agent.
//
// Leases take the offline path, unlike compute commands: a lease replayed late
// is stale information rather than a destructive action.
func (s *Service) RecordLease(ctx context.Context, event GuestEvent) {
	networkID := event.Detail["networkId"]
	mac := event.Detail["mac"]
	if networkID == "" || mac == "" {
		return
	}

	if strings.HasSuffix(event.Kind, "_expired") || strings.HasSuffix(event.Kind, "_released") {
		_, _ = s.db.Exec(ctx, `
			DELETE FROM plugin_addon_compute_leases WHERE network_id = ? AND mac = ?`,
			networkID, mac)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	// The guest-supplied hostname is stored as-is and rendered as text. It is
	// never used to build a link, a command or an authorization decision.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_leases
			(id, network_id, mac, ip, hostname, guest_kind, guest_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(network_id, mac) DO UPDATE SET
			ip = excluded.ip, hostname = excluded.hostname,
			guest_kind = excluded.guest_kind, guest_id = excluded.guest_id,
			updated_at = excluded.updated_at`,
		xid.New().String(), networkID, mac, event.Detail["ip"], event.Detail["hostname"],
		event.GuestKind, event.GuestID, now, now); err != nil {
		s.logger.Debug("recording lease failed", "network", networkID, "error", err)
	}
}

// --- Images ---------------------------------------------------------------

// ListImages returns the catalog, with per-environment cache state when scoped.
func (s *Service) ListImages(ctx context.Context, envID, kind string) ([]Image, error) {
	query := `
		SELECT id, kind, distro, release, arch, COALESCE(variant, ''), display_name,
		       source_url, COALESCE(checksum_url, ''), COALESCE(checksum, ''),
		       checksum_algo, COALESCE(signature_url, ''), COALESCE(size_bytes, 0),
		       is_builtin, COALESCE(init_system, ''), COALESCE(net_config_format, ''),
		       COALESCE(origin, 'catalog'), COALESCE(library_file, ''),
		       COALESCE(uploaded_by, ''), COALESCE(uploaded_at, ''),
		       created_at, updated_at
		  FROM plugin_addon_compute_images`
	args := []any{}
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY distro, release DESC LIMIT ?`
	args = append(args, maxRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Image
	for rows.Next() {
		image, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, image)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if envID != "" {
		s.reconcileImageCache(ctx, envID)
		caches, err := s.imageCaches(ctx, envID)
		if err != nil {
			return nil, err
		}
		for i := range out {
			if cache, ok := caches[out[i].ID]; ok {
				entry := cache
				out[i].Cache = &entry
			}
		}
	}
	return out, nil
}

// imageCaches reads every cache row for one environment in a single query, so
// a catalog listing is two queries rather than one per image.
// reconcileInterval bounds how often a host is asked what it actually holds.
//
// The answer only changes when somebody downloads, evicts or rebuilds a host,
// so a minute is generous. It exists to keep an agent round trip off every
// render of the images page, not because the answer is expensive.
const reconcileInterval = time.Minute

// reconcileImageCache makes the cache table agree with the host's disk.
//
// Nothing else ever checked. A row saying `ready` was believed forever, so
// replacing an agent, clearing the cache directory or rebuilding a host left
// rows pointing at files that no longer existed — and the operator found out
// at the end of the create form, as a qemu-img error naming a path.
//
// Both directions matter. A row whose file is gone is deleted, so the UI offers
// Download again; a file the host has with no row is adopted, so a rebuilt
// McHarbor does not re-download what is already there.
//
// Deliberately best-effort: an offline or older agent leaves the table exactly
// as it was. This runs on a read path, and failing a list because a host is
// unreachable would be worse than showing what was last known.
func (s *Service) reconcileImageCache(ctx context.Context, envID string) {
	if s.rpc == nil || envID == "" || !s.shouldReconcile(envID) {
		return
	}

	hosted, err := s.rpc.ImageList(ctx, envID)
	if err != nil {
		s.logger.Debug("image cache reconcile skipped", "env", envID, "error", err)
		return
	}

	onHost := make(map[string]CachedImage, len(hosted))
	for _, image := range hosted {
		onHost[image.ImageID] = image
	}

	known, err := s.imageCaches(ctx, envID)
	if err != nil {
		s.logger.Debug("image cache reconcile skipped", "env", envID, "error", err)
		return
	}

	for imageID, cache := range known {
		// A download in flight has no file yet, and deleting its row would
		// strip the progress bar out from under it.
		if cache.State == CacheStateDownloading {
			continue
		}
		if _, ok := onHost[imageID]; ok {
			continue
		}
		if cache.State != CacheStateReady {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			DELETE FROM plugin_addon_compute_image_cache
			 WHERE environment_id = ? AND image_id = ?`, envID, imageID); err != nil {
			s.logger.Warn("dropping stale image cache row failed",
				"env", envID, "image", imageID, "error", err)
			continue
		}
		s.logger.Info("image no longer on host; offering it for download again",
			"env", envID, "image", imageID, "path", cache.LocalPath)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for imageID, image := range onHost {
		if cache, ok := known[imageID]; ok && cache.State == CacheStateReady {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO plugin_addon_compute_image_cache
				(id, image_id, environment_id, local_path, size_bytes, verified,
				 state, progress_pct, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 100, ?, ?)
			ON CONFLICT(image_id, environment_id) DO UPDATE SET
				local_path = excluded.local_path, size_bytes = excluded.size_bytes,
				verified = excluded.verified, state = excluded.state,
				progress_pct = 100, last_error = NULL, updated_at = excluded.updated_at`,
			xid.New().String(), imageID, envID, image.LocalPath, image.SizeBytes,
			boolToInt(image.Verified), CacheStateReady, now, now); err != nil {
			// An image the host holds that is not in the catalog has no row to
			// adopt, and the foreign key says so. Not a fault.
			s.logger.Debug("adopting host image failed",
				"env", envID, "image", imageID, "error", err)
			continue
		}
		s.logger.Info("adopted an image already on the host", "env", envID, "image", imageID)
	}
}

// invalidateReconcile forgets when this environment was last asked, so the next
// read re-asks immediately.
//
// Used when something changed the answer rather than time passing — turning the
// shared library on means files this host never downloaded may already be
// visible to it, and waiting out the interval would show them as missing.
func (s *Service) invalidateReconcile(envID string) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	delete(s.reconciledAt, envID)
}

// shouldReconcile reports whether this environment is due, and marks it done.
func (s *Service) shouldReconcile(envID string) bool {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	if s.reconciledAt == nil {
		s.reconciledAt = make(map[string]time.Time)
	}
	if last, ok := s.reconciledAt[envID]; ok && time.Since(last) < reconcileInterval {
		return false
	}
	s.reconciledAt[envID] = time.Now()
	return true
}

func (s *Service) imageCaches(ctx context.Context, envID string) (map[string]ImageCache, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, image_id, environment_id, local_path, COALESCE(size_bytes, 0),
		       verified, signed, state, progress_pct, COALESCE(phase, ''),
		       COALESCE(last_error, ''),
		       COALESCE(last_used_at, '')
		  FROM plugin_addon_compute_image_cache
		 WHERE environment_id = ? LIMIT ?`, envID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading image cache: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]ImageCache)
	for rows.Next() {
		var cache ImageCache
		var verified, signed int
		var lastUsed string
		if err := rows.Scan(&cache.ID, &cache.ImageID, &cache.EnvironmentID, &cache.LocalPath,
			&cache.SizeBytes, &verified, &signed, &cache.State, &cache.ProgressPct,
			&cache.Phase, &cache.LastError, &lastUsed); err != nil {
			return nil, err
		}
		cache.Verified = verified == 1
		cache.Signed = signed == 1
		cache.LastUsedAt = parseTimePtr(lastUsed)
		out[cache.ImageID] = cache
	}
	return out, rows.Err()
}

// imageCacheRecord is a cache row with its last write time, which is what tells
// a download still in flight from one nobody is watching any more.
type imageCacheRecord struct {
	ImageCache
	UpdatedAt time.Time
}

// hygieneQueryTimeout bounds each estate-wide read the hygiene report makes.
const hygieneQueryTimeout = 30 * time.Second

// allImageCaches reads every host's cache rows in one query, without asking any
// agent, because a report must describe what McHarbor believes rather than
// trigger a reconcile that changes it.
func (s *Service) allImageCaches(ctx context.Context) ([]imageCacheRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, hygieneQueryTimeout)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		SELECT id, image_id, environment_id, local_path, COALESCE(size_bytes, 0),
		       verified, signed, state, progress_pct, COALESCE(phase, ''),
		       COALESCE(last_error, ''), COALESCE(last_used_at, ''), updated_at
		  FROM plugin_addon_compute_image_cache
		 ORDER BY environment_id, image_id LIMIT ?`, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading every image cache row: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []imageCacheRecord
	for rows.Next() {
		var record imageCacheRecord
		var verified, signed int
		var lastUsed, updated string
		if err := rows.Scan(&record.ID, &record.ImageID, &record.EnvironmentID, &record.LocalPath,
			&record.SizeBytes, &verified, &signed, &record.State, &record.ProgressPct,
			&record.Phase, &record.LastError, &lastUsed, &updated); err != nil {
			return nil, fmt.Errorf("scanning image cache row: %w", err)
		}
		record.Verified = verified == 1
		record.Signed = signed == 1
		record.LastUsedAt = parseTimePtr(lastUsed)
		record.UpdatedAt = parseTime(updated)
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading every image cache row: %w", err)
	}
	return out, nil
}

// leaseRecord is a lease with the network it claims to belong to; NetworkFound
// is false when that network row is gone, which the foreign key does not stop
// unless SQLite enforcement happens to be on.
type leaseRecord struct {
	Lease
	EnvironmentID string
	NetworkName   string
	NetworkFound  bool
}

// allLeases reads every DHCP lease joined to its network, so the hygiene report
// can find leases whose network or host no longer exists.
func (s *Service) allLeases(ctx context.Context) ([]leaseRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, hygieneQueryTimeout)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		SELECT l.id, l.network_id, l.mac, l.ip, COALESCE(l.hostname, ''),
		       COALESCE(l.guest_kind, ''), COALESCE(l.guest_id, ''),
		       COALESCE(l.expires_at, ''), l.created_at, l.updated_at,
		       COALESCE(n.environment_id, ''), COALESCE(n.name, ''),
		       CASE WHEN n.id IS NULL THEN 0 ELSE 1 END
		  FROM plugin_addon_compute_leases l
		  LEFT JOIN plugin_addon_compute_networks n ON n.id = l.network_id
		 ORDER BY l.updated_at LIMIT ?`, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading every lease: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []leaseRecord
	for rows.Next() {
		var record leaseRecord
		var expires, created, updated string
		var found int
		if err := rows.Scan(&record.ID, &record.NetworkID, &record.MAC, &record.IP,
			&record.Hostname, &record.GuestKind, &record.GuestID,
			&expires, &created, &updated,
			&record.EnvironmentID, &record.NetworkName, &found); err != nil {
			return nil, fmt.Errorf("scanning lease: %w", err)
		}
		record.ExpiresAt = parseTimePtr(expires)
		record.CreatedAt = parseTime(created)
		record.UpdatedAt = parseTime(updated)
		record.NetworkFound = found == 1
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading every lease: %w", err)
	}
	return out, nil
}

// GetImage reads one catalog entry.
func (s *Service) GetImage(ctx context.Context, id string) (Image, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, kind, distro, release, arch, COALESCE(variant, ''), display_name,
		       source_url, COALESCE(checksum_url, ''), COALESCE(checksum, ''),
		       checksum_algo, COALESCE(signature_url, ''), COALESCE(size_bytes, 0),
		       is_builtin, COALESCE(init_system, ''), COALESCE(net_config_format, ''),
		       COALESCE(origin, 'catalog'), COALESCE(library_file, ''),
		       COALESCE(uploaded_by, ''), COALESCE(uploaded_at, ''),
		       created_at, updated_at
		  FROM plugin_addon_compute_images WHERE id = ?`, id)
	if err != nil {
		return Image{}, fmt.Errorf("reading image: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Image{}, err
		}
		return Image{}, ErrNotFound
	}
	return scanImage(rows)
}

// CreateImage adds an operator-supplied catalog entry.
func (s *Service) CreateImage(ctx context.Context, req ImageCreateRequest) (Image, error) {
	if err := validateImageRequest(req); err != nil {
		return Image{}, err
	}

	now := time.Now().UTC()
	image := Image{
		ID:           xid.New().String(),
		Kind:         req.Kind,
		Distro:       req.Distro,
		Release:      req.Release,
		Arch:         req.Arch,
		DisplayName:  req.DisplayName,
		SourceURL:    req.SourceURL,
		ChecksumURL:  req.ChecksumURL,
		Checksum:     req.Checksum,
		ChecksumAlgo: defaultString(req.ChecksumAlgo, "sha256"),
		SignatureURL: req.SignatureURL,
		InitSystem:   req.InitSystem,
		IsBuiltin:    false,
		Origin:       OriginCatalog,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_images
			(id, kind, distro, release, arch, display_name, source_url, checksum_url,
			 checksum, checksum_algo, signature_url, is_builtin, init_system,
			 origin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		image.ID, image.Kind, image.Distro, image.Release, image.Arch, image.DisplayName,
		image.SourceURL, nullIfEmpty(image.ChecksumURL), nullIfEmpty(image.Checksum),
		image.ChecksumAlgo, nullIfEmpty(image.SignatureURL), nullIfEmpty(image.InitSystem),
		image.Origin, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Image{}, fmt.Errorf("recording image: %w", err)
	}
	return image, nil
}

// UploadImage receives an operator-supplied image into the shared library.
//
// The row is written only after the bytes are safely on disk and hashed, so a
// failed or abandoned upload leaves no catalog entry claiming an image that is
// not there.
func (s *Service) UploadImage(ctx context.Context, req ImageUploadRequest, actor string, src io.Reader) (Image, error) {
	if err := validateUploadRequest(req); err != nil {
		return Image{}, err
	}

	now := time.Now().UTC()
	image := Image{
		ID:           xid.New().String(),
		Kind:         req.Kind,
		Distro:       req.Distro,
		Release:      req.Release,
		Arch:         req.Arch,
		Variant:      req.Variant,
		DisplayName:  req.DisplayName,
		ChecksumAlgo: "sha256",
		InitSystem:   req.InitSystem,
		Origin:       OriginUpload,
		UploadedBy:   actor,
		UploadedAt:   &now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	name, size, sum, err := s.receiveUpload(ctx, image.ID, req.FileName, src)
	if err != nil {
		return Image{}, err
	}
	image.LibraryFile = name
	image.SizeBytes = size
	image.Checksum = sum

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_images
			(id, kind, distro, release, arch, variant, display_name, source_url,
			 checksum, checksum_algo, size_bytes, is_builtin, init_system,
			 origin, library_file, uploaded_by, uploaded_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?)`,
		image.ID, image.Kind, image.Distro, image.Release, image.Arch,
		nullIfEmpty(image.Variant), image.DisplayName, image.Checksum, image.ChecksumAlgo,
		image.SizeBytes, nullIfEmpty(image.InitSystem), image.Origin, image.LibraryFile,
		nullIfEmpty(actor), now.Format(time.RFC3339),
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		// The row is what makes the file reachable, so a file with no row is
		// an orphan nothing will ever clean up.
		s.removeLibraryFile(name)
		return Image{}, fmt.Errorf("recording uploaded image: %w", err)
	}
	return image, nil
}

// validateUploadRequest checks the metadata beside an upload.
//
// No URL to check here — that is the difference between the two origins — so
// what is left is the kind, which decides how every guest plugin treats the
// file, and a name the operator will have to recognize later.
func validateUploadRequest(req ImageUploadRequest) error {
	switch req.Kind {
	case "cloud_image", "rootfs", "iso":
	default:
		return fmt.Errorf("%q is not an image kind", req.Kind)
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		return errors.New("a display name is required")
	}
	if strings.TrimSpace(req.FileName) == "" {
		return ErrUploadEmpty
	}
	return nil
}

// imageFetchCeiling bounds one download. It matches the agent's own
// computeLongTimeout, so the two ends give up at the same point rather than one
// waiting on a transfer the other abandoned hours ago.
const imageFetchCeiling = 6 * time.Hour

// FetchImage starts a download onto a host and returns as soon as it is under
// way.
//
// **The transfer does not run on the caller's context, and that is the whole
// point.** It used to: the entire multi-hundred-megabyte download happened
// inside the HTTP request, so the browser's POST had to stay open for the
// length of it. When it did not — a closed tab, a proxy timeout — the context
// was cancelled, the RPC returned, and the failure write that should have
// recorded it used that same dead context and was silently discarded. The row
// was left claiming `downloading` with nothing behind it, forever. One had been
// sitting at 46% with no file on the agent and no fetch in flight.
//
// Progress is written to the cache row rather than streamed, so a reload of the
// images page picks up an in-flight download instead of showing it as absent.
func (s *Service) FetchImage(ctx context.Context, imageID string, req ImageFetchRequest) (ImageCache, error) {
	if s.rpc == nil {
		return ImageCache{}, ErrNotAgentEnv
	}
	image, err := s.GetImage(ctx, imageID)
	if err != nil {
		return ImageCache{}, err
	}

	// An image with nothing to verify against downloads only on an explicit
	// acknowledgement, which reaches the audit row through the handler. This
	// covers a catalog entry whose publisher ships no checksum as well as an
	// operator-supplied URL. An upload can never need the waiver: McHarbor
	// hashed those bytes itself as they arrived. A waiver never suppresses a
	// mismatch — only the absence of a checksum.
	allowUnverified := false
	if image.Origin != OriginUpload && image.Checksum == "" && image.ChecksumURL == "" {
		if !req.AcknowledgeUnverified {
			return ImageCache{}, ErrUnverifiedRefused
		}
		allowUnverified = true
	}

	cacheID := s.upsertCache(ctx, image.ID, req.EnvironmentID, CacheStateDownloading, 0)

	go s.runImageFetch(image, req, cacheID, allowUnverified)

	return ImageCache{
		ID:            cacheID,
		ImageID:       image.ID,
		EnvironmentID: req.EnvironmentID,
		State:         CacheStateDownloading,
		ProgressPct:   0,
	}, nil
}

// FetchImageEverywhere downloads an image on every host that needs it.
//
// Hosts marked as sharing the library download once between them: a second
// fetch would pull the same bytes into the same directory. The others are
// reported as covered rather than marked ready, because only the adoption pass
// — which asks each host what it actually holds — may claim that.
func (s *Service) FetchImageEverywhere(ctx context.Context, imageID string) (ImageFetchAllResult, error) {
	result := ImageFetchAllResult{ImageID: imageID, Skipped: map[string]string{}}
	if s.rpc == nil {
		return result, ErrNotAgentEnv
	}
	if _, err := s.GetImage(ctx, imageID); err != nil {
		return result, err
	}

	environments, err := s.ListEnvironments(ctx)
	if err != nil {
		return result, err
	}

	sharedClaimed := false
	for _, env := range environments {
		if !env.ComputeEnabled {
			result.Skipped[env.EnvironmentID] = "compute_disabled"
			continue
		}
		if !env.Available {
			result.Skipped[env.EnvironmentID] = defaultString(env.UnavailableReason, "agent_offline")
			continue
		}

		caches, err := s.imageCaches(ctx, env.EnvironmentID)
		if err != nil {
			return result, err
		}
		if cache, ok := caches[imageID]; ok && cache.State == CacheStateReady {
			result.Skipped[env.EnvironmentID] = "already_present"
			continue
		}

		if env.SharedLibrary {
			if sharedClaimed {
				result.SharedWith = append(result.SharedWith, env.EnvironmentID)
				continue
			}
			sharedClaimed = true
			result.SharedWith = append(result.SharedWith, env.EnvironmentID)
		}

		if _, err := s.FetchImage(ctx, imageID, ImageFetchRequest{EnvironmentID: env.EnvironmentID}); err != nil {
			// One host refusing — an unverified entry, a pool that vanished —
			// must not stop the others, so the reason is recorded and the loop
			// carries on.
			result.Skipped[env.EnvironmentID] = "refused"
			s.logger.Warn("fan-out download refused",
				"env", env.EnvironmentID, "image", imageID, "error", err)
			continue
		}
		result.Started = append(result.Started, env.EnvironmentID)
	}
	return result, nil
}

// runImageFetch performs the transfer and records its outcome.
//
// Its context is deliberately unrelated to any request: it is created here and
// lives until the download ends or the ceiling expires, so every write it makes
// — progress, failure, success — is on a context that is still alive when it
// needs to be. That is the property the old code lacked.
func (s *Service) runImageFetch(image Image, req ImageFetchRequest, cacheID string, allowUnverified bool) {
	ctx, cancel := context.WithTimeout(context.Background(), imageFetchCeiling)
	defer cancel()

	// A host that mounts the library reads the file directly and never uses
	// this. One that does not needs somewhere to pull it from, and it cannot
	// be handed a session cookie, so it gets a scoped grant instead.
	var libraryURL, libraryToken string
	if image.Origin == OriginUpload && image.LibraryFile != "" {
		grantID, token, grantErr := s.grants.issue(image.ID, s.libraryFilePath(image.LibraryFile))
		if grantErr != nil {
			s.logger.Warn("compute: library pull grant not issued", "image", image.ID, "error", grantErr)
		} else {
			libraryURL = "/api/compute/internal/library/" + grantID
			libraryToken = token
			defer s.grants.revoke(grantID)
		}
	}

	// Names the mode and the file the agent is being told to look for. The
	// library path builds its filename from this, and a fetch that arrived
	// without it went looking for an extensionless name that was never written
	// — with nothing in the log to say so.
	s.logger.Info("compute: image fetch starting",
		"image", image.ID, "origin", image.Origin,
		"libraryFile", image.LibraryFile, "pull", libraryURL != "")

	result, err := s.rpc.ImageFetch(ctx, req.EnvironmentID, ImageRequest{
		ImageID:         image.ID,
		Kind:            image.Kind,
		SourceURL:       image.SourceURL,
		ChecksumURL:     image.ChecksumURL,
		Checksum:        image.Checksum,
		ChecksumAlgo:    image.ChecksumAlgo,
		SignatureURL:    image.SignatureURL,
		AllowUnverified: allowUnverified,
		FromLibrary:     image.Origin == OriginUpload,
		Filename:        image.LibraryFile,
		LibraryURL:      libraryURL,
		LibraryToken:    libraryToken,
	}, func(progress ImageProgress) {
		// Best-effort: a missed progress write costs a stale percentage for a
		// few seconds, and failing the download over it would be absurd.
		_, _ = s.db.Exec(ctx, `
			UPDATE plugin_addon_compute_image_cache
			   SET progress_pct = ?, phase = ?, updated_at = ?
			 WHERE id = ?`,
			progress.Percent, nullIfBlank(progress.Phase),
			time.Now().UTC().Format(time.RFC3339), cacheID)
	})
	if err != nil {
		s.logger.Warn("image fetch failed", "image", image.ID, "env", req.EnvironmentID, "error", err)
		if _, dbErr := s.db.Exec(ctx, `
			UPDATE plugin_addon_compute_image_cache
			   SET state = ?, last_error = ?, updated_at = ? WHERE id = ?`,
			CacheStateFailed, err.Error(), time.Now().UTC().Format(time.RFC3339), cacheID); dbErr != nil {
			s.logger.Warn("recording image fetch failure failed", "image", image.ID, "error", dbErr)
		}
		return
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_image_cache
		   SET local_path = ?, size_bytes = ?, verified = ?, signed = ?, state = ?,
		       progress_pct = 100, last_error = NULL, last_used_at = ?, updated_at = ?
		 WHERE id = ?`,
		result.LocalPath, result.SizeBytes, boolToInt(result.Verified), boolToInt(result.Signed),
		CacheStateReady, now.Format(time.RFC3339), now.Format(time.RFC3339), cacheID); err != nil {
		s.logger.Warn("recording image cache failed", "image", image.ID, "error", err)
	}

	// A download into the shared library landed on every host that mounts it,
	// so their inventories are stale the moment this one finishes.
	if s.sharedLibrary(ctx, req.EnvironmentID) {
		s.invalidateSharedReconciles(ctx, req.EnvironmentID)
	}
}

// invalidateSharedReconciles forgets the reconcile timestamp of every other
// host sharing the library, so the next read of their images asks the host
// rather than repeating what it last knew.
func (s *Service) invalidateSharedReconciles(ctx context.Context, exceptEnvID string) {
	environments, err := s.storedEnvironments(ctx)
	if err != nil {
		s.logger.Debug("invalidating shared reconciles failed", "error", err)
		return
	}
	for envID, env := range environments {
		if envID == exceptEnvID || !env.SharedLibrary {
			continue
		}
		s.invalidateReconcile(envID)
	}
}

// ResetInterruptedDownloads marks every row still claiming `downloading` as
// failed, and is called once at start-up.
//
// Nothing survives a server restart mid-transfer: the goroutine driving it is
// gone, and the agent's side is driven by that call. A row left saying
// `downloading` is therefore a claim that can never become true, and it hides
// the Download button behind a progress bar that will never move. Failed is the
// honest state, and it puts the retry back in reach.
func (s *Service) ResetInterruptedDownloads(ctx context.Context) {
	// Ask each host what it actually has before calling anything failed. The
	// fetch runs on the agent: a McHarbor restart ends the goroutine watching
	// it, not the download, so a row marked failed here was often an image
	// that arrived perfectly well a minute later.
	s.adoptFinishedDownloads(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	affected, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_image_cache
		   SET state = ?, last_error = ?, updated_at = ?
		 WHERE state = ?`,
		CacheStateFailed, "the download was interrupted by a McHarbor restart", now,
		CacheStateDownloading)
	if err != nil {
		s.logger.Warn("resetting interrupted image downloads failed", "error", err)
		return
	}
	if affected > 0 {
		s.logger.Info("reset interrupted image downloads", "count", affected)
	}
}

// adoptFinishedDownloads marks ready every stranded row whose image the host
// turns out to be holding.
//
// The agent renames an image into place only when it is complete and verified,
// so its presence in the host's cache is proof the fetch finished — whatever
// this process was doing at the time.
func (s *Service) adoptFinishedDownloads(ctx context.Context) {
	if s.rpc == nil {
		return
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, image_id, environment_id FROM plugin_addon_compute_image_cache
		 WHERE state = ? LIMIT 100`, CacheStateDownloading)
	if err != nil {
		return
	}
	type stranded struct{ id, imageID, envID string }
	pending := make([]stranded, 0, 4)
	for rows.Next() {
		var row stranded
		if err := rows.Scan(&row.id, &row.imageID, &row.envID); err != nil {
			break
		}
		pending = append(pending, row)
	}
	_ = rows.Close()
	if len(pending) == 0 {
		return
	}

	cached := make(map[string]map[string]CachedImage)
	for _, row := range pending {
		if _, ok := cached[row.envID]; !ok {
			listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			images, err := s.rpc.ImageList(listCtx, row.envID)
			cancel()
			if err != nil {
				cached[row.envID] = nil
				continue
			}
			byID := make(map[string]CachedImage, len(images))
			for _, image := range images {
				byID[image.ImageID] = image
			}
			cached[row.envID] = byID
		}
		image, ok := cached[row.envID][row.imageID]
		if !ok {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_compute_image_cache
			   SET state = ?, progress_pct = 100, phase = NULL, local_path = ?,
			       size_bytes = ?, verified = ?, last_error = NULL, updated_at = ?
			 WHERE id = ?`,
			CacheStateReady, image.LocalPath, image.SizeBytes, boolToInt(image.Verified),
			time.Now().UTC().Format(time.RFC3339), row.id); err != nil {
			s.logger.Warn("adopting a finished download failed", "image", row.imageID, "error", err)
			continue
		}
		s.logger.Info("adopted an image the host finished while McHarbor was away",
			"image", row.imageID, "path", image.LocalPath)
	}
}

// upsertCache creates or resets a cache row and returns its id.
func (s *Service) upsertCache(ctx context.Context, imageID, envID, state string, progress int) string {
	now := time.Now().UTC().Format(time.RFC3339)
	id := xid.New().String()

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_image_cache
			(id, image_id, environment_id, local_path, state, progress_pct, created_at, updated_at)
		VALUES (?, ?, ?, '', ?, ?, ?, ?)
		ON CONFLICT(image_id, environment_id) DO UPDATE SET
			state = excluded.state, progress_pct = excluded.progress_pct,
			last_error = NULL, updated_at = excluded.updated_at`,
		id, imageID, envID, state, progress, now, now); err != nil {
		s.logger.Warn("creating image cache row failed", "image", imageID, "error", err)
	}

	// Read the id back: on a conflict the row keeps the id it already had.
	rows, err := s.db.Query(ctx, `
		SELECT id FROM plugin_addon_compute_image_cache
		 WHERE image_id = ? AND environment_id = ?`, imageID, envID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		if rows.Next() {
			var existing string
			if err := rows.Scan(&existing); err == nil {
				return existing
			}
		}
	}
	return id
}

// EvictImage removes a cached copy from a host.
func (s *Service) EvictImage(ctx context.Context, imageID, envID string) error {
	if s.rpc == nil {
		return ErrNotAgentEnv
	}
	// The agent refuses an image backing a live guest disk, which is the check
	// that actually protects a running VM from losing its backing file.
	if err := s.rpc.ImageRemove(ctx, envID, imageID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		DELETE FROM plugin_addon_compute_image_cache
		 WHERE image_id = ? AND environment_id = ?`, imageID, envID)
	return err
}

// DeleteImage removes an operator-supplied catalog entry.
//
// A built-in entry cannot be deleted — only its cached copies evicted — because
// the seed would recreate it on the next migration run and the operator would
// reasonably conclude the delete had failed.
func (s *Service) DeleteImage(ctx context.Context, id string) error {
	image, err := s.GetImage(ctx, id)
	if err != nil {
		return err
	}
	if image.IsBuiltin {
		return fmt.Errorf("%w; evict the cached copy instead", ErrBuiltinImage)
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM plugin_addon_compute_images WHERE id = ?`, id); err != nil {
		return err
	}
	// An upload has no mirror to be fetched from again, so its bytes go with
	// the row. Deleted after the row, not before: a failed delete that left the
	// row behind would leave an entry pointing at a file that is gone.
	if image.Origin == OriginUpload {
		s.removeLibraryFile(image.LibraryFile)
	}
	return nil
}

// --- Overview -------------------------------------------------------------

// Overview assembles the landing page.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	environments, err := s.ListEnvironments(ctx)
	if err != nil {
		return Overview{}, err
	}
	pools, err := s.ListPools(ctx, "")
	if err != nil {
		return Overview{}, err
	}
	networks, err := s.ListNetworks(ctx, "")
	if err != nil {
		return Overview{}, err
	}

	poolsByEnv := map[string][]Pool{}
	for _, pool := range pools {
		poolsByEnv[pool.EnvironmentID] = append(poolsByEnv[pool.EnvironmentID], pool)
	}
	networksByEnv := map[string][]Network{}
	for _, network := range networks {
		networksByEnv[network.EnvironmentID] = append(networksByEnv[network.EnvironmentID], network)
	}

	out := Overview{}
	for _, env := range environments {
		entry := OverviewEntry{
			EnvironmentID:     env.EnvironmentID,
			Name:              env.Name,
			ComputeEnabled:    env.ComputeEnabled,
			Available:         env.Available,
			UnavailableReason: env.UnavailableReason,
			Capability:        env.Capability,
			Pools:             poolsByEnv[env.EnvironmentID],
			Networks:          networksByEnv[env.EnvironmentID],
			SharedLibrary:     env.SharedLibrary,
		}
		entry.SetupComplete = env.ComputeEnabled &&
			len(entry.Pools) > 0 && len(entry.Networks) > 0
		out.Environments = append(out.Environments, entry)
	}
	return out, nil
}

// --- Validation -----------------------------------------------------------

// poolNameRE-equivalent check, kept as a function so the error names the
// offending character rather than only failing.
func validateName(name string) error {
	if name == "" {
		return errors.New("a name is required")
	}
	if len(name) > 64 {
		return errors.New("that name is too long")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == ' ':
		default:
			return fmt.Errorf("that name cannot contain %q", r)
		}
	}
	return nil
}

func validatePoolRequest(req PoolCreateRequest) error {
	if req.EnvironmentID == "" {
		return errors.New("an environment is required")
	}
	if err := validateName(req.Name); err != nil {
		return err
	}
	switch req.Backend {
	case BackendDir, BackendZFS, BackendLVM, BackendBtrfs:
	default:
		return fmt.Errorf("%q is not a storage backend", req.Backend)
	}
	if req.Target == "" {
		return errors.New("a target is required")
	}
	// Path-shaped targets get the deny-list check here as well as on the
	// agent. This copy is UX — it stops a typo before it becomes a request —
	// and the agent's is the security boundary.
	if req.Backend == BackendDir || req.Backend == BackendBtrfs {
		if !strings.HasPrefix(req.Target, "/") {
			return fmt.Errorf("%w: a %s pool needs an absolute path", ErrInvalidTarget, req.Backend)
		}
		clean := path.Clean(req.Target)
		if clean != req.Target {
			return fmt.Errorf("%w: %q is not a normalized path", ErrInvalidTarget, req.Target)
		}
		for _, denied := range []string{"/", "/bin", "/boot", "/dev", "/etc", "/lib", "/proc",
			"/root", "/run", "/sbin", "/sys", "/usr", "/var/lib/docker"} {
			if clean == denied || (denied != "/" && strings.HasPrefix(clean, denied+"/")) {
				return fmt.Errorf("%w: %q is a system directory", ErrInvalidTarget, clean)
			}
		}
	}
	return nil
}

func validateNetworkRequest(req NetworkCreateRequest) error {
	if req.EnvironmentID == "" {
		return errors.New("an environment is required")
	}
	if err := validateName(req.Name); err != nil {
		return err
	}
	switch req.Mode {
	case "bridged":
		if req.Bridge == "" {
			return errors.New("a bridged network needs an existing bridge")
		}
	case "managed":
		if req.SubnetCIDR == "" || req.GatewayIP == "" {
			return errors.New("a managed network needs a subnet and a gateway address")
		}
		if req.NATEnabled && req.UplinkLink == "" {
			return errors.New("routing guest traffic out needs an outbound interface")
		}
	case "external":
		if req.UplinkLink == "" {
			return errors.New("an external network needs an adapter to share")
		}
	case "macvlan":
		if err := validateDirectRequest(req); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%q is not a network mode", req.Mode)
	}
	if req.VLANTag != 0 && (req.VLANTag < 1 || req.VLANTag > 4094) {
		return fmt.Errorf("VLAN %d is out of range", req.VLANTag)
	}
	return nil
}

// validateDirectRequest checks a macvlan network.
//
// Two of these refuse combinations the kernel would accept and the operator
// would then have to debug: McHarbor's own DHCP on a segment that already has
// one, and a host shim on a passthru network that can only ever have a single
// child.
func validateDirectRequest(req NetworkCreateRequest) error {
	if req.UplinkLink == "" {
		return fmt.Errorf("a %s network needs a parent interface", req.Mode)
	}
	if err := validateLinkMode(req.Mode, req.LinkMode); err != nil {
		return err
	}
	if req.DHCPEnabled || req.NATEnabled {
		return errors.New("a direct network is addressed by the LAN it joins, " +
			"so McHarbor runs neither DHCP nor NAT on it")
	}
	if err := validateHostLinkFields(req); err != nil {
		return err
	}
	if req.HostAccess {
		if req.Mode == "macvlan" && req.LinkMode == "passthru" {
			return errors.New("a passthru network hands the adapter to one guest, " +
				"so the host cannot also have an address on it")
		}
		if req.HostAccessIP != "" {
			if _, _, err := net.ParseCIDR(req.HostAccessIP); err != nil {
				return errors.New("the host address must be written as an address and prefix, like 192.168.1.9/24")
			}
		}
	}
	if req.SubnetCIDR != "" {
		if _, _, err := net.ParseCIDR(req.SubnetCIDR); err != nil {
			return errors.New("that is not a valid subnet")
		}
	}
	return nil
}

// validateLinkMode rejects a sub-mode the driver does not have. The agent
// checks this again; a bad value there would otherwise be taken as the
// driver's default and quietly produce a network nobody asked for.
func validateLinkMode(mode, linkMode string) error {
	if linkMode == "" {
		return nil
	}
	switch mode {
	case "macvlan":
		switch linkMode {
		case "bridge", "vepa", "private", "passthru":
			return nil
		}
	}
	return fmt.Errorf("%q is not a %s mode", linkMode, mode)
}

func validateImageRequest(req ImageCreateRequest) error {
	switch req.Kind {
	case "cloud_image", "rootfs", "iso":
	default:
		return fmt.Errorf("%w: %q is not an image kind", ErrInvalidImage, req.Kind)
	}
	if req.DisplayName == "" {
		return fmt.Errorf("%w: a display name is required", ErrInvalidImage)
	}
	parsed, err := url.Parse(req.SourceURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: that is not a valid URL", ErrInvalidImage)
	}
	// https only. Over plain http the checksum is the only integrity defence,
	// and anyone who can rewrite the image in flight can rewrite the checksum
	// file fetched over the same channel.
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: image sources must use https", ErrInvalidImage)
	}
	return nil
}
