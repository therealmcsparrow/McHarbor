# ABI and libc

A McHarbor plugin is a real Go plugin: a `-buildmode=plugin` shared object opened with
`plugin.Open`. That makes it fast and gives it the full host API, and it comes with one
hard constraint that shapes everything about how plugins are built and shipped:

**A plugin only loads into the exact host build it was compiled against.**

This page is what that means in practice, and what you have to keep true.

## What `plugin.Open` actually checks

Go records a per-package ABI hash for every package the host and the plugin share, and
`plugin.Open` compares them. Any mismatch is refused outright:

```
plugin.Open("…/comm-ntfy.so"): different version of package
github.com/therealmcsparrow/mcharbor/core/plugin
```

The hash is derived from compiled content, so it moves when any of these move:

| Input | Example of it changing |
|---|---|
| Go toolchain version | 1.27.2 → 1.27.3 rebuilds the whole standard library |
| `core/plugin` source | a field added to a struct a plugin imports |
| anything `core/plugin` imports | a dependency bump, directly or transitively |
| build flags | `-trimpath` present in one build and not the other |
| `GOOS` / `GOARCH` | linux/amd64 vs linux/arm64 |
| **libc** | musl vs glibc. `-buildmode=plugin` forces cgo, so this is real |
| **a shared dependency the plugin's own `go.mod` moves** | the plugin pins `golang.org/x/sys v0.47.0`, the host is on `v0.48.0` |

That last row is the one that surprises people, because it names a package the plugin
never mentions. The packager adds each of the plugin's third-party dependencies to the
host's `go.mod` so they resolve in the host module context, and `go get` will just as
happily **downgrade** a module the host already pins. The `.so` then compiles against a
different `golang.org/x/sys` than the host binary, and `plugin.Open` refuses it naming
`golang.org/x/sys/cpu` rather than anything the plugin author wrote.

So the rule the packager enforces: **a module the host already requires is never moved,
whichever version the plugin names.** Building inside the host module tree only means
something if the host's versions win. The build logs each one it holds:

```
keeping host pin for golang.org/x/sys (plugin asked for v0.47.0)
```

A plugin that genuinely cannot work against the host's version of a shared dependency is
telling you the host needs upgrading, not that the pin needs overriding.

This is also why `core/pluginruntime` exists separately from `core/plugin`. A plugin
imports only `core/plugin`, and the plugin linker prunes package symbols it cannot prove
reachable, so if the host's loader code lived in the same package, the plugin's view of
it would be a strict subset of the host's and the hashes would differ by construction.
Keeping `core/plugin` small and fully reachable from both sides is what makes them match.

## One libc: glibc, everywhere

Every McHarbor artifact links **glibc**, so exactly one build of each plugin serves every
install type.

| Stage | Base image | Links |
|---|---|---|
| `backend` (the host binary) | `golang:1.27.2-bookworm` | glibc |
| `production` (the runtime image) | `debian:bookworm-slim` | glibc |
| `plugin-packager` (plugin `.so` builds) | `golang:1.27.2-bookworm` | glibc |
| native payload (`.deb`, `.rpm`, tarball) | `golang:1.27.2-bookworm` | glibc |
| `frontend` | `node:26-bookworm-slim` | n/a |

Before this, the container was Alpine and linked musl while the `.deb` and `.rpm` linked
glibc, which meant two builds of all 61 plugins and two of everything downstream.
Standardising on glibc collapsed that, and Alpine was dropped as a supported platform
in the same change. There is no `.apk`, and Alpine is not a supported container host
either.

Going the other way, all-musl, is not available: `-buildmode=plugin` requires dynamic
linking, and a Debian host has no musl libc to link against.

### The remote agent is the deliberate exception

`docker/Dockerfile.agent` and the `agentbin` stage still build on Alpine and still link
musl, statically (`-extldflags=-static`). That is on purpose and should stay:

