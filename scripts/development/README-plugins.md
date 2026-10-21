<!--
Copyright (c) 2026 McSparrow. All rights reserved.
McHarbor is licensed under the McHarbor License. See LICENSE for details.
-->

# Plugin build scripts

| Script | Does |
|---|---|
| `catalog.sh` | **Start here.** Wraps the three below with progress you can watch and a status view |
| `build-plugins.sh` | Builds, signs and indexes **every** plugin for a release |
| `build-plugin.sh` | Builds, signs, and stages **one** plugin into `output/` |
| `build-plugin-catalog-index.sh` | Aggregates everything in `output/` into `output/index.json` |

Plugins ship from `https://www.mcharbor.eu/catalog/plugins`, not inside McHarbor, so
`build-plugins.sh` output **must be published before** the matching image or packages are
released. An install that upgrades first has no matching artifact to repair from.

Typical flow — build each plugin, then index them all:

```sh
./build-plugin.sh comm-telegram  file:///mnt/plugin-catalog
./build-plugin.sh storage-dropbox file:///mnt/plugin-catalog
./build-plugin-catalog-index.sh
# then copy the contents of output/ over the catalog root
```

---

## `catalog.sh`

The manual front door. Same work as `build-plugins.sh`, but it tells you what is
happening while it happens, and it can tell you what needs building before you
start.

```sh
cd scripts/development

./catalog.sh status                 # what is current, stale or missing
./catalog.sh build                  # build what the last sweep left behind
./catalog.sh build --all            # build everything
./catalog.sh build addon-compute    # build just these
./catalog.sh index                  # regenerate index.json only
```

### `status` first, always

```
PLUGIN                             CATALOG BUILD    STATE
--------------------------------------------------------------
addon-compute                      202609222309     current
addon-kvm                          202609222310     current
ai-provider-xai                    202609221959     STALE
--------------------------------------------------------------
  60 current   1 stale   0 missing   of 61
  newest entry: 202609222313   host BUILD: 202609230023
  index.json: 2026-09-23 01:14
```

A **stale** entry is the one that bites: the host's reconciler
(`modules/appstore/plugins/reconcile.go`) only repairs a plugin whose *catalog*
stamp differs from its *installed* stamp. An entry still carrying its old stamp
is never repaired, so that plugin stays unloadable and says nothing about it —
its pages just render empty.

### What "stale" does and does not mean

Stale means **left behind by the last build sweep** — more than `--window`
minutes (default 120) behind the newest entry in the catalog. That is the set a
partial or interrupted run leaves broken.

It is *not* a claim that the `.so` matches your source. A build stamp cannot
know you edited `core/plugin/specs/compute`, and that edit invalidates the ABI
of **every** plugin at once:

```
plugin was built with a different version of package
github.com/therealmcsparrow/mcharbor/core/plugin/specs/compute
```

After touching anything under `core/plugin/**` or `core/compute/**`, the answer
is `./catalog.sh build --all`, then recreate the container.

### Failure handling

Stops at the first failure by default, printing the last few lines of that
plugin's log inline and naming the retry command. Full logs land in
`<catalog>/.build-logs/<slug>.log`.

`index.json` is **not** regenerated when anything failed — the index is what the
host fetches, and advertising entries whose artifacts were never rebuilt is
worse than an older index. Use `--keep-going` to build through failures, and
`--index-anyway` only when you understand that trade.

### Options

| Flag | Effect |
|---|---|
| `--all` | every plugin in `plugins.list` |
| `--stale` | only what the last sweep left behind (default) |
| `--keep-going` | continue past a failure |
| `--index-anyway` | write `index.json` even after a failure |
| `--catalog <url>` | url baked into each entry (default `file:///mnt/plugin-catalog`) |
| `--window <min>` | how far behind the newest entry counts as stale |

`NO_COLOR=1` or piping to a file disables colour.

### After a build

```sh
docker compose up -d --force-recreate mcharbor
```

The restart is what makes the reconciler notice the changed stamps and reinstall
the affected plugins, keeping their enabled flag and role grants.

---

## `build-plugin.sh`

Builds one McHarbor plugin, signs it, and fills `output/` in the shape of a
live plugin catalog.

```sh
./build-plugin.sh <slug> <catalog-url>
```

Both parameters are mandatory. Run with no parameters, `--h`, or `--help`
for the full contract.

```sh
./build-plugin.sh comm-telegram file:///mnt/plugin-catalog
./build-plugin.sh storage-dropbox https://www.mcharbor.eu/catalog/plugins
```

