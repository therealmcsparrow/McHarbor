#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Generate output/index.json from the per-plugin entries already sitting in
# the output folder, so a whole catalog can be published in one copy.
#
# build-signed-plugin.sh writes one <slug>.json per plugin. This script
# aggregates every one of them into the single index the App Store fetches,
# and cross-checks what the entries claim against what files/ actually holds.
#
# Schema comes from src/backend/modules/appstore/plugins/remote_catalog.go
# (parseRemotePayload) — an index may be a JSON array, a {"plugins":[...]}
# wrapper, or a single object. The array is what McHarbor-AddOns publishes,
# so that is what is emitted here.
#
# PORTABILITY: strict POSIX sh, same as build-signed-plugin.sh — no arrays,
# no [[ ]], no =~, no ${var//a/b}. Bash-only syntax fails at PARSE time under
# sh, killing the whole script on line 1 rather than at the line that used it.
#
# Usage:
#   ./scripts/plugins/build-catalog-index.sh
#
# See --help for the full contract.

set -eu

# ---------------------------------------------------------------------------
# Constants (override any of these via the environment)
# ---------------------------------------------------------------------------

script_dir="$(cd "$(dirname "$0")" && pwd)"

# The catalog folder to index — the same one build-signed-plugin.sh fills.
OUTPUT_DIR="${MCHARBOR_PLUGIN_OUTPUT_DIR:-${script_dir}/output}"

# Name of the generated index inside that folder.
INDEX_NAME="${MCHARBOR_PLUGIN_INDEX_NAME:-index.json}"

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

