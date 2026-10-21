#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Build and sign a single McHarbor plugin.
#
# The plugin .so must be compiled in the *host's* Go module context or
# plugin.Open() rejects it with "plugin was built with a different version
# of package .../core/plugin". docker/Dockerfile.plugin-packager does that
# staging; this script drives it, pre-builds the plugin's frontend UMD
# bundle, and signs the packaged tarball with ed25519.
#
# The `output` folder next to this script is filled in the shape of a live
# plugin catalog, so its contents can be copied straight over one:
#
#   output/<slug>.json                        catalog entry
#   output/files/<slug>-<version>.tar.gz      signed payload
#   output/files/<slug>-<version>.tar.gz.sig  ed25519 signature
#   output/icons/<slug>/plugin.svg            brand icon (+ any others)
#
# The catalog URL parameter is not a write target — it is the location the
# catalog will serve the artifact from, and is baked into the generated
# entry's download_url.
#
# PORTABILITY: the body is strict POSIX sh — no arrays, no [[ ]], no =~, no
# ${var//a/b}, no pipefail. The shebang still says bash so `./script` behaves
# as before, but `sh script.sh` inside an Alpine container or a minimal
# MINGW shell works identically. Keep it that way when editing: bash-only
# syntax fails at PARSE time under sh, so the whole script dies on line 1
# rather than at the line that used it.
#
# Usage:
#   ./scripts/plugins/build-signed-plugin.sh <slug> <catalog-url>
#
# See --help for the full contract.

set -eu

# ---------------------------------------------------------------------------
# Constants (override any of these via the environment)
# ---------------------------------------------------------------------------

script_dir="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="${MCHARBOR_REPO_ROOT:-$(cd "${script_dir}/../.." && pwd)}"

# Default plugin source tree: <application path>/src/plugins/Plugins
PLUGIN_SOURCE_ROOT="${MCHARBOR_PLUGIN_SOURCE_ROOT:-${REPO_ROOT}/src/plugins/Plugins}"

# Where the signed artifacts land: the `output` folder next to this script.
OUTPUT_DIR="${MCHARBOR_PLUGIN_OUTPUT_DIR:-${script_dir}/output}"

# Source of the catalog entry that gets rewritten into output/<slug>.json.
CATALOG_SOURCE_DIR="${MCHARBOR_PLUGIN_CATALOG_SOURCE_DIR:-${REPO_ROOT}/src/plugins/catalog}"

# ed25519 signing material and the signer that consumes it.
#
# docker/Dockerfile copies signing/plugin-signing.pub to /app/trust/ and
# points MCHARBOR_PLUGIN_TRUST_KEYS at it, so a tarball signed with any
# other key installs as unsigned. src/plugins/signing/ holds a second,
# unrelated keypair that predates that wiring - do not sign with it.
SIGNING_KEY="${MCHARBOR_PLUGIN_SIGNING_KEY:-${REPO_ROOT}/signing/plugin-signing.key}"
TRUST_KEY="${MCHARBOR_PLUGIN_TRUST_KEY:-${REPO_ROOT}/signing/plugin-signing.pub}"

# Packager stage that compiles the .so inside the host module tree.
PACKAGER_DOCKERFILE="${MCHARBOR_PLUGIN_PACKAGER_DOCKERFILE:-${REPO_ROOT}/docker/Dockerfile.plugin-packager}"
PACKAGER_TARGET="plugin-packager"

# A slug must start with one of these families. Space-separated, not an
# array — see the PORTABILITY note above.
VALID_SLUG_PREFIXES="addon- ai-provider- comm- identity- logging- scanner- snapshot- storage-"

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

