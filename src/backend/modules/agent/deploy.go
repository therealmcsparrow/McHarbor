// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
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
	// DeployDocker runs the agent as a container on the remote host.
	DeployDocker DeployMethod = "docker"
	// DeployBinary installs the agent as a native systemd service.
	DeployBinary DeployMethod = "binary"
)

// DeployRequest is the JSON body for POST /agents/{envId}/deploy.
type DeployRequest struct {
	SSHHost     string `json:"sshHost"`
	SSHPort     int    `json:"sshPort"`
	SSHUser     string `json:"sshUser"`
	SSHAuthType string `json:"sshAuthType"` // "key" or "password"
	SSHKey      string `json:"sshKey,omitempty"`
	SSHPassword string `json:"sshPassword,omitempty"`
	// RootPassword is used only where su is the sole escalation route (a
	// Debian host with no sudo). It is never stored, never remembered by
	// SaveCredentials and never returned.
	RootPassword       string       `json:"rootPassword,omitempty"`
	HostKeyFingerprint string       `json:"hostKeyFingerprint"`
	Method             DeployMethod `json:"method"`
	AgentImage         string       `json:"agentImage,omitempty"`

	// SSHSecretID reads the key or the password from the secrets vault instead
	// of carrying it in the request. It is the id of a vault row; the value is
	// resolved here and never returned to the caller.
	SSHSecretID string `json:"sshSecretId,omitempty"`
	// SaveCredentials stores what was used so a later redeploy needs no
	// re-entry. A literal key or password is written into the vault first —
	// the environment row holds the vault id, never the credential.
	SaveCredentials bool `json:"saveCredentials,omitempty"`

	// InstallPrereqs names the prerequisites the operator agreed to install.
	// Empty means install nothing, which is the default: deploying is not
	// consent to run a package manager on a host that may be running guests.
	InstallPrereqs []string `json:"installPrereqs,omitempty"`
	// ProbeOnly connects, reports the prerequisites and stops. Nothing is
	// installed and no agent is deployed.
	ProbeOnly bool `json:"probeOnly,omitempty"`
	// Scope narrows a probe to one question — Docker readiness or virtual
	// machine readiness. Empty reports everything, which is what a deploy does.
	Scope ProbeScope `json:"scope,omitempty"`
}

// DeployResult contains the outcome of a deployment attempt.
type DeployResult struct {
	Success bool         `json:"success"`
	Output  string       `json:"output,omitempty"`
	Error   string       `json:"error,omitempty"`
	Code    i18n.MsgCode `json:"code,omitempty"`
	OS      string       `json:"os,omitempty"`
	Arch    string       `json:"arch,omitempty"`
	// Prerequisites is what the host was found to have, always populated once
	// the SSH connection succeeded — including on a failed deploy, because a
	// missing dependency is usually the reason.
	Prerequisites *PrerequisiteReport `json:"prerequisites,omitempty"`
	// CredentialSaved reports whether the connection details were remembered.
	CredentialSaved bool `json:"credentialSaved,omitempty"`
}

