// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// agentTokenFormatRE matches the plaintext shape the agent binary
// issues at install time (see core/agent/install.go issueToken —
// `agt_` + 32 random bytes hex-encoded → 64 hex chars). Enforced on
// the import-existing-token path so we never store a malformed value
// that the live agent would refuse to authenticate against.
var agentTokenFormatRE = regexp.MustCompile(`^agt_[0-9a-f]{64}$`)

// ErrInvalidAgentToken is returned when an operator-provided agent
// token doesn't match the canonical agt_<64 hex> shape. The handler
// maps this to i18n.ErrEnvInvalidAgentToken so the UI can surface
// the localized reason instead of a generic 500.
var ErrInvalidAgentToken = errors.New("invalid agent token format")

// Service handles environment CRUD operations against the database.
type Service struct {
	db         *sql.DB
	dockerPool *docker.ClientPool
	agentPool  *coreagent.AgentPool
	enc        *encryption.Service
}

// NewService creates a new environment service.
func NewService(db *sql.DB, dockerPool *docker.ClientPool, agentPool *coreagent.AgentPool, enc *encryption.Service) *Service {
	return &Service{db: db, dockerPool: dockerPool, agentPool: agentPool, enc: enc}
}

// ListContext returns all environments from the database.
func (s *Service) ListContext(ctx context.Context) ([]Environment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, connection_type, socket_path, host, port,
		       tls_ca, tls_cert, tls_key, ssh_host, ssh_port, ssh_user, ssh_key,
		       is_default, is_active, docker_version, last_connected,
		       agent_token, agent_status, agent_version, agent_hostname, agent_os, agent_arch, agent_last_seen, agent_install_kind,
		       scheduled_update_check_enabled, automatic_image_pruning_enabled,
		       track_container_events_enabled, collect_container_metrics_enabled,
		       highlight_container_changes_enabled, docker_disk_usage_notifications_enabled,
		       docker_disk_usage_threshold_percent, docker_disk_usage_use_global_default, timezone,
		       log_retention_days, container_prune_enabled, container_prune_stopped_days,
		       image_prune_dangling_only,
		       auto_update_enabled, auto_update_window_start, auto_update_window_end, auto_update_days,
		       metric_retention_hours,
		       pending_connection_type, pending_migration_started_at, migration_revert_to,
		       created_at, updated_at,
		       (SELECT COUNT(*) FROM stacks st WHERE st.environment_id = environments.id) AS stack_count
		FROM environments
		ORDER BY is_default DESC, name ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying environments: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	var envs []Environment
	for rows.Next() {
		env, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		s.applyLiveAgentMetadata(&env)
		s.redactSecrets(&env)
		envs = append(envs, env)
	}
	if envs == nil {
		envs = []Environment{}
	}
	return envs, rows.Err()
}

