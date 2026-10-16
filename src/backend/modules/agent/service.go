// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

const agentDirectTransferProbeMinVersion = "1.3.7"

// Service handles agent-related business logic.
type Service struct {
	db        *sql.DB
	enc       *encryption.Service
	agentPool *coreagent.AgentPool

	// Cross-module collaborators for the offline replay path, injected by
	// main.go via SetOfflineBridges. Nil until then; see bridges.go.
	eventClassifier OfflineEventClassifier
	metricsIngester OfflineMetricsIngester
	// vault reads this module's adopted credential through the secrets vault
	// once Phase 5b's start-up backfill has linked it, and falls back to the
	// legacy column until then. Built here rather than injected: every
	// construction site already supplies the database and the encryption
	// service, and a service built without it would quietly read the legacy
	// column forever.
	vault *secrets.Service

	// dataDir locates $DATA_DIR/tls, which the install-script generator reads
	// to pin the agent to this host's certificate instead of telling it to
	// skip verification.
	dataDir string
}

// NewService creates a new agent service.
func NewService(db *sql.DB, enc *encryption.Service, agentPool *coreagent.AgentPool, dataDir string) *Service {
	svc := &Service{db: db, enc: enc, agentPool: agentPool, dataDir: dataDir}
	if db != nil && enc != nil {
		svc.vault = secrets.NewService(db, enc, nil)
	}
	return svc
}

// GenerateAgentToken creates a new agent token with "agt_" prefix.
func (s *Service) GenerateAgentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating agent token: %w", err)
	}
	return "agt_" + hex.EncodeToString(b), nil
}

// ValidateAgentToken looks up an environment by matching the decrypted agent_token.
// Returns the environment ID on success; ctx comes from the handshake request so
// a dropped agent connection cancels the vault read.
func (s *Service) ValidateAgentToken(ctx context.Context, token string) (string, error) {
	tokenHash := s.enc.StableHash(token)

	var envID string
	var encToken string
	var tokenSecretID sql.NullString
	err := s.db.QueryRowContext(ctx,
		// A socket environment mid-migration is matched too. Its row still
		// reads as `socket` on purpose — it keeps working until the agent
		// arrives — so gating on connection_type alone would refuse the very
		// handshake the migration is waiting for.
		`SELECT id, agent_token, agent_token_secret_id
		 FROM environments
		 WHERE (connection_type = 'agent' OR pending_connection_type = 'agent')
		   AND agent_token_hash = ?
		 LIMIT 1`,
		tokenHash,
	).Scan(&envID, &encToken, &tokenSecretID)
	if err == nil {
		decrypted, decryptErr := s.readAgentToken(ctx, tokenSecretID, encToken)
		if decryptErr != nil {
			return "", fmt.Errorf("decrypting agent token: %w", decryptErr)
		}
		if subtle.ConstantTimeCompare([]byte(decrypted), []byte(token)) == 1 {
			return envID, nil
		}
		return "", fmt.Errorf("invalid agent token")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("querying agent token: %w", err)
	}

	return s.validateLegacyAgentToken(ctx, token, tokenHash)
}

