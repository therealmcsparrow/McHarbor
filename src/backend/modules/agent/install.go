// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/certgen"
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
	Token                string `json:"token"`
	AgentToken           string `json:"agentToken,omitempty"`
	ExpiresAt            string `json:"expiresAt"`
	Script               string `json:"script"`
	ServerURL            string `json:"serverUrl"`
	AgentImage           string `json:"agentImage"`
	TransferListen       string `json:"transferListen"`
	TransferAdvertiseURL string `json:"transferAdvertiseUrl,omitempty"`
	// ForceTLS is true when the host is reachable only over HTTPS.
	// The generated install script exports MCHARBOR_FORCE_TLS=1 so
	// the agent dials wss:// instead of ws://.
	ForceTLS bool `json:"forceTls"`
	// CAFingerprint is the local root's SHA-256 when McHarbor issued the
	// served certificate, so every install snippet can set MCHARBOR_PIN
	// instead of telling the agent to skip verification. Empty for an
	// uploaded certificate, which a CA outside McHarbor may reissue.
	CAFingerprint string `json:"caFingerprint,omitempty"`
}

// CreateInstallToken generates a one-time install token for an agent environment.
// The token expires after 24 hours and can only be used once.
//
// `forceTLS` is propagated into the generated install script so the
// agent dials wss:// instead of ws:// when the host is HTTPS-only.
func (s *Service) CreateInstallToken(ctx context.Context, envID, serverURL string, forceTLS bool) (*CreateInstallTokenResponse, error) {
	// Verify this is an agent environment
	var connType string
	err := s.db.QueryRowContext(ctx, "SELECT connection_type FROM environments WHERE id = ?", envID).Scan(&connType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("environment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying environment: %w", err)
	}
	if connType != "agent" {
		return nil, fmt.Errorf("environment is not an agent connection type")
	}

	agentToken, err := s.agentTokenPlaintext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting agent token: %w", err)
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

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO install_tokens (id, env_id, token_hash, expires_at, used, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, envID, hash, expiresAt.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("storing install token: %w", err)
	}

	script := fmt.Sprintf("curl -sSL %s/api/agent/install/%s | bash", serverURL, token)

	pinFingerprint, _ := s.tlsPinning()

	return &CreateInstallTokenResponse{
		Token:          token,
		AgentToken:     agentToken,
		ExpiresAt:      expiresAt.Format(time.RFC3339),
		Script:         script,
		ServerURL:      serverURL,
		AgentImage:     servedImageRef(ctx),
		TransferListen: "0.0.0.0:8788",
		ForceTLS:       forceTLS,
		CAFingerprint:  pinFingerprint,
	}, nil
}

