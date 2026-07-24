// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
)

// Service handles environment CRUD operations against the database.
type Service struct {
	db         *sql.DB
	dockerPool *docker.ClientPool
	k8sPool    *kubernetes.ClientPool
	agentPool  *coreagent.AgentPool
	enc        *encryption.Service
}

// NewService creates a new environment service.
func NewService(db *sql.DB, dockerPool *docker.ClientPool, k8sPool *kubernetes.ClientPool, agentPool *coreagent.AgentPool, enc *encryption.Service) *Service {
	return &Service{db: db, dockerPool: dockerPool, k8sPool: k8sPool, agentPool: agentPool, enc: enc}
}

// List returns all environments from the database.
func (s *Service) List() ([]Environment, error) {
	rows, err := s.db.Query(`
		SELECT id, name, orchestrator_type, connection_type, socket_path, host, port,
		       tls_ca, tls_cert, tls_key, ssh_host, ssh_port, ssh_user, ssh_key,
		       is_default, is_active, docker_version, last_connected,
		       kubeconfig, k8s_namespace, k8s_server_url, k8s_bearer_token, k8s_ca_cert, k8s_version,
		       agent_token, agent_status, agent_version, agent_hostname, agent_os, agent_arch, agent_last_seen,
		       scheduled_update_check_enabled, automatic_image_pruning_enabled,
		       track_container_events_enabled, collect_container_metrics_enabled,
		       highlight_container_changes_enabled, docker_disk_usage_notifications_enabled,
		       docker_disk_usage_threshold_percent, docker_disk_usage_use_global_default, timezone,
		       log_retention_days, container_prune_enabled, container_prune_stopped_days,
		       image_prune_dangling_only,
		       auto_update_enabled, auto_update_window_start, auto_update_window_end, auto_update_days,
		       metric_retention_hours,
		       created_at, updated_at
		FROM environments
		ORDER BY is_default DESC, name ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying environments: %w", err)
	}
	defer rows.Close()

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
func (s *Service) ByID(id string) (*Environment, error) {
	row := s.db.QueryRow(`
		SELECT id, name, orchestrator_type, connection_type, socket_path, host, port,
		       tls_ca, tls_cert, tls_key, ssh_host, ssh_port, ssh_user, ssh_key,
		       is_default, is_active, docker_version, last_connected,
		       kubeconfig, k8s_namespace, k8s_server_url, k8s_bearer_token, k8s_ca_cert, k8s_version,
		       agent_token, agent_status, agent_version, agent_hostname, agent_os, agent_arch, agent_last_seen,
		       scheduled_update_check_enabled, automatic_image_pruning_enabled,
		       track_container_events_enabled, collect_container_metrics_enabled,
		       highlight_container_changes_enabled, docker_disk_usage_notifications_enabled,
		       docker_disk_usage_threshold_percent, docker_disk_usage_use_global_default, timezone,
		       log_retention_days, container_prune_enabled, container_prune_stopped_days,
		       image_prune_dangling_only,
		       auto_update_enabled, auto_update_window_start, auto_update_window_end, auto_update_days,
		       metric_retention_hours,
		       created_at, updated_at
		FROM environments WHERE id = ?
	`, id)

	env, err := scanEnvironmentRow(row)
	if err == sql.ErrNoRows {
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
}

// Create inserts a new environment into the database.
// Returns the environment and, for agent connections, the plaintext token (shown once).
func (s *Service) Create(req CreateRequest) (*Environment, string, error) {
	if req.Name == "" {
		return nil, "", fmt.Errorf("name is required")
	}
	if req.OrchestratorType == "" {
		req.OrchestratorType = "docker"
	}
	if req.ConnectionType == "" && req.OrchestratorType == "docker" {
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

	// Encrypt sensitive Kubernetes fields
	kubeconfig, err := s.encryptOptional(req.Kubeconfig)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting kubeconfig: %w", err)
	}
	k8sBearerToken, err := s.encryptOptional(req.K8sBearerToken)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting k8s_bearer_token: %w", err)
	}
	k8sCACert, err := s.encryptOptional(req.K8sCACert)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting k8s_ca_cert: %w", err)
	}

	// Generate agent token if agent connection type
	var agentToken *string
	var agentTokenHash *string
	var plainToken string
	if req.ConnectionType == "agent" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, "", fmt.Errorf("generating agent token: %w", err)
		}
		plainToken = "agt_" + hex.EncodeToString(b)
		encrypted, err := s.enc.Encrypt(plainToken)
		if err != nil {
			return nil, "", fmt.Errorf("encrypting agent token: %w", err)
		}
		agentToken = &encrypted
		hash := s.enc.StableHash(plainToken)
		agentTokenHash = &hash
	}

	// If setting as default, clear other defaults first
	if req.IsDefault {
		if _, err := s.db.Exec("UPDATE environments SET is_default = 0"); err != nil {
			return nil, "", fmt.Errorf("clearing default environments: %w", err)
		}
	}

	isDefault := 0
	if req.IsDefault {
		isDefault = 1
	}

	_, err = s.db.Exec(`
		INSERT INTO environments (id, name, orchestrator_type, connection_type, socket_path, host, port,
		                          tls_ca, tls_cert, tls_key, ssh_host, ssh_port, ssh_user, ssh_key,
		                          is_default, is_active,
		                          kubeconfig, k8s_namespace, k8s_server_url, k8s_bearer_token, k8s_ca_cert,
		                          agent_token, agent_token_hash,
		                          created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.Name, req.OrchestratorType, req.ConnectionType, req.SocketPath, req.Host, req.Port,
		tlsCa, tlsCert, tlsKey, req.SSHHost, req.SSHPort, req.SSHUser, sshKey,
		isDefault,
		kubeconfig, req.K8sNamespace, req.K8sServerURL, k8sBearerToken, k8sCACert,
		agentToken, agentTokenHash,
		now, now)
	if err != nil {
		return nil, "", fmt.Errorf("inserting environment: %w", err)
	}

	env, err := s.ByID(id)
	return env, plainToken, err
}

// Update modifies an existing environment.
func (s *Service) Update(id string, req UpdateRequest) (*Environment, error) {
	existing, err := s.ByID(id)
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
	orchestratorType := existing.OrchestratorType
	if req.OrchestratorType != nil {
		orchestratorType = *req.OrchestratorType
	}
	connType := existing.ConnectionType
	if req.ConnectionType != nil {
		connType = *req.ConnectionType
	}
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

	// Encrypt K8s secrets if provided
	var kubeconfig, k8sBearerToken, k8sCACert *string
	if req.Kubeconfig != nil {
		v, encErr := s.encryptOptional(req.Kubeconfig)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting kubeconfig: %w", encErr)
		}
		kubeconfig = v
	}
	if req.K8sBearerToken != nil {
		v, encErr := s.encryptOptional(req.K8sBearerToken)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting k8s_bearer_token: %w", encErr)
		}
		k8sBearerToken = v
	}
	if req.K8sCACert != nil {
		v, encErr := s.encryptOptional(req.K8sCACert)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting k8s_ca_cert: %w", encErr)
		}
		k8sCACert = v
	}

	k8sNamespace := existing.K8sNamespace
	if req.K8sNamespace != nil {
		k8sNamespace = req.K8sNamespace
	}
	k8sServerURL := existing.K8sServerURL
	if req.K8sServerURL != nil {
		k8sServerURL = req.K8sServerURL
	}

	isDefault := existing.IsDefault
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
		if isDefault {
			if _, err := s.db.Exec("UPDATE environments SET is_default = 0"); err != nil {
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

	_, err = s.db.Exec(`
		UPDATE environments
		SET name = ?, orchestrator_type = ?, connection_type = ?, socket_path = ?, host = ?, port = ?,
		    tls_ca = COALESCE(?, tls_ca), tls_cert = COALESCE(?, tls_cert),
		    tls_key = COALESCE(?, tls_key),
		    ssh_host = ?, ssh_port = ?, ssh_user = ?,
		    ssh_key = COALESCE(?, ssh_key),
		    is_default = ?, is_active = ?,
		    kubeconfig = COALESCE(?, kubeconfig),
		    k8s_namespace = ?, k8s_server_url = ?,
		    k8s_bearer_token = COALESCE(?, k8s_bearer_token),
		    k8s_ca_cert = COALESCE(?, k8s_ca_cert),
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
	`, name, orchestratorType, connType, socketPath, host, port,
		tlsCa, tlsCert, tlsKey,
		sshHost, sshPort, sshUser, sshKey,
		defaultInt, activeInt,
		kubeconfig, k8sNamespace, k8sServerURL, k8sBearerToken, k8sCACert,
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

	// Invalidate cached clients for this environment
	s.dockerPool.Remove(id)
	s.k8sPool.Remove(id)

	return s.ByID(id)
}

// Delete removes an environment from the database.
func (s *Service) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM environments WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting environment: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("environment not found")
	}

	// Remove cached clients
	s.dockerPool.Remove(id)
	s.k8sPool.Remove(id)

	return nil
}