- The agent is a standalone binary people copy onto arbitrary Linux hosts: a NAS, a
  homelab box, whatever. A static musl build runs on all of them with no libc dependency.
  A glibc build would be *less* portable, which is the opposite of what it needs.
- It is not a plugin host. `plugin.Open` never compares hashes with it, so its libc is
  nobody else's concern.

This is a property of how the agent binary is *built*, not a statement about supported
platforms: `Dockerfile.agent` still uses `apk` internally, so tooling changes there keep
Alpine package names. Only the main image's production stage moved to `apt-get`.

## `go.work` must not come back

The host image used to build the host and all 61 plugins in a single `RUN` under a
`go.work` workspace, because that was the only way to force one resolved module graph.

That is gone. The `backend` stage now builds the host alone, in the plain
`src/backend` module context, which is exactly the context
`docker/Dockerfile.plugin-packager` builds plugins in. It stages plugin source at
`src/backend/_plugin_build/<slug>/backend`, *inside* the host module tree, so the host's
`go.mod` governs every import and `core/plugin` resolves as an ordinary sub-package.

Both sides therefore see the same module graph and the hashes match. **Reintroducing
`go.work` to the host build would break that**, because it would put the host in a
different module context from the packager.

## `built_for`, and what it protects

Plugins are downloaded from `https://www.mcharbor.eu/catalog/plugins`; none ship inside
McHarbor. Every catalog entry records the release it was compiled against:

```json
{ "version": "2026.09.03",
  "built_for": "2026.09.01",
  "download_url": "https://www.mcharbor.eu/catalog/plugins/files/comm-slack-2026.09.03.tar.gz",
  "checksum": "…",
  "signature": "…" }
```

The installer compares `built_for` against the running release and refuses a mismatch
before unpacking. An empty `built_for` is accepted as a legacy entry.

This is a deliberately small guard rather than a computed ABI token. It works because the
catalog is republished with every release, so the only thing that can invalidate an
artifact is a core release, which is precisely what `built_for` records. What it buys is
a message naming both versions instead of the opaque `different version of package`
failure arriving later, at load.

**Release ordering is therefore not a convention but a requirement:** the catalog is
published *before* the matching image or packages are. An install that upgrades first has
no matching artifact to repair from.

### What it does not protect

`built_for` records a **version string**, and `Reconcile` repairs a plugin only when that
string differs from the running release. An ABI change that does not move `VERSION` is
therefore invisible to it, and the affected plugins are skipped while being unloadable.

That is not hypothetical. When the container moved from Alpine to Debian, every installed
`.so` was musl-linked and could not open on the glibc host, but `VERSION` had not moved,
so `built_for` still matched and the repair pass walked straight past them:

```
plugin runtime: load failed slug=addon-compute version=1.0.0
  err="… libc.musl-x86_64.so.1: cannot open shared object file"
```

The recovery is to rebuild the affected plugins, republish them, and clear `built_for` on
their `appstore_plugins` rows so the next boot treats them as legacy entries and repairs
them. The lasting fix (recording something ABI-shaped rather than a version string) is
not implemented; until it is, **a change to the toolchain, the base image or the libc is a
reason to bump `VERSION`**, precisely so this guard fires.

## What happens on upgrade

A core release invalidates every installed `.so` at once. On the first boot afterwards,
McHarbor reconciles before the runtime loads anything:

1. Every installed plugin whose `built_for` differs from the running release is
   re-downloaded at **the version already installed**; a repair never silently upgrades
   you to a newer release.
2. Its configuration, its enabled state, and the role grants on its permission keys are
   preserved across the uninstall/reinstall cycle.
3. Anything that cannot be fetched (no network, catalog unreachable, artifact not
   published) is left **disabled** with the reason recorded, and appears in the Plugin
   Store as **Needs repair** naming the release it was built for. **Repair** retries, and
   requires `plugins.manage`.