// ByID returns a single environment by ID.
func (s *Service) ByID(ctx context.Context, id string) (*Environment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, connection_type, socket_path, host, port,
		       tls_ca, tls_cert, tls_key, ssh_host, ssh_port, ssh_user, ssh_key,
		       is_default, is_active, docker_version, last_connected,
		       agent_token, agent_status, agent_version, agent_hostname, agent_os, agent_arch, agent_last_seen, agent_install_kind,
		       scheduled_update_check_enabled, automatic_image_pruning_enabled,
		       track_container_events_enabled, collect_container_metrics_enabled,
		       highlight_container_changes_enabled, docker_disk_usage_notifications_enabled,
		       docker_disk_usage_threshold_percent, docker_disk_usage_use_global_default, timezone,
		       log_retention_days, container_prune_enabled, container_prune_stopped_days,
		       image_prune_dangling_only,
		       auto_update_enabled, auto_update_window_start, auto_update_window_end, auto_update_days,
		       metric_retention_hours,
		       pending_connection_type, pending_migration_started_at, migration_revert_to,
		       created_at, updated_at
		FROM environments WHERE id = ?
	`, id)

	env, err := scanEnvironmentRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying environment %s: %w", id, err)
	}
	s.applyLiveAgentMetadata(env)
	s.redactSecrets(env)
	return env, nil
}

func (s *Service) applyLiveAgentMetadata(env *Environment) {
	if env == nil || env.ConnectionType != "agent" || s.agentPool == nil {
		return
	}
	conn, ok := s.agentPool.Get(env.ID)
	if !ok {
		return
	}
	connected := "connected"
	env.AgentStatus = &connected
	env.AgentVersion = stringPtr(conn.Version)
	env.AgentHostname = stringPtr(conn.Hostname)
	env.AgentOS = stringPtr(conn.OS)
	env.AgentArch = stringPtr(conn.Arch)
	env.DockerVersion = stringPtr(conn.DockerVer)
	env.AgentHostIP = stringPtr(conn.RemoteIP)
	env.AgentBuild = stringPtr(conn.Build)
	env.AgentTLSMode = stringPtr(conn.TLSMode)
}

// Create inserts a new environment into the database.
// Returns the environment and, for agent connections, the plaintext token (shown once).
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Environment, string, error) {
	if req.Name == "" {
		return nil, "", fmt.Errorf("name is required")
	}
	if req.ConnectionType == "" {
		req.ConnectionType = "socket"
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Encrypt sensitive Docker TLS/SSH fields
	tlsCa, err := s.encryptOptional(req.TLSCa)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting tls_ca: %w", err)
	}
	tlsCert, err := s.encryptOptional(req.TLSCert)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting tls_cert: %w", err)
	}
	tlsKey, err := s.encryptOptional(req.TLSKey)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting tls_key: %w", err)
	}
	sshKey, err := s.encryptOptional(req.SSHKey)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting ssh_key: %w", err)
	}

	// Generate agent token if agent connection type. When the operator
	// already has an agent deployed on another host and is registering
	// that agent here (typical for the "I run McHarbor on my laptop
	// and on my homelab, both pointing at the same control plane"
	// workflow), they pass the existing plaintext via req.AgentToken
	// and the host skips the "show token once" UX — the user already
	// has it. The token still has to match the `agt_<64 hex>` shape
	// the agent binary expects (modules/agent/install.go:issueToken).
	//
	// On the import path we deliberately leave `plainToken` empty so
	// the handler's `if agentToken != ""` check correctly skips the
	// "show token once" echo. We still persist an encrypted copy in
	// `agent_token` so the legacy decrypt path keeps working, and the
	// stable hash in `agent_token_hash` is what live agents
	// authenticate against.
	var agentToken *string
	var agentTokenHash *string
	var plainToken string
	if req.ConnectionType == "agent" {
		var resolved string
		if req.AgentToken != nil && strings.TrimSpace(*req.AgentToken) != "" {
			candidate := strings.TrimSpace(*req.AgentToken)
			if !agentTokenFormatRE.MatchString(candidate) {
				return nil, "", fmt.Errorf("%w (must be agt_<64 hex chars>)", ErrInvalidAgentToken)
			}
			resolved = candidate
		} else {
			minted, mintErr := s.mintAgentToken()
			if mintErr != nil {
				return nil, "", mintErr
			}
			resolved = minted
			// Only mint-new path surfaces the plaintext to the
			// caller. The import-existing path keeps `plainToken`
			// empty so the handler's "show token once" check
			// (`if agentToken != ""`) correctly skips echoing back
			// something the user already has.
			plainToken = resolved
		}
		encrypted, err := s.enc.Encrypt(resolved)
		if err != nil {
			return nil, "", fmt.Errorf("encrypting agent token: %w", err)
		}
		agentToken = &encrypted
		hash := s.enc.StableHash(resolved)
		agentTokenHash = &hash
	}

	// If setting as default, clear other defaults first
	if req.IsDefault {
		if _, err := s.db.ExecContext(ctx, "UPDATE environments SET is_default = 0"); err != nil {
			return nil, "", fmt.Errorf("clearing default environments: %w", err)
		}
	}

	isDefault := 0
	if req.IsDefault {
		isDefault = 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO environments (id, name, connection_type, socket_path, host, port,
		                          tls_ca, tls_cert, tls_key, ssh_host, ssh_port, ssh_user, ssh_key,
		                          is_default, is_active,
		                          agent_token, agent_token_hash,
		                          created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)
	`, id, req.Name, req.ConnectionType, req.SocketPath, req.Host, req.Port,
		tlsCa, tlsCert, tlsKey, req.SSHHost, req.SSHPort, req.SSHUser, sshKey,
		isDefault,
		agentToken, agentTokenHash,
		now, now)
	if err != nil {
		return nil, "", fmt.Errorf("inserting environment: %w", err)
	}

	env, err := s.ByID(ctx, id)
	return env, plainToken, err
}

