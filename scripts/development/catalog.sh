#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

set -uo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="${MCHARBOR_REPO_ROOT:-$(cd "${script_dir}/../.." && pwd)}"
PLUGIN_LIST="${MCHARBOR_PLUGIN_LIST:-${script_dir}/plugins.list}"
CATALOG_DIR="${MCHARBOR_PLUGIN_OUTPUT_DIR:-${REPO_ROOT}/mnt/plugin-catalog}"
export MCHARBOR_PLUGIN_OUTPUT_DIR="${CATALOG_DIR}"
CATALOG_URL="${MCHARBOR_CATALOG_URL:-file:///mnt/plugin-catalog}"
SIGNING_KEY="${MCHARBOR_PLUGIN_SIGNING_KEY:-${REPO_ROOT}/signing/plugin-signing.key}"
STALE_WINDOW_MIN="${MCHARBOR_STALE_WINDOW_MIN:-120}"
BUILD_JOBS="${MCHARBOR_CATALOG_JOBS:-6}"
BUILD_TIMEOUT_MIN="${MCHARBOR_CATALOG_TIMEOUT_MIN:-10}"

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    C_RESET=$'\033[0m'; C_DIM=$'\033[2m';  C_BOLD=$'\033[1m'
    C_RED=$'\033[31m';  C_GRN=$'\033[32m'; C_YEL=$'\033[33m'
    C_BLU=$'\033[34m';  C_CYA=$'\033[36m'
else
    C_RESET=''; C_DIM=''; C_BOLD=''; C_RED=''; C_GRN=''; C_YEL=''; C_BLU=''; C_CYA=''
fi

