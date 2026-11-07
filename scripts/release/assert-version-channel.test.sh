#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../.." && pwd)"
guard="$here/assert-version-channel.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

pass=0
fail=0

check() {
  local name="$1" want="$2" channel="$3" content="$4"
  local f="$work/VERSION"
  printf '%b' "$content" > "$f"
  bash "$guard" "$channel" "$f" >"$work/out" 2>"$work/err"
  local got=$?
  if [[ "$got" -eq "$want" ]]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    echo "FAIL $name: want exit $want, got $got" >&2
  fi
}

wired() {
  local name="$1" file="$2" pattern="$3"
  if grep -qE -e "$pattern" "$root/$file"; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    echo "FAIL $name: $file lacks /$pattern/" >&2
  fi
}

absent() {
  local name="$1" file="$2" pattern="$3"
  if grep -qE -e "$pattern" "$root/$file"; then
    fail=$((fail + 1))
    echo "FAIL $name: $file contains /$pattern/" >&2
  else
    pass=$((pass + 1))
  fi
}

check "traversal in stable" 1 stable '../../etc\n'
check "traversal in prerelease" 1 prerelease '2026.11.02-../x\n'
check "dotdot suffix" 1 prerelease '2026.11.02-rc..1\n'
check "command substitution" 1 prerelease '2026.11.02-$(id)\n'
check "backtick" 1 stable '`id`\n'
check "semicolon" 1 prerelease '2026.11.02-rc.1;rm\n'
check "newline smuggles a suffix" 1 stable '2026.11.01\n-rc.1\n'
check "second line" 1 prerelease '2026.11.02-rc.1\nx\n'
check "NUL byte" 1 stable '2026.11.01\0-rc.1\n'
check "inner space" 1 stable '2026.11.01 -rc.1\n'
check "leading space" 1 stable ' 2026.11.01\n'
check "empty file" 1 stable ''
check "blank line" 1 prerelease '\n'
check "only suffix" 1 prerelease '-rc.1\n'
check "dangling dash" 1 prerelease '2026.11.02-\n'
check "empty identifier" 1 prerelease '2026.11.02-rc.\n'
check "two dashes" 1 prerelease '2026.11.02-rc-1\n'
check "non-ascii digit" 1 stable '٢٠٢٦.11.01\n'
check "v prefix" 1 stable 'v2026.11.01\n'
check "two parts" 1 stable '2026.11\n'
check "four parts" 1 stable '2026.11.01.1\n'
check "over 64 chars" 1 prerelease '2026.11.02-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n'
check "suffix on a stable gate" 1 stable '2026.11.02-rc.1\n'
check "alpha suffix on a stable gate" 1 stable '2026.11.02-alpha\n'
check "stable on the prerelease gate" 1 prerelease '2026.11.01\n'
check "metadata only on the prerelease gate" 1 prerelease '2026.11.01+build.5\n'
check "unknown channel" 2 latest '2026.11.01\n'

check "stable" 0 stable '2026.11.01\n'
check "stable without newline" 0 stable '2026.11.01'
check "stable with CRLF" 0 stable '2026.11.01\r\n'
check "stable leading zeros" 0 stable '2026.09.01\n'
check "stable with metadata" 0 stable '2026.11.01+build.5\n'
check "prerelease rc" 0 prerelease '2026.11.02-rc.1\n'
check "prerelease numeric" 0 prerelease '2026.11.02-rc.10\n'
check "prerelease word" 0 prerelease '2026.11.02-beta\n'
check "prerelease with metadata" 0 prerelease '2026.11.02-rc.1+build.5\n'

bash "$guard" stable "$work/missing" >/dev/null 2>&1
if [[ $? -eq 2 ]]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL missing file" >&2; fi

printf '2026.11.02-rc.1\n' > "$work/VERSION"
out="$(bash "$guard" prerelease "$work/VERSION" 2>/dev/null)"
if [[ "$out" == "2026.11.02-rc.1" ]]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL stdout carries the version" >&2; fi

guarded='scripts/release/assert-version-channel.sh stable'
for wf in prod release alpha beta promote catalog; do
  wired "$wf runs the stable guard" ".github/workflows/$wf.yml" "$guarded"
done
wired "prerelease runs the prerelease guard" .github/workflows/prerelease.yml 'assert-version-channel.sh prerelease'
absent "prerelease has no stable guard" .github/workflows/prerelease.yml 'assert-version-channel.sh stable'
absent "prerelease has no latest tag" .github/workflows/prerelease.yml 'value=latest|:latest|[^-]latest[^.]|--latest($|[^=])'
absent "prerelease has no release tag" .github/workflows/prerelease.yml 'mcharbor(-agent)?:release'
wired "prerelease marks the release" .github/workflows/prerelease.yml '--prerelease --latest=false'
wired "prerelease scans before pushing" .github/workflows/prerelease.yml 'TRIVY_IMAGE'
absent "prerelease pins govulncheck" .github/workflows/prerelease.yml 'govulncheck@latest'
absent "prerelease keeps the docker socket out" .github/workflows/prerelease.yml 'docker\.sock'
absent "prerelease never interpolates in run" .github/workflows/prerelease.yml 'version="\$\{\{'

guard_line() { grep -n "$1" "$root/$2" | head -n1 | cut -d: -f1; }
for wf in prod release alpha beta; do
  g="$(guard_line 'assert-version-channel.sh stable' ".github/workflows/$wf.yml")"
  p="$(guard_line 'docker/build-push-action' ".github/workflows/$wf.yml")"
  if [[ -n "$g" && -n "$p" && "$g" -lt "$p" ]]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL $wf: guard does not precede the first publish step" >&2; fi
done

g="$(guard_line 'assert-version-channel.sh prerelease' .github/workflows/prerelease.yml)"
p="$(guard_line 'build-push-action' .github/workflows/prerelease.yml)"
if [[ -n "$g" && -n "$p" && "$g" -lt "$p" ]]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL prerelease: guard does not precede the first build" >&2; fi

echo "passed $pass, failed $fail"
[[ "$fail" -eq 0 ]]