The **version is not a parameter** — it is read from the plugin's own
`plugin.yaml`, so the tarball name, the catalog entry, and the manifest
inside the payload can never disagree. Bump `plugin.yaml` to release a new
version.

### Output

```
output/
  <slug>.json                        catalog entry
  files/<slug>-<version>.tar.gz      signed payload
  files/<slug>-<version>.tar.gz.sig  ed25519 signature
  icons/<slug>/plugin.svg            brand icon (plus any others the plugin ships)
```

Copy the contents over a catalog root, then restart the container so the
installer re-reads it:

```sh
docker compose up -d --force-recreate mcharbor
```

`<catalog-url>` is **not** a write target. It is the location the catalog will
serve from, and is baked into the generated entry's `download_url`.

---

## `build-plugin-catalog-index.sh`

Aggregates every `output/<slug>.json` into the single `index.json` the App
Store fetches. Takes no parameters.

```sh
./build-plugin-catalog-index.sh
```

Output is a **JSON array** — the shape McHarbor-AddOns publishes. The
backend's `parseRemotePayload` also accepts a `{"plugins":[...]}` wrapper or a
bare single object, so either is a valid hand-written alternative.

It also:

- merges entries that share a slug, so one plugin ends up with one entry
  carrying every version found;
- normalises the legacy top-level `version` / `download_url` form into
  `versions[]`;
- adds a `checksum` (hex sha256) per version whose tarball is present.
  Note the current installer plumbs `Checksum` but does **not** verify it —
  `installer_default.go` uses sha256 only for diagnostic fingerprints — so
  treat it as informational for now;
- sorts entries by slug and versions newest-first by numeric component
  (`1.10.0` above `1.9.0`, which a plain string sort gets backwards), making
  the file byte-identical across runs.

### Drift warnings

Non-fatal — the index is still written:

| Warning | Meaning |
|---|---|
| `files/<tarball> is missing` | An entry names a version with no tarball. |
| `files/<tarball> is not listed by any entry` | Usually an older version left behind: `build-plugin.sh` rewrites `<slug>.json` with only the version it just built, while `files/` keeps both. Re-add it to the entry's `versions[]` if you meant to publish both. |
| `no .sig sidecar` | The tarball was not signed, or the sidecar was lost. |
| `no icons/<slug>/plugin.svg` | The catalog entry names `plugin.svg` but the mirror has none. |
| `skipped <file>: no slug` | A `.json` in `output/` that is not a catalog entry. |

Exits 1 when the folder is missing or holds no usable entries.

---

## Dependencies

| Tool | Needed for | Verified with |
|---|---|---|
| **Docker** + **buildx** | Compiles the plugin `.so` in the host's Go module context | Docker 29.5.3, buildx v0.34.1 |
| **Python 3** + **PyNaCl** | ed25519 signing via `src/plugins/scripts/sign-plugin.py`. `build-plugin-catalog-index.sh` needs Python too, but stdlib only — no PyNaCl. | Python 3.13.7, PyNaCl 1.6.2 |
| **Node** + **npm** | Rebuilds the plugin's frontend UMD bundle — only for plugins with a `frontend/` folder | Node 26, npm 11+ |
| **tar** | Extracts the packaged artifact from the buildx tar stream | GNU tar 1.35 |
| **A POSIX shell** | `bash`, `sh`, `dash`, or busybox `ash` all work | — |

No local Go toolchain is required. The `.so` is built inside
`golang:1.27.2-bookworm` by `docker/Dockerfile.plugin-packager`, which is the
whole point: the plugin and the host binary must share a Go module context or
`plugin.Open()` rejects the `.so` with *"plugin was built with a different
version of package .../core/plugin"*.

### Install

```sh
# Signing (required)
pip install -r ../../src/plugins/scripts/requirements-signing.txt

# Docker Desktop — https://docs.docker.com/desktop/
# Node 26         — https://nodejs.org/  (or nvm / fnm / winget)
```

### Signing key

The script needs the private ed25519 key at:

```
src/plugins/signing/plugin-signing.key
```

It is gitignored and never committed. The public half
(`plugin-signing.pub`) stays in the repo so anyone can verify a build:

```sh
python - <<'PY'
import base64, pathlib
from nacl.signing import VerifyKey
tar = pathlib.Path("output/files/comm-telegram-1.0.0.tar.gz").read_bytes()
sig = base64.b64decode(pathlib.Path("output/files/comm-telegram-1.0.0.tar.gz.sig").read_text())
VerifyKey(pathlib.Path("../../src/plugins/signing/plugin-signing.pub").read_bytes().strip()).verify(tar, sig)
print("signature valid")
PY
```