usage() {
    cat <<EOF
build-catalog-index.sh — aggregate output/*.json into output/${INDEX_NAME}

USAGE
    $(basename "$0")

    Takes no parameters. Run with --h or --help to print this text.

WHAT IT DOES
    1. Reads every *.json in the output folder except ${INDEX_NAME} itself.
    2. Merges entries that share a slug, so a plugin listed twice ends up
       with one entry carrying both versions.
    3. Adds a checksum (hex sha256) for each version whose tarball is
       present in files/.
    4. Cross-checks the entries against files/ and icons/, warning about
       any drift (see WARNINGS).
    5. Writes ${INDEX_NAME} as a JSON array, sorted by slug with versions
       newest-first, so re-running produces a byte-identical file.

INPUT
    ${OUTPUT_DIR}/<slug>.json
    ${OUTPUT_DIR}/files/<slug>-<version>.tar.gz
    ${OUTPUT_DIR}/icons/<slug>/plugin.svg

OUTPUT
    ${OUTPUT_DIR}/${INDEX_NAME}

WARNINGS (non-fatal — the index is still written)
    - an entry names a version with no tarball in files/
    - a tarball in files/ that no entry lists. This is normal after building
      two versions of one plugin: build-signed-plugin.sh rewrites <slug>.json
      with only the version it just built, while files/ keeps both.
    - a tarball with no .sig sidecar
    - a plugin with no icons/<slug>/plugin.svg

ENVIRONMENT OVERRIDES
    MCHARBOR_PLUGIN_OUTPUT_DIR   default: ${OUTPUT_DIR}
    MCHARBOR_PLUGIN_INDEX_NAME   default: ${INDEX_NAME}
    MCHARBOR_PYTHON              default: first working python3/python/py

REQUIREMENTS
    python3 (no third-party packages — stdlib json/hashlib only)

TYPICAL FLOW
    ./build-signed-plugin.sh comm-telegram file:///mnt/plugin-catalog
    ./build-signed-plugin.sh storage-dropbox file:///mnt/plugin-catalog
    ./build-catalog-index.sh
    # then copy the output folder's contents over the catalog root
EOF
}

die() {
    echo "error: $*" >&2
    exit 1
}

step() {
    echo ">>> $*"
}

# ---------------------------------------------------------------------------
# Argument handling — this script takes none
# ---------------------------------------------------------------------------

for arg in "$@"; do
    case "${arg}" in
        --h|--help|-h|help)
            usage
            exit 0
            ;;
    esac
done

if [ $# -ne 0 ]; then
    echo "error: expected no parameters, got $#" >&2
    echo "" >&2
    usage >&2
    exit 2
fi

# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

[ -d "${OUTPUT_DIR}" ] || die "output folder not found: ${OUTPUT_DIR} — run build-signed-plugin.sh first"

# Only the stdlib is needed here, so unlike the signer any working
# interpreter will do — but keep the same preference order and the same
# Microsoft-Store-stub guard (a `python3` that exists but cannot run).
python_bin=""
python_found=""
for candidate in "${MCHARBOR_PYTHON:-}" python3 python py; do
    [ -n "${candidate}" ] || continue
    command -v "${candidate}" >/dev/null 2>&1 || continue
    python_found="${python_found}${python_found:+, }${candidate}"
    if "${candidate}" -c 'import json, hashlib' >/dev/null 2>&1; then
        python_bin="${candidate}"
        break
    fi
done
if [ -z "${python_bin}" ]; then
    if [ -z "${python_found}" ]; then
        die "python3 not found on PATH — needed to build the index"
    fi
    die "no working python (tried: ${python_found}) — set MCHARBOR_PYTHON"
fi

step "indexing ${OUTPUT_DIR}"

# ---------------------------------------------------------------------------
# Build the index
# ---------------------------------------------------------------------------

# No `|| die` here: the generator prints its own diagnosis and `set -e`
# propagates the exit status, so a second generic line would only bury it.
"${python_bin}" - "${OUTPUT_DIR}" "${INDEX_NAME}" <<'PY'
import hashlib
import json
import os
import re
import sys

out_dir, index_name = sys.argv[1:3]
files_dir = os.path.join(out_dir, "files")
icons_dir = os.path.join(out_dir, "icons")
index_path = os.path.join(out_dir, index_name)

warnings = []


def sha256(path):
    digest = hashlib.sha256()
    with open(path, "rb") as fh:
        for block in iter(lambda: fh.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def version_key(value):
    """Newest first. Numeric runs compare as numbers so 1.10.0 > 1.9.0;
    anything non-numeric falls back to its string form."""
    parts = []
    for chunk in re.split(r"[.\-+]", str(value)):
        parts.append((0, int(chunk), "") if chunk.isdigit() else (1, 0, chunk))
    return parts


# --- collect the per-plugin entries -----------------------------------------

entries = {}
for name in sorted(os.listdir(out_dir)):
    if not name.endswith(".json") or name == index_name:
        continue
    path = os.path.join(out_dir, name)
    if not os.path.isfile(path):
        continue
    try:
        with open(path, encoding="utf-8") as fh:
            entry = json.load(fh)
    except (OSError, ValueError) as exc:
        warnings.append("skipped %s: %s" % (name, exc))
        continue
    if not isinstance(entry, dict):
        warnings.append("skipped %s: not a JSON object" % name)
        continue

    slug = str(entry.get("slug") or "").strip()
    if not slug:
        warnings.append("skipped %s: no slug" % name)
        continue

    # A legacy entry keeps version/download_url at the top level instead of
    # in versions[]; local_catalog.go accepts both, so normalise here.
    versions = entry.get("versions")
    if not versions and entry.get("version"):
        versions = [{
            "version": entry["version"],
            "download_url": entry.get("download_url", ""),
        }]
    if not isinstance(versions, list) or not versions:
        warnings.append("skipped %s: no versions" % name)
        continue

    if slug in entries:
        # Two files for one plugin: keep the first file's metadata and
        # merge the version lists, the way readLocalCatalogDir does.
        entries[slug]["versions"].extend(v for v in versions if isinstance(v, dict))
        continue

    merged = {k: v for k, v in entry.items() if k not in ("versions", "version", "download_url")}
    merged["slug"] = slug
    merged["versions"] = [v for v in versions if isinstance(v, dict)]
    entries[slug] = merged

if not entries:
    sys.stderr.write("error: no plugin entries found in %s\n" % out_dir)
    sys.exit(1)

# --- checksum + integrity cross-checks ---------------------------------------

listed_tarballs = set()
for slug in sorted(entries):
    entry = entries[slug]

    seen = {}
    for version in entry["versions"]:
        number = str(version.get("version") or "").strip()
        if not number:
            warnings.append("%s: a version entry has no version number" % slug)
            continue
        # Later duplicates lose to the first, matching the host's merge.
        seen.setdefault(number, version)

        tarball = "%s-%s.tar.gz" % (slug, number)
        tar_path = os.path.join(files_dir, tarball)
        if os.path.isfile(tar_path):
            listed_tarballs.add(tarball)
            version["checksum"] = sha256(tar_path)
            if not os.path.isfile(tar_path + ".sig"):
                warnings.append("%s %s: no .sig sidecar next to %s" % (slug, number, tarball))
        else:
            warnings.append("%s %s: files/%s is missing" % (slug, number, tarball))

    entry["versions"] = sorted(seen.values(), key=lambda v: version_key(v.get("version")), reverse=True)

    if not os.path.isfile(os.path.join(icons_dir, slug, "plugin.svg")):
        warnings.append("%s: no icons/%s/plugin.svg" % (slug, slug))

# Tarballs nobody references — usually an older version left behind after a
# rebuild, since <slug>.json only carries the version most recently built.
if os.path.isdir(files_dir):
    for name in sorted(os.listdir(files_dir)):
        if name.endswith(".tar.gz") and name not in listed_tarballs:
            warnings.append("files/%s is not listed by any entry" % name)

# --- write --------------------------------------------------------------------

index = [entries[slug] for slug in sorted(entries)]
with open(index_path, "w", encoding="utf-8") as fh:
    json.dump(index, fh, indent=2, ensure_ascii=False)
    fh.write("\n")

for line in warnings:
    sys.stderr.write(">>> WARNING: %s\n" % line)

total_versions = sum(len(e["versions"]) for e in index)
print(">>> indexed %d plugin(s), %d version(s)" % (len(index), total_versions))
for entry in index:
    print("      %-28s %s" % (
        entry["slug"],
        ", ".join(str(v.get("version")) for v in entry["versions"]),
    ))
PY

index_path="${OUTPUT_DIR}/${INDEX_NAME}"
size="$(du -h "${index_path}" | cut -f1)"

echo ""
step "wrote ${index_path} (${size})"
echo ""
echo "    Copy the contents of ${OUTPUT_DIR}"
echo "    over the catalog root, then restart the container so the"
echo "    installer sees it:"
echo "      docker compose up -d --force-recreate mcharbor"
