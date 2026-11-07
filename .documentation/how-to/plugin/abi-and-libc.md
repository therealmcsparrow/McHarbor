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

## `min_mcharbor`, and what it protects

Plugins are downloaded from `https://raw.githubusercontent.com/therealmcsparrow/McHarbor-Catalog/main/Plugins`; none ship inside
McHarbor. Every catalog entry records the oldest McHarbor release it accepts, copied at build
time from the `mcharbor:` range in the plugin's `plugin.yaml`:

```json
{ "version": "2026.09.03",
  "build": "202609181858",
  "min_mcharbor": "2026.09.01",
  "download_url": "https://raw.githubusercontent.com/therealmcsparrow/McHarbor-Catalog/main/Plugins/files/comm-slack-2026.09.03.tar.gz",
  "checksum": "…",
  "signature": "…" }
```

The installer refuses an artifact whose minimum is newer than the running release, before the
download and again against the tarball's own manifest, which is all an uploaded plugin has. An
empty value is accepted. Only the floor of a range is enforced; an upper bound is not.

A newer McHarbor does not need a new plugin build, because neither `VERSION` nor `BUILD` is part
of what `plugin.Open` compares.

### What it does not protect

`min_mcharbor` is a version string and says nothing about ABI. A plugin still has to be rebuilt
when something it links changes:

- a package it links, directly or through its imports; `go list -deps` on what the plugin imports
  names them, and `core/plugin/**`, `core/i18n`, `core/response` and `core/auth` are among them
- a backend dependency in `go.mod`
- the Go toolchain, the base image or the libc

The load is what proves it. When the container moved from Alpine to Debian, every installed
`.so` was musl-linked and could not open on the glibc host:

```
plugin runtime: load failed slug=addon-compute version=1.0.0
  err="… libc.musl-x86_64.so.1: cannot open shared object file"
```

The plugin shows as **Needs repair** with that error until a rebuilt artifact is published.

**Raise the floor when the ABI moves in a new release.** Otherwise an older host sees the new
build in the catalog, installs it and can no longer load the plugin. Set `mcharbor:` in the
affected plugins to the new release before rebuilding them, and publish the catalog before the
matching image or packages. A rebuild for an ABI change that does not move `VERSION` has no floor
to raise, so it reaches every host on that release.

## What happens on upgrade

On every boot McHarbor reconciles before the runtime loads anything:

1. A plugin whose installed build differs from the catalog's build of the same version is
   re-downloaded at **the version already installed**.
2. A plugin whose installed version the catalog no longer carries moves to the newest version this
   release meets the minimum of, but only on the first boot of a new release.
3. Anything else is left alone. A new release with an unchanged catalog downloads nothing.
4. Configuration, enabled state and the role grants on a plugin's permission keys are preserved
   across a reinstall.

The load then decides what the Plugin Store shows. A plugin that fails to load appears as **Needs
repair** with the load error. **Repair** fetches the catalog copy again and requires
`plugins.manage`. It only helps once the catalog holds a build that fits this host.

Boot never fails because of this. An air-gapped host installs a plugin by uploading its tarball in
the Plugin Store instead.

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

The guard only makes the failure survivable; it does not repair anything. Rebuild the plugins
and publish the catalog: the next boot sees the new build stamps and reinstalls them. See
[Building a plugin that will load](#building-a-plugin-that-will-load).

## Building a plugin that will load

Use `scripts/development/build-plugin.sh <slug> <catalog-url>`. It builds through
`Dockerfile.plugin-packager`, which is the only path that guarantees the module context
is right, and copies `min_mcharbor` from the plugin's `mcharbor:` range.

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