// DeployViaSSH connects to a remote host over SSH and deploys the agent.
//
// `forceTLS` propagates the host scheme into the generated deploy
// command — true means the host is HTTPS-only and the agent should
// dial wss:// instead of ws:// (Phase 5: default-HTTPS).
func (s *Service) DeployViaSSH(ctx context.Context, envID string, req DeployRequest, serverURL string, forceTLS bool, logger *slog.Logger) (*DeployResult, error) {
	// A probe installs nothing and deploys nothing, so it needs no agent token
	// — which is what lets it run before the environment exists and tell an
	// operator whether the host is suitable at all.
	var token string
	if !req.ProbeOnly {
		var err error
		token, err = s.agentTokenPlaintext(ctx, envID)
		if err != nil {
			return nil, fmt.Errorf("retrieving agent token: %w", err)
		}
	}

	// A saved credential is read here and nowhere else: it never reaches the
	// browser, and the request that triggers a redeploy carries only the id.
	if req.SSHSecretID != "" {
		value, err := s.deployCredential(ctx, req.SSHSecretID)
		if err != nil {
			return &DeployResult{Success: false, Code: i18n.ErrAgentDeploySSHRequired}, nil
		}
		if req.SSHAuthType == "password" {
			req.SSHPassword = value
		} else {
			req.SSHKey = value
		}
	}

	// A `${secret:NAME}` typed into any of these fields is resolved here, the
	// same syntax the rest of McHarbor accepts. Without this the reference
	// itself was handed to ssh.ParsePrivateKey as if it were a key.
	if err := s.resolveSecretRefs(ctx, envID, &req); err != nil {
		logger.Warn("agent deploy: secret reference did not resolve",
			"env", envID, "host", req.SSHHost, "error", err)
		return &DeployResult{Success: false, Code: i18n.ErrAgentDeploySecretUnresolved}, nil
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
		Timeout: 15 * time.Second,
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
	defer func() {
		if err := client.Close(); err != nil {
			slog.Warn("closing SSH client failed", "error", err)
		}
	}()

	// Detect OS and architecture
	osName, arch, err := detectRemoteOS(client)
	if err != nil {
		logger.Warn("agent deploy: failed to detect remote OS", "env", envID, "host", req.SSHHost, "error", err)
		return &DeployResult{Success: false, Code: i18n.ErrAgentDeployOSDetectFailed}, nil
	}

	logger.Info("agent deploy: detected remote OS", "os", osName, "arch", arch, "env", envID)

	// Probed before anything is installed or deployed, so a failure downstream
	// is reported next to the dependency that explains it.
	// Detected once per connection: every probe and install below asks the
	// same question, and `su -c true` is not free.
	priv := detectEscalation(client, req.SSHPassword, req.RootPassword)
	logger.Info("agent deploy: privilege escalation",
		"env", envID, "host", req.SSHHost, "mode", priv.Mode, "needsRootPassword", priv.NeedsRootPassword)

	report := ProbePrerequisites(client, req.Method, priv, req.Scope)
	if req.ProbeOnly {
		if len(req.InstallPrereqs) == 0 {
			return &DeployResult{Success: true, OS: osName, Arch: arch, Prerequisites: &report}, nil
		}

		// Install-only run: the operator asked for the packages without a
		// deployment, so the host keeps them and nothing else is created.
		logger.Info("agent prereqs: installing without deploying",
			"env", envID, "host", req.SSHHost, "keys", req.InstallPrereqs)
		installLog, err := InstallPrerequisites(client, report, req.InstallPrereqs, priv)
		if err != nil {
			logger.Warn("agent prereqs: install failed", "env", envID, "host", req.SSHHost, "error", err)
			code := i18n.ErrAgentDeployPrereqFailed
			if errors.Is(err, errEscalationUnavailable) {
				code = i18n.ErrAgentPrereqNoEscalation
			}
			return &DeployResult{
				Success:       false,
				Output:        installLog,
				Code:          code,
				OS:            osName,
				Arch:          arch,
				Prerequisites: &report,
			}, nil
		}
		// Re-probed so the panel shows the host as it now is, not as it was.
		fresh := ProbePrerequisites(client, req.Method, priv, req.Scope)
		return &DeployResult{
			Success:       true,
			Output:        installLog,
			OS:            osName,
			Arch:          arch,
			Prerequisites: &fresh,
		}, nil
	}

	var prereqLog string
	if len(req.InstallPrereqs) > 0 {
		logger.Info("agent deploy: installing prerequisites",
			"env", envID, "host", req.SSHHost, "keys", req.InstallPrereqs)
		prereqLog, err = InstallPrerequisites(client, report, req.InstallPrereqs, priv)
		if err != nil {
			logger.Warn("agent deploy: prerequisite install failed", "env", envID, "error", err)
			return &DeployResult{
				Success:       false,
				Output:        prereqLog,
				Code:          i18n.ErrAgentDeployPrereqFailed,
				OS:            osName,
				Arch:          arch,
				Prerequisites: &report,
			}, nil
		}
		// Re-probe so the result describes the host as it now is.
		report = ProbePrerequisites(client, req.Method, priv, req.Scope)
	}

	// The agent is pinned to this host's certificate, and the download
	// verifies against the root that signed it, so neither step accepts an
	// unverified certificate.
	pinFingerprint, caPEM := s.tlsPinning()

	var deployCmd string
	switch req.Method {
	case DeployDocker:
		deployCmd = buildDockerDeployCmd(ctx, serverURL, token, req.AgentImage, osName, arch, forceTLS, pinFingerprint, caPEM)
	case DeployBinary:
		deployCmd = buildBinaryDeployCmd(ctx, serverURL, token, osName, arch, forceTLS, pinFingerprint, caPEM)
	default:
		return &DeployResult{Success: false, Code: i18n.ErrAgentDeployInvalidMethod}, nil
	}
	// There is no public fallback any more: a deploy McHarbor cannot serve is
	// refused before anything runs on the host, naming what to do instead.
	if deployCmd == "" {
		logger.Warn("agent deploy: McHarbor holds no agent for the host's platform", "env", envID, "os", osName, "arch", arch, "method", req.Method)
		return &DeployResult{Success: false, Code: i18n.ErrAgentReleaseNoArtifact, OS: osName, Arch: arch, Prerequisites: &report}, nil
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

	saved := false
	if req.SaveCredentials {
		// Only after a working deploy. Remembering details that were never
		// proven would hand the next redeploy a credential nobody has tested.
		if err := s.rememberDeployment(ctx, envID, req, actualFingerprint); err != nil {
			// Not fatal: the agent is installed and running, and saying the
			// deploy failed because a convenience did would be a lie.
			logger.Warn("agent deploy: could not remember connection", "env", envID, "error", err)
		} else {
			saved = true
		}
	}

	return &DeployResult{
		Success:         true,
		Output:          prereqLog + output,
		OS:              osName,
		Arch:            arch,
		Prerequisites:   &report,
		CredentialSaved: saved,
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
func (s *Service) agentTokenPlaintext(ctx context.Context, envID string) (string, error) {
	var encToken string
	var tokenSecretID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT agent_token, agent_token_secret_id FROM environments WHERE id = ? AND connection_type = 'agent'",
		envID,
	).Scan(&encToken, &tokenSecretID)
	if err != nil {
		return "", fmt.Errorf("querying agent token: %w", err)
	}

	token, err := s.readAgentToken(ctx, tokenSecretID, encToken)
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
	defer func() {
		if err := session.Close(); err != nil {
			slog.Warn("closing SSH session failed", "error", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		combined := stdout.String() + stderr.String()
		return combined, err
	}

	return stdout.String(), nil
}

// legacyDefaultAgentImage is what older frontends sent as the image of every
// Docker deploy. It was never an operator's choice, so it is not treated as a pin.
const legacyDefaultAgentImage = "ghcr.io/therealmcsparrow/mcharbor-agent:latest"

// buildDockerDeployCmd generates the shell command to deploy the agent via
// Docker, or "" when McHarbor has nothing to serve for the platform.
func buildDockerDeployCmd(ctx context.Context, serverURL, token, agentImage, osName, arch string, forceTLS bool, pinFingerprint, caPEM string) string {
	// An image the operator explicitly chose wins and is pulled as asked.
	// Otherwise the agent comes from this McHarbor: the target release's image
	// tarball, loaded without a registry, else an image assembled on the host
	// from the binary McHarbor serves.
	pinned := strings.TrimSpace(agentImage)
	if pinned == legacyDefaultAgentImage {
		pinned = ""
	}
	var imageStep string
	switch {
	case pinned != "":
		agentImage = pinned
		imageStep = "docker pull " + shellEscape(agentImage)
	case imageAvailable(ctx, osName, arch):
		agentImage = "$AGENT_IMAGE"
		imageStep = dockerLoadImageStep(serverURL, token, osName, arch, caPEM)
	case binaryPath(ctx, osName, arch) != "":
		agentImage = "mcharbor-agent:mcharbor-local"
		imageStep = dockerLocalImageStep(serverURL, osName, arch, caPEM)
	default:
		return ""
	}
	imageArg := shellEscape(agentImage)
	if agentImage == "$AGENT_IMAGE" {
		imageArg = `"$AGENT_IMAGE"`
	}
	forceTLSLine := "0"
	transferScheme := "http"
	if forceTLS {
		forceTLSLine = "1"
		transferScheme = "https"
	}

	// Stop and remove existing agent container if any, then run new one
	return fmt.Sprintf(`
set -e
MCHARBOR_FORCE_TLS=%s
MCHARBOR_TRANSFER_LISTEN="${MCHARBOR_TRANSFER_LISTEN:-0.0.0.0:8788}"
MCHARBOR_COMPOSE_DIR="${MCHARBOR_COMPOSE_DIR:-/var/lib/mcharbor-agent/compose}"
MCHARBOR_DATA_DIR="${MCHARBOR_DATA_DIR:-/var/lib/mcharbor-agent}"
if [ -z "${MCHARBOR_TRANSFER_ADVERTISE_URL:-}" ]; then
  HOST_IP=""
  if command -v ip >/dev/null 2>&1; then
    HOST_IP=$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i=="src") {print $(i+1); exit}}')
  fi
  if [ -z "$HOST_IP" ] && command -v hostname >/dev/null 2>&1; then
    HOST_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
  fi
  if [ -n "$HOST_IP" ]; then
    MCHARBOR_TRANSFER_ADVERTISE_URL="%s://$HOST_IP:8788"
  fi
fi
mkdir -p "$MCHARBOR_COMPOSE_DIR" "$MCHARBOR_DATA_DIR"
%s
docker ps -a --filter "name=^/mcharbor-agent-old-" --filter "status=exited" --filter "status=created" --format "{{.ID}}" | xargs -r docker rm -f
# Remove existing agent container if present
docker rm -f mcharbor-agent 2>/dev/null || true
TRANSFER_ARGS="-e MCHARBOR_TRANSFER_LISTEN=$MCHARBOR_TRANSFER_LISTEN -p 8788:8788"
if [ -n "${MCHARBOR_TRANSFER_ADVERTISE_URL:-}" ]; then
  TRANSFER_ARGS="$TRANSFER_ARGS -e MCHARBOR_TRANSFER_ADVERTISE_URL=$MCHARBOR_TRANSFER_ADVERTISE_URL"
fi
# Pull and run the agent
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$MCHARBOR_COMPOSE_DIR":"$MCHARBOR_COMPOSE_DIR" \
  -v "$MCHARBOR_DATA_DIR":"$MCHARBOR_DATA_DIR" \
  -e MCHARBOR_URL=%s \
  -e MCHARBOR_AGENT_TOKEN=%s \
  -e MCHARBOR_FORCE_TLS=%s \
  -e MCHARBOR_INSECURE="${MCHARBOR_TLS_INSECURE:-false}" \
  -e MCHARBOR_PIN=%s \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e LOG_LEVEL="${LOG_LEVEL:-info}" \
  -e MCHARBOR_COMPOSE_DIR="$MCHARBOR_COMPOSE_DIR" \
  -e MCHARBOR_DATA_DIR="$MCHARBOR_DATA_DIR" \
  $TRANSFER_ARGS \
  %s
echo "Agent container started successfully"
`, forceTLSLine, transferScheme, imageStep, shellEscape(serverURL), shellEscape(token), forceTLSLine, shellEscape(pinFingerprint), imageArg)
}

// buildBinaryDeployCmd generates the shell command to deploy the agent as a
// binary, or "" when McHarbor serves no binary for the platform. The binary
// always comes from this McHarbor, so the host gets the agent it expects.
func buildBinaryDeployCmd(ctx context.Context, serverURL, token, osName, arch string, forceTLS bool, pinFingerprint, caPEM string) string {
	if binaryPath(ctx, osName, arch) == "" {
		return ""
	}
	downloadURL := fmt.Sprintf("%s/api/agent/binary/%s/%s", strings.TrimRight(serverURL, "/"), osName, arch)
	forceTLSLine := "0"
	if forceTLS {
		forceTLSLine = "1"
	}
	return fmt.Sprintf(`
set -e
MCHARBOR_FORCE_TLS=%s

%s
# Downloaded beside the target, verified, and only then installed: curl creates
# its output file before it knows the response status, so a failed fetch
# straight onto /usr/local/bin leaves a unit pointing at an empty path.
AGENT_TMP=$(mktemp)
trap 'rm -f "$AGENT_TMP"' EXIT
mcharbor_fetch "$AGENT_TMP" %s
if [ ! -s "$AGENT_TMP" ] || [ "$(head -c 4 "$AGENT_TMP" | od -An -tx1 | tr -d "[:space:]")" != "7f454c46" ]; then
  echo "The agent downloaded from this McHarbor is not a Linux executable." >&2
  exit 1
fi
install -m 0755 "$AGENT_TMP" /usr/local/bin/mcharbor-agent
# The agent binary carries the CLI. The link is made only where it would not
# replace a CLI installed separately.
if [ ! -e /usr/local/bin/mcharbor ] || [ "$(readlink /usr/local/bin/mcharbor)" = mcharbor-agent ]; then
  ln -sfn mcharbor-agent /usr/local/bin/mcharbor
fi
mkdir -p /var/lib/mcharbor-agent/compose

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
Environment=MCHARBOR_FORCE_TLS=%s
Environment=MCHARBOR_INSECURE=__MCHARBOR_INSECURE__
Environment=MCHARBOR_PIN=%s
Environment=DOCKER_HOST=unix:///var/run/docker.sock
Environment=LOG_LEVEL=info
Environment="GODEBUG=cpu.avx512f=off,cpu.avx512vl=off"
Environment=MCHARBOR_TRANSFER_LISTEN=0.0.0.0:8788
Environment=MCHARBOR_TRANSFER_ADVERTISE_URL=
Environment=MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose
Environment=MCHARBOR_DATA_DIR=/var/lib/mcharbor-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICEEOF

# The agent has to accept whatever certificate the download just accepted, or
# it installs cleanly and then never completes a handshake.
sed -i "s|__MCHARBOR_INSECURE__|$MCHARBOR_TLS_INSECURE|" /etc/systemd/system/mcharbor-agent.service

if grep -qw avx512f /proc/cpuinfo 2>/dev/null; then
    for unit in containerd docker; do
        systemctl cat "$unit.service" >/dev/null 2>&1 || continue
        mkdir -p "/etc/systemd/system/$unit.service.d"
        printf '[Service]\nEnvironment="GODEBUG=cpu.avx512f=off,cpu.avx512vl=off"\n' | tee "/etc/systemd/system/$unit.service.d/mcharbor-go-avx512.conf" >/dev/null
    done
fi
systemctl daemon-reload
systemctl enable mcharbor-agent
systemctl restart mcharbor-agent
echo "Agent binary installed and service started"
`, forceTLSLine, curlFetchAgent(caPEM), shellEscape(downloadURL),
		shellEscape(serverURL), shellEscape(token), forceTLSLine, shellEscape(pinFingerprint))
}

// shellEscape escapes a string for safe use in shell commands.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// curlFetchAgent emits a shell function that downloads the agent from this
// McHarbor, strict first and once more with -k.
//
// A stock McHarbor signs its own certificate, so a strict-only fetch fails
// with exit 60 and takes the whole deploy with it; a -k-only fetch would throw
// away verification against hosts that have a real certificate. The retry is
// announced, because silently not verifying is the part worth seeing.
func curlFetchAgent(caPEM string) string {
	if caPEM == "" {
		return `MCHARBOR_TLS_INSECURE=false
mcharbor_fetch() {
  if curl -fsSL -o "$1" "$2"; then return 0; fi
  echo "Strict TLS failed for $2; retrying without certificate verification (no local CA to verify against)." >&2
  MCHARBOR_TLS_INSECURE=true
  curl -k -fsSL -o "$1" "$2"
}
`
	}
	// With the root in hand there is no reason to ever drop verification: the
	// certificate the download refuses is exactly the one the agent would
	// refuse a minute later, and failing here says so while it is still fixable.
	return `MCHARBOR_TLS_INSECURE=false
MCHARBOR_CA_FILE=$(mktemp)
chmod 600 "$MCHARBOR_CA_FILE"
cat > "$MCHARBOR_CA_FILE" <<'MCHARBOR_CA_EOF'
` + caPEM + `MCHARBOR_CA_EOF
mcharbor_fetch() {
  curl --cacert "$MCHARBOR_CA_FILE" -fsSL -o "$1" "$2"
}
`
}

// dockerLocalImageStep builds the agent image on the remote host from the
// binary this McHarbor serves.
//
// McHarbor ships a binary rather than an image tarball, so the image is
// assembled there. The layers mirror docker/Dockerfile.agent, which is what
// keeps a deployed agent identical to the one this server was built with.
func dockerLocalImageStep(serverURL, osName, arch, caPEM string) string {
	return fmt.Sprintf(`%sAGENT_BUILD_DIR=$(mktemp -d)
trap 'rm -rf "$AGENT_BUILD_DIR"' EXIT
mcharbor_fetch "$AGENT_BUILD_DIR/mcharbor-agent" %s/api/agent/binary/`+osName+`/`+arch+`
chmod +x "$AGENT_BUILD_DIR/mcharbor-agent"
cat > "$AGENT_BUILD_DIR/Dockerfile" <<'AGENTDOCKERFILE'
FROM alpine:3.20
RUN apk add --no-cache tini ca-certificates docker-cli docker-cli-compose iptables
COPY mcharbor-agent /usr/local/bin/mcharbor-agent
RUN ln -s mcharbor-agent /usr/local/bin/mcharbor
ENTRYPOINT ["tini", "--"]
CMD ["mcharbor-agent"]
AGENTDOCKERFILE
docker build -q -t mcharbor-agent:mcharbor-local "$AGENT_BUILD_DIR" >/dev/null`,
		curlFetchAgent(caPEM), shellEscape(strings.TrimRight(serverURL, "/")))
}

// imageAvailable reports whether the target release carries an image tarball
// for the platform.
func imageAvailable(ctx context.Context, osName, arch string) bool {
	path, _ := imagePath(ctx, osName, arch)
	return path != ""
}

// dockerLoadImageStep loads the target release's image from this McHarbor on
// the remote host and leaves its reference in $AGENT_IMAGE. The agent token is
// the credential, and the digest is checked before docker sees the bytes.
func dockerLoadImageStep(serverURL, token, osName, arch, caPEM string) string {
	url := strings.TrimRight(serverURL, "/") + "/api/agent/image/" + osName + "/" + arch
	return curlFetchAgent(caPEM) + `AGENT_IMAGE_TMP=$(mktemp)
AGENT_IMAGE_HDR=$(mktemp)
trap 'rm -f "$AGENT_IMAGE_TMP" "$AGENT_IMAGE_HDR"' EXIT
MCHARBOR_CURL_TLS=""
if [ -n "${MCHARBOR_CA_FILE:-}" ]; then MCHARBOR_CURL_TLS="--cacert $MCHARBOR_CA_FILE"; fi
if ! curl $MCHARBOR_CURL_TLS -fsSL -H "Authorization: Bearer ` + token + `" -D "$AGENT_IMAGE_HDR" -o "$AGENT_IMAGE_TMP" ` + shellEscape(url) + `; then
  if [ -n "${MCHARBOR_CA_FILE:-}" ]; then echo "Downloading the agent image from McHarbor failed." >&2; exit 1; fi
  echo "Strict TLS failed for the agent image; retrying without certificate verification." >&2
  MCHARBOR_TLS_INSECURE=true
  curl -k -fsSL -H "Authorization: Bearer ` + token + `" -D "$AGENT_IMAGE_HDR" -o "$AGENT_IMAGE_TMP" ` + shellEscape(url) + `
fi
AGENT_IMAGE_WANT=$(awk 'tolower($1) == "x-checksum-sha256:" { print $2 }' "$AGENT_IMAGE_HDR" | tr -d '[:space:]' | tail -1)
if [ -n "$AGENT_IMAGE_WANT" ] && [ "$(sha256sum "$AGENT_IMAGE_TMP" | awk '{ print $1 }')" != "$AGENT_IMAGE_WANT" ]; then
  echo "The agent image from McHarbor does not match its checksum." >&2
  exit 1
fi
AGENT_IMAGE=$(docker load -i "$AGENT_IMAGE_TMP" | sed -n 's/^Loaded image: //p' | tail -1)
if [ -z "$AGENT_IMAGE" ]; then echo "docker load reported no image." >&2; exit 1; fi`
}