usage() {
    cat <<EOF
build-signed-plugin.sh — build and sign one McHarbor plugin

USAGE
    $(basename "$0") <slug> <catalog-url>

    Both parameters are mandatory. Run with no parameters, --h, or
    --help to print this text.

PARAMETERS
    <slug>
        Plugin folder name under the plugin source root. Must start with
        one of: ${VALID_SLUG_PREFIXES}
        The folder ${PLUGIN_SOURCE_ROOT}/<slug> must exist.
        e.g. comm-telegram, storage-dropbox, ai-provider-openai

        The version is NOT a parameter — it is read from that folder's
        plugin.yaml, so the tarball name, the catalog entry, and the
        manifest inside the payload can never disagree. Bump plugin.yaml
        to release a new version.

    <catalog-url>
        Full URL of the plugin catalog ROOT — the location that serves the
        per-plugin *.json entries and their files/ folder. This is NOT a
        write target: it is baked into the download_url of the generated
        catalog entry.
        Accepts http://, https://, file://, or an absolute local path
        (turned into a file:// URL).
        e.g. file:///mnt/plugin-catalog
        e.g. https://plugins.mcharbor.io/catalog

OUTPUT
    The fixed output folder next to this script is filled in the shape of a
    live catalog (folders created when missing), so it can be copied over
    one wholesale:

        ${OUTPUT_DIR}/<slug>.json
        ${OUTPUT_DIR}/files/<slug>-<version>.tar.gz
        ${OUTPUT_DIR}/files/<slug>-<version>.tar.gz.sig
        ${OUTPUT_DIR}/icons/<slug>/plugin.svg

WHAT IT DOES
    1. Validates the slug prefix and the catalog URL, and reads the version
       from <slug>/plugin.yaml.
    2. Builds the plugin's frontend UMD bundle (skipped when the plugin has
       no frontend/ dir) and checks it exposes its global + mount().
    3. Runs docker buildx against ${PACKAGER_TARGET} so the .so is compiled
       in the host Go module context (ABI hash must match the host binary).
    4. Signs the tarball with ed25519 via cmd/pluginsigner, and verifies the
       signature against the trust key.
    5. Writes the tarball + .sig to files/, mirrors the plugin's icons/ into
       icons/<slug>/, and generates <slug>.json from
       ${CATALOG_SOURCE_DIR}/<slug>.json with icon set to plugin.svg and a
       single version entry whose download_url points at <catalog-url>.

REQUIREMENTS
    docker (with buildx), tar, go, npm (only for plugins that ship a frontend),
    and python3 (stdlib only - no pip install).
    A private signing key at:
        ${SIGNING_KEY}

ENVIRONMENT OVERRIDES
    MCHARBOR_REPO_ROOT                    default: ${REPO_ROOT}
    MCHARBOR_PLUGIN_SOURCE_ROOT           default: ${PLUGIN_SOURCE_ROOT}
    MCHARBOR_PLUGIN_OUTPUT_DIR            default: ${OUTPUT_DIR}
    MCHARBOR_PLUGIN_CATALOG_SOURCE_DIR    default: ${CATALOG_SOURCE_DIR}
    MCHARBOR_PLUGIN_SIGNING_KEY           default: ${SIGNING_KEY}
    MCHARBOR_PLUGIN_PACKAGER_DOCKERFILE   default: ${PACKAGER_DOCKERFILE}
    MCHARBOR_PYTHON                       default: first usable of python3/python/py

EXAMPLES
    $(basename "$0") comm-telegram file:///mnt/plugin-catalog
    $(basename "$0") storage-dropbox https://plugins.mcharbor.io/catalog

AFTER A BUILD
    Copy the contents of the output folder over the catalog root, then
    restart the container so the installer re-reads it:
        docker compose up -d --force-recreate mcharbor
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
# Argument handling — every parameter is mandatory
# ---------------------------------------------------------------------------

for arg in "$@"; do
    case "${arg}" in
        --h|--help|-h|help)
            usage
            exit 0
            ;;
    esac
done