// ValidateInstallToken checks if an install token is valid (not used, not expired).
// Returns the environment ID on success, and marks the token as used.
func (s *Service) ValidateInstallToken(ctx context.Context, token string) (string, error) {
	hash := hashToken(token)

	var id, envID, expiresAt string
	var used int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, env_id, expires_at, used
		FROM install_tokens
		WHERE token_hash = ?
	`, hash).Scan(&id, &envID, &expiresAt, &used)

	if errors.Is(err, sql.ErrNoRows) {
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
	_, err = s.db.ExecContext(ctx, "UPDATE install_tokens SET used = 1, updated_at = ? WHERE id = ?", now, id)
	if err != nil {
		return "", fmt.Errorf("marking token used: %w", err)
	}

	return envID, nil
}

// tlsPinning returns the local root's fingerprint and its PEM, for an install
// that verifies this host instead of skipping verification. Both are empty when
// the host serves a certificate McHarbor did not issue, because an externally
// renewed certificate would break a pin at its CA's next reissue.
//
// The root is pinned rather than the served certificate so the automatic
// 90-day leaf renewal does not strand every agent at once.
func (s *Service) tlsPinning() (fingerprint, caPEM string) {
	if s.dataDir == "" {
		return "", ""
	}
	tlsDir := filepath.Join(s.dataDir, "tls")
	certPath := filepath.Join(tlsDir, "cert.pem")
	if !certgen.IsLocallyIssued(certPath) {
		return "", ""
	}
	fp, err := certgen.CAFingerprint(tlsDir)
	if err != nil {
		return "", ""
	}
	ca, err := os.ReadFile(certgen.CACertPath(tlsDir))
	if err != nil {
		return fp, ""
	}
	return fp, string(ca)
}

// InstallScript generates a shell script that installs and configures the agent.
// The script auto-detects OS/arch and installs via Docker or binary.
//
// `forceTLS` is propagated into the generated script so the agent
// dials wss:// instead of ws:// when the host is HTTPS-only. The
// script also flips the auto-detected direct-transfer advertise URL
// from http:// to https:// when the host is HTTPS.
func (s *Service) InstallScript(ctx context.Context, envID, serverURL string, forceTLS, insecure bool, method string) (string, error) {
	// Get agent token
	agentToken, err := s.agentTokenPlaintext(ctx, envID)
	if err != nil {
		return "", fmt.Errorf("getting agent token: %w", err)
	}
	pinFingerprint, caPEM := s.tlsPinning()
	return renderInstallScript(agentToken, serverURL, forceTLS, insecure, method, pinFingerprint, caPEM), nil
}

// renderInstallScript builds the install script from resolved inputs, apart
// from the database and the certificate store so what it emits can be tested.
func renderInstallScript(agentToken, serverURL string, forceTLS, insecure bool, method, pinFingerprint, caPEM string) string {
	// When the host is HTTPS, advertise the transfer helper over
	// https:// (Phase 3: same cert as the main listener). The
	// detect_advertise_url function inside the generated script
	// picks up this flag via the exported MCHARBOR_FORCE_TLS env.
	forceTLSLine := "MCHARBOR_FORCE_TLS=0"
	if forceTLS {
		forceTLSLine = "MCHARBOR_FORCE_TLS=1"
	}
	transferScheme := "http"
	if forceTLS {
		transferScheme = "https"
	}
	// Pinning replaces the old answer to a privately-issued certificate. The
	// agent verifies this one certificate, and the script's own curl calls
	// verify against the root that signed it, so nothing in the install path
	// accepts an unverified certificate any more. MCHARBOR_INSECURE survives
	// only for a host with no local CA whose caller still asked for it.
	// `insecure` is the operator's answer to "this server uses a self-signed
	// certificate". Yes means pin the root; no means they have arranged trust
	// on the remote host, and a pin there would only be one more thing to
	// rotate.
	if !insecure {
		pinFingerprint = ""
	}
	insecureLine := "MCHARBOR_INSECURE=false"
	if insecure && pinFingerprint == "" {
		insecureLine = "MCHARBOR_INSECURE=true"
	}
	pinLine := "MCHARBOR_PIN=" + shellEscape(pinFingerprint)
	caLine := `MCHARBOR_CA_PEM=""`
	if caPEM != "" {
		caLine = "MCHARBOR_CA_PEM=$(cat <<'MCHARBOR_CA_EOF'\n" + caPEM + "MCHARBOR_CA_EOF\n)"
	}
	// "docker" and "binary" pin the install; anything else keeps the original
	// auto-detection, so an install URL without ?method= behaves as before.
	if method != "docker" && method != "binary" {
		method = "auto"
	}
	methodLine := "MCHARBOR_INSTALL_METHOD=" + method

	script := fmt.Sprintf(`#!/bin/bash
# McHarbor Agent Install Script
# This script installs the McHarbor agent on the current machine.
# Generated automatically — do not edit.

set -e

MCHARBOR_URL=%s
MCHARBOR_AGENT_TOKEN=%s
%s
%s
%s
%s
%s
# Set when the operator pinned an image, which then wins over anything this
# McHarbor carries. It is the only way this script ever pulls from a registry.
AGENT_IMAGE_PINNED="${MCHARBOR_AGENT_IMAGE:-}"
AGENT_IMAGE=""
# McHarbor issues its own certificate, so this script carries the root that
# signed it and verifies against that. Every curl back to McHarbor has to agree
# with what the agent will do, or the probes below silently decide McHarbor has
# nothing — and there is no public release to fall back to any more.
CURL_TLS=""
MCHARBOR_CA_FILE=""
if [ -n "$MCHARBOR_CA_PEM" ]; then
  MCHARBOR_CA_FILE=$(mktemp)
  chmod 600 "$MCHARBOR_CA_FILE"
  printf '%%s\n' "$MCHARBOR_CA_PEM" > "$MCHARBOR_CA_FILE"
  trap 'rm -f "$MCHARBOR_CA_FILE"' EXIT
  CURL_TLS="--cacert $MCHARBOR_CA_FILE"
elif [ "$MCHARBOR_INSECURE" = "true" ] || [ "$MCHARBOR_INSECURE" = "1" ]; then
  CURL_TLS="-k"
fi
if [ -n "$MCHARBOR_PIN" ]; then
  echo "Pinning McHarbor certificate $MCHARBOR_PIN"
fi
# The agent is ~18MB, and a TLS record corrupted anywhere in it (a tunnel
# with a broken PMTU, a NIC offload bug, a middlebox) aborts the whole
# transfer. Resume rather than restart, and verify the length afterwards: a
# truncated ELF still has valid magic bytes and would install happily.
mcharbor_download() {
  download_target="$1"
  download_url="$2"
  download_attempt=1
  download_rc=0
  while [ "$download_attempt" -le 6 ]; do
    curl $CURL_TLS -fsSL --retry 5 --retry-delay 2 --retry-connrefused \
      --continue-at - -o "$download_target" "$download_url" && return 0
    download_rc=$?
    # --continue-at on a server that refuses ranges fails with 33; there is
    # nothing to resume from, so the next attempt starts clean.
    if [ "$download_rc" -eq 33 ]; then
      rm -f "$download_target"
    fi
    # curl does not count a corrupted TLS record (56) as retryable, so its
    # own --retry never fires here. Resuming costs the bytes already on
    # disk, not the whole 18MB, and a discarded record is never written.
    echo "  attempt $download_attempt failed (curl $download_rc); resuming..."
    download_attempt=$((download_attempt + 1))
    sleep 2
  done
  return "$download_rc"
}

mcharbor_expected_size() {
  curl $CURL_TLS -fsI "$1" 2>/dev/null \
    | awk 'tolower($1) == "content-length:" { print $2 }' \
    | tr -d '[:space:]' \
    | tail -1
}

# Probe McHarbor for a binary, keeping the reason it failed, so a TLS failure,
# a proxy that refuses HEAD and a genuine 404 each get their own explanation.
mcharbor_probe() {
  PROBE_STATUS=""
  PROBE_ERROR=""
  probe_err=$(mktemp)
  PROBE_STATUS=$(curl $CURL_TLS -sS -I -o /dev/null -w '%%{http_code}' "$1" 2>"$probe_err") || true
  PROBE_ERROR=$(tr -d '\r' < "$probe_err" | tr '\n' ' ' | sed 's/  */ /g; s/^ *//; s/ *$//')
  rm -f "$probe_err"
}

mcharbor_probe_explain() {
  if [ "$PROBE_STATUS" = "404" ]; then
    echo "This McHarbor serves no ${OS}/${ARCH} agent (HTTP 404)."
  elif [ -n "$PROBE_STATUS" ] && [ "$PROBE_STATUS" != "000" ]; then
    echo "McHarbor answered HTTP $PROBE_STATUS for $1 rather than the agent."
    echo "A proxy in front of McHarbor refusing HEAD looks exactly like this."
  else
    echo "Could not reach $1."
    echo "  ${PROBE_ERROR:-curl failed without reporting a reason}"
    echo "This is a connection or TLS failure, not a missing agent binary."
  fi
}

# Every agent comes from this McHarbor. When it has nothing for the platform the
# install stops here and says how to fix that, instead of reaching for GitHub.
mcharbor_no_agent() {
  echo "Error: this McHarbor has no agent for ${OS}/${ARCH}."
  echo "Download an agent release in McHarbor under Settings > Updates > Agents"
  echo "and make it the target (with images for linux/${ARCH} for a container install),"
  echo "or build one on the McHarbor host with scripts/development/build-agent.sh."
  exit 1
}

# Loads the target release's agent image from McHarbor and prints the reference
# it loaded as. The agent token is the credential; the digest is checked before
# docker ever sees the bytes.
mcharbor_load_image() {
  image_tmp=$(mktemp)
  image_headers=$(mktemp)
  loaded=""
  if curl $CURL_TLS -fsSL --retry 5 --retry-delay 2 --retry-connrefused \
      -H "Authorization: Bearer $MCHARBOR_AGENT_TOKEN" -D "$image_headers" -o "$image_tmp" "$1" 2>/dev/null; then
    want_sha=$(awk 'tolower($1) == "x-checksum-sha256:" { print $2 }' "$image_headers" | tr -d '[:space:]' | tail -1)
    got_sha=$(sha256sum "$image_tmp" | awk '{ print $1 }')
    if [ -n "$want_sha" ] && [ "$want_sha" != "$got_sha" ]; then
      echo "Error: the agent image from McHarbor does not match its checksum." >&2
      rm -f "$image_tmp" "$image_headers"
      exit 1
    fi
    loaded=$(docker load -i "$image_tmp" | sed -n 's/^Loaded image: //p' | tail -1)
  fi
  rm -f "$image_tmp" "$image_headers"
  printf '%%s' "$loaded"
}

MCHARBOR_TRANSFER_LISTEN="${MCHARBOR_TRANSFER_LISTEN:-0.0.0.0:8788}"
MCHARBOR_COMPOSE_DIR="${MCHARBOR_COMPOSE_DIR:-/var/lib/mcharbor-agent/compose}"
# Local state (the offline buffer the agent fills while McHarbor is unreachable).
MCHARBOR_DATA_DIR="${MCHARBOR_DATA_DIR:-/var/lib/mcharbor-agent}"

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

# The agent runs in the host's network namespace. That is what lets it see the
# host's real interfaces — for guest networking, and for reporting the host's
# addresses honestly rather than reporting a container's veth. --isolated puts
# it back on Docker's own network for anyone who wants the container boundary
# and does not intend to host guests here.
HOST_NETWORK=1
if [ "${MCHARBOR_AGENT_ISOLATED:-0}" = "1" ]; then
  HOST_NETWORK=0
fi
for arg in "$@"; do
  case "$arg" in
    --isolated) HOST_NETWORK=0 ;;
    --host-network) HOST_NETWORK=1 ;;
    --help|-h)
      echo "Usage: $0 [--isolated]"
      echo ""
      echo "  The agent runs with --network host --cap-add NET_ADMIN by default."
      echo "  It shares this host's network namespace, which is what lets guest"
      echo "  networks see your bridges and uplinks and create their own, and"
      echo "  what makes the transfer listener reachable on this host's address"
      echo "  without a published port."
      echo ""
      echo "  --isolated  Run on Docker's own bridge network instead, publishing"
      echo "              8788. The agent then sees only its own interfaces:"
      echo "              guest networks will find no bridge and no outbound"
      echo "              interface, so this host cannot host virtual machines"
      echo "              or system containers."
      echo ""
      echo "  Piping this script into a shell leaves no way to pass a flag, so"
      echo "  MCHARBOR_AGENT_ISOLATED=1 does the same as --isolated."
      echo ""
      echo "  Neither applies to a binary install, which already runs on this"
      echo "  host's network."
      exit 0
      ;;
  esac
