#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
    cat <<'USAGE'
install.sh [--uninstall] [--purge] [--run-as-root | --no-run-as-root]

  (default)          installs McHarbor to /opt/mcharbor and starts the service
  --uninstall        stops the service and removes /opt/mcharbor, keeping data
  --purge            with --uninstall, also removes /var/lib/mcharbor and /etc/mcharbor
  --no-run-as-root   runs the service as the mcharbor account, sandboxed to
                     /var/lib/mcharbor (also MCHARBOR_RUN_AS_ROOT=0); a later install
                     keeps the choice. The default is root.
  --run-as-root      returns the service to root (MCHARBOR_RUN_AS_ROOT=1)
USAGE
}

UNINSTALL=0
PURGE=0
RUN_AS_ROOT="${MCHARBOR_RUN_AS_ROOT:-}"
while [ $# -gt 0 ]; do
    case "$1" in
        --uninstall) UNINSTALL=1; shift ;;
        --purge) PURGE=1; shift ;;
        --run-as-root) RUN_AS_ROOT=1; shift ;;
        --no-run-as-root) RUN_AS_ROOT=0; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage; exit 2 ;;
    esac
done

apply_run_mode() {
    dropin_dir=/etc/systemd/system/mcharbor.service.d
    dropin="$dropin_dir/run-as-mcharbor.conf"
    rm -f "$dropin_dir/run-as-root.conf"
    case "${1:-}" in
        0)
            mkdir -p "$dropin_dir"
            cat > "$dropin" <<'UNIT'
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
[Service]
User=mcharbor
Group=mcharbor
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/mcharbor
UNIT
            echo "McHarbor runs as the mcharbor account, sandboxed to /var/lib/mcharbor."
            echo "  A local storage path elsewhere needs a ReadWritePaths= drop-in and mcharbor ownership."
            echo "  Run it as root again with --run-as-root (packages: MCHARBOR_RUN_AS_ROOT=1)."
            ;;
        1)
            if [ -f "$dropin" ]; then
                rm -f "$dropin"
                echo "McHarbor runs as root again."
            fi
            rmdir "$dropin_dir" 2>/dev/null || true
            ;;
        *)
            if [ -f "$dropin" ]; then
                echo "McHarbor runs as the mcharbor account ($dropin). Run it as root with --run-as-root."
            else
                rmdir "$dropin_dir" 2>/dev/null || true
                echo "McHarbor runs as root. Install with --no-run-as-root to run it as the sandboxed mcharbor account."
            fi
            ;;
    esac
}

protect_go_runtimes() {
    grep -qw avx512f /proc/cpuinfo 2>/dev/null || return 0
    command -v systemctl >/dev/null 2>&1 || return 0
    for unit in containerd docker; do
        systemctl cat "$unit.service" >/dev/null 2>&1 || continue
        mkdir -p "/etc/systemd/system/$unit.service.d"
        printf '[Service]\nEnvironment="GODEBUG=cpu.avx512f=off,cpu.avx512vl=off"\n' > "/etc/systemd/system/$unit.service.d/mcharbor-go-avx512.conf"
    done
    systemctl daemon-reload
    echo "AVX-512 CPU: Docker and containerd will start with GODEBUG=cpu.avx512f=off,cpu.avx512vl=off from their next restart."
}

pin_encryption_key() {
    key_env=/etc/mcharbor/encryption-key.env
    if [ -f "$key_env" ]; then
        return 0
    fi
    if grep -qE '^(ENCRYPTION_KEY|ENCRYPTION_KEY_FILE)=.' /etc/mcharbor/mcharbor.env 2>/dev/null; then
        return 0
    fi
    key_file=/var/lib/mcharbor/.encryption_key
    if [ -f "$key_file" ]; then
        if [ "$(wc -c < "$key_file" | tr -d ' ')" != "32" ]; then
            echo "warning: $key_file is not a 32-byte key; ENCRYPTION_KEY was not pinned" >&2
            return 0
        fi
        key="$(base64 < "$key_file" | tr -d '\n')"
    else
        key="$(head -c 32 /dev/urandom | base64 | tr -d '\n')"
    fi
    old_umask="$(umask)"
    umask 077
    printf 'ENCRYPTION_KEY=%s\n' "$key" > "$key_env"
    umask "$old_umask"
    chmod 600 "$key_env"
    echo "Data encryption key pinned in $key_env. Back that file up: without it, stored secrets cannot be decrypted."
}