usage() {
    cat <<EOF
${C_BOLD}catalog.sh${C_RESET} - build, sign and index McHarbor's plugin catalog

  Wraps build-plugin.sh with progress you can watch, and a status view that
  says which plugins are already current.

${C_BOLD}USAGE${C_RESET}
  catalog.sh status                 show every plugin's catalog state
  catalog.sh build [--stale|--all]  build plugins (default: --stale)
  catalog.sh build <slug>...        build only the named plugins
  catalog.sh index                  regenerate index.json only

${C_BOLD}OPTIONS${C_RESET}
  --all              build every plugin in plugins.list
  --stale            build only entries the last sweep left behind (default)
  --keep-going       do not stop after the first failure
  --index-anyway     write index.json even when a plugin failed
  --catalog <url>    catalog url baked into each entry
                     (default: ${CATALOG_URL})
  --window <min>     how far behind the newest entry counts as stale
                     (default: ${STALE_WINDOW_MIN})
  -j, --jobs <n>     plugins built at the same time
                     (default: ${BUILD_JOBS}, env MCHARBOR_CATALOG_JOBS)
  --timeout <min>    stop a single plugin build after this long, counted
                     as a failure (default: ${BUILD_TIMEOUT_MIN},
                     env MCHARBOR_CATALOG_TIMEOUT_MIN; 0 = no limit)
  --dry-run          print the resolved settings and the plugins that would
                     be built, then stop
  --no-cache         build without the BuildKit cache (a cache record
                     BuildKit never finished makes every cached build hang)
  -h, --help         this text

${C_BOLD}WHAT "STALE" MEANS${C_RESET}
  An entry left behind by the last build sweep: its stamp is more than
  --window minutes older than the newest entry in the catalog. That is the
  set a partial or interrupted run leaves broken, because the host's
  reconciler only repairs a plugin whose catalog stamp CHANGED - one still
  carrying its old stamp stays unloadable and silent.

  Stamps cannot tell you that a source change invalidated every .so. After
  editing core/plugin/** or core/compute/**, run ${C_BOLD}build --all${C_RESET}.

${C_BOLD}AFTER A BUILD${C_RESET}
  docker compose up -d --force-recreate mcharbor

${C_BOLD}ENVIRONMENT${C_RESET}
  MCHARBOR_PYTHON             interpreter for the catalog entry (stdlib only)
  MCHARBOR_PLUGIN_OUTPUT_DIR  ${CATALOG_DIR}
  MCHARBOR_PLUGIN_SIGNING_KEY ${SIGNING_KEY}
  NO_COLOR                    disable colour
EOF
}

die() { printf '%serror:%s %s\n' "${C_RED}${C_BOLD}" "${C_RESET}" "$*" >&2; exit 1; }
info() { printf '%s==>%s %s\n' "${C_BLU}${C_BOLD}" "${C_RESET}" "$*"; }

host_build() {
    tr -d '[:space:]' < "${REPO_ROOT}/BUILD" 2>/dev/null
}

newest_entry_build() {
    "${PYTHON_BIN}" - "${CATALOG_DIR}" <<'PY' 2>/dev/null
import glob, io, json, os, sys
newest = ""
for path in glob.glob(os.path.join(sys.argv[1], "*.json")):
    if os.path.basename(path) == "index.json":
        continue
    try:
        doc = json.load(io.open(path, encoding="utf-8"))
        build = (doc.get("versions") or [{}])[0].get("build") or ""
    except Exception:
        continue
    if build.isdigit() and build > newest:
        newest = build
print(newest)
PY
}

is_stale() {
    "${PYTHON_BIN}" - "$1" "$2" "${STALE_WINDOW_MIN}" <<'PY' 2>/dev/null
import sys
from datetime import datetime

entry, newest, window = sys.argv[1], sys.argv[2], int(sys.argv[3])
fmt = "%Y%m%d%H%M"
try:
    delta = datetime.strptime(newest, fmt) - datetime.strptime(entry, fmt)
except Exception:
    sys.exit(0)
sys.exit(0 if delta.total_seconds() > window * 60 else 1)
PY
}

entry_build() {
    local slug="$1" file="${CATALOG_DIR}/$1.json"
    [ -f "${file}" ] || { printf 'missing'; return; }
    "${PYTHON_BIN:-python}" - "${file}" <<'PY' 2>/dev/null || printf 'unreadable'
import io, json, sys
try:
    doc = json.load(io.open(sys.argv[1], encoding="utf-8"))
    print((doc.get("versions") or [{}])[0].get("build") or "none")
except Exception:
    print("unreadable")
PY
}

all_slugs() {
    grep -vE '^[[:space:]]*(#|$)' "${PLUGIN_LIST}"
}

pick_python() {
    local candidate
    for candidate in "${MCHARBOR_PYTHON:-}" python3 python py; do
        [ -n "${candidate}" ] || continue
        command -v "${candidate}" >/dev/null 2>&1 || continue
        if "${candidate}" -c 'import json' >/dev/null 2>&1; then
            PYTHON_BIN="${candidate}"
            return 0
        fi
    done
    return 1
}

signing_ready() {
    command -v go >/dev/null 2>&1
}

fmt_duration() {
    local s="$1"
    if [ "${s}" -ge 60 ]; then printf '%dm%02ds' "$((s / 60))" "$((s % 60))"
    else printf '%ds' "${s}"; fi
}

cmd_status() {
    local reference; reference="$(newest_entry_build)"
    local total=0 fresh=0 stale=0 missing=0

    printf '%s%-34s %-16s %s%s\n' "${C_BOLD}" "PLUGIN" "CATALOG BUILD" "STATE" "${C_RESET}"
    printf '%s%s%s\n' "${C_DIM}" "$(printf '%.0s-' $(seq 1 62))" "${C_RESET}"

    local slug build state colour
    while read -r slug; do
        [ -n "${slug}" ] || continue
        total=$((total + 1))
        build="$(entry_build "${slug}")"
        case "${build}" in
            missing|unreadable|none)
                state="MISSING"; colour="${C_RED}"; missing=$((missing + 1)) ;;
            *)
                if [ -n "${reference}" ] && is_stale "${build}" "${reference}"; then
                    state="STALE"; colour="${C_YEL}"; stale=$((stale + 1))
                else
                    state="current"; colour="${C_GRN}"; fresh=$((fresh + 1))
                fi ;;
        esac
        printf '%-34s %-16s %s%s%s\n' "${slug}" "${build}" "${colour}" "${state}" "${C_RESET}"
    done <<< "$(all_slugs)"

    printf '%s%s%s\n' "${C_DIM}" "$(printf '%.0s-' $(seq 1 62))" "${C_RESET}"
    printf '  %s%d current%s   %s%d stale%s   %s%d missing%s   of %d\n' \
        "${C_GRN}" "${fresh}" "${C_RESET}" \
        "${C_YEL}" "${stale}" "${C_RESET}" \
        "${C_RED}" "${missing}" "${C_RESET}" "${total}"
    printf '  newest entry: %s%s%s   host BUILD: %s%s%s\n' "${C_CYA}" "${reference:-unknown}" "${C_RESET}" "${C_CYA}" "$(host_build)" "${C_RESET}"

    if [ -f "${CATALOG_DIR}/index.json" ]; then
        printf '  index.json: %s\n' "$(date -r "${CATALOG_DIR}/index.json" '+%Y-%m-%d %H:%M' 2>/dev/null || echo present)"
    else
        printf '  index.json: %sabsent%s\n' "${C_RED}" "${C_RESET}"
    fi
}

cmd_index() {
    info "regenerating index.json"
    if "${script_dir}/build-plugin-catalog-index.sh"; then
        printf '%s  index.json written%s\n' "${C_GRN}" "${C_RESET}"
        return 0
    fi
    die "indexing failed"
}

cmd_build() {
    local mode="stale" keep_going=0 index_anyway=0 dry_run=0
    local -a selected=()

    while [ $# -gt 0 ]; do
        case "$1" in
            --all) mode="all" ;;
            --stale) mode="stale" ;;
            --keep-going) keep_going=1 ;;
            --index-anyway) index_anyway=1 ;;
            --catalog) shift; CATALOG_URL="${1:-}" ;;
            --window) shift; STALE_WINDOW_MIN="${1:-120}" ;;
            -j|--jobs) shift; BUILD_JOBS="${1:-1}" ;;
            --timeout) shift; BUILD_TIMEOUT_MIN="${1:-0}" ;;
            --no-cache) export MCHARBOR_PLUGIN_NO_CACHE=1 ;;
            --dry-run) dry_run=1 ;;
            -*) die "unknown option: $1" ;;
            *) selected+=("$1"); mode="named" ;;
        esac
        shift
    done

    local -a targets=()
    local reference; reference="$(newest_entry_build)"

    if [ "${mode}" = "named" ]; then
        targets=("${selected[@]}")
    else
        local slug build
        while read -r slug; do
            [ -n "${slug}" ] || continue
            if [ "${mode}" = "all" ]; then
                targets+=("${slug}")
                continue
            fi
            build="$(entry_build "${slug}")"
            case "${build}" in
                missing|unreadable|none) targets+=("${slug}") ;;
                *) if [ -n "${reference}" ] && is_stale "${build}" "${reference}"; then
                       targets+=("${slug}")
                   fi ;;
            esac
        done <<< "$(all_slugs)"
    fi

    local count="${#targets[@]}"
    if [ "${count}" -eq 0 ]; then
        printf '%severything is current - nothing to build%s\n' "${C_GRN}" "${C_RESET}"
        return 0
    fi

    info "building ${C_BOLD}${count}${C_RESET} plugin(s) into ${CATALOG_DIR}"
    printf '  catalog url: %s%s%s\n' "${C_CYA}" "${CATALOG_URL}" "${C_RESET}"
    printf '  signing key: %s\n\n' "${SIGNING_KEY}"

    local log_dir; log_dir="${CATALOG_DIR}/.build-logs"
    mkdir -p "${log_dir}"

    case "${BUILD_JOBS}" in ''|*[!0-9]*|0) die "--jobs needs a positive number, got: ${BUILD_JOBS}" ;; esac
    case "${BUILD_TIMEOUT_MIN}" in ''|*[!0-9]*) die "--timeout needs minutes, got: ${BUILD_TIMEOUT_MIN}" ;; esac
    printf '  jobs: %s, timeout per plugin: %s\n\n' "${BUILD_JOBS}" \
        "$( [ "${BUILD_TIMEOUT_MIN}" -eq 0 ] && echo none || echo "${BUILD_TIMEOUT_MIN}m" )"
    printf '  cache: %s\n' "$( [ "${MCHARBOR_PLUGIN_NO_CACHE:-0}" = 1 ] && echo off || echo on )"
    if [ "${dry_run}" -eq 1 ]; then
        printf '  dry run: %s\n' "${targets[*]}"
        return 0
    fi

    local -a failed=()
    local index=0 slug width
    width="${#count}"
    local run_started; run_started="$(date +%s)"
    local state_dir; state_dir="$(mktemp -d)"
    local -a pending=("${targets[@]}") running=()
    local stop_launching=0

    build_one() {
        local one="$1" t0 rc
        t0="$(date +%s)"
        if [ "${BUILD_TIMEOUT_MIN}" -gt 0 ]; then
            timeout "$(( BUILD_TIMEOUT_MIN * 60 ))" "${script_dir}/build-plugin.sh" "${one}" "${CATALOG_URL}" \
                > "${log_dir}/${one}.log" 2>&1
        else
            "${script_dir}/build-plugin.sh" "${one}" "${CATALOG_URL}" > "${log_dir}/${one}.log" 2>&1
        fi
        rc=$?
        printf '%s %s\n' "${rc}" "$(( $(date +%s) - t0 ))" > "${state_dir}/${one}.done"
    }

    report_one() {
        local one="$1" rc elapsed
        read -r rc elapsed < "${state_dir}/${one}.done"
        index=$((index + 1))
        printf '%s[%*d/%d]%s %-32s ' \
            "${C_DIM}" "${width}" "${index}" "${count}" "${C_RESET}" "${one}"
        if [ "${rc}" -eq 0 ]; then
            printf '%sok%s %s(%s)%s\n' \
                "${C_GRN}" "${C_RESET}" "${C_DIM}" "$(fmt_duration "${elapsed}")" "${C_RESET}"
            return 0
        fi
        local why=""
        [ "${rc}" -eq 124 ] && why="timed out, "
        printf '%sFAILED%s %s(%s%s, %s)%s\n' \
            "${C_RED}${C_BOLD}" "${C_RESET}" \
            "${C_DIM}" "${why}" "$(fmt_duration "${elapsed}")" "${log_dir}/${one}.log" "${C_RESET}"
        tail -n 3 "${log_dir}/${one}.log" 2>/dev/null \
            | sed "s/^/      ${C_DIM}/; s/\$/${C_RESET}/"
        failed+=("${one}")
        if [ "${keep_going}" -eq 0 ] && [ "${stop_launching}" -eq 0 ]; then
            stop_launching=1
            printf '\n%sstopping after first failure%s (use --keep-going to continue); letting running builds finish\n' \
                "${C_YEL}" "${C_RESET}"
        fi
    }

    while [ "${#pending[@]}" -gt 0 ] || [ "${#running[@]}" -gt 0 ]; do
        while [ "${stop_launching}" -eq 0 ] && [ "${#pending[@]}" -gt 0 ] \
              && [ "${#running[@]}" -lt "${BUILD_JOBS}" ]; do
            slug="${pending[0]}"
            pending=("${pending[@]:1}")
            build_one "${slug}" &
            running+=("${slug}")
        done
        [ "${stop_launching}" -eq 1 ] && pending=()
        [ "${#running[@]}" -gt 0 ] || break
        wait -n 2>/dev/null || true
        local -a still=()
        for slug in "${running[@]}"; do
            if [ -f "${state_dir}/${slug}.done" ]; then
                report_one "${slug}"
            else
                still+=("${slug}")
            fi
        done
        running=("${still[@]}")
    done
    rm -rf "${state_dir}"

    local total_elapsed=$(( $(date +%s) - run_started ))
    printf '\n%s%s%s\n' "${C_DIM}" "$(printf '%.0s-' $(seq 1 62))" "${C_RESET}"
    printf '  built %s%d%s of %d in %s\n' \
        "${C_GRN}" "$(( index - ${#failed[@]} ))" "${C_RESET}" "${count}" \
        "$(fmt_duration "${total_elapsed}")"

    if [ "${#failed[@]}" -gt 0 ]; then
        printf '  %sfailed:%s %s\n' "${C_RED}${C_BOLD}" "${C_RESET}" "${failed[*]}"
        printf '  retry with: %scatalog.sh build %s%s\n' "${C_CYA}" "${failed[*]}" "${C_RESET}"
        if [ "${index_anyway}" -eq 0 ]; then
            printf '\n  %sindex.json NOT regenerated%s - the host would advertise\n' \
                "${C_YEL}" "${C_RESET}"
            printf '  entries whose artifacts were never rebuilt. Fix the above, or\n'
            printf '  pass --index-anyway if you know what you are doing.\n'
            return 1
        fi
    fi

    printf '\n'
    cmd_index || return 1
    printf '\n%sNext:%s docker compose up -d --force-recreate mcharbor\n' \
        "${C_BOLD}" "${C_RESET}"
}

[ -f "${PLUGIN_LIST}" ] || die "plugin list not found: ${PLUGIN_LIST}"
pick_python || die "no usable python found - set MCHARBOR_PYTHON"

case "${1:-}" in
    -h|--help|help|'') usage; exit 0 ;;
    status) shift; cmd_status "$@" ;;
    index)  shift; cmd_index "$@" ;;
    build)
        shift
        if ! signing_ready; then
            printf '%swarning:%s go not found on PATH - signing will fail.
' \
                "${C_YEL}${C_BOLD}" "${C_RESET}"
            printf '  Signing runs through %ssrc/backend/cmd/pluginsigner%s.

' "${C_CYA}" "${C_RESET}"
        fi
        cmd_build "$@"
        ;;
    *) die "unknown command: $1 (try --help)" ;;
esac
