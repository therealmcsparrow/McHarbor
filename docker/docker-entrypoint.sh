#!/bin/bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

set -e

PUID=${PUID:-1000}
PGID=${PGID:-1000}
DATA_DIR=${DATA_DIR:-/app/data}

# Ensure data directory exists
mkdir -p "$DATA_DIR"
mkdir -p "$DATA_DIR/tls"

# Detect Docker socket
if [ -S /var/run/docker.sock ]; then
    echo "Docker socket detected at /var/run/docker.sock"
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)
    echo "Docker socket GID: $DOCKER_GID"
fi

# Set permissions on data directory
chown -R "$PUID:$PGID" "$DATA_DIR" 2>/dev/null || true

# Auto-generate secret if not set
if [ -z "$MCHARBOR_SECRET" ]; then
    if [ ! -f "$DATA_DIR/.secret" ]; then
        head -c 32 /dev/urandom | base64 > "$DATA_DIR/.secret"
        echo "Generated new session secret"
    fi
    export MCHARBOR_SECRET=$(cat "$DATA_DIR/.secret")
fi

# Default database path
export DATABASE_PATH="${DATABASE_PATH:-$DATA_DIR/mcharbor.db}"

# Default Docker socket
export DOCKER_HOST="${DOCKER_HOST:-unix:///var/run/docker.sock}"

# Log TLS cert status
if [ -f "$DATA_DIR/tls/cert.pem" ] && [ -f "$DATA_DIR/tls/key.pem" ]; then
    echo "TLS certificates found in $DATA_DIR/tls/"
else
    echo "No TLS certificates found (HTTP mode)"
fi

echo "Starting McHarbor..."
echo "  Port: ${PORT:-5474}"
echo "  Database: ${DATABASE_PATH}"
echo "  Docker: ${DOCKER_HOST}"

exec "$@"
