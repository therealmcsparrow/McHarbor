# Packaging and publishing a plugin

Build, sign, publish, install, and the loop you actually work in while developing.

## Build

One script builds a plugin end to end:

```bash
scripts/development/build-plugin.sh <slug> <catalog-url>
```

Both parameters are mandatory. The **version is not one of them**. It is read from the
plugin's own `plugin.yaml`, so the tarball name, the catalog entry and the manifest inside
the payload can never disagree. Bump `plugin.yaml` to release a new version.

```bash
scripts/development/build-plugin.sh comm-telegram  file:///mnt/plugin-catalog
scripts/development/build-plugin.sh storage-dropbox https://www.mcharbor.eu/catalog/plugins
```

The `<catalog-url>` is **not a write target**. It is the location the catalog will serve
from, and is baked into the generated entry's `download_url`.

In order, the script builds the frontend UMD bundle and checks it exposes its global and
`mount()` (skipped for a Go-only plugin), runs `docker/Dockerfile.plugin-packager` to
compile the `.so` **inside the host Go module tree** (the only context in which the ABI
hash matches the host binary; see [ABI and libc](./abi-and-libc.md)), signs the tarball,
verifies that signature, and fills `scripts/development/output/` in the shape of a live
catalog.

`scripts/development/build-plugins.sh [catalog-url]` does the same for every plugin in
`scripts/development/plugins.list`, then writes the index, but only on a fully clean run,
because a partially published catalog is worse than none. Point `MCHARBOR_PLUGIN_LIST` at
another file to build a subset.

There is no `make` path. `src/plugins/Makefile` came from the separate McHarbor-AddOns
repository and delegates to `Plugins/*/Makefile`, which do not exist here, so `make build`
and `make sign` iterate over an empty list and report success having done nothing.

## Sign

Plugin tarballs are signed with **ed25519**. The host verifies a
tarball against the keys named by `MCHARBOR_PLUGIN_TRUST_KEYS`, which the container image
sets to `/app/trust/plugin-signing.pub`, a copy of `signing/plugin-signing.pub` from the
repository root. An administrator can add further keys for a self-hosted catalog.

**Sign with the root `signing/` keypair, and no other.** The repository also carries
`src/plugins/signing/`, an unrelated pair that predates the trust-key wiring; a tarball
signed with it verifies against nothing the host knows. The failure is quiet by design
(an unsigned plugin is allowed to install, because a host that can only run what its
vendor signed is not self-hosted), so the only trace is one line at install time:

```
installing unsigned plugin source=file:///mnt/plugin-catalog/files/addon-compute-1.0.0.tar.gz
```

`build-plugin.sh` verifies each fresh signature against the trust key and fails the build
on a mismatch, so the only way to reach that log line is to sign by some other route.

A signature reaches the installer by three carriers, tried in order: the `signature` field
on the catalog entry's version row, a `<tarball>.sig` sidecar beside the archive, and the
legacy shape where the last 64 bytes of the archive are the signature. `build-plugin.sh`
writes the first two, so a catalog served off plain static hosting need not expose the
sidecar at all.

Signing happens inside `build-plugin.sh`, through `src/plugins/scripts/sign-plugin.py`,
which needs **PyNaCl**:

```bash
python -m venv .venv-signing
.venv-signing/bin/pip install -r src/plugins/scripts/requirements-signing.txt
export MCHARBOR_PYTHON=$PWD/.venv-signing/bin/python
```

A virtual environment is the sane way to keep it off the system Python, and
`MCHARBOR_PYTHON` is how the script is told which interpreter has it. Without that, it
tries `python3`, `python` and `py` in turn and stops on the first one that can
`import nacl`.

The private key never leaves the signer. The public key is committed, because the whole point
is that anyone can verify.

## Publish to the catalog

A catalog is a directory of per-plugin JSON entries, the tarballs and signatures they name,
the icons they reference, and one `index.json` aggregating the entries. `build-plugin.sh`
already writes that shape into `scripts/development/output/`:

```
output/
  index.json                         every entry, written by build-plugin-catalog-index.sh
  <slug>.json                        one catalog entry
  files/<slug>-<version>.tar.gz      signed payload
  files/<slug>-<version>.tar.gz.sig  ed25519 signature
  icons/<slug>/plugin.svg            brand icon (plus any others the plugin ships)
```

So publishing is:

1. Bump `version` in `plugin.yaml`.
2. `scripts/development/build-plugin.sh <slug> <catalog-url>`.
3. `scripts/development/build-plugin-catalog-index.sh`: aggregates every `output/*.json`
   into `output/index.json`, adds a sha256 per version, and warns about drift: an entry
   naming a version with no tarball, a tarball no entry lists, a tarball with no `.sig`,
   a plugin with no `plugin.svg`.