done

detect_advertise_url() {
  if [ -n "${MCHARBOR_TRANSFER_ADVERTISE_URL:-}" ]; then
    echo "$MCHARBOR_TRANSFER_ADVERTISE_URL"
    return
  fi

  HOST_IP=""
  if command -v ip >/dev/null 2>&1; then
    HOST_IP=$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i=="src") {print $(i+1); exit}}')
  fi
  if [ -z "$HOST_IP" ] && command -v hostname >/dev/null 2>&1; then
    HOST_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
  fi

  if [ -n "$HOST_IP" ]; then
    echo "%s://$HOST_IP:8788"
  fi
}

MCHARBOR_TRANSFER_ADVERTISE_URL="$(detect_advertise_url)"
if [ -n "$MCHARBOR_TRANSFER_ADVERTISE_URL" ]; then
  echo "Direct transfer advertise URL: $MCHARBOR_TRANSFER_ADVERTISE_URL"
else
  echo "Direct transfer advertise URL not detected here; the agent will advertise its own addresses and McHarbor will add the address it sees this agent connect from. Set MCHARBOR_TRANSFER_ADVERTISE_URL only if the receiver is reached through port forwarding or a reverse proxy."
fi

# The install URL can pin the shape; otherwise Docker decides.
if [ "$MCHARBOR_INSTALL_METHOD" = "docker" ] && ! (command -v docker &>/dev/null && docker info &>/dev/null 2>&1); then
  echo "Error: this install URL asks for the container install, but Docker is not usable on this host."
  echo "Use the binary install URL instead, or install Docker first."
  exit 1
