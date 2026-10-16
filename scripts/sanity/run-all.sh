#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Runs the McHarbor sanity suite from Git Bash / WSL / a POSIX shell.
# Arguments are passed through to run-all.ps1, e.g.:
#   ./scripts/sanity/run-all.sh -Check security,permissions -SkipNetwork
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$here/run-all.ps1"

if command -v pwsh >/dev/null 2>&1; then
  exec pwsh -NoProfile -ExecutionPolicy Bypass -File "$script" "$@"
elif command -v powershell.exe >/dev/null 2>&1; then
  if command -v cygpath >/dev/null 2>&1; then script="$(cygpath -w "$script")"; fi
  exec powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$script" "$@"
elif command -v powershell >/dev/null 2>&1; then
  exec powershell -NoProfile -ExecutionPolicy Bypass -File "$script" "$@"
else
  echo "run-all.sh: neither pwsh nor powershell.exe found on PATH" >&2
  exit 2
fi
