#!/bin/bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

set -e

PUID=${PUID:-1000}
PGID=${PGID:-1000}
DATA_DIR=${DATA_DIR:-/app/data}

# Detect Docker socket
if [ -S /var/run/docker.sock ]; then
    echo "Docker socket detected at /var/run/docker.sock"
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)
    echo "Docker socket GID: $DOCKER_GID"
fi

mkdir -p "$DATA_DIR"
mkdir -p "$DATA_DIR/tls"

# Set permissions on data directory
chown -R "$PUID:$PGID" "$DATA_DIR" 2>/dev/null || true

# Default database path
export DATABASE_PATH="${DATABASE_PATH:-$DATA_DIR/mcharbor.db}"

# Default Docker socket
export DOCKER_HOST="${DOCKER_HOST:-unix:///var/run/docker.sock}"

# Trust any extra CA certificates the operator has mounted.
#
# /usr/local/share/ca-certificates is the stock location both Debian and Alpine
# scan, so putting a root there and refreshing the bundle is all that is needed:
# Go's crypto/x509, curl, and everything else in the container read the same
# /etc/ssl/certs/ca-certificates.crt afterwards. Nothing in the application has
# to know about it.
#
# This exists because the licence server can legitimately be an internal host
# with a private or self-signed certificate. Go offers no click-through for that
# the way a browser does - it either chains to a trusted root or the request
# fails - so without this the only workaround would be disabling verification,
# which would put an unauthenticated licence handshake on the wire for good.
#
# The mount is expected to be read-only: update-ca-certificates only READS the
# directory and writes the rebuilt bundle to /etc/ssl/certs, which is in the
# image's own writable layer.
#
# An install that mounts nothing here finds no *.crt, skips the refresh, and
# behaves exactly as before - which is the production case.
if ls /usr/local/share/ca-certificates/*.crt >/dev/null 2>&1; then
    extra_ca_count=$(ls -1 /usr/local/share/ca-certificates/*.crt | wc -l)
    if update-ca-certificates >/dev/null 2>&1; then
        echo "Trusted $extra_ca_count extra CA certificate(s) from /usr/local/share/ca-certificates"
    else
        # Not fatal: the server still starts and every public CA keeps working.
        # Only the host whose certificate this was stays untrusted, and that
        # failure is already reported where it happens.
        echo "WARNING: failed to refresh the CA bundle; mounted certificates are NOT trusted" >&2
    fi
fi

# Log TLS cert status
if [ -f "$DATA_DIR/tls/cert.pem" ] && [ -f "$DATA_DIR/tls/key.pem" ]; then
    echo "TLS certificates found in $DATA_DIR/tls/"
else
    echo "No TLS certificates found (HTTP mode)"
fi

echo "Starting McHarbor..."
echo "  Port: ${PORT:-8705}"
echo "  Database: ${DATABASE_PATH}"
echo "  Docker: ${DOCKER_HOST}"

exec "$@"