4. Copy the contents of `output/` over the catalog root.

**Publish before you release.** A core release invalidates every installed `.so` at once,
and the boot-time repair pass has nothing to repair from until the matching artifacts are
reachable, so the catalog goes up *before* the image or the packages do.

The index is read as JSON by `parseRemotePayload`, which accepts a bare array, a single
object, or a `{"plugins": [...]}` wrapper. The array is what is published.
`src/plugins/index.yaml` and the `make sign-index` / `verify-index` targets that maintain
it are **vestigial**: nothing in the host parses YAML, and the key they reference
(`signing/key.pem`) does not exist in this repository.

A self-hosted catalog is the same shape: a directory of entries, or a URL that publishes
them, added as a catalog source in settings. The host probes one with
`POST /api/catalogs/{id}/test/stream` and streams a step-by-step report, because a remote
fetch can spend fifteen seconds and the operator should be able to see which step is spending
it.

## Install

End users install from **App Store → Plugins**. The host downloads the tarball, verifies the
signature, checks the manifest against its own compatibility range, reserves the permission
keys, runs the migrations, loads the `.so`, calls the registries and `Init`, and emits an
audit event. The frontend loader then fetches the contribution tables and the bridges wire
everything into the interface.

**Be aware of who can do this.** The plugin store routes are authenticated but carry no
permission gate in the running server, so any signed-in account can install, configure,
update or uninstall a plugin. See
[App store and plugin endpoints](../../api/endpoints/store.md).

## The developer loop

This is where most of the time goes, so it is worth getting right.

Editing a plugin's source and restarting the container does nothing: the container serves
what is installed under `data/plugins/<slug>@<version>/`. **Rebuilding the image does not
help either**: `docker/Dockerfile` no longer builds or bundles plugins, only the theme
pack, which ships no `.so`. A rebuilt plugin reaches a running McHarbor through a catalog
and nothing else.

Compose mounts `./mnt/plugin-catalog` read-only at `/mnt/plugin-catalog`, and
`scripts/development/add-dev-storage-and-catalogs.sh` registers it as a catalog source
once. After that this is the shortest loop:

```bash
scripts/development/build-plugin.sh <slug> file:///mnt/plugin-catalog
cp -r scripts/development/output/. mnt/plugin-catalog/
```

Then, in **App Store → Plugins**, use **Update** on the plugin. That is the only path that
moves all four of the values the loader cross-checks (the database row, the install
directory name, `plugin.yaml` and the `.so`'s `Version` symbol), because `Install` alone
refuses a slug that is already installed. It uninstalls and reinstalls at the catalog
version, keeping `plugin_config` rows and the enabled flag.

**Update** is offered when the catalog carries a newer version than the one installed, or the
same version with a newer build stamp. Both hold for a remote catalog such as
`https://www.mcharbor.eu/catalog/plugins/index.json` as well as a local one.

If the container will not come down cleanly on Windows (stuck "unhealthy", or `docker stop`
reporting that it tried to kill the container but received no exit event), bounce the WSL2 VM
with `wsl --shutdown`, wait for the distro to report stopped, and bring it up again. The CLI
cannot kill it because the process lives inside the VM and the signal never arrives.

## Changing something shared

A plugin and the host must compile the shared packages in one module graph. If you change
anything under the host's shared plugin packages, **rebuild the host image before rebuilding
the plugin**. Otherwise `plugin.Open` reports a version mismatch that reads like a dependency
problem and is a build-context problem.

[ABI and libc](./abi-and-libc.md) is the full account: what `plugin.Open` compares, why
every artifact links glibc, why `go.work` must not come back, and how `built_for` and the
boot-time repair pass work.

## Versioning

- Plugins are semver.
- The host refuses a downgrade.
- A breaking change in the shared plugin contract bumps the manifest `schema`, and the host
  enforces compatibility by inspecting both that and the version range.

## Uninstall

The seven-step gate chain: `Shutdown`; unregister SCIM schemas; remove ABAC rules by origin;
drop the routes; remove the permissions (a **hard failure** if any role still references one);
drop the plugin's tables by prefix; delete the files and clear the contribution cache.

Two consequences worth planning for. The permission failure means a plugin cannot be
uninstalled while a role still grants one of its keys, so strip the key from the roles first.
And the table drop is the rollback: uninstall does not reverse your migration SQL, it drops
everything matching the prefix, so the prefix has to be right and has to cover everything you
created.

## Related

- [Build a plugin](./README.md): the end-to-end path
- [Manifest](./manifest.md): the version ranges and prefixes referenced here
- [ABI and libc](./abi-and-libc.md): the build-context rules behind the mismatch above
- [App store and plugin endpoints](../../api/endpoints/store.md): the install API
- [Plugin architecture](../../development/plugins/architecture.md): what the loader does with the tarball
