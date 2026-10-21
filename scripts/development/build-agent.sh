#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${AGENT_OUT:-$ROOT/agents}"
BUILD_STAMP="$(tr -d '[:space:]' < "$ROOT/BUILD")"

usage() {
  cat <<'USAGE'
build-agent.sh [--cross] [--linux-arm64] [--out DIR]

  (default)      linux/amd64 through the Dockerfile's agentexport stage: CGO
                 against musl with seccomp, statically linked — the same bytes
                 the image serves from GET /api/agent/binary/linux/amd64.
  --cross        adds pure-Go builds for darwin/* and windows/amd64.
  --linux-arm64  adds linux/arm64 through the same Docker stage under QEMU
                 emulation. Slow, and it is the only way to get one: every
                 linux build needs CGO, because runc's nsenter is a C
                 constructor with no pure-Go fallback.

Linux is deliberately absent from --cross for that reason — CGO_ENABLED=0 with
GOOS=linux fails outright on libcontainer/nsenter rather than producing a
lesser binary.

Binaries land in agent/ at the repo root, named mcharbor-agent-<os>-<arch> to
match AgentBinaryPath in src/backend/modules/agent/binary.go.

SHA256SUMS over BUILD and the binaries is written beside them, and signed into
SHA256SUMS.sig when signing/agent-signing.key (or $AGENT_SIGNING_KEY_FILE)
exists. That directory is then a valid <version>/ folder for an agent release
mirror; McHarbor itself serves agents/ as a built-in agent without checking it.

The release is then copied into agents/catalog/<version>/<build>/ and
agents/catalog/index.json is rewritten to list every build there (publish-agent-catalog.mjs); upload
agents/catalog/ as-is to https://www.mcharbor.eu/catalog/agents/.
USAGE
}

CROSS=0
LINUX_ARM64=0
while [ $# -gt 0 ]; do
  case "$1" in
    --cross) CROSS=1; shift ;;
    --linux-arm64) LINUX_ARM64=1; shift ;;
    --out) OUT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

mkdir -p "$OUT"

echo "Building agent linux/amd64 (seccomp, static) -> $OUT/"
docker buildx build --target agentexport \
  --output "type=local,dest=$OUT" \
  -f "$ROOT/docker/Dockerfile" "$ROOT"

if [ "$LINUX_ARM64" = "1" ]; then
  echo "Building agent linux/arm64 (seccomp, static, emulated) -> $OUT/"
  stage="$(mktemp -d)"
  trap 'rm -rf "$stage"' EXIT
  docker buildx build --platform linux/arm64 --target agentexport \
    --output "type=local,dest=$stage" \
    -f "$ROOT/docker/Dockerfile" "$ROOT"
  # The stage names its output after the architecture it was written for, not
  # the one it was built for, so the emulated build arrives under the amd64
  # name and is renamed here.
  mv "$stage/mcharbor-agent-linux-amd64" "$OUT/mcharbor-agent-linux-arm64"
fi

if [ "$CROSS" = "1" ]; then
  for platform in darwin/amd64 darwin/arm64 windows/amd64; do
    os="${platform%%/*}"
    arch="${platform##*/}"
    out="$OUT/mcharbor-agent-$os-$arch"
    [ "$os" = "windows" ] && out="$out.exe"
    echo "Building agent $os/$arch -> $out"
    (
      cd "$ROOT/src/agent"
      GOWORK=off GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -tags urfave_cli_no_docs \
          -ldflags "-s -w -X main.agentBuild=$BUILD_STAMP" \
          -o "$out" .
    )
  done
fi

# The stamp the binaries were built with, beside them. Settings -> Updates reads
# it to decide whether a connected agent is running something older; without it
# the binaries are still served, they just take no part in that comparison.
printf '%s
' "$BUILD_STAMP" > "$OUT/BUILD"
printf '%s
' "$(tr -d '[:space:]' < "$ROOT/VERSION")" > "$OUT/VERSION"

(
  cd "$OUT"
  rm -f SHA256SUMS SHA256SUMS.sig
  sha256sum BUILD mcharbor-agent-* > SHA256SUMS
)
AGENT_SIGNING_KEY_FILE="${AGENT_SIGNING_KEY_FILE:-$ROOT/signing/agent-signing.key}"
if [ -f "$AGENT_SIGNING_KEY_FILE" ]; then
  echo "Signing SHA256SUMS with $AGENT_SIGNING_KEY_FILE"
  (
    cd "$ROOT/src/backend"
    GOWORK=off GOTOOLCHAIN=auto go run ./cmd/pluginsigner \
      -file "$OUT/SHA256SUMS" -key "$AGENT_SIGNING_KEY_FILE" \
      -trust "$ROOT/signing/agent-signing.pub"
  )
else
  echo "No agent signing key at $AGENT_SIGNING_KEY_FILE: SHA256SUMS is unsigned."
  echo "McHarbor serves these binaries as built-in; a mirror would need them signed."
fi

AGENT_OUT="$OUT" node "$ROOT/scripts/development/publish-agent-catalog.mjs"

echo
ls -la "$OUT"
echo
echo "Build stamp: $BUILD_STAMP"
