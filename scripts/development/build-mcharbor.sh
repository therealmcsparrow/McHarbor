#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${1:-$ROOT/dist/payload}"
GO_BASE=golang:1.27.2-bookworm

rm -rf "$OUT"
mkdir -p "$OUT"

docker buildx build \
    --target payload \
    --build-arg "GO_BASE=$GO_BASE" \
    --output "type=local,dest=$OUT" \
    -f "$ROOT/docker/Dockerfile" \
    "$ROOT"


if [ "${MCHARBOR_TARBALL:-0}" = "1" ]; then
    version="$(tr -d '[:space:]' < "$ROOT/VERSION")"
    mkdir -p "$ROOT/dist/packages"
    tarball="$ROOT/dist/packages/mcharbor-$version-linux-amd64.tar.gz"
    stage="$(mktemp -d)"
    cleanup() {
        rm -rf "$stage"
        rm -f "$tarball.partial"
    }
    trap cleanup EXIT
    mkdir -p "$stage/mcharbor/payload"
    cp -a "$OUT/." "$stage/mcharbor/payload/"
    cp "$ROOT/packaging/tarball/install.sh" "$stage/mcharbor/install.sh"
    cp "$ROOT/packaging/mcharbor.env" "$stage/mcharbor/mcharbor.env"
    cp "$ROOT/packaging/systemd/mcharbor.service" "$stage/mcharbor/mcharbor.service"
    chmod 0755 "$stage/mcharbor/install.sh"
    tar -C "$stage" -czf "$tarball.partial" mcharbor
    mv "$tarball.partial" "$tarball"
fi

echo "payload ready at $OUT"