if [ "$(id -u)" -ne 0 ]; then
    echo "install.sh must run as root" >&2
    exit 1
fi

if [ "$PURGE" = "1" ] && [ "$UNINSTALL" != "1" ]; then
    echo "--purge requires --uninstall" >&2
    usage
    exit 2
fi

if [ "$UNINSTALL" = "1" ]; then
    systemctl stop mcharbor.service 2>/dev/null || true
    systemctl disable mcharbor.service 2>/dev/null || true
    rm -f /usr/lib/systemd/system/mcharbor.service /etc/mcharbor/install-kind.env
    rm -f /etc/systemd/system/mcharbor.service.d/run-as-root.conf /etc/systemd/system/mcharbor.service.d/run-as-mcharbor.conf
    rm -rf /opt/mcharbor
    systemctl daemon-reload 2>/dev/null || true
    if [ "$PURGE" = "1" ]; then
        rm -rf /var/lib/mcharbor /etc/mcharbor
        userdel mcharbor 2>/dev/null || deluser mcharbor 2>/dev/null || true
    fi
    echo "McHarbor removed"
    exit 0
fi

if ! getent group mcharbor >/dev/null 2>&1; then
    if command -v groupadd >/dev/null 2>&1; then
        groupadd --system mcharbor
    else
        addgroup -S mcharbor
    fi
fi
if ! getent passwd mcharbor >/dev/null 2>&1; then
    if command -v useradd >/dev/null 2>&1; then
        useradd --system --gid mcharbor --home-dir /var/lib/mcharbor \
            --shell /sbin/nologin --comment "McHarbor service account" mcharbor
    else
        adduser -S -D -H -h /var/lib/mcharbor -s /sbin/nologin -G mcharbor mcharbor
    fi
fi
if getent group docker >/dev/null 2>&1; then
    if command -v usermod >/dev/null 2>&1; then
        usermod --append --groups docker mcharbor
    else
        addgroup mcharbor docker
    fi
fi

rm -rf /opt/mcharbor
mkdir -p /opt/mcharbor /etc/mcharbor /var/lib/mcharbor
cp -a "$HERE/payload/." /opt/mcharbor/
chmod 0755 /opt/mcharbor/mcharbor

if [ ! -f /etc/mcharbor/mcharbor.env ]; then
    cp "$HERE/mcharbor.env" /etc/mcharbor/mcharbor.env
fi
echo "MCHARBOR_INSTALL_KIND=tarball" > /etc/mcharbor/install-kind.env
pin_encryption_key

chown -R mcharbor:mcharbor /var/lib/mcharbor
chmod 750 /var/lib/mcharbor

if command -v systemctl >/dev/null 2>&1; then
    cp "$HERE/mcharbor.service" /usr/lib/systemd/system/mcharbor.service
    apply_run_mode "$RUN_AS_ROOT"
    protect_go_runtimes
    systemctl daemon-reload
    systemctl enable mcharbor.service
    started="$(date '+%Y-%m-%d %H:%M:%S')"
    systemctl restart mcharbor.service
    echo "McHarbor installed. Service status:"
    systemctl --no-pager status mcharbor.service || true

    token=""
    for _ in $(seq 1 30); do
        token="$(journalctl -u mcharbor.service --since "$started" -o cat --no-pager 2>/dev/null \
            | grep 'first-run setup token' | tail -n 1 | grep -oE '[0-9a-f]{32}' | tail -n 1 || true)"
        [ -n "$token" ] && break
        sleep 1
    done
    echo ""
    if [ -n "$token" ]; then
        echo "Setup token: $token"
        echo "  Open https://$(hostname -I 2>/dev/null | awk '{print $1}'):8705 and paste it into the Setup token field."
        echo "  It changes every time McHarbor restarts; read the current one with:"
        echo "    journalctl -u mcharbor | grep 'setup token' | tail -n 1"
    else
        echo "No setup token was logged. If this instance already has an admin account, none is needed."
        echo "Otherwise read it once the service is up with:"
        echo "  journalctl -u mcharbor | grep 'setup token' | tail -n 1"
    fi
else
    echo "McHarbor installed at /opt/mcharbor, but no systemd was found on this host."
    echo "Supervise it manually: run /opt/mcharbor/mcharbor as the mcharbor user,"
    echo "with working directory /opt/mcharbor and EnvironmentFile /etc/mcharbor/mcharbor.env."
fi
