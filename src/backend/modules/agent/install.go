// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// InstallToken represents a one-time install token stored in the DB.
type InstallToken struct {
	ID        string `json:"id"`
	EnvID     string `json:"envId"`
	Token     string `json:"token,omitempty"` // Only populated on creation
	ExpiresAt string `json:"expiresAt"`
	Used      bool   `json:"used"`
	CreatedAt string `json:"createdAt"`
}

// CreateInstallTokenResponse is returned when a new install token is created.
type CreateInstallTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
	Script    string `json:"script"`
}

// CreateInstallToken generates a one-time install token for an agent environment.
// The token expires after 24 hours and can only be used once.
func (s *Service) CreateInstallToken(envID, serverURL string) (*CreateInstallTokenResponse, error) {
	// Verify this is an agent environment
	var connType string
	err := s.db.QueryRow("SELECT connection_type FROM environments WHERE id = ?", envID).Scan(&connType)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying environment: %w", err)
	}
	if connType != "agent" {
		return nil, fmt.Errorf("environment is not an agent connection type")
	}

	// Generate random token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generating install token: %w", err)
	}
	token := "inst_" + hex.EncodeToString(b)

	// Hash the token for storage (we don't need to decrypt, just verify)
	hash := hashToken(token)

	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)
	id := xid.New().String()

	_, err = s.db.Exec(`
		INSERT INTO install_tokens (id, env_id, token_hash, expires_at, used, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, envID, hash, expiresAt.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("storing install token: %w", err)
	}

	script := fmt.Sprintf("curl -sSL %s/api/agent/install/%s | bash", serverURL, token)

	return &CreateInstallTokenResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		Script:    script,
	}, nil
}

// ValidateInstallToken checks if an install token is valid (not used, not expired).
// Returns the environment ID on success, and marks the token as used.
func (s *Service) ValidateInstallToken(token string) (string, error) {
	hash := hashToken(token)

	var id, envID, expiresAt string
	var used int
	err := s.db.QueryRow(`
		SELECT id, env_id, expires_at, used
		FROM install_tokens
		WHERE token_hash = ?
	`, hash).Scan(&id, &envID, &expiresAt, &used)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("invalid install token")
	}
	if err != nil {
		return "", fmt.Errorf("querying install token: %w", err)
	}

	if used == 1 {
		return "", fmt.Errorf("install token already used")
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "", fmt.Errorf("parsing expiry: %w", err)
	}
	if time.Now().UTC().After(expires) {
		return "", fmt.Errorf("install token expired")
	}

	// Mark as used
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec("UPDATE install_tokens SET used = 1, updated_at = ? WHERE id = ?", now, id)
	if err != nil {
		return "", fmt.Errorf("marking token used: %w", err)
	}

	return envID, nil
}

// InstallScript generates a shell script that installs and configures the agent.
// The script auto-detects OS/arch and installs via Docker or binary.
func (s *Service) InstallScript(envID, serverURL string) (string, error) {
	// Get agent token
	agentToken, err := s.agentTokenPlaintext(envID)
	if err != nil {
		return "", fmt.Errorf("getting agent token: %w", err)
	}

	script := fmt.Sprintf(`#!/bin/bash
# McHarbor Agent Install Script
# This script installs the McHarbor agent on the current machine.
# Generated automatically — do not edit.

set -e

MCHARBOR_URL=%s
MCHARBOR_AGENT_TOKEN=%s
AGENT_IMAGE="ghcr.io/therealmcsparrow/mcharbor-agent:latest"

echo "=== McHarbor Agent Installer ==="
echo ""

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l) ARCH="arm" ;;
esac

echo "Detected: $OS/$ARCH"

# Check if Docker is available
if command -v docker &>/dev/null && docker info &>/dev/null 2>&1; then
  echo "Docker detected — installing as container..."
  docker rm -f mcharbor-agent 2>/dev/null || true
  docker run -d \
    --name mcharbor-agent \
    --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -e MCHARBOR_URL="$MCHARBOR_URL" \
    -e MCHARBOR_AGENT_TOKEN="$MCHARBOR_AGENT_TOKEN" \
    "$AGENT_IMAGE"
  echo ""
  echo "Agent container started successfully."
  echo "Run 'docker logs mcharbor-agent' to check status."
else
  echo "Docker not found — installing as binary..."
  DOWNLOAD_URL="https://github.com/therealmcsparrow/mcharbor-agent/releases/latest/download/mcharbor-agent-${OS}-${ARCH}"

  if command -v curl &>/dev/null; then
    curl -fsSL -o /usr/local/bin/mcharbor-agent "$DOWNLOAD_URL"
  elif command -v wget &>/dev/null; then
    wget -qO /usr/local/bin/mcharbor-agent "$DOWNLOAD_URL"
  else
    echo "Error: neither curl nor wget found"
    exit 1
  fi

  chmod +x /usr/local/bin/mcharbor-agent

  # Create systemd service if available
  if command -v systemctl &>/dev/null; then
    cat > /etc/systemd/system/mcharbor-agent.service << 'SERVICEEOF'
[Unit]
Description=McHarbor Remote Agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mcharbor-agent
Environment=MCHARBOR_URL=${MCHARBOR_URL}
Environment=MCHARBOR_AGENT_TOKEN=${MCHARBOR_AGENT_TOKEN}
Environment=DOCKER_HOST=unix:///var/run/docker.sock
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICEEOF

    # Fix environment variable expansion in service file
    sed -i "s|\${MCHARBOR_URL}|$MCHARBOR_URL|g" /etc/systemd/system/mcharbor-agent.service
    sed -i "s|\${MCHARBOR_AGENT_TOKEN}|$MCHARBOR_AGENT_TOKEN|g" /etc/systemd/system/mcharbor-agent.service

    systemctl daemon-reload
    systemctl enable mcharbor-agent
    systemctl restart mcharbor-agent
    echo ""
    echo "Agent binary installed and systemd service started."
    echo "Run 'systemctl status mcharbor-agent' to check status."
  else
    # No systemd — run directly in background
    nohup /usr/local/bin/mcharbor-agent > /var/log/mcharbor-agent.log 2>&1 &
    echo ""
    echo "Agent binary installed and started in background."
    echo "Check /var/log/mcharbor-agent.log for output."
  fi
fi

echo ""
echo "Done! The agent will connect to $MCHARBOR_URL automatically."
`, shellEscape(serverURL), shellEscape(agentToken))

	return script, nil
}

// hashToken creates a SHA-256 hash of a token for storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
