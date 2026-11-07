#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -euo pipefail

channel="${1:-}"
file="${2:-VERSION}"

core='[0-9]{1,9}\.[0-9]{1,9}\.[0-9]{1,9}'
meta='(\+[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?'
stable_re="^${core}${meta}\$"
pre_re="^${core}-[0-9A-Za-z]+(\\.[0-9A-Za-z]+)*${meta}\$"

case "$channel" in
  stable | prerelease) ;;
  *)
    echo "usage: assert-version-channel.sh stable|prerelease [VERSION file]" >&2
    exit 2
    ;;
esac

if [[ ! -f "$file" || ! -r "$file" ]]; then
  echo "::error::Cannot read the version file." >&2
  exit 2
fi

lines="$(LC_ALL=C grep -c '' "$file" || true)"
if [[ "$lines" -gt 1 ]]; then
  echo "::error::The version file must hold exactly one line." >&2
  exit 1
fi

if ! tr -d '\000' < "$file" | cmp -s - "$file"; then
  echo "::error::The version file holds a NUL byte." >&2
  exit 1
fi

IFS= read -r raw < "$file" || true
version="${raw%$'\r'}"

if [[ "${#version}" -gt 64 ]] || ! LC_ALL=C grep -qE '^[0-9A-Za-z.+-]+$' <<<"$version"; then
  echo "::error::The version file does not hold a version." >&2
  exit 1
fi

is_stable=0
is_pre=0
[[ "$version" =~ $stable_re ]] && is_stable=1
[[ "$version" =~ $pre_re ]] && is_pre=1

if [[ "$is_stable" -eq 0 && "$is_pre" -eq 0 ]]; then
  echo "::error::VERSION ${version} is not a McHarbor version." >&2
  exit 1
fi

if [[ "$channel" == "stable" && "$is_stable" -ne 1 ]]; then
  echo "::error::VERSION ${version} is a prerelease; this workflow publishes stable versions only." >&2
  exit 1
fi

if [[ "$channel" == "prerelease" && "$is_pre" -ne 1 ]]; then
  echo "::error::VERSION ${version} is not a prerelease; the prerelease workflow needs a - suffix such as -rc.1." >&2
  exit 1
fi

printf '%s\n' "$version"