// PromotePendingAgentMigration completes a migration to an agent.
//
// Called once the agent has authenticated, which is the only proof that the
// environment is reachable the new way — and the moment the failsafe stops
// being able to revert it. The old endpoint is cleared here and nowhere
// earlier: it is what a stalled migration is rolled back onto, and leaving it
// behind afterwards is how a host ends up polled twice.
//
// Matches both shapes of in-flight migration: the graceful one, still on its
// old connection_type with a pending marker, and the immediate one, already on
// `agent` with a revert target recorded. A revert target on its own also
// qualifies, so a row left marked by an earlier race heals on the next
// handshake instead of showing a migration that finished long ago.
func (s *Service) PromotePendingAgentMigration(ctx context.Context, envID string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE environments
		SET connection_type = 'agent',
		    socket_path = NULL, host = NULL, port = NULL,
		    tls_ca = NULL, tls_cert = NULL, tls_key = NULL,
		    tls_ca_secret_id = NULL, tls_cert_secret_id = NULL, tls_key_secret_id = NULL,
		    ssh_host = NULL, ssh_port = NULL, ssh_user = NULL, ssh_key = NULL,
		    ssh_key_secret_id = NULL,
		    pending_connection_type = NULL, pending_migration_started_at = NULL,
		    migration_revert_to = NULL,
		    updated_at = ?
		WHERE id = ?
		  AND (pending_migration_started_at IS NOT NULL OR migration_revert_to IS NOT NULL)`,
		now, envID)
	if err != nil {
		return false, fmt.Errorf("promoting pending agent migration: %w", err)
	}
	return db.RowsAffected(res) > 0, nil
}

// UpdateAgentStatus updates the agent connection metadata in the DB.
func (s *Service) UpdateAgentStatus(ctx context.Context, envID, status string, auth *coreagent.AuthPayload) error {
	now := time.Now().UTC().Format(time.RFC3339)

	if auth != nil {
		// An older agent sends no install kind. Writing the empty string would
		// replace a known value with an unknown one on every reconnect, so it
		// is preserved instead.
		installKind := auth.InstallKind
		_, err := s.db.ExecContext(ctx, `
			UPDATE environments
			SET agent_status = ?, agent_version = ?, agent_hostname = ?,
			    agent_os = ?, agent_arch = ?, agent_last_seen = ?,
			    agent_install_kind = COALESCE(NULLIF(?, ''), agent_install_kind),
			    docker_version = ?, last_connected = ?, updated_at = ?
			WHERE id = ?`,
			status, auth.AgentVersion, auth.Hostname,
			auth.OS, auth.Arch, now, installKind,
			auth.DockerVersion, now, now,
			envID,
		)
		return err
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE environments SET agent_status = ?, agent_last_seen = ?, updated_at = ? WHERE id = ?`,
		status, now, now, envID,
	)
	return err
}

// ListAgents returns agent info for all agent-type environments.
func (s *Service) ListAgents(ctx context.Context) ([]Info, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, agent_status, agent_hostname, agent_os, agent_arch,
		       agent_version, docker_version, agent_last_seen
		FROM environments
		WHERE connection_type = 'agent'
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	var agents []Info
	for rows.Next() {
		var a Info
		var status, hostname, os, arch, version, dockerVer, lastSeen sql.NullString
		if err := rows.Scan(&a.EnvID, &a.EnvName, &status, &hostname, &os, &arch, &version, &dockerVer, &lastSeen); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}
		a.Status = "disconnected"
		if status.Valid {
			a.Status = status.String
		}
		// Override with live status from pool
		if s.agentPool.IsConnected(a.EnvID) {
			a.Status = "connected"
		}
		if hostname.Valid {
			a.Hostname = hostname.String
		}
		if os.Valid {
			a.OS = os.String
		}
		if arch.Valid {
			a.Arch = arch.String
		}
		if version.Valid {
			a.AgentVersion = version.String
		}
		if dockerVer.Valid {
			a.DockerVersion = dockerVer.String
		}
		if lastSeen.Valid {
			a.LastSeen = lastSeen.String
		}
		s.applyLiveAgentMetadata(ctx, &a)
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []Info{}
	}
	return agents, rows.Err()
}