if [ $# -eq 0 ]; then
    usage
    exit 2
fi

if [ $# -ne 2 ]; then
    echo "error: expected exactly 2 parameters, got $#" >&2
    echo "" >&2
    usage >&2
    exit 2
fi

slug="$1"
catalog_arg="$2"

[ -n "${slug}" ]        || die "<slug> must not be empty"
[ -n "${catalog_arg}" ] || die "<catalog-url> must not be empty"

# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

prefix_ok=0
for prefix in ${VALID_SLUG_PREFIXES}; do
    case "${slug}" in
        "${prefix}"*)
            prefix_ok=1
            break
            ;;
    esac
done
if [ "${prefix_ok}" -ne 1 ]; then
    die "slug '${slug}' must start with one of: ${VALID_SLUG_PREFIXES}"
fi

plugin_dir="${PLUGIN_SOURCE_ROOT}/${slug}"

# The minute this artifact is compiled. Every rebuild gets a new one, which is
# what lets the store offer a rebuild at an unchanged version.
build_stamp="$(date -u +%Y%m%d%H%M)"
[ -d "${plugin_dir}" ] || die "no plugin source at ${plugin_dir}"
[ -f "${plugin_dir}/plugin.yaml" ] || die "${plugin_dir}/plugin.yaml not found — not a plugin folder"

# The version is the manifest's, never an argument — that way the tarball
# filename, the catalog entry, and the plugin.yaml shipped inside the payload
# can never disagree. Bump plugin.yaml to release a new version.
version="$(awk '/^version:[[:space:]]*/{print $2; exit}' "${plugin_dir}/plugin.yaml" | tr -d '"')"
[ -n "${version}" ] || die "no top-level 'version:' line in ${plugin_dir}/plugin.yaml"

# The version lands in the tarball filename and in catalog download URLs;
# a slash or space there produces a path that no installer can resolve.
case "${version}" in
    *[!A-Za-z0-9._+-]*)
        die "version '${version}' from plugin.yaml contains characters that are not safe in a filename"
        ;;
esac
case "${version}" in
    [A-Za-z0-9]*) ;;
    *) die "version '${version}' from plugin.yaml must start with a letter or digit" ;;
esac