// TestConnection pings the Docker daemon or Kubernetes cluster for the
// given environment and returns a per-step report suitable for the
// frontend dialog. The report lists each connection phase (pool reset,
// client acquisition, daemon ping, version parse, DB persist) with
// its own pass/fail status and latency, plus an overall rolled-up
// status and the resolved Docker / Kubernetes version when the test
// succeeds.
func (s *Service) TestConnection(ctx context.Context, id string) *EnvTestResult {
	// Look up environment basics so the dialog can show the name.
	env, err := s.ByID(id)
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

	var result *EnvTestResult
	if env.OrchestratorType == "kubernetes" {
		result = s.testK8sConnection(ctx, id, now)
	} else {
		result = s.testDockerConnection(ctx, id, now)
	}
	result.EnvID = id
	result.EnvName = env.Name
	result.Orchestrator = env.OrchestratorType
	result.StartedAt = now
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	result.Duration = result.CompletedAt
	started, _ := time.Parse(time.RFC3339, now)
	if !started.IsZero() {
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
	cli, err := s.dockerPool.Get(id)
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
	ping, err := cli.Ping(ctx)
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
	if info, err := cli.Info(ctx); err == nil && info.ServerVersion != "" {
		version = info.ServerVersion
		appendStep(&result.Steps, "version", "pass", "server version: "+info.ServerVersion, started)
	} else {
		appendStep(&result.Steps, "version", "warn",
			"API version only: "+ping.APIVersion+"; /info unavailable", started)
	}

	// Step 5: persist the resolved version so the rest of the app
	// (overview metrics, etc.) reads it without re-pinging.
	started = time.Now()
	if _, err := s.db.Exec(
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

func (s *Service) testK8sConnection(ctx context.Context, id string, now string) *EnvTestResult {
	result := &EnvTestResult{}

	started := time.Now()
	s.k8sPool.Remove(id)
	appendStep(&result.Steps, "reset", "pass", "cleared cached client", started)

	started = time.Now()
	version, err := s.k8sPool.Ping(ctx, id)
	if err != nil {
		appendStep(&result.Steps, "ping", "fail", "Kubernetes ping failed", started)
		slog.Error("environments: failed to connect to Kubernetes", "error", err, "id", id)
		result.Overall = "fail"
		return result
	}
	appendStep(&result.Steps, "ping", "pass", "cluster responded to ping", started)

	started = time.Now()
	if _, err := s.db.Exec(
		"UPDATE environments SET k8s_version = ?, last_connected = ?, updated_at = ? WHERE id = ?",
		version, now, now, id,
	); err != nil {
		appendStep(&result.Steps, "persist", "fail", "database update failed", started)
		slog.Error("environments: failed to update k8s version", "error", err, "id", id)
		result.Overall = rollupStatus(result.Steps)
		return result
	}
	appendStep(&result.Steps, "persist", "pass", "stored k8s_version + last_connected", started)

	result.K8sVersion = &version
	result.Overall = rollupStatus(result.Steps)
	return result
}

// DetectSocket auto-detects Docker and Podman sockets on the host.
func (s *Service) DetectSocket() []DetectedSocket {
	var sockets []DetectedSocket

	if p := docker.DetectDockerSocket(); p != "" {
		sockets = append(sockets, DetectedSocket{Path: p, Runtime: "docker"})
	}
	if p := docker.DetectPodmanSocket(); p != "" {
		sockets = append(sockets, DetectedSocket{Path: p, Runtime: "podman"})
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
	if env.Kubeconfig != nil && *env.Kubeconfig != "" {
		env.Kubeconfig = &redacted
	}
	if env.K8sBearerToken != nil && *env.K8sBearerToken != "" {
		env.K8sBearerToken = &redacted
	}
	if env.K8sCACert != nil && *env.K8sCACert != "" {
		env.K8sCACert = &redacted
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
	var kubeconfig, k8sNamespace, k8sServerURL, k8sBearerToken, k8sCACert, k8sVersion sql.NullString
	var agentToken, agentStatus, agentVersion, agentHostname, agentOS, agentArch, agentLastSeen sql.NullString
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

	err := rows.Scan(
		&env.ID, &env.Name, &env.OrchestratorType, &env.ConnectionType,
		&socketPath, &host, &port,
		&tlsCa, &tlsCert, &tlsKey,
		&sshHost, &sshPort, &sshUser, &sshKey,
		&isDefault, &isActive, &dockerVersion, &lastConnected,
		&kubeconfig, &k8sNamespace, &k8sServerURL, &k8sBearerToken, &k8sCACert, &k8sVersion,
		&agentToken, &agentStatus, &agentVersion, &agentHostname, &agentOS, &agentArch, &agentLastSeen,
		&scheduledUpdateCheckEnabled, &automaticImagePruningEnabled,
		&trackContainerEventsEnabled, &collectContainerMetricsEnabled,
		&highlightContainerChangesEnabled, &dockerDiskUsageNotificationsEnabled,
		&dockerDiskUsageThresholdPercent, &dockerDiskUsageUseGlobalDefault, &timezone,
		&logRetentionDays, &containerPruneEnabled, &containerPruneStoppedDays,
		&imagePruneDanglingOnly,
		&autoUpdateEnabled, &autoUpdateWindowStart, &autoUpdateWindowEnd, &autoUpdateDays,
		&metricRetentionHours,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err != nil {
		return env, fmt.Errorf("scanning environment: %w", err)
	}

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
	env.Kubeconfig = nullStringPtr(kubeconfig)
	env.K8sNamespace = nullStringPtr(k8sNamespace)
	env.K8sServerURL = nullStringPtr(k8sServerURL)
	env.K8sBearerToken = nullStringPtr(k8sBearerToken)
	env.K8sCACert = nullStringPtr(k8sCACert)
	env.K8sVersion = nullStringPtr(k8sVersion)
	env.AgentToken = nullStringPtr(agentToken)
	env.AgentStatus = nullStringPtr(agentStatus)
	env.AgentVersion = nullStringPtr(agentVersion)
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
	var kubeconfig, k8sNamespace, k8sServerURL, k8sBearerToken, k8sCACert, k8sVersion sql.NullString
	var agentToken, agentStatus, agentVersion, agentHostname, agentOS, agentArch, agentLastSeen sql.NullString
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

	err := row.Scan(
		&env.ID, &env.Name, &env.OrchestratorType, &env.ConnectionType,
		&socketPath, &host, &port,
		&tlsCa, &tlsCert, &tlsKey,
		&sshHost, &sshPort, &sshUser, &sshKey,
		&isDefault, &isActive, &dockerVersion, &lastConnected,
		&kubeconfig, &k8sNamespace, &k8sServerURL, &k8sBearerToken, &k8sCACert, &k8sVersion,
		&agentToken, &agentStatus, &agentVersion, &agentHostname, &agentOS, &agentArch, &agentLastSeen,
		&scheduledUpdateCheckEnabled, &automaticImagePruningEnabled,
		&trackContainerEventsEnabled, &collectContainerMetricsEnabled,
		&highlightContainerChangesEnabled, &dockerDiskUsageNotificationsEnabled,
		&dockerDiskUsageThresholdPercent, &dockerDiskUsageUseGlobalDefault, &timezone,
		&logRetentionDays, &containerPruneEnabled, &containerPruneStoppedDays,
		&imagePruneDanglingOnly,
		&autoUpdateEnabled, &autoUpdateWindowStart, &autoUpdateWindowEnd, &autoUpdateDays,
		&metricRetentionHours,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

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
	env.Kubeconfig = nullStringPtr(kubeconfig)
	env.K8sNamespace = nullStringPtr(k8sNamespace)
	env.K8sServerURL = nullStringPtr(k8sServerURL)
	env.K8sBearerToken = nullStringPtr(k8sBearerToken)
	env.K8sCACert = nullStringPtr(k8sCACert)
	env.K8sVersion = nullStringPtr(k8sVersion)
	env.AgentToken = nullStringPtr(agentToken)
	env.AgentStatus = nullStringPtr(agentStatus)
	env.AgentVersion = nullStringPtr(agentVersion)
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