fi

if [ "$MCHARBOR_INSTALL_METHOD" != "binary" ] && command -v docker &>/dev/null && docker info &>/dev/null 2>&1; then
  echo "Docker detected — installing as container..."
  mkdir -p "$MCHARBOR_COMPOSE_DIR" "$MCHARBOR_DATA_DIR"

  # The agent always comes from this McHarbor: first the image tarball of the
  # release it targets, loaded without any registry; failing that, an image
  # assembled here from the binary it serves. Only an image the operator pinned
  # with MCHARBOR_AGENT_IMAGE is ever pulled from a registry.
  AGENT_IMAGE_URL="$MCHARBOR_URL/api/agent/image/${OS}/${ARCH}"
  AGENT_BINARY_URL="$MCHARBOR_URL/api/agent/binary/${OS}/${ARCH}"
  BUILT_LOCAL=0
  PROBE_STATUS=""
  if [ -n "$AGENT_IMAGE_PINNED" ]; then
    echo "Using the image pinned with MCHARBOR_AGENT_IMAGE: $AGENT_IMAGE_PINNED"
    docker pull "$AGENT_IMAGE_PINNED"
    AGENT_IMAGE="$AGENT_IMAGE_PINNED"
    BUILT_LOCAL=1
  else
    AGENT_IMAGE="$(mcharbor_load_image "$AGENT_IMAGE_URL")"
    if [ -n "$AGENT_IMAGE" ]; then
      BUILT_LOCAL=1
      echo "Loaded the agent image this McHarbor serves: $AGENT_IMAGE"
    else
      mcharbor_probe "$AGENT_BINARY_URL"
      [ "$PROBE_STATUS" = "200" ] || mcharbor_probe_explain "$AGENT_BINARY_URL"
    fi
  fi
  if [ "$BUILT_LOCAL" != "1" ] && [ "$PROBE_STATUS" = "200" ]; then
    echo "Building the agent image from the binary this McHarbor serves..."
    BUILD_DIR=$(mktemp -d)
    if mcharbor_download "$BUILD_DIR/mcharbor-agent" "$AGENT_BINARY_URL"; then
      chmod +x "$BUILD_DIR/mcharbor-agent"
      cat > "$BUILD_DIR/Dockerfile" <<'AGENTDOCKERFILE'