// Update modifies an existing environment.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Environment, error) {
	existing, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Build update fields
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	// Deliberately ignored. A connection-type change mints or revokes a token,
	// clears agent metadata and drops a live connection; writing the column
	// alone leaves an environment that claims one transport and holds the
	// credentials of another. MigrateConnection is the one path that does it.
	connType := existing.ConnectionType
	socketPath := existing.SocketPath
	if req.SocketPath != nil {
		socketPath = req.SocketPath
	}
	host := existing.Host
	if req.Host != nil {
		host = req.Host
	}
	port := existing.Port
	if req.Port != nil {
		port = req.Port
	}
	sshHost := existing.SSHHost
	if req.SSHHost != nil {
		sshHost = req.SSHHost
	}
	sshPort := existing.SSHPort
	if req.SSHPort != nil {
		sshPort = req.SSHPort
	}
	sshUser := existing.SSHUser
	if req.SSHUser != nil {
		sshUser = req.SSHUser
	}

	// Encrypt TLS/SSH secrets if provided
	var tlsCa, tlsCert, tlsKey, sshKey *string
	if req.TLSCa != nil {
		v, encErr := s.encryptOptional(req.TLSCa)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting tls_ca: %w", encErr)
		}
		tlsCa = v
	}
	if req.TLSCert != nil {
		v, encErr := s.encryptOptional(req.TLSCert)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting tls_cert: %w", encErr)
		}
		tlsCert = v
	}
	if req.TLSKey != nil {
		v, encErr := s.encryptOptional(req.TLSKey)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting tls_key: %w", encErr)
		}
		tlsKey = v
	}
	if req.SSHKey != nil {
		v, encErr := s.encryptOptional(req.SSHKey)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting ssh_key: %w", encErr)
		}
		sshKey = v
	}

	isDefault := existing.IsDefault
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
		if isDefault {
			if _, err := s.db.ExecContext(ctx, "UPDATE environments SET is_default = 0"); err != nil {
				return nil, fmt.Errorf("clearing default environments: %w", err)
			}
		}
	}
	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	scheduledUpdateCheckEnabled := existing.ScheduledUpdateCheckEnabled
	if req.ScheduledUpdateCheckEnabled != nil {
		scheduledUpdateCheckEnabled = *req.ScheduledUpdateCheckEnabled
	}
	automaticImagePruningEnabled := existing.AutomaticImagePruningEnabled
	if req.AutomaticImagePruningEnabled != nil {
		automaticImagePruningEnabled = *req.AutomaticImagePruningEnabled
	}
	trackContainerEventsEnabled := existing.TrackContainerEventsEnabled
	if req.TrackContainerEventsEnabled != nil {
		trackContainerEventsEnabled = *req.TrackContainerEventsEnabled
	}
	collectContainerMetricsEnabled := existing.CollectContainerMetricsEnabled
	if req.CollectContainerMetricsEnabled != nil {
		collectContainerMetricsEnabled = *req.CollectContainerMetricsEnabled
	}
	highlightContainerChangesEnabled := existing.HighlightContainerChangesEnabled
	if req.HighlightContainerChangesEnabled != nil {
		highlightContainerChangesEnabled = *req.HighlightContainerChangesEnabled
	}
	dockerDiskUsageNotificationsEnabled := existing.DockerDiskUsageNotificationsEnabled
	if req.DockerDiskUsageNotificationsEnabled != nil {
		dockerDiskUsageNotificationsEnabled = *req.DockerDiskUsageNotificationsEnabled
	}
	dockerDiskUsageThresholdPercent := existing.DockerDiskUsageThresholdPercent
	if req.DockerDiskUsageThresholdPercent != nil {
		dockerDiskUsageThresholdPercent = *req.DockerDiskUsageThresholdPercent
	}
	dockerDiskUsageUseGlobalDefault := existing.DockerDiskUsageUseGlobalDefault
	if req.DockerDiskUsageUseGlobalDefault != nil {
		dockerDiskUsageUseGlobalDefault = *req.DockerDiskUsageUseGlobalDefault
	}
	timezone := existing.Timezone
	if req.Timezone != nil {
		timezone = *req.Timezone
	}
	logRetentionDays := existing.LogRetentionDays
	if req.LogRetentionDays != nil {
		logRetentionDays = *req.LogRetentionDays
	}
	containerPruneEnabled := existing.ContainerPruneEnabled
	if req.ContainerPruneEnabled != nil {
		containerPruneEnabled = *req.ContainerPruneEnabled
	}
	containerPruneStoppedDays := existing.ContainerPruneStoppedDays
	if req.ContainerPruneStoppedDays != nil {
		containerPruneStoppedDays = *req.ContainerPruneStoppedDays
	}
	imagePruneDanglingOnly := existing.ImagePruneDanglingOnly
	if req.ImagePruneDanglingOnly != nil {
		imagePruneDanglingOnly = *req.ImagePruneDanglingOnly
	}
	autoUpdateEnabled := existing.AutoUpdateEnabled
	if req.AutoUpdateEnabled != nil {
		autoUpdateEnabled = *req.AutoUpdateEnabled
	}
	autoUpdateWindowStart := existing.AutoUpdateWindowStart
	if req.AutoUpdateWindowStart != nil {
		autoUpdateWindowStart = *req.AutoUpdateWindowStart
	}
	autoUpdateWindowEnd := existing.AutoUpdateWindowEnd
	if req.AutoUpdateWindowEnd != nil {
		autoUpdateWindowEnd = *req.AutoUpdateWindowEnd
	}
	autoUpdateDays := existing.AutoUpdateDays
	if req.AutoUpdateDays != nil {
		autoUpdateDays = *req.AutoUpdateDays
	}
	metricRetentionHours := existing.MetricRetentionHours
	if req.MetricRetentionHours != nil {
		metricRetentionHours = *req.MetricRetentionHours
	}

	defaultInt := 0
	if isDefault {
		defaultInt = 1
	}
	activeInt := 0
	if isActive {
		activeInt = 1
	}
	scheduledUpdateCheckEnabledInt := 0
	if scheduledUpdateCheckEnabled {
		scheduledUpdateCheckEnabledInt = 1
	}
	automaticImagePruningEnabledInt := 0
	if automaticImagePruningEnabled {
		automaticImagePruningEnabledInt = 1
	}
	trackContainerEventsEnabledInt := 0
	if trackContainerEventsEnabled {
		trackContainerEventsEnabledInt = 1
	}
	collectContainerMetricsEnabledInt := 0
	if collectContainerMetricsEnabled {
		collectContainerMetricsEnabledInt = 1
	}
	highlightContainerChangesEnabledInt := 0
	if highlightContainerChangesEnabled {
		highlightContainerChangesEnabledInt = 1
	}
	dockerDiskUsageNotificationsEnabledInt := 0
	if dockerDiskUsageNotificationsEnabled {
		dockerDiskUsageNotificationsEnabledInt = 1
	}
	dockerDiskUsageUseGlobalDefaultInt := 0
	if dockerDiskUsageUseGlobalDefault {
		dockerDiskUsageUseGlobalDefaultInt = 1
	}
	containerPruneEnabledInt := 0
	if containerPruneEnabled {
		containerPruneEnabledInt = 1
	}
	imagePruneDanglingOnlyInt := 0
	if imagePruneDanglingOnly {
		imagePruneDanglingOnlyInt = 1
	}
	autoUpdateEnabledInt := 0
	if autoUpdateEnabled {
		autoUpdateEnabledInt = 1
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE environments
		SET name = ?, connection_type = ?, socket_path = ?, host = ?, port = ?,
		    tls_ca = COALESCE(?, tls_ca), tls_cert = COALESCE(?, tls_cert),
		    tls_key = COALESCE(?, tls_key),
		    ssh_host = ?, ssh_port = ?, ssh_user = ?,
		    ssh_key = COALESCE(?, ssh_key),
		    -- The four *_secret_id pointers are cleared on every update so a
		    -- rewritten certificate or key can never be shadowed by a stale
		    -- vault copy. The start-up adoption pass re-links them.
		    tls_ca_secret_id = NULL, tls_cert_secret_id = NULL,
		    tls_key_secret_id = NULL, ssh_key_secret_id = NULL,
		    is_default = ?, is_active = ?,
		    scheduled_update_check_enabled = ?, automatic_image_pruning_enabled = ?,
		    track_container_events_enabled = ?, collect_container_metrics_enabled = ?,
		    highlight_container_changes_enabled = ?, docker_disk_usage_notifications_enabled = ?,
		    docker_disk_usage_threshold_percent = ?, docker_disk_usage_use_global_default = ?, timezone = ?,
		    log_retention_days = ?, container_prune_enabled = ?, container_prune_stopped_days = ?,
		    image_prune_dangling_only = ?,
		    auto_update_enabled = ?, auto_update_window_start = ?, auto_update_window_end = ?, auto_update_days = ?,
		    metric_retention_hours = ?,
		    updated_at = ?
		WHERE id = ?
	`, name, connType, socketPath, host, port,
		tlsCa, tlsCert, tlsKey,
		sshHost, sshPort, sshUser, sshKey,
		defaultInt, activeInt,
		scheduledUpdateCheckEnabledInt, automaticImagePruningEnabledInt,
		trackContainerEventsEnabledInt, collectContainerMetricsEnabledInt,
		highlightContainerChangesEnabledInt, dockerDiskUsageNotificationsEnabledInt,
		dockerDiskUsageThresholdPercent, dockerDiskUsageUseGlobalDefaultInt, timezone,
		logRetentionDays, containerPruneEnabledInt, containerPruneStoppedDays,
		imagePruneDanglingOnlyInt,
		autoUpdateEnabledInt, autoUpdateWindowStart, autoUpdateWindowEnd, autoUpdateDays,
		metricRetentionHours,
		now, id)
	if err != nil {
		return nil, fmt.Errorf("updating environment: %w", err)
	}

	// Invalidate cached clients for this environment. Both pools cache by
	// environment ID and neither notices a credential change on its own, so an
	// edited cluster would keep talking through the stale clientset.
	s.dockerPool.Remove(id)

	return s.ByID(ctx, id)
}

// mintAgentToken issues a new plaintext agent token.
//
// The shape is the one the agent binary expects and agentTokenFormatRE
// enforces; crypto/rand because this is the environment's only credential.
func (s *Service) mintAgentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating agent token: %w", err)
	}
	return "agt_" + hex.EncodeToString(b), nil
}

// Delete removes an environment from the database.
func (s *Service) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM environments WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting environment: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("environment not found")
	}

	// Remove cached clients
	s.dockerPool.Remove(id)

	return nil
}

// TestConnection pings the Docker daemon for the
// given environment and returns a per-step report suitable for the
// frontend dialog. The report lists each connection phase (pool reset,
// client acquisition, daemon ping, version parse, DB persist) with
// its own pass/fail status and latency, plus an overall rolled-up
// status and the resolved Docker version when the test
// succeeds.
func (s *Service) TestConnection(ctx context.Context, id string) *EnvTestResult {
	// Look up environment basics so the dialog can show the name.
	env, err := s.ByID(ctx, id)
	if err != nil {
		return &EnvTestResult{
			EnvID:   id,
			Overall: "fail",
			Steps: []EnvTestStep{{
				Name:   "load",
				Status: "fail",
				Detail: "environment not found",
			}},
			StartedAt:   time.Now().UTC().Format(time.RFC3339),
			CompletedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}

	if env == nil {
		return &EnvTestResult{
			EnvID:   id,
			Overall: "fail",
			Steps: []EnvTestStep{{
				Name:   "load",
				Status: "fail",
				Detail: "environment not found",
			}},
			StartedAt:   time.Now().UTC().Format(time.RFC3339),
			CompletedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}

	startedAt := time.Now().UTC()
	now := startedAt.Format(time.RFC3339)

	result := s.testDockerConnection(ctx, id, now)
	result.EnvID = id
	result.EnvName = env.Name
	result.Orchestrator = "docker"
	result.StartedAt = now
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	result.Duration = result.CompletedAt
	if started, parseErr := time.Parse(time.RFC3339, now); parseErr == nil && !started.IsZero() {
		result.Duration = time.Since(started).String()
	}
	return result
}

// appendStep records a single connection-test phase. Latency is
// measured automatically; the caller just supplies name, status,
// and an optional detail string.
func appendStep(steps *[]EnvTestStep, name, status, detail string, started time.Time) *EnvTestStep {
	latency := time.Since(started).String()
	s := EnvTestStep{Name: name, Status: status, Detail: detail, Latency: latency}
	*steps = append(*steps, s)
	return &s
}

// rollupStatus returns the worst status in steps (fail > warn > pass >
// skip), defaulting to pass for an empty list. It mirrors the storage
// test's rollup so the dialog can render a single overall badge.
func rollupStatus(steps []EnvTestStep) string {
	worst := "pass"
	for _, s := range steps {
		switch s.Status {
		case "fail":
			return "fail"
		case "warn":
			worst = "warn"
		case "skip":
			// skip doesn't demote a passing overall status.
		}
	}
	return worst
}

func (s *Service) testDockerConnection(ctx context.Context, id string, now string) *EnvTestResult {
	result := &EnvTestResult{}

	// Step 1: clear cached client so the test exercises a fresh
	// connection (and picks up any TLS / socket changes from a
	// reconfigured environment).
	started := time.Now()
	s.dockerPool.Remove(id)
	appendStep(&result.Steps, "reset", "pass", "cleared cached client", started)

	// Step 2: acquire a client through the pool. This is where TCP /
	// TLS / SSH / agent handshake errors surface.
	started = time.Now()
	cli, err := s.dockerPool.GetContext(ctx, id)
	if err != nil {
		appendStep(&result.Steps, "connect", "fail", "failed to acquire Docker client", started)
		slog.Error("environments: failed to connect to Docker", "error", err, "id", id)
		result.Overall = "fail"
		return result
	}
	appendStep(&result.Steps, "connect", "pass", "client acquired", started)

	// Step 3: ping the daemon. cli's Ping calls /_ping which is the
	// cheapest liveness probe.
	started = time.Now()
	pingCtx, cancelPing := context.WithTimeout(ctx, docker.CallTimeout)
	ping, err := cli.Ping(pingCtx, client.PingOptions{})
	cancelPing()
	if err != nil {
		appendStep(&result.Steps, "ping", "fail", "Docker /_ping failed", started)
		slog.Error("environments: connection test failed", "error", err, "id", id)
		result.Overall = "fail"
		return result
	}
	appendStep(&result.Steps, "ping", "pass", "daemon responded to ping", started)

	// Step 4: parse the version string from the Ping response. The
	// Ping payload includes APIVersion (e.g. "1.45") but not the
	// full server version. We try /info as a follow-up to get the
	// human-readable version too; failure to do so is a warning,
	// not a failure, because APIVersion is already informative.
	started = time.Now()
	version := ping.APIVersion
	infoCtx, cancelInfo := context.WithTimeout(ctx, docker.CallTimeout)
	infoRes, err := cli.Info(infoCtx, client.InfoOptions{})
	info := infoRes.Info
	cancelInfo()
	if err == nil && info.ServerVersion != "" {
		version = info.ServerVersion
		appendStep(&result.Steps, "version", "pass", "server version: "+info.ServerVersion, started)
	} else {
		appendStep(&result.Steps, "version", "warn",
			"API version only: "+ping.APIVersion+"; /info unavailable", started)
	}

	// Step 5: persist the resolved version so the rest of the app
	// (overview metrics, etc.) reads it without re-pinging.
	started = time.Now()
	if _, err := s.db.ExecContext(ctx,
		"UPDATE environments SET docker_version = ?, last_connected = ?, updated_at = ? WHERE id = ?",
		version, now, now, id,
	); err != nil {
		appendStep(&result.Steps, "persist", "fail", "database update failed", started)
		slog.Error("environments: failed to update docker version", "error", err, "id", id)
		result.Overall = rollupStatus(result.Steps)
		return result
	}
	appendStep(&result.Steps, "persist", "pass", "stored docker_version + last_connected", started)

	result.DockerVersion = &version
	result.Overall = rollupStatus(result.Steps)
	return result
}

// DetectSocket auto-detects Docker sockets on the host.
func (s *Service) DetectSocket() []DetectedSocket {
	var sockets []DetectedSocket

	if p := docker.DetectDockerSocket(); p != "" {
		sockets = append(sockets, DetectedSocket{Path: p, Runtime: "docker"})
	}

	if sockets == nil {
		sockets = []DetectedSocket{}
	}
	return sockets
}

// encryptOptional encrypts a value if non-nil and non-empty.
func (s *Service) encryptOptional(val *string) (*string, error) {
	if val == nil || *val == "" {
		return val, nil
	}
	encrypted, err := s.enc.Encrypt(*val)
	if err != nil {
		return val, err
	}
	return &encrypted, nil
}

// redactSecrets clears sensitive secret values from the response.
func (s *Service) redactSecrets(env *Environment) {
	redacted := "[encrypted]"
	if env.TLSCa != nil && *env.TLSCa != "" {
		env.TLSCa = &redacted
	}
	if env.TLSCert != nil && *env.TLSCert != "" {
		env.TLSCert = &redacted
	}
	if env.TLSKey != nil && *env.TLSKey != "" {
		env.TLSKey = &redacted
	}
	if env.SSHKey != nil && *env.SSHKey != "" {
		env.SSHKey = &redacted
	}
	if env.AgentToken != nil && *env.AgentToken != "" {
		env.AgentToken = &redacted
	}
}

// scanEnvironment scans a row set into an Environment struct.
func scanEnvironment(rows *sql.Rows) (Environment, error) {
	var env Environment
	var socketPath, host, tlsCa, tlsCert, tlsKey sql.NullString
	var sshHost, sshUser, sshKey, dockerVersion, lastConnected sql.NullString
	var agentToken, agentStatus, agentVersion, agentHostname, agentOS, agentArch, agentLastSeen sql.NullString
	var agentInstallKind sql.NullString
	var port, sshPort sql.NullInt64
	var isDefault, isActive, scheduledUpdateCheckEnabled, automaticImagePruningEnabled int
	var trackContainerEventsEnabled, collectContainerMetricsEnabled int
	var highlightContainerChangesEnabled, dockerDiskUsageNotificationsEnabled int
	var dockerDiskUsageThresholdPercent, dockerDiskUsageUseGlobalDefault int
	var timezone string
	var logRetentionDays, containerPruneEnabled, containerPruneStoppedDays int
	var imagePruneDanglingOnly, autoUpdateEnabled int
	var autoUpdateWindowStart, autoUpdateWindowEnd, autoUpdateDays string
	var metricRetentionHours int
	var pendingConnectionType, pendingMigrationStartedAt, migrationRevertTo sql.NullString
	var stackCount int

	err := rows.Scan(
		&env.ID, &env.Name, &env.ConnectionType,
		&socketPath, &host, &port,
		&tlsCa, &tlsCert, &tlsKey,
		&sshHost, &sshPort, &sshUser, &sshKey,
		&isDefault, &isActive, &dockerVersion, &lastConnected,
		&agentToken, &agentStatus, &agentVersion, &agentHostname, &agentOS, &agentArch, &agentLastSeen, &agentInstallKind,
		&scheduledUpdateCheckEnabled, &automaticImagePruningEnabled,
		&trackContainerEventsEnabled, &collectContainerMetricsEnabled,
		&highlightContainerChangesEnabled, &dockerDiskUsageNotificationsEnabled,
		&dockerDiskUsageThresholdPercent, &dockerDiskUsageUseGlobalDefault, &timezone,
		&logRetentionDays, &containerPruneEnabled, &containerPruneStoppedDays,
		&imagePruneDanglingOnly,
		&autoUpdateEnabled, &autoUpdateWindowStart, &autoUpdateWindowEnd, &autoUpdateDays,
		&metricRetentionHours,
		&pendingConnectionType, &pendingMigrationStartedAt, &migrationRevertTo,
		&env.CreatedAt, &env.UpdatedAt,
		&stackCount,
	)
	if err != nil {
		return env, fmt.Errorf("scanning environment: %w", err)
	}
	env.StackCount = &stackCount

	env.PendingConnectionType = nullStringPtr(pendingConnectionType)
	env.MigrationRevertTo = nullStringPtr(migrationRevertTo)
	env.PendingMigrationStartedAt = nullStringPtr(pendingMigrationStartedAt)
	env.SocketPath = nullStringPtr(socketPath)
	env.Host = nullStringPtr(host)
	env.Port = nullIntPtr(port)
	env.TLSCa = nullStringPtr(tlsCa)
	env.TLSCert = nullStringPtr(tlsCert)
	env.TLSKey = nullStringPtr(tlsKey)
	env.SSHHost = nullStringPtr(sshHost)
	env.SSHPort = nullIntPtr(sshPort)
	env.SSHUser = nullStringPtr(sshUser)
	env.SSHKey = nullStringPtr(sshKey)
	env.DockerVersion = nullStringPtr(dockerVersion)
	env.LastConnected = nullStringPtr(lastConnected)
	env.AgentToken = nullStringPtr(agentToken)
	env.AgentStatus = nullStringPtr(agentStatus)
	env.AgentVersion = nullStringPtr(agentVersion)
	env.AgentInstallKind = nullStringPtr(agentInstallKind)
	env.AgentHostname = nullStringPtr(agentHostname)
	env.AgentOS = nullStringPtr(agentOS)
	env.AgentArch = nullStringPtr(agentArch)
	env.AgentLastSeen = nullStringPtr(agentLastSeen)
	env.IsDefault = isDefault == 1
	env.IsActive = isActive == 1
	env.ScheduledUpdateCheckEnabled = scheduledUpdateCheckEnabled == 1
	env.AutomaticImagePruningEnabled = automaticImagePruningEnabled == 1
	env.TrackContainerEventsEnabled = trackContainerEventsEnabled == 1
	env.CollectContainerMetricsEnabled = collectContainerMetricsEnabled == 1
	env.HighlightContainerChangesEnabled = highlightContainerChangesEnabled == 1
	env.DockerDiskUsageNotificationsEnabled = dockerDiskUsageNotificationsEnabled == 1
	env.DockerDiskUsageThresholdPercent = dockerDiskUsageThresholdPercent
	env.DockerDiskUsageUseGlobalDefault = dockerDiskUsageUseGlobalDefault == 1
	env.Timezone = timezone
	env.LogRetentionDays = logRetentionDays
	env.ContainerPruneEnabled = containerPruneEnabled == 1
	env.ContainerPruneStoppedDays = containerPruneStoppedDays
	env.ImagePruneDanglingOnly = imagePruneDanglingOnly == 1
	env.AutoUpdateEnabled = autoUpdateEnabled == 1
	env.AutoUpdateWindowStart = autoUpdateWindowStart
	env.AutoUpdateWindowEnd = autoUpdateWindowEnd
	env.AutoUpdateDays = autoUpdateDays
	env.MetricRetentionHours = metricRetentionHours

	return env, nil
}

// scanEnvironmentRow scans a single row into an Environment struct.
func scanEnvironmentRow(row *sql.Row) (*Environment, error) {
	var env Environment
	var socketPath, host, tlsCa, tlsCert, tlsKey sql.NullString
	var sshHost, sshUser, sshKey, dockerVersion, lastConnected sql.NullString
	var agentToken, agentStatus, agentVersion, agentHostname, agentOS, agentArch, agentLastSeen sql.NullString
	var agentInstallKind sql.NullString
	var port, sshPort sql.NullInt64
	var isDefault, isActive, scheduledUpdateCheckEnabled, automaticImagePruningEnabled int
	var trackContainerEventsEnabled, collectContainerMetricsEnabled int
	var highlightContainerChangesEnabled, dockerDiskUsageNotificationsEnabled int
	var dockerDiskUsageThresholdPercent, dockerDiskUsageUseGlobalDefault int
	var timezone string
	var logRetentionDays, containerPruneEnabled, containerPruneStoppedDays int
	var imagePruneDanglingOnly, autoUpdateEnabled int
	var autoUpdateWindowStart, autoUpdateWindowEnd, autoUpdateDays string
	var metricRetentionHours int
	var pendingConnectionType, pendingMigrationStartedAt, migrationRevertTo sql.NullString

	err := row.Scan(
		&env.ID, &env.Name, &env.ConnectionType,
		&socketPath, &host, &port,
		&tlsCa, &tlsCert, &tlsKey,
		&sshHost, &sshPort, &sshUser, &sshKey,
		&isDefault, &isActive, &dockerVersion, &lastConnected,
		&agentToken, &agentStatus, &agentVersion, &agentHostname, &agentOS, &agentArch, &agentLastSeen, &agentInstallKind,
		&scheduledUpdateCheckEnabled, &automaticImagePruningEnabled,
		&trackContainerEventsEnabled, &collectContainerMetricsEnabled,
		&highlightContainerChangesEnabled, &dockerDiskUsageNotificationsEnabled,
		&dockerDiskUsageThresholdPercent, &dockerDiskUsageUseGlobalDefault, &timezone,
		&logRetentionDays, &containerPruneEnabled, &containerPruneStoppedDays,
		&imagePruneDanglingOnly,
		&autoUpdateEnabled, &autoUpdateWindowStart, &autoUpdateWindowEnd, &autoUpdateDays,
		&metricRetentionHours,
		&pendingConnectionType, &pendingMigrationStartedAt, &migrationRevertTo,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	env.PendingConnectionType = nullStringPtr(pendingConnectionType)
	env.MigrationRevertTo = nullStringPtr(migrationRevertTo)
	env.PendingMigrationStartedAt = nullStringPtr(pendingMigrationStartedAt)
	env.SocketPath = nullStringPtr(socketPath)
	env.Host = nullStringPtr(host)
	env.Port = nullIntPtr(port)
	env.TLSCa = nullStringPtr(tlsCa)
	env.TLSCert = nullStringPtr(tlsCert)
	env.TLSKey = nullStringPtr(tlsKey)
	env.SSHHost = nullStringPtr(sshHost)
	env.SSHPort = nullIntPtr(sshPort)
	env.SSHUser = nullStringPtr(sshUser)
	env.SSHKey = nullStringPtr(sshKey)
	env.DockerVersion = nullStringPtr(dockerVersion)
	env.LastConnected = nullStringPtr(lastConnected)
	env.AgentToken = nullStringPtr(agentToken)
	env.AgentStatus = nullStringPtr(agentStatus)
	env.AgentVersion = nullStringPtr(agentVersion)
	env.AgentInstallKind = nullStringPtr(agentInstallKind)
	env.AgentHostname = nullStringPtr(agentHostname)
	env.AgentOS = nullStringPtr(agentOS)
	env.AgentArch = nullStringPtr(agentArch)
	env.AgentLastSeen = nullStringPtr(agentLastSeen)
	env.IsDefault = isDefault == 1
	env.IsActive = isActive == 1
	env.ScheduledUpdateCheckEnabled = scheduledUpdateCheckEnabled == 1
	env.AutomaticImagePruningEnabled = automaticImagePruningEnabled == 1
	env.TrackContainerEventsEnabled = trackContainerEventsEnabled == 1
	env.CollectContainerMetricsEnabled = collectContainerMetricsEnabled == 1
	env.HighlightContainerChangesEnabled = highlightContainerChangesEnabled == 1
	env.DockerDiskUsageNotificationsEnabled = dockerDiskUsageNotificationsEnabled == 1
	env.DockerDiskUsageThresholdPercent = dockerDiskUsageThresholdPercent
	env.DockerDiskUsageUseGlobalDefault = dockerDiskUsageUseGlobalDefault == 1
	env.Timezone = timezone
	env.LogRetentionDays = logRetentionDays
	env.ContainerPruneEnabled = containerPruneEnabled == 1
	env.ContainerPruneStoppedDays = containerPruneStoppedDays
	env.ImagePruneDanglingOnly = imagePruneDanglingOnly == 1
	env.AutoUpdateEnabled = autoUpdateEnabled == 1
	env.AutoUpdateWindowStart = autoUpdateWindowStart
	env.AutoUpdateWindowEnd = autoUpdateWindowEnd
	env.AutoUpdateDays = autoUpdateDays
	env.MetricRetentionHours = metricRetentionHours

	return &env, nil
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nullIntPtr(ni sql.NullInt64) *int {
	if ni.Valid {
		v := int(ni.Int64)
		return &v
	}
	return nil
}