---

## Windows notes

Two traps this script already handles, worth knowing when something looks off:

- **`python3` is usually the Microsoft Store stub.** It resolves on `PATH`,
  prints *"Python was not found"*, and exits non-zero. The script therefore
  picks the first interpreter that can actually `import nacl.signing`, not the
  first one that exists. Point `MCHARBOR_PYTHON` at a specific interpreter to
  override.
- **The Docker CLI is a native Windows binary** and does not understand MSYS
  paths. MSYS rewrites bare arguments but not a value after a comma, so
  `type=tar,dest=/tmp/...` would land in `C:\tmp\...`. The script converts the
  build context, dockerfile, and output destination with `cygpath` when it is
  available.

Keep the script POSIX-clean when editing it — no arrays, no `[[ ]]`, no `=~`,
no `${var//a/b}`. Bash-only syntax fails at *parse* time under `sh`, so the
whole script dies on line 1 rather than at the line that used it.

---

## Configuration

Every path is a constant at the top of the script, each overridable by an
environment variable.

| Variable | Default |
|---|---|
| `MCHARBOR_REPO_ROOT` | two levels above this folder |
| `MCHARBOR_PLUGIN_SOURCE_ROOT` | `<repo>/src/plugins/Plugins` |
| `MCHARBOR_PLUGIN_OUTPUT_DIR` | `<this folder>/output` |
| `MCHARBOR_PLUGIN_CATALOG_SOURCE_DIR` | `<repo>/src/plugins/catalog` |
| `MCHARBOR_PLUGIN_SIGNING_KEY` | `<repo>/src/plugins/signing/plugin-signing.key` |
| `MCHARBOR_PLUGIN_SIGN_SCRIPT` | `<repo>/src/plugins/scripts/sign-plugin.py` |
| `MCHARBOR_PLUGIN_PACKAGER_DOCKERFILE` | `<repo>/docker/Dockerfile.plugin-packager` |
| `MCHARBOR_PYTHON` | first of `python3` / `python` / `py` that works |
| `MCHARBOR_PLUGIN_INDEX_NAME` | `index.json` (`build-plugin-catalog-index.sh` only) |

### Valid slug prefixes

`addon-` `ai-provider-` `comm-` `identity-` `logging-` `scanner-` `snapshot-`
`storage-`

A slug outside these families is rejected. The list is the
`VALID_SLUG_PREFIXES` constant — a **space-separated string**, not an array.
Dropping a space silently merges two prefixes into one that matches nothing,
so re-run a slug from each affected family after editing it.

---

## Troubleshooting

| Message | Cause |
|---|---|
| `syntax error: bad substitution` / `unexpected "("` | A bash-ism crept back into the script and it was run under `sh`. See the Windows notes. |
| `no working python with PyNaCl` | `pip install -r ../../src/plugins/scripts/requirements-signing.txt`, or set `MCHARBOR_PYTHON`. |
| `signing key not found` | The private key is not at `src/plugins/signing/plugin-signing.key`. |
| `does not expose window.McharborPlugin_<slug>` | The plugin's `vite-plugin.config.ts` uses raw `rollupOptions` UMD instead of `build.lib { formats: ['umd'] }`. |
| `does not export a mount() function` | The plugin's `src/index.tsx` is missing the named `mount` export the host's `PluginFormOutlet` calls. |
| `plugin.svg not found` | Every plugin needs `icons/plugin.svg`; the catalog entry always names it. |
| `no top-level 'version:' line in ...plugin.yaml` | The manifest needs an unindented `version: "1.0.0"` line — that is where the version comes from. |
| `version '...' from plugin.yaml contains characters that are not safe in a filename` | The manifest's version ends up in a filename and a URL; keep it to letters, digits, `.`, `_`, `+`, `-`. |
| `WARNING: no catalog entry at src/plugins/catalog/<slug>.json` | A minimal entry was synthesised from `plugin.yaml`. Review it before publishing. |
| First build takes 3–5 minutes | Cold Go module download. Later builds reuse the BuildKit cache and finish in 30–60s. |

A build that hangs or leaves an unresponsive container on Windows Docker
Desktop is cleared with `wsl --shutdown`, then retry — see
`.claude/rules/docker-windows.md`.