# The catalog URL is only used to compose the download_url reported at the
# end. Accept http(s):// and file:// as given; promote a bare absolute path
# to a file:// URL so the printed entry is always a valid URL.
catalog_url="${catalog_arg%/}"
case "${catalog_url}" in
    http://*|https://*|file://*)
        ;;
    *://*)
        die "<catalog-url> scheme must be http, https, or file, got '${catalog_arg}'"
        ;;
    /*)
        catalog_url="file://${catalog_url}"
        ;;
    [A-Za-z]:[\\/]*)
        # Windows drive path -> file:///C:/... with forward slashes.
        catalog_url="file:///$(printf '%s' "${catalog_url}" | tr '\\' '/')"
        ;;
    *)
        die "<catalog-url> must be a full URL or an absolute path, got '${catalog_arg}'"
        ;;
esac

[ -f "${PACKAGER_DOCKERFILE}" ] || die "packager dockerfile not found: ${PACKAGER_DOCKERFILE}"
[ -f "${SIGNING_KEY}" ] || die "signing key not found: ${SIGNING_KEY} (see --help)"

command -v docker >/dev/null 2>&1 || die "docker not found on PATH"
command -v tar >/dev/null 2>&1 || die "tar not found on PATH"
command -v go >/dev/null 2>&1 || die "go not found on PATH — needed to sign the tarball"

# Signing moved to cmd/pluginsigner, so python is only needed for the catalog
# entry below - stdlib json, no third-party import. On Windows, `python3` is
# usually the Microsoft Store execution-alias stub: it resolves through
# command -v, prints "Python was not found", and exits non-zero, so "exists" is
# not the same as "usable".
python_bin=""
for candidate in "${MCHARBOR_PYTHON:-}" python3 python py; do
    [ -n "${candidate}" ] || continue
    command -v "${candidate}" >/dev/null 2>&1 || continue
    if "${candidate}" -c 'import json' >/dev/null 2>&1; then
        python_bin="${candidate}"
        break
    fi
done
[ -n "${python_bin}" ] || die "no usable python found — needed to write the catalog entry"

# The output folder mirrors a live catalog: files/ for payloads, icons/<slug>/
# for the brand icon, and the entry JSON at the root.
output_files_dir="${OUTPUT_DIR}/files"
output_icons_dir="${OUTPUT_DIR}/icons/${slug}"
mkdir -p "${output_files_dir}" || die "cannot create ${output_files_dir}"
mkdir -p "${output_icons_dir}" || die "cannot create ${output_icons_dir}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

echo ">>> plugin      ${slug}@${version}"
echo ">>> source      ${plugin_dir}"
echo ">>> output      ${OUTPUT_DIR}"
echo ">>> catalog     ${catalog_url}"
echo ""

# ---------------------------------------------------------------------------
# 1. Frontend UMD bundle
#
# The packager stage only COPIES a pre-existing frontend/<slug>.umd.cjs into
# the payload. A plugin whose src/ changed since the last build silently
# ships a stale bundle, which surfaces in the UI as "plugin X did not export
# a mount() function". Rebuild it here so that cannot happen.
# ---------------------------------------------------------------------------

frontend_dir="${plugin_dir}/frontend"
if [ -d "${frontend_dir}" ]; then
    command -v npm >/dev/null 2>&1 || die "npm not found on PATH but ${slug} ships a frontend/"

    umd_target="${frontend_dir}/${slug}.umd.cjs"
    step "building frontend UMD bundle for ${slug}"

    if [ ! -d "${frontend_dir}/node_modules" ]; then
        step "installing frontend dependencies (first run / clean checkout)"
        # --legacy-peer-deps: @vitejs/plugin-react@4 peers vite ^4-7 while the
        # project pins ^8; without it npm aborts with ERESOLVE and vite never
        # lands, leaving a plugin that cannot build its bundle.
        (cd "${frontend_dir}" && npm install --legacy-peer-deps --no-audit --no-fund) \
            || die "npm install failed in ${frontend_dir}"
    fi

    (cd "${frontend_dir}" && npm run build) || die "frontend build failed for ${slug}"
    [ -f "${umd_target}" ] || die "frontend build did not produce ${umd_target}"

    # A vite config regression (raw rollupOptions UMD instead of build.lib)
    # emits dead code with no global and no mount(). Catch it here rather
    # than in the plugin picker.
    expected_global="McharborPlugin_$(printf '%s' "${slug}" | tr '-' '_')"
    grep -q "${expected_global}" "${umd_target}" || \
        die "${umd_target} does not expose window.${expected_global} — check vite-plugin.config.ts uses build.lib { formats: ['umd'] }"
    tail -c 500 "${umd_target}" | grep -qE '\.mount=|\.default=\{[^}]*mount:' || \
        die "${umd_target} does not export a mount() function — check src/index.tsx"

    # An import.meta.glob that Vite left unresolved in the UMD build ships as
    # `{}.glob(` and blanks the plugin's pages at runtime, not at build time.
    command -v node >/dev/null 2>&1 || die "node not found on PATH (needed to check ${umd_target})"
    node "${REPO_ROOT}/scripts/plugins/check-umd-bundle.mjs" "${umd_target}" || \
        die "${umd_target} holds build-time-only code that cannot run in the browser (see above)"

    step "built ${umd_target} (has ${expected_global} + mount)"
else
    step "no frontend/ dir — Go-only plugin, skipping UMD build"
fi

# ---------------------------------------------------------------------------
# 2. Package the .so in the host module context
#
# `--output type=tar` streams the scratch stage's rootfs through a pipe so
# only packaged/ has to be extracted; `type=local` would dump the whole
# builder filesystem (~150 MB of Go module cache).
# ---------------------------------------------------------------------------

# On Windows (Git Bash / MINGW) the docker CLI is a native Windows binary that
# does not understand MSYS paths. MSYS auto-converts bare arguments but not a
# value embedded after a comma, so `type=tar,dest=/tmp/...` would land in
# C:\tmp\... while the shell looks under the MSYS root. Convert explicitly.
if command -v cygpath >/dev/null 2>&1; then
    docker_context="$(cygpath -w "${REPO_ROOT}")"
    docker_dockerfile="$(cygpath -w "${PACKAGER_DOCKERFILE}")"
    docker_dest="$(cygpath -w "${tmp_dir}/packager.tar")"
else
    docker_context="${REPO_ROOT}"
    docker_dockerfile="${PACKAGER_DOCKERFILE}"
    docker_dest="${tmp_dir}/packager.tar"
fi

step "running ${PACKAGER_TARGET} for ${slug}@${version} (first run: 3-5 min cold)"
no_cache_flag=()
[ "${MCHARBOR_PLUGIN_NO_CACHE:-0}" = "1" ] && no_cache_flag=(--no-cache)
docker buildx build \
    "${no_cache_flag[@]}" \
    --target "${PACKAGER_TARGET}" \
    --build-arg "PLUGIN_SLUG=${slug}" \
    --build-arg "PLUGIN_VERSION=${version}" \
    --build-arg "PLUGIN_BUILD=${build_stamp}" \
    --output "type=tar,dest=${docker_dest}" \
    -f "${docker_dockerfile}" \
    "${docker_context}" \
    || die "docker buildx build failed for ${slug}@${version}"

tarball_name="${slug}-${version}.tar.gz"

step "extracting packaged/${tarball_name}"
tar -x -f "${tmp_dir}/packager.tar" -C "${tmp_dir}" "packaged/${tarball_name}" \
    || die "packager output did not contain packaged/${tarball_name}"
[ -f "${tmp_dir}/packaged/${tarball_name}" ] || \
    die "packager output did not contain packaged/${tarball_name}"

# ---------------------------------------------------------------------------
# 3. Sign
#
# pluginsigner writes <tarball>.sig next to the tarball: 64 raw ed25519 bytes,
# base64-encoded. This is what modules/appstore/plugins/installer.go verifies
# against the trust key before extracting a package. The same binary runs in
# docker/Dockerfile, so the release image and a workstation build cannot drift.
#
# -trust makes it verify what it just wrote, against the host's own PEM parser.
# A signature the host cannot verify is worse than none: the install succeeds,
# logs "installing unsigned plugin", and the mismatch is only visible in a line
# nobody reads.
# ---------------------------------------------------------------------------

sig_name="${tarball_name}.sig"

signer_args="-dir ${tmp_dir}/packaged -key ${SIGNING_KEY}"
if [ -f "${TRUST_KEY}" ]; then
    step "signing ${tarball_name} with $(basename "${SIGNING_KEY}") and verifying against $(basename "${TRUST_KEY}")"
    signer_args="${signer_args} -trust ${TRUST_KEY}"
else
    step "signing ${tarball_name} with $(basename "${SIGNING_KEY}")"
    echo ">>> WARNING: no trust key at ${TRUST_KEY}; signature not verified" >&2
fi

# GOTOOLCHAIN is honoured rather than overridden, except that a workstation
# pinned below what go.work asks for cannot run the signer at all. `auto` is
# Go's own default and lets it fetch the required toolchain once.
# shellcheck disable=SC2086
( cd "${REPO_ROOT}/src/backend"     && GOTOOLCHAIN="${MCHARBOR_GOTOOLCHAIN:-auto}" go run ./cmd/pluginsigner ${signer_args} )     || die "signing failed for ${tarball_name}"

[ -f "${tmp_dir}/packaged/${sig_name}" ] || die "signer did not produce ${sig_name}"

# ---------------------------------------------------------------------------
# 4. Fill the output folder in the shape of a live catalog
# ---------------------------------------------------------------------------

cp "${tmp_dir}/packaged/${tarball_name}" "${output_files_dir}/${tarball_name}" \
    || die "cannot write ${output_files_dir}/${tarball_name}"
cp "${tmp_dir}/packaged/${sig_name}" "${output_files_dir}/${sig_name}" \
    || die "cannot write ${output_files_dir}/${sig_name}"

# Drop the un-versioned copies an older packaging convention left behind, so
# files/ holds exactly one artifact per plugin version.
rm -f "${output_files_dir}/${slug}.tar.gz" "${output_files_dir}/${slug}.tar.gz.sig"

# Icons. The catalog mirrors every icon a plugin ships under icons/<slug>/,
# and the entry's "icon" field always names plugin.svg — a plugin without
# one renders an empty tile in the App Store, so treat it as fatal rather
# than shipping a catalog that looks broken.
icons_src="${plugin_dir}/icons"
[ -d "${icons_src}" ] || die "no icons/ folder at ${icons_src} — plugin.svg is required"
[ -f "${icons_src}/plugin.svg" ] || die "${icons_src}/plugin.svg not found — it is the plugin's brand icon"
cp -R "${icons_src}/." "${output_icons_dir}/" || die "cannot copy icons into ${output_icons_dir}"
icon_count="$(find "${output_icons_dir}" -type f -name '*.svg' | wc -l | tr -d ' ')"

# Catalog entry. src/plugins/catalog/<slug>.json is the tracked source, but
# it carries a Tabler component name in "icon" and a stale download_url —
# both are rewritten to the deployed form here. A plugin with no source
# entry yet gets a minimal one synthesised from plugin.yaml.
entry_src="${CATALOG_SOURCE_DIR}/${slug}.json"
entry_out="${OUTPUT_DIR}/${slug}.json"

# The release the .so was compiled against. The installer refuses an artifact
# whose built_for differs from the running host rather than letting plugin.Open
# fail with "different version of package" after a successful download.
built_for="$(tr -d '[:space:]' < "${REPO_ROOT}/VERSION")"
download_url="${catalog_url}/files/${tarball_name}"

if [ ! -f "${entry_src}" ]; then
    echo ">>> WARNING: no catalog entry at ${entry_src};" >&2
    echo "    synthesising ${slug}.json from plugin.yaml — review it before publishing." >&2
fi

signature="$(tr -d '[:space:]' < "${output_files_dir}/${sig_name}")"

step "generating $(basename "${entry_out}")"
"${python_bin}" - "${entry_src}" "${entry_out}" "${slug}" "${version}" \
    "${download_url}" "${plugin_dir}/plugin.yaml" "${built_for}" "${signature}" \
    "${build_stamp}" \
    <<'PY' || die "failed to generate ${entry_out}"
import json, os, sys

src, out, slug, version, url, manifest, built_for, signature, build = sys.argv[1:10]

entry = {}
if os.path.isfile(src):
    with open(src, encoding="utf-8") as fh:
        entry = json.load(fh)
else:
    # Best-effort scrape of the manifest's top-level scalars. plugin.yaml
    # keeps these as `key: "value"` one-liners; anything block-scalar is
    # skipped rather than mangled.
    wanted = ("name", "author", "license", "homepage", "description", "category")
    with open(manifest, encoding="utf-8") as fh:
        for line in fh:
            key, sep, raw = line.partition(":")
            if not sep or key != key.strip() or key.strip() not in wanted:
                continue
            value = raw.strip().strip('"')
            if value and value not in ("|", ">"):
                entry.setdefault(key.strip(), value)

entry["slug"] = slug
entry.setdefault("name", slug)
# The deployed catalog always names the mirrored file, never a component.
entry["icon"] = "plugin.svg"
# The signature travels with the entry that names the archive, so the
# installer can verify it without a second fetch for the .sig sidecar -
# a catalog served over plain static hosting need not expose one at all.
entry["versions"] = [{
    "version": version,
    "build": build,
    "download_url": url,
    "built_for": built_for,
    "signature": signature,
}]

with open(out, "w", encoding="utf-8") as fh:
    json.dump(entry, fh, indent=2, ensure_ascii=False)
    fh.write("\n")
PY

size="$(du -h "${output_files_dir}/${tarball_name}" | cut -f1)"

echo ""
step "wrote ${entry_out}"
step "wrote ${output_files_dir}/${tarball_name} (${size})"
step "wrote ${output_files_dir}/${sig_name}"
step "wrote ${output_icons_dir}/ (${icon_count} icon(s))"
echo ""
echo "    download_url: ${download_url}"
echo ""
echo "    Copy the contents of ${OUTPUT_DIR}"
echo "    over the catalog root, then restart the container so the"
echo "    installer sees it:"
echo "      docker compose up -d --force-recreate mcharbor"
