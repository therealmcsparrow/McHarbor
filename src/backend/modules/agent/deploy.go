// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"golang.org/x/crypto/ssh"
)

// DeployMethod specifies how to deploy the agent on the remote host.
type DeployMethod string

const (
	DeployDocker DeployMethod = "docker"
	DeployBinary DeployMethod = "binary"
)

// DeployRequest is the JSON body for POST /agents/{envId}/deploy.
type DeployRequest struct {
	SSHHost            string       `json:"sshHost"`
	SSHPort            int          `json:"sshPort"`
	SSHUser            string       `json:"sshUser"`
	SSHAuthType        string       `json:"sshAuthType"` // "key" or "password"
	SSHKey             string       `json:"sshKey,omitempty"`
	SSHPassword        string       `json:"sshPassword,omitempty"`
	HostKeyFingerprint string       `json:"hostKeyFingerprint"`
	Method             DeployMethod `json:"method"`
	AgentImage         string       `json:"agentImage,omitempty"`
}

// DeployResult contains the outcome of a deployment attempt.
type DeployResult struct {
	Success bool         `json:"success"`
	Output  string       `json:"output,omitempty"`
	Error   string       `json:"error,omitempty"`
	Code    i18n.MsgCode `json:"code,omitempty"`
	OS      string       `json:"os,omitempty"`
	Arch    string       `json:"arch,omitempty"`
}

// DeployViaSSH connects to a remote host over SSH and deploys the agent.
func (s *Service) DeployViaSSH(ctx context.Context, envID string, req DeployRequest, serverURL string, logger *slog.Logger) (*DeployResult, error) {
	// Get the agent token for this environment
	token, err := s.agentTokenPlaintext(envID)
	if err != nil {
		return nil, fmt.Errorf("retrieving agent token: %w", err)
	}

	// Build SSH auth methods
	var authMethods []ssh.AuthMethod
	if req.SSHAuthType == "password" {
		if req.SSHPassword == "" {
			return &DeployResult{Success: false, Code: i18n.ErrAgentDeploySSHRequired}, nil
		}
		// Offer both password and keyboard-interactive — most Linux servers
		// use keyboard-interactive (PAM) even for password prompts.
		authMethods = []ssh.AuthMethod{
			ssh.Password(req.SSHPassword),
			ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = req.SSHPassword
				}
				return answers, nil
			}),
		}
	} else {
		if req.SSHKey == "" {
			return &DeployResult{Success: false, Code: i18n.ErrAgentDeploySSHRequired}, nil
		}
		signer, err := ssh.ParsePrivateKey([]byte(req.SSHKey))
		if err != nil {
			return &DeployResult{Success: false, Code: i18n.ErrAgentDeploySSHKeyInvalid}, nil
		}
		authMethods = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	}

	port := req.SSHPort
	if port == 0 {
		port = 22
	}

	expectedFingerprint := normalizeHostKeyFingerprint(req.HostKeyFingerprint)
	if expectedFingerprint == "" {
		return &DeployResult{Success: false, Code: i18n.ErrAgentDeployHostKeyRequired}, nil
	}

	var hostKeyMismatch bool
	var actualFingerprint string
	sshConfig := &ssh.ClientConfig{
		User: req.SSHUser,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			actualFingerprint = ssh.FingerprintSHA256(key)
			if subtle.ConstantTimeCompare([]byte(actualFingerprint), []byte(expectedFingerprint)) == 1 {
				return nil
			}
			hostKeyMismatch = true
			return fmt.Errorf("ssh host key verification failed")
		},
		Timeout:         15 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", req.SSHHost, port)
	logger.Info("agent deploy: connecting via SSH", "addr", addr, "user", req.SSHUser, "env", envID)

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		if hostKeyMismatch {
			logger.Warn("agent deploy: SSH host key mismatch",
				"env", envID,
				"host", req.SSHHost,
				"expectedFingerprint", expectedFingerprint,
				"actualFingerprint", actualFingerprint,
			)
			return &DeployResult{Success: false, Code: i18n.ErrAgentDeployHostKeyMismatch}, nil
		}

		logger.Warn("agent deploy: SSH connection failed", "env", envID, "host", req.SSHHost, "error", err)
		return &DeployResult{Success: false, Code: i18n.ErrAgentDeploySSHConnectFailed}, nil
	}
	defer client.Close()

	// Detect OS and architecture
	osName, arch, err := detectRemoteOS(client)
	if err != nil {
		logger.Warn("agent deploy: failed to detect remote OS", "env", envID, "host", req.SSHHost, "error", err)
		return &DeployResult{Success: false, Code: i18n.ErrAgentDeployOSDetectFailed}, nil
	}

	logger.Info("agent deploy: detected remote OS", "os", osName, "arch", arch, "env", envID)

	var deployCmd string
	switch req.Method {
	case DeployDocker:
		deployCmd = buildDockerDeployCmd(serverURL, token, req.AgentImage)
	case DeployBinary:
		deployCmd = buildBinaryDeployCmd(serverURL, token, osName, arch)
	default:
		return &DeployResult{Success: false, Code: i18n.ErrAgentDeployInvalidMethod}, nil
	}

	output, err := runSSHCommand(client, deployCmd)
	if err != nil {
		logger.Error("agent deploy: command failed", "error", err, "output", output, "env", envID)
		return &DeployResult{
			Success: false,
			Output:  output,
			Code:    i18n.ErrAgentDeployCommandFailed,
			OS:      osName,
			Arch:    arch,
		}, nil
	}

	logger.Info("agent deploy: success", "env", envID, "method", req.Method, "os", osName, "arch", arch)
	return &DeployResult{
		Success: true,
		Output:  output,
		OS:      osName,
		Arch:    arch,
	}, nil
}

