#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${CLI_OUT:-$ROOT/cli}"
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"
BUILD_STAMP="$(tr -d '[:space:]' < "$ROOT/BUILD")"
PLATFORMS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"

usage() {
  cat <<'USAGE'
build-cli.sh [--out DIR] [--only OS/ARCH]

Builds the mcharbor CLI for linux/{amd64,arm64}, darwin/{amd64,arm64} and
windows/amd64, stamped with the repo's VERSION and BUILD, and packs each into a
release archive with shell completions and man pages:

  mcharbor_<version>_<os>_<arch>.tar.gz   (zip for windows)
  mcharbor_<version>_checksums.txt

The CLI is pure Go, so every target cross-compiles here with CGO_ENABLED=0.
Output lands in cli/ at the repo root. Nothing is published.
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    --only) PLATFORMS="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

PKG="github.com/therealmcsparrow/mcharbor-cli/internal/cli"
LDFLAGS="-s -w -X $PKG.Version=$VERSION -X $PKG.Build=$BUILD_STAMP"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$OUT"

cd "$ROOT/src/cli"
export GOWORK=off GOTOOLCHAIN=auto

HOST_BIN="$WORK/mcharbor-host"
CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$HOST_BIN" .
mkdir -p "$WORK/share/man" "$WORK/share/completions"
"$HOST_BIN" docs man "$WORK/share/man"
"$HOST_BIN" completion bash > "$WORK/share/completions/mcharbor.bash"
"$HOST_BIN" completion zsh > "$WORK/share/completions/_mcharbor"
"$HOST_BIN" completion fish > "$WORK/share/completions/mcharbor.fish"
"$HOST_BIN" completion powershell > "$WORK/share/completions/mcharbor.ps1"

for target in $PLATFORMS; do
  os="${target%/*}"
  arch="${target#*/}"
  name="mcharbor_${VERSION}_${os}_${arch}"
  stage="$WORK/$name"
  rm -f "$OUT/$name.tar.gz" "$OUT/$name.zip"
  mkdir -p "$stage"
  bin="mcharbor"
  [ "$os" = "windows" ] && bin="mcharbor.exe"
  echo "Building $target -> $OUT/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" -o "$stage/$bin" .
  cp -r "$WORK/share/completions" "$stage/completions"
  if [ "$os" != "windows" ]; then
    cp -r "$WORK/share/man" "$stage/man"
    tar -C "$WORK" -czf "$OUT/$name.tar.gz" "$name"
  else
    if command -v zip >/dev/null 2>&1; then
      (cd "$WORK" && zip -qr "$OUT/$name.zip" "$name")
    else
      (cd "$WORK" && python - "$name" "$(cygpath -m "$OUT/$name.zip" 2>/dev/null || echo "$OUT/$name.zip")" <<'PY'
import os, sys, zipfile
root, dest = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(dest, "w", zipfile.ZIP_DEFLATED) as z:
    for base, _, files in os.walk(root):
        for f in files:
            path = os.path.join(base, f)
            z.write(path, path.replace(os.sep, "/"))
PY
      )
    fi
  fi
done

rm -f "$OUT/SHA256SUMS"
(cd "$OUT" && sha256sum mcharbor_*.tar.gz mcharbor_*.zip > "mcharbor_${VERSION}_checksums.txt")
echo "$VERSION" > "$OUT/VERSION"
echo "$BUILD_STAMP" > "$OUT/BUILD"
echo "Done: $(ls "$OUT" | grep -cE '^mcharbor_.*\.(tar\.gz|zip)$') archives in $OUT (version $VERSION, build $BUILD_STAMP)"