FROM alpine:3.20
RUN apk add --no-cache tini ca-certificates docker-cli docker-cli-compose iptables
COPY mcharbor-agent /usr/local/bin/mcharbor-agent
RUN ln -s mcharbor-agent /usr/local/bin/mcharbor
ENTRYPOINT ["tini", "--"]
CMD ["mcharbor-agent"]
AGENTDOCKERFILE
      if docker build -q -t mcharbor-agent:mcharbor-local "$BUILD_DIR" >/dev/null; then
        AGENT_IMAGE="mcharbor-agent:mcharbor-local"
        BUILT_LOCAL=1
        echo "Using the agent served by this McHarbor."
      else
        echo "Could not build the local agent image from the served binary."
      fi
    else
      echo "Could not download the agent from McHarbor."
    fi
    rm -rf "$BUILD_DIR"
  fi
  if [ "$BUILT_LOCAL" != "1" ]; then
    mcharbor_no_agent
  fi
  docker ps -a --filter "name=^/mcharbor-agent-old-" --filter "status=exited" --filter "status=created" --format "{{.ID}}" | xargs -r docker rm -f
  docker rm -f mcharbor-agent 2>/dev/null || true
  # Host networking publishes 8788 directly, so the port mapping would be
  # rejected as a conflict rather than ignored.
  NETWORK_ARGS="--network host --cap-add NET_ADMIN"
  TRANSFER_PORT_ARG=""
  if [ "$HOST_NETWORK" != "1" ]; then
    NETWORK_ARGS=""
    TRANSFER_PORT_ARG="-p 8788:8788"
    echo "Isolated mode: the agent runs on Docker's own network."
    echo "It will see only its own interfaces, so this host cannot host guests."
  else
    echo "The agent will share this host's network namespace and hold NET_ADMIN."
    echo "That is what lets guest networks see your bridges and uplinks and create their own."
    echo "It also means the agent has no network isolation from this host."
  fi
  TRANSFER_ARGS="-e MCHARBOR_TRANSFER_LISTEN=$MCHARBOR_TRANSFER_LISTEN $TRANSFER_PORT_ARG"
  if [ -n "$MCHARBOR_TRANSFER_ADVERTISE_URL" ]; then
    TRANSFER_ARGS="$TRANSFER_ARGS -e MCHARBOR_TRANSFER_ADVERTISE_URL=$MCHARBOR_TRANSFER_ADVERTISE_URL"
  fi
  docker run -d \
    --name mcharbor-agent \
    --restart unless-stopped \
    $NETWORK_ARGS \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$MCHARBOR_COMPOSE_DIR":"$MCHARBOR_COMPOSE_DIR" \
    -v "$MCHARBOR_DATA_DIR":"$MCHARBOR_DATA_DIR" \
    -e MCHARBOR_URL="$MCHARBOR_URL" \
    -e MCHARBOR_AGENT_TOKEN="$MCHARBOR_AGENT_TOKEN" \
    -e MCHARBOR_FORCE_TLS="$MCHARBOR_FORCE_TLS" \
    -e MCHARBOR_INSECURE="$MCHARBOR_INSECURE" \
    -e MCHARBOR_PIN="$MCHARBOR_PIN" \
    -e DOCKER_HOST=unix:///var/run/docker.sock \
    -e LOG_LEVEL="${LOG_LEVEL:-info}" \
    -e MCHARBOR_COMPOSE_DIR="$MCHARBOR_COMPOSE_DIR" \
    -e MCHARBOR_DATA_DIR="$MCHARBOR_DATA_DIR" \
    $TRANSFER_ARGS \
    "$AGENT_IMAGE"
  echo ""
  echo "Agent container started successfully."
  echo "Run 'docker logs mcharbor-agent' to check status."
  if [ "$HOST_NETWORK" != "1" ]; then
    echo ""
    echo "Note: this agent runs on Docker's own network, where it can see only its own"
    echo "interfaces. Guest networks will offer no bridge and no outbound interface."
    echo "Re-run this script without --isolated if this host should host virtual machines"
    echo "or system containers."
  fi
