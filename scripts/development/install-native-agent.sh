#!/bin/bash
# Install the McHarbor agent binary on this host as a systemd service.
#
# Run it on the target host, as a user with sudo, with the binary
# mcharbor-agent-linux-amd64 next to it (or pass the path as $1).
#
#   scp agents/mcharbor-agent-linux-amd64 scripts/development/install-native-agent.sh host:
#   ssh host 'chmod +x install-native-agent.sh && ./install-native-agent.sh'
#
# Settings are taken from, in order: an existing mcharbor-agent systemd unit, a
# container named mcharbor-agent, then the environment. That makes the script
# re-runnable for upgrades — the earlier homelab script could only read a
# container, so it stopped working once the container was gone.
#
# It is idempotent: run it again to upgrade the binary in place.

set -euo pipefail

BINARY="${1:-./mcharbor-agent-linux-amd64}"
DATA_DIR=/var/lib/mcharbor-agent
COMPOSE_DIR="$DATA_DIR/compose"
COMPUTE_ROOT=/var/lib/mcharbor/compute
UNIT=/etc/systemd/system/mcharbor-agent.service

if [ ! -f "$BINARY" ]; then
  echo "Binary not found: $BINARY" >&2
  echo "Copy agents/mcharbor-agent-linux-amd64 next to this script, or pass its path." >&2
  exit 1
fi

echo "== 1. Settings =="

from_unit() {
  [ -f "$UNIT" ] || return 0
  sudo sed -n "s/^Environment=$1=//p" "$UNIT" | head -1
}

from_container() {
  docker inspect mcharbor-agent >/dev/null 2>&1 || return 0
  docker inspect mcharbor-agent --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | sed -n "s/^$1=//p" | head -1
}

resolve() {
  local name="$1" value
  value="$(from_unit "$name")"
  [ -n "$value" ] || value="$(from_container "$name")"
  [ -n "$value" ] || value="${!name:-}"
  printf '%s' "$value"
}

MCHARBOR_URL="$(resolve MCHARBOR_URL)"
MCHARBOR_AGENT_TOKEN="$(resolve MCHARBOR_AGENT_TOKEN)"
MCHARBOR_FORCE_TLS="$(resolve MCHARBOR_FORCE_TLS)"
MCHARBOR_INSECURE="$(resolve MCHARBOR_INSECURE)"
MCHARBOR_TRANSFER_ADVERTISE_URL="$(resolve MCHARBOR_TRANSFER_ADVERTISE_URL)"

if [ -z "$MCHARBOR_URL" ] || [ -z "$MCHARBOR_AGENT_TOKEN" ]; then
  echo "Could not determine the server URL and agent token." >&2
  echo "Re-run with them set, e.g.:" >&2
  echo "  MCHARBOR_URL=https://mcharbor.lan:8705 MCHARBOR_AGENT_TOKEN=... ./install-native-agent.sh" >&2
  exit 1
fi
echo "   server: $MCHARBOR_URL"

echo "== 2. Host packages for guest hosting =="
if command -v apt-get >/dev/null 2>&1; then
  sudo apt-get update -qq
  sudo apt-get install -y -qq qemu-system-x86 qemu-utils ovmf swtpm
else
  echo "   not apt — install qemu-system-x86, qemu-utils, ovmf and swtpm yourself"
fi

# The agent creates /var/lib/mcharbor/compute at startup and fails hard if it
# cannot. Creating it here, owned by the user the unit runs as, is what keeps
# "mkdir /var/lib/mcharbor/compute/vm: permission denied" from coming back.
echo "== 3. Directories =="
sudo mkdir -p "$DATA_DIR" "$COMPOSE_DIR" "$COMPUTE_ROOT/images" "$COMPUTE_ROOT/vm" "$COMPUTE_ROOT/ct"
sudo chmod 0700 "$COMPUTE_ROOT"
sudo chown -R root:root /var/lib/mcharbor "$DATA_DIR"
echo "   $COMPUTE_ROOT ready"