func normalizeHostKeyFingerprint(fingerprint string) string {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return ""
	}
	if len(fingerprint) >= 7 && strings.EqualFold(fingerprint[:7], "SHA256:") {
		return "SHA256:" + fingerprint[7:]
	}
	return "SHA256:" + fingerprint
}

// agentTokenPlaintext retrieves and decrypts the agent token for an environment.
func (s *Service) agentTokenPlaintext(envID string) (string, error) {
	var encToken string
	err := s.db.QueryRow(
		"SELECT agent_token FROM environments WHERE id = ? AND connection_type = 'agent'",
		envID,
	).Scan(&encToken)
	if err != nil {
		return "", fmt.Errorf("querying agent token: %w", err)
	}

	token, err := s.enc.Decrypt(encToken)
	if err != nil {
		return "", fmt.Errorf("decrypting agent token: %w", err)
	}
	return token, nil
}

// detectRemoteOS detects the OS and architecture of the remote machine.
func detectRemoteOS(client *ssh.Client) (string, string, error) {
	osName, err := runSSHCommand(client, "uname -s 2>/dev/null || echo unknown")
	if err != nil {
		return "", "", fmt.Errorf("detecting OS: %w", err)
	}
	osName = strings.TrimSpace(strings.ToLower(osName))

	archRaw, err := runSSHCommand(client, "uname -m 2>/dev/null || echo unknown")
	if err != nil {
		return "", "", fmt.Errorf("detecting arch: %w", err)
	}
	archRaw = strings.TrimSpace(strings.ToLower(archRaw))

	// Normalize architecture names to Go conventions
	arch := archRaw
	switch archRaw {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	case "armv7l":
		arch = "arm"
	}

	return osName, arch, nil
}

// runSSHCommand executes a command on the remote host and returns combined output.
func runSSHCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		combined := stdout.String() + stderr.String()
		return combined, err
	}

	return stdout.String(), nil
}

// buildDockerDeployCmd generates the shell command to deploy the agent via Docker.
func buildDockerDeployCmd(serverURL, token, agentImage string) string {
	if agentImage == "" {
		agentImage = "ghcr.io/therealmcsparrow/mcharbor-agent:latest"
	}

	// Stop and remove existing agent container if any, then run new one
	return fmt.Sprintf(`
set -e
# Remove existing agent container if present
docker rm -f mcharbor-agent 2>/dev/null || true
# Pull and run the agent
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e MCHARBOR_URL=%s \
  -e MCHARBOR_AGENT_TOKEN=%s \
  %s
echo "Agent container started successfully"
`, shellEscape(serverURL), shellEscape(token), shellEscape(agentImage))
}

// buildBinaryDeployCmd generates the shell command to deploy the agent as a binary.
func buildBinaryDeployCmd(serverURL, token, osName, arch string) string {
	// Download URL pattern — the binary is published per OS/arch
	downloadURL := fmt.Sprintf(
		"https://github.com/therealmcsparrow/mcharbor-agent/releases/latest/download/mcharbor-agent-%s-%s",
		osName, arch,
	)

	return fmt.Sprintf(`
set -e

# Download agent binary
curl -fsSL -o /usr/local/bin/mcharbor-agent %s
chmod +x /usr/local/bin/mcharbor-agent

# Create systemd service
cat > /etc/systemd/system/mcharbor-agent.service << 'SERVICEEOF'
[Unit]
Description=McHarbor Remote Agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mcharbor-agent
Environment=MCHARBOR_URL=%s
Environment=MCHARBOR_AGENT_TOKEN=%s
Environment=DOCKER_HOST=unix:///var/run/docker.sock
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICEEOF

systemctl daemon-reload
systemctl enable mcharbor-agent
systemctl restart mcharbor-agent
echo "Agent binary installed and service started"
`, shellEscape(downloadURL), shellEscape(serverURL), shellEscape(token))
}

// shellEscape escapes a string for safe use in shell commands.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