else
  if [ "$MCHARBOR_INSTALL_METHOD" = "binary" ]; then
    echo "Installing as binary (asked for by the install URL)..."
  else
    echo "Docker not found — installing as binary..."
  fi
  # This McHarbor serves the agent: its target release, else what was built
  # locally, else the one its image shipped with. Nothing else is tried.
  DOWNLOAD_URL="$MCHARBOR_URL/api/agent/binary/${OS}/${ARCH}"
  EXPECTED_SIZE=""
  mcharbor_probe "$DOWNLOAD_URL"
  if [ "$PROBE_STATUS" = "200" ]; then
    EXPECTED_SIZE="$(mcharbor_expected_size "$DOWNLOAD_URL")"
    echo "Downloading the agent this McHarbor serves..."
  else
    mcharbor_probe_explain "$DOWNLOAD_URL"
    mcharbor_no_agent
  fi

  # Downloaded beside the target rather than onto it. curl creates its output
  # file before it knows the response status, so writing straight to
  # /usr/local/bin leaves an empty binary behind a unit that is already
  # enabled — which reports as "Unable to locate executable" at every restart.
  AGENT_TMP=$(mktemp)
  trap 'rm -f "$AGENT_TMP"' EXIT
  if command -v curl &>/dev/null; then
    if ! mcharbor_download "$AGENT_TMP" "$DOWNLOAD_URL"; then
      echo "Error: downloading the agent from $DOWNLOAD_URL failed."
      echo "If curl reported a TLS record error, the transfer is being corrupted"
      echo "between this host and McHarbor - check MTU on any tunnel in between."
      exit 1
    fi
  elif command -v wget &>/dev/null; then
    wget -qO "$AGENT_TMP" "$DOWNLOAD_URL"
  else
    echo "Error: neither curl nor wget found"
    exit 1
  fi

  if [ ! -s "$AGENT_TMP" ]; then
    echo "Error: the agent downloaded from $DOWNLOAD_URL is empty."
    exit 1
  fi
  ACTUAL_SIZE="$(wc -c < "$AGENT_TMP" | tr -d '[:space:]')"
  if [ -n "$EXPECTED_SIZE" ] && [ "$ACTUAL_SIZE" != "$EXPECTED_SIZE" ]; then
    echo "Error: the agent arrived truncated - got $ACTUAL_SIZE bytes, expected $EXPECTED_SIZE."
    exit 1
  fi
  EXPECTED_SHA="$(curl $CURL_TLS -fsI "$DOWNLOAD_URL" 2>/dev/null | awk 'tolower($1) == "x-checksum-sha256:" { print $2 }' | tr -d '[:space:]' | tail -1)"
  if [ -n "$EXPECTED_SHA" ] && command -v sha256sum >/dev/null 2>&1 && \
     [ "$(sha256sum "$AGENT_TMP" | awk '{ print $1 }')" != "$EXPECTED_SHA" ]; then
    echo "Error: the agent downloaded from $DOWNLOAD_URL does not match its checksum."
    exit 1
  fi
  # Checked by magic bytes rather than by running it: the agent has no
  # --version flag, so a probe would start a second agent. A 404 page saved
  # under the binary's name is the failure this catches.
  if [ "$OS" = "linux" ] && [ "$(head -c 4 "$AGENT_TMP" | od -An -tx1 | tr -d "[:space:]")" != "7f454c46" ]; then
    echo "Error: what was downloaded from $DOWNLOAD_URL is not a Linux executable."
    echo "First line: $(head -c 200 "$AGENT_TMP" | tr -d "[:cntrl:]")"
    exit 1
  fi
  chmod +x "$AGENT_TMP"

  install -m 0755 "$AGENT_TMP" /usr/local/bin/mcharbor-agent
  # The agent binary carries the CLI. The link is made only where it would not
  # replace a CLI installed separately.
  if [ ! -e /usr/local/bin/mcharbor ] || [ "$(readlink /usr/local/bin/mcharbor)" = mcharbor-agent ]; then
    ln -sfn mcharbor-agent /usr/local/bin/mcharbor
  fi
  mkdir -p "$MCHARBOR_COMPOSE_DIR" "$MCHARBOR_DATA_DIR"

  # Create systemd service if available
  if command -v systemctl &>/dev/null; then
    cat > /etc/systemd/system/mcharbor-agent.service <<SERVICEEOF
