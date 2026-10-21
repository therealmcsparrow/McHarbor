#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Build, sign and index every first-party plugin for a release.
#
# Plugins no longer ship inside the McHarbor image. They are published to
# https://www.mcharbor.eu/catalog/plugins and downloaded by the Plugin Store,
# so this script produces the whole catalog in one pass: one signed tarball
# per plugin, one <slug>.json per plugin, and the index.json the host fetches.
#
# RELEASE ORDERING IS NOT OPTIONAL. The catalog must be published before the
# image that matches it is pullable. A core release changes the ABI of every
# .so, and an install that upgrades before the new artifacts exist has nothing
# to repair from — every plugin stays disabled until the catalog catches up.
#
# PORTABILITY: strict POSIX sh, same as build-plugin.sh — no arrays, no [[ ]],
# no =~, no ${var//a/b}. Bash-only syntax fails at PARSE time under sh.
#
# Usage:
#   ./scripts/development/build-plugins.sh [catalog-url]
#
# catalog-url defaults to the production catalog. Pass
# file:///mnt/plugin-catalog to build into the dev bind mount instead.

set -eu

script_dir="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="${MCHARBOR_REPO_ROOT:-$(cd "${script_dir}/../.." && pwd)}"

CATALOG_URL="${1:-https://www.mcharbor.eu/catalog/plugins}"
PLUGIN_LIST="${MCHARBOR_PLUGIN_LIST:-${script_dir}/plugins.list}"
OUTPUT_DIR="${MCHARBOR_PLUGIN_OUTPUT_DIR:-${script_dir}/output}"

[ -f "${PLUGIN_LIST}" ] || { echo "plugin list not found: ${PLUGIN_LIST}" >&2; exit 2; }

version="$(tr -d '[:space:]' < "${REPO_ROOT}/VERSION")"

echo "Building every plugin for McHarbor ${version}"
echo "  catalog url: ${CATALOG_URL}"
echo "  output dir:  ${OUTPUT_DIR}"
echo ""

total=0
failed=""

# Comment and blank lines keep the list readable; everything else is a slug.
slugs="$(grep -v '^[[:space:]]*#' "${PLUGIN_LIST}" | grep -v '^[[:space:]]*$')"

for slug in ${slugs}; do
    total=$((total + 1))
    echo "=== [${total}] ${slug}"
    if "${script_dir}/build-plugin.sh" "${slug}" "${CATALOG_URL}"; then
        continue
    fi
    # Keep going so one broken plugin does not hide the state of the other 60,
    # then fail the run below: a partially published catalog is worse than none.
    echo "!!! ${slug} FAILED" >&2
    failed="${failed} ${slug}"
done

echo ""
if [ -n "${failed}" ]; then
    echo "FAILED plugins:${failed}" >&2
    echo "Catalog NOT indexed. Fix the above and re-run before publishing." >&2
    exit 1
fi

"${script_dir}/build-plugin-catalog-index.sh"

echo ""
echo "Built ${total} plugins for ${version}."
echo ""
echo "Publish by copying the contents of ${OUTPUT_DIR} to the catalog root"
echo "served at ${CATALOG_URL}, so that these resolve:"
echo "    ${CATALOG_URL}/index.json"
echo "    ${CATALOG_URL}/files/<slug>-<version>.tar.gz"
echo "    ${CATALOG_URL}/icons/<slug>/plugin.svg"
echo ""
echo "Publish this BEFORE the matching McHarbor image or packages are released."