# A fresh instance id claims a second licence seat, so carry the old one over
# when a previous install or container left one behind.
if [ ! -f "$DATA_DIR/instance-id" ]; then
  VOLUME_PATH="$(docker volume inspect mcharbor-agent-data --format '{{.Mountpoint}}' 2>/dev/null || true)"
  if [ -n "$VOLUME_PATH" ] && sudo test -f "$VOLUME_PATH/instance-id"; then
    sudo cp "$VOLUME_PATH/instance-id" "$DATA_DIR/instance-id"
    echo "   instance id carried over from the container volume"
  fi
fi

echo "== 4. Binary =="
sudo systemctl stop mcharbor-agent 2>/dev/null || true
sudo install -m 0755 "$BINARY" /usr/local/bin/mcharbor-agent
if [ ! -e /usr/local/bin/mcharbor ] || [ "$(readlink /usr/local/bin/mcharbor)" = mcharbor-agent ]; then
  sudo ln -sfn mcharbor-agent /usr/local/bin/mcharbor
fi
/usr/local/bin/mcharbor-agent --version 2>/dev/null || true

echo "== 5. systemd unit =="
sudo tee "$UNIT" >/dev/null <<UNITFILE
[Unit]
Description=McHarbor Remote Agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
Environment=MCHARBOR_URL=$MCHARBOR_URL
Environment=MCHARBOR_AGENT_TOKEN=$MCHARBOR_AGENT_TOKEN
Environment=MCHARBOR_FORCE_TLS=${MCHARBOR_FORCE_TLS:-1}
Environment=MCHARBOR_INSECURE=${MCHARBOR_INSECURE:-true}
Environment=MCHARBOR_TRANSFER_ADVERTISE_URL=$MCHARBOR_TRANSFER_ADVERTISE_URL
Environment=MCHARBOR_DATA_DIR=$DATA_DIR
Environment=MCHARBOR_COMPOSE_DIR=$COMPOSE_DIR
Environment=MCHARBOR_TRANSFER_LISTEN=0.0.0.0:8788
Environment=MCHARBOR_OFFLINE_BUFFER=true
Environment=DOCKER_HOST=unix:///var/run/docker.sock
Environment=LOG_LEVEL=info
Environment="GODEBUG=cpu.avx512f=off,cpu.avx512vl=off"
ExecStart=/usr/local/bin/mcharbor-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNITFILE
sudo chmod 0600 "$UNIT"
if grep -qw avx512f /proc/cpuinfo 2>/dev/null; then
    for unit in containerd docker; do
        sudo systemctl cat "$unit.service" >/dev/null 2>&1 || continue
        sudo mkdir -p "/etc/systemd/system/$unit.service.d"
        printf '[Service]\nEnvironment="GODEBUG=cpu.avx512f=off,cpu.avx512vl=off"\n' | sudo tee "/etc/systemd/system/$unit.service.d/mcharbor-go-avx512.conf" >/dev/null
    done
fi
sudo systemctl daemon-reload

# A container agent holding the same token would fight this one for the
# environment, so stand it down without destroying it.
if docker inspect mcharbor-agent >/dev/null 2>&1; then
  echo "== 6. Standing down the container agent (kept, not removed) =="
  docker rename mcharbor-agent mcharbor-agent-pre-native 2>/dev/null || true
  docker stop mcharbor-agent-pre-native >/dev/null 2>&1 || true
fi

echo "== 7. Starting =="
sudo systemctl enable --now mcharbor-agent
sleep 4
sudo systemctl --no-pager --lines=20 status mcharbor-agent || true

echo
echo "Done. In McHarbor, Compute -> Hosts should report this host with"
echo "kvm_device, qemu, qemu_img and systemd green."
echo
echo "Logs:     sudo journalctl -u mcharbor-agent -f"
echo "Rollback: sudo systemctl disable --now mcharbor-agent"
echo "          docker rename mcharbor-agent-pre-native mcharbor-agent && docker start mcharbor-agent"