// AgentStatus returns agent info for a single environment.
func (s *Service) AgentStatus(ctx context.Context, envID string) (*Info, error) {
	var a Info
	var status, hostname, os, arch, version, dockerVer, lastSeen sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, agent_status, agent_hostname, agent_os, agent_arch,
		       agent_version, docker_version, agent_last_seen
		FROM environments
		WHERE id = ? AND connection_type = 'agent'
	`, envID).Scan(&a.EnvID, &a.EnvName, &status, &hostname, &os, &arch, &version, &dockerVer, &lastSeen)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying agent status: %w", err)
	}

	a.Status = "disconnected"
	if status.Valid {
		a.Status = status.String
	}
	if s.agentPool.IsConnected(a.EnvID) {
		a.Status = "connected"
		if conn, ok := s.agentPool.Get(a.EnvID); ok && conn != nil {
			a.AgentBuild = conn.Build
		}
	}
	if hostname.Valid {
		a.Hostname = hostname.String
	}
	if os.Valid {
		a.OS = os.String
	}
	if arch.Valid {
		a.Arch = arch.String
	}
	if version.Valid {
		a.AgentVersion = version.String
	}
	if dockerVer.Valid {
		a.DockerVersion = dockerVer.String
	}
	if lastSeen.Valid {
		a.LastSeen = lastSeen.String
	}
	s.applyLiveAgentMetadata(ctx, &a)
	return &a, nil
}

func (s *Service) applyLiveAgentMetadata(ctx context.Context, a *Info) {
	if a == nil || s.agentPool == nil {
		return
	}
	// LastSeen is the only evidence available for "has this agent ever been up",
	// and it is read before the live branch overwrites Status, so a disconnected
	// agent that once worked reads as unhealthy while one that never connected
	// reads as unknown.
	everConnected := a.LastSeen != ""
	conn, ok := s.agentPool.Get(a.EnvID)
	if !ok {
		a.Health, a.LatencyMs = s.health(ctx, a.EnvID, everConnected)
		return
	}
	a.Status = "connected"
	a.Hostname = conn.Hostname
	a.OS = conn.OS
	a.Arch = conn.Arch
	a.AgentVersion = conn.Version
	a.DockerVersion = conn.DockerVer
	a.Health, a.LatencyMs = s.health(ctx, a.EnvID, true)
}

// RegenerateToken creates a new token for an agent environment.
// Returns the plaintext token.
func (s *Service) RegenerateToken(ctx context.Context, envID string) (string, error) {
	token, err := s.GenerateAgentToken()
	if err != nil {
		return "", err
	}

	encrypted, err := s.enc.Encrypt(token)
	if err != nil {
		return "", fmt.Errorf("encrypting agent token: %w", err)
	}
	tokenHash := s.enc.StableHash(token)

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		// agent_token_secret_id is cleared so the read-through falls straight
		// back to the token just written; the next boot re-adopts it. Leaving
		// a stale vault copy here would disconnect the agent on its next
		// reconnect, which is the failure §5b.11 is most worried about.
		`UPDATE environments
		 SET agent_token = ?, agent_token_hash = ?, agent_token_secret_id = NULL, updated_at = ?
		 WHERE id = ? AND connection_type = 'agent'`,
		encrypted, tokenHash, now, envID,
	)
	if err != nil {
		return "", fmt.Errorf("updating agent token: %w", err)
	}

	return token, nil
}

// DirectTransferTest verifies that one source agent can reach one target agent's
// direct transfer listener without loading a Docker image.
func (s *Service) DirectTransferTest(ctx context.Context, req DirectTransferTestRequest) (result DirectTransferTestResult, err error) {
	started := time.Now()
	result = DirectTransferTestResult{
		Phase:       "validate",
		SourceEnvID: strings.TrimSpace(req.SourceEnvID),
		TargetEnvID: strings.TrimSpace(req.TargetEnvID),
	}
	defer func() {
		result.DurationMs = time.Since(started).Milliseconds()
	}()

	source, err := s.AgentStatus(ctx, result.SourceEnvID)
	if err != nil {
		return result, err
	}
	target, err := s.AgentStatus(ctx, result.TargetEnvID)
	if err != nil {
		return result, err
	}
	if source != nil {
		result.SourceName = source.EnvName
		result.SourceVersion = source.AgentVersion
		result.SourceConnected = source.Status == "connected"
	}
	if target != nil {
		result.TargetName = target.EnvName
		result.TargetVersion = target.AgentVersion
		result.TargetConnected = target.Status == "connected"
	}
	if result.SourceEnvID == "" || result.TargetEnvID == "" {
		result.Error = "Select a source and target agent."
		return result, nil
	}
	if result.SourceEnvID == result.TargetEnvID {
		result.Error = "Select two different agents."
		return result, nil
	}
	if source == nil {
		result.Error = "Source agent environment was not found."
		return result, nil
	}
	if target == nil {
		result.Error = "Target agent environment was not found."
		return result, nil
	}

	sourceConn, ok := s.agentPool.Get(result.SourceEnvID)
	if !ok || sourceConn.Transport == nil {
		result.SourceConnected = false
		result.Error = "Source agent is not connected."
		return result, nil
	}
	targetConn, ok := s.agentPool.Get(result.TargetEnvID)
	if !ok || targetConn.Transport == nil {
		result.TargetConnected = false
		result.Error = "Target agent is not connected."
		return result, nil
	}
	result.SourceConnected = true
	result.TargetConnected = true
	result.SourceVersion = sourceConn.Version
	result.TargetVersion = targetConn.Version
	result.TargetTransferURL = targetConn.TransferURL

	if !agentVersionAtLeast(sourceConn.Version, agentDirectTransferProbeMinVersion) {
		result.Error = fmt.Sprintf("Source agent must be updated to mcharbor-agent %s or newer.", agentDirectTransferProbeMinVersion)
		return result, nil
	}
	if !agentVersionAtLeast(targetConn.Version, agentDirectTransferProbeMinVersion) {
		result.Error = fmt.Sprintf("Target agent must be updated to mcharbor-agent %s or newer.", agentDirectTransferProbeMinVersion)
		return result, nil
	}
	if len(targetConn.TransferCandidates) == 0 && strings.TrimSpace(targetConn.TransferURL) == "" {
		result.Error = "Target agent is connected but does not advertise a direct transfer address."
		return result, nil
	}
	result.CandidateCount = len(targetConn.TransferCandidates)

	transferID, err := randomTransferValue(16)
	if err != nil {
		return result, err
	}
	token, err := randomTransferValue(32)
	if err != nil {
		return result, err
	}
	slog.Info("agent direct transfer probe lifecycle started",
		"transferId", transferID,
		"sourceEnv", result.SourceEnvID,
		"sourceVersion", result.SourceVersion,
		"targetEnv", result.TargetEnvID,
		"targetVersion", result.TargetVersion,
		"targetTransferUrl", result.TargetTransferURL,
	)

	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer sourceConn.Transport.CancelTransfer(transferID)
	defer targetConn.Transport.CancelTransfer(transferID)

	result.Phase = "prepare"
	probeURL, receiver, err := targetConn.Transport.PrepareProbe(testCtx, transferID, token)
	if err != nil {
		slog.Warn("agent direct transfer probe prepare failed",
			"transferId", transferID,
			"sourceEnv", result.SourceEnvID,
			"targetEnv", result.TargetEnvID,
			"targetTransferUrl", result.TargetTransferURL,
			"error", err,
		)
		return result, nil
	}
	result.ProbeURL = probeURL
	if receiver != nil {
		result.Receiver = &DirectTransferReceiver{
			TransferID:       receiver.TransferID,
			Kind:             receiver.Kind,
			ExpiresAt:        receiver.ExpiresAt,
			TokenFingerprint: receiver.TokenFingerprint,
			AgentMarker:      receiver.AgentMarker,
		}
	}
	slog.Info("agent direct transfer probe receiver prepared",
		"transferId", transferID,
		"sourceEnv", result.SourceEnvID,
		"sourceVersion", result.SourceVersion,
		"targetEnv", result.TargetEnvID,
		"targetVersion", result.TargetVersion,
		"targetTransferUrl", result.TargetTransferURL,
		"probeUrl", result.ProbeURL,
		"receiverAgentMarker", receiverAgentMarker(receiver),
		"receiverTokenFingerprint", receiverTokenFingerprint(receiver),
	)

	// One receiver answers on every address the target host is reachable at,
	// because it is keyed by transfer ID and bearer token rather than by
	// address. So the candidates are probed against the single receiver
	// prepared above, and the first that answers wins.
	result.Phase = "probe"
	candidates := coreagent.TransferCandidatesFor(sourceConn, targetConn, probeURL, result.SourceEnvID)
	var statusCode int
	var responderMarker string
	for i, candidateURL := range candidates {
		result.CandidatesTried = i + 1
		result.ProbeURL = candidateURL
		statusCode, responderMarker, err = sourceConn.Transport.StartTransferProbe(testCtx, transferID, candidateURL, token)
		if err == nil {
			targetConn.RememberTransferRoute(result.SourceEnvID, candidateURL)
			break
		}
		slog.Debug("agent direct transfer candidate failed",
			"transferId", transferID,
			"sourceEnv", result.SourceEnvID,
			"targetEnv", result.TargetEnvID,
			"candidate", candidateURL,
			"candidateIndex", i+1,
			"candidateCount", len(candidates),
			"statusCode", statusCode,
			"error", err,
		)
		if testCtx.Err() != nil {
			break
		}
	}
	if err != nil {
		targetConn.ForgetTransferRoute(result.SourceEnvID)
	}
	result.StatusCode = statusCode
	result.ResponderMarker = responderMarker
	if err != nil {
		if diagnostic := targetConn.Transport.WaitTransferDiagnostic(testCtx, transferID, 2*time.Second); diagnostic != nil {
			result.Diagnostic = &DirectTransferDiagnostic{
				ReceiverExists:  diagnostic.ReceiverExists,
				ReceiverExpired: diagnostic.ReceiverExpired,
				ReceiverKind:    diagnostic.ReceiverKind,
				KindMatched:     diagnostic.KindMatched,
				BearerPresent:   diagnostic.BearerPresent,
				TokenMatched:    diagnostic.TokenMatched,
				RemoteAddr:      diagnostic.RemoteAddr,
				ResponderMarker: diagnostic.ResponderAgentMarker,
			}
		}
		slog.Warn("agent direct transfer probe failed",
			"transferId", transferID,
			"sourceEnv", result.SourceEnvID,
			"sourceVersion", result.SourceVersion,
			"targetEnv", result.TargetEnvID,
			"targetVersion", result.TargetVersion,
			"targetTransferUrl", result.TargetTransferURL,
			"probeUrl", result.ProbeURL,
			"statusCode", result.StatusCode,
			"receiverAgentMarker", receiverAgentMarker(receiver),
			"responderMarker", result.ResponderMarker,
			"targetReportedDiagnostics", result.Diagnostic != nil,
			"error", err,
		)
		return result, nil
	}

	result.Phase = "complete"
	result.Success = true
	slog.Info("agent direct transfer probe completed",
		"transferId", transferID,
		"sourceEnv", result.SourceEnvID,
		"sourceVersion", result.SourceVersion,
		"targetEnv", result.TargetEnvID,
		"targetVersion", result.TargetVersion,
		"targetTransferUrl", result.TargetTransferURL,
		"probeUrl", result.ProbeURL,
		"statusCode", result.StatusCode,
		"receiverAgentMarker", receiverAgentMarker(receiver),
		"responderMarker", result.ResponderMarker,
	)
	return result, nil
}

func receiverAgentMarker(receiver *coreagent.TransferReceiverMarker) string {
	if receiver == nil {
		return ""
	}
	return receiver.AgentMarker
}

func receiverTokenFingerprint(receiver *coreagent.TransferReceiverMarker) string {
	if receiver == nil {
		return ""
	}
	return receiver.TokenFingerprint
}

func randomTransferValue(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating transfer token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func agentVersionAtLeast(version, minimum string) bool {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	minimum = strings.TrimPrefix(strings.TrimSpace(minimum), "v")
	versionParts := strings.Split(version, ".")
	minimumParts := strings.Split(minimum, ".")
	for i := 0; i < 3; i++ {
		versionPart := agentVersionPart(versionParts, i)
		minimumPart := agentVersionPart(minimumParts, i)
		if versionPart > minimumPart {
			return true
		}
		if versionPart < minimumPart {
			return false
		}
	}
	return true
}

func agentVersionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(parts[index])
	if err != nil {
		return 0
	}
	return value
}

func (s *Service) validateLegacyAgentToken(ctx context.Context, token, tokenHash string) (string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_token
		 FROM environments
		 WHERE connection_type = 'agent' AND agent_token IS NOT NULL AND (agent_token_hash IS NULL OR agent_token_hash = '')
		 LIMIT 1000`,
	)
	if err != nil {
		return "", fmt.Errorf("querying legacy agent environments: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	// bounded by the first matching legacy row: the loop returns after one backfill write
	for rows.Next() {
		var envID, encToken string
		if err := rows.Scan(&envID, &encToken); err != nil {
			continue
		}
		decrypted, err := s.enc.Decrypt(encToken)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(decrypted), []byte(token)) != 1 {
			continue
		}
		if _, updateErr := s.db.ExecContext(ctx,
			"UPDATE environments SET agent_token_hash = ? WHERE id = ?",
			tokenHash,
			envID,
		); updateErr != nil {
			return "", fmt.Errorf("backfilling agent token hash: %w", updateErr)
		}
		return envID, nil
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating legacy agent environments: %w", err)
	}

	return "", fmt.Errorf("invalid agent token")
}

// readAgentToken reads an environment's enrolment token through the vault
// when Phase 5b has adopted it, and falls back to the legacy agent_token
// column when it has not.
//
// The fallback matters more here than anywhere else in the catalogue: this
// token is what every remote agent presents to reconnect, and a vault read
// that failed without a fallback would disconnect the whole estate. Note also
// that `agent_token_hash` — which is what actually authenticates a connecting
// agent — is untouched by adoption; the ciphertext is only ever read back to
// show or redeploy a token.
func (s *Service) readAgentToken(ctx context.Context, secretID sql.NullString, legacy string) (string, error) {
	if s.vault == nil {
		return s.enc.Decrypt(legacy)
	}
	return s.vault.ReadThrough(ctx, secretID, legacy)
}