[Unit]
Description=McHarbor Remote Agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mcharbor-agent
Environment=MCHARBOR_URL=$MCHARBOR_URL
Environment=MCHARBOR_AGENT_TOKEN=$MCHARBOR_AGENT_TOKEN
Environment=MCHARBOR_FORCE_TLS=$MCHARBOR_FORCE_TLS
Environment=MCHARBOR_INSECURE=$MCHARBOR_INSECURE
Environment=MCHARBOR_PIN=$MCHARBOR_PIN
Environment=DOCKER_HOST=unix:///var/run/docker.sock
Environment=LOG_LEVEL=info
Environment="GODEBUG=cpu.avx512f=off,cpu.avx512vl=off"
Environment=MCHARBOR_TRANSFER_LISTEN=$MCHARBOR_TRANSFER_LISTEN
Environment=MCHARBOR_TRANSFER_ADVERTISE_URL=$MCHARBOR_TRANSFER_ADVERTISE_URL
Environment=MCHARBOR_COMPOSE_DIR=$MCHARBOR_COMPOSE_DIR
Environment=MCHARBOR_DATA_DIR=$MCHARBOR_DATA_DIR
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICEEOF

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
    echo ""
    echo "Agent binary installed and systemd service started."
    echo "Run 'systemctl status mcharbor-agent' to check status."
  else
    # No systemd — run directly in background
    MCHARBOR_URL="$MCHARBOR_URL" \
    MCHARBOR_AGENT_TOKEN="$MCHARBOR_AGENT_TOKEN" \
    MCHARBOR_FORCE_TLS="$MCHARBOR_FORCE_TLS" \
    MCHARBOR_INSECURE="$MCHARBOR_INSECURE" \
    MCHARBOR_PIN="$MCHARBOR_PIN" \
    DOCKER_HOST=unix:///var/run/docker.sock \
    LOG_LEVEL=info \
    MCHARBOR_TRANSFER_LISTEN="$MCHARBOR_TRANSFER_LISTEN" \
    MCHARBOR_TRANSFER_ADVERTISE_URL="$MCHARBOR_TRANSFER_ADVERTISE_URL" \
    MCHARBOR_COMPOSE_DIR="$MCHARBOR_COMPOSE_DIR" \
    MCHARBOR_DATA_DIR="$MCHARBOR_DATA_DIR" \
    nohup /usr/local/bin/mcharbor-agent > /var/log/mcharbor-agent.log 2>&1 &
    echo ""
    echo "Agent binary installed and started in background."
    echo "Check /var/log/mcharbor-agent.log for output."
  fi
fi

echo ""
echo "Done! The agent will connect to $MCHARBOR_URL automatically."
`, shellEscape(serverURL), shellEscape(agentToken), forceTLSLine, insecureLine, pinLine, caLine, methodLine, transferScheme)

	return script
}

// hashToken creates a SHA-256 hash of a token for storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
