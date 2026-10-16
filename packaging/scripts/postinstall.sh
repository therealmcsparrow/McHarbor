#!/bin/sh
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -e

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

mkdir -p /var/lib/mcharbor
chown -R mcharbor:mcharbor /var/lib/mcharbor
chmod 750 /var/lib/mcharbor

kind=tarball
if command -v dpkg >/dev/null 2>&1 && dpkg -s mcharbor >/dev/null 2>&1; then
    kind=deb
elif command -v rpm >/dev/null 2>&1 && rpm -q mcharbor >/dev/null 2>&1; then
    kind=rpm
elif command -v apk >/dev/null 2>&1 && apk info -e mcharbor >/dev/null 2>&1; then
    kind=apk
fi
echo "MCHARBOR_INSTALL_KIND=${kind}" > /etc/mcharbor/install-kind.env
pin_encryption_key

if command -v systemctl >/dev/null 2>&1; then
    apply_run_mode "${MCHARBOR_RUN_AS_ROOT:-}"
    protect_go_runtimes
    systemctl daemon-reload
    systemctl enable mcharbor.service
    systemctl restart mcharbor.service
fi