Boot never fails because of this. The trade is that the first boot after an upgrade needs
outbound HTTPS to the catalog; an air-gapped host installs a plugin by uploading its
tarball in the Plugin Store instead.

## A toolchain mismatch fails one plugin, not the host

Since Go 1.27, `plugin.Open` on a `.so` built by a different toolchain does not always
return an error: it can throw `fatal error: runtime: no plugin module data`, which no
`recover` catches. One stale plugin used to crash-loop the whole host.

Before opening a `.so`, the loader now reads the Go version recorded in its build info and
compares it with the toolchain the host was built with. A mismatch fails that plugin alone:

```
plugin runtime: load failed slug=addon-kvm version=2026.09.04
  err="plugin built with a different Go toolchain: …/plugin.so was built with go1.26.6,
       host runs go1.27.2; rebuild it from the catalog"
```

The same message is the plugin's error in `GET /api/plugins/loader/status`. A file with no
Go build info at all is refused the same way (`plugin has no Go build info`).

The guard only makes the failure survivable; it does not repair anything. The boot repair
keys on `built_for`, which records `VERSION`, so a toolchain bump that leaves `VERSION`
alone is not noticed by it. Rebuild the plugins and the catalog, then **Update** each one
from the Plugin Store; see [Building a plugin that will load](#building-a-plugin-that-will-load).

## Building a plugin that will load

Use `scripts/development/build-plugin.sh <slug> <catalog-url>`. It builds through
`Dockerfile.plugin-packager`, which is the only path that guarantees the module context
is right, and stamps `built_for` from the repo's `VERSION`.

```sh
./scripts/development/build-plugin.sh comm-ntfy file:///mnt/plugin-catalog
./scripts/development/build-plugin-catalog-index.sh
```

`scripts/development/build-plugins.sh` does all 61 for a release, reading
`scripts/development/plugins.list`.

If you change anything under `core/plugin` or bump the Go toolchain, **rebuild the host
image before rebuilding any plugin**. A version mismatch at load reads like a dependency
problem and is a build-context problem.

### Two traps worth recognising

Both bite only some plugins, which is why they survived a long time.

**A carriage return in a module path.** The packager reads the host and plugin module
paths out of their `go.mod` files with `awk`, and rewrites a plugin's sub-package imports
onto the staged location. On a Windows clone `go.mod` is checked out CRLF, so `awk`'s
field carries a trailing `\r` that lands *inside* the rewritten path:

```
invalid import path: github.com/therealmcsparrow/mcharbor/_plugin_build/addon-reporting/backend/fontkit
```

The printed path looks correct, because the terminal does not render the CR. Only plugins
with Go sub-packages in `backend/` reach the rewrite at all, so everything else built
fine. If you see `invalid import path` for a path you cannot fault, read the staged file
with `cat -A` rather than trusting the message. The packager now strips CRs from every
module and version it reads.

**A missing `migrations/` directory.** `runMigrations` returns nil when a plugin's install
directory has no `migrations/`, so a payload that forgot to carry them installs perfectly
and then fails at runtime, one table at a time:

```
[plugin:addon-compute] compute: reading the image catalog failed
  error="… no such table: plugin_addon_compute_images"
```

The packaged payload is `plugin.yaml`, `backend/<slug>.so`, and whichever of `i18n/`,
`frontend/<slug>.umd.cjs`, `icons/` and `migrations/` the plugin ships. If a plugin's
tables are missing after a clean install, check the tarball first:

```sh
tar -tzf output/files/<slug>-<version>.tar.gz | grep migrations
```

## Related

- [Packaging](./packaging.md): build, sign, publish, install, and the dev loop
- [Build a plugin](./README.md): the end-to-end path
- [Plugin architecture](../../development/plugins/architecture.md): what the loader does with the tarball
- [Native Linux install](../../getting-started/installation/native-linux.md): how plugins reach a native install
