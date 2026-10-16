# Plugin authoring from the command line

`mcharbor plugin dev` is the plugin author's local toolchain: scaffold a plugin, check it,
build it, package it, sign it. It is the one command group in `mcharbor` that reaches no
server.

| Page | Covers |
|---|---|
| This page | Every verb, what `validate` checks and why, and the traps the checks exist for |
| [Developing a plugin](../developing.md) | Writing the plugin itself: the manifest, the Go entry point, the UMD bundle, the adapters |
| [Architecture](../architecture.md) | How the runtime loads a plugin, and what it refuses |
| [Command-line client](../../../cli/README.md) | The rest of `mcharbor`, which does talk to a server |

## It has no permission keys, and that is not an omission

Every other group in `mcharbor` is an API client: it authenticates with an API key, its
requests pass the same permission checks as the web interface, and every change it makes is
written to the audit log. This group is not. `plugin dev` reads and writes your checkout and
drives the repository's own build scripts and signer.

So there is **no permission key, no route, no API key, no context and no audit row** for any
verb below. The thing that governs what it can do is filesystem access to the checkout.

The sibling commands are the runtime half and are entirely separate: `plugin list`,
`plugin catalog`, `plugin get`, `plugin status`, `plugin install`, `plugin update` and
`plugin uninstall` all talk to a server and need `plugins.view`, `plugins.manage` or
`plugins.delete`. The two halves share the word "plugin" and nothing else. `plugin dev
--help` says so, on both levels.

## Where it works

The checkout is found by walking up from the working directory until a
`src/plugins/Plugins` directory appears. `--repo <path>` names it explicitly, and
`MCHARBOR_REPO_ROOT` does the same from the environment.

## The verbs

| Command | Does |
|---|---|
| `plugin dev new <slug>` | Writes a minimal working plugin to `src/plugins/Plugins/<slug>` from the template embedded in the CLI binary, rewriting the placeholder slug in every file that names it: the manifest, the Go module path, the exported `Version` symbol, the frontend bundle name, the npm package name. The scaffold includes `frontend/src/mountGuard.tsx`, and its entry point creates the React root through `guardedRoot(el)`. It refuses to overwrite an existing plugin, and refuses a slug outside the families the packager accepts before writing anything |
| `plugin dev validate [slug...]` | Reads the source tree and reports what would go wrong once the plugin is installed. It also refuses a `contributes.capabilities` entry the host would drop. With no slug it checks every plugin in the checkout. Exits `1` when any finding is an error, `0` when the worst is a warning |
| `plugin dev build [slug...]` | Drives `scripts/development/catalog.sh build`: rebuilds the frontend bundle, compiles the `.so` inside the host's Go module context through the packager stage, signs the tarball and regenerates `index.json`. With no slug it builds what the last sweep left stale; `--all` builds everything |
| `plugin dev package <slug>` | Drives `scripts/development/build-plugin.sh` for one plugin: bundle, packager, signature and catalog entry, but not `index.json`. For iterating on one plugin |
| `plugin dev sign [slug]` | Runs the host's own signer over a directory of tarballs and catalog entries, so the signature the store verifies and the signature written here cannot differ |

`--dry-run` on any verb that drives a script prints the command instead of running it, which
is also how you find out exactly which script a verb wraps.

A slug must start with one of `addon-`, `ai-provider-`, `comm-`, `identity-`, `logging-`,
`scanner-`, `snapshot-` or `storage-`. That is the packager's own rule; `new` applies it up
front so a mistyped slug fails in a second rather than three minutes into a Docker build.

## What `validate` checks

### Route drift: the check worth running it for

A plugin's routes are declared twice: registered in Go, and declared in `plugin.yaml` with
the permission each one needs. The loader looks up the registration in the manifest by
`(method, path)` (case-insensitive on the method, exact on the path) and wraps it in
`PluginPermissionMiddleware` with the `requires_permission` it finds.

On a miss it finds nothing, so the permission is the empty string, and the middleware
short-circuits an empty permission to a pass-through. The route mounts, it works, it looks
correct, and **any authenticated user can reach it whatever their role**. Nothing logs a
warning and nothing fails.

`validate` reports it as an error, naming the route, the file it was registered in, and why
it matters:

```text
PLUGIN        SEVERITY  CHECK   DETAIL
comm-example  error     routes  POST /api/comm-example/send is registered non-public in
                                backend/routes.go and plugin.yaml does not declare it; it
                                has no permission middleware and any authenticated user
                                can reach it
```

Three registration shapes are recognised, which are the three the shipped plugins use:
`reg.Protected(...)` / `reg.Public(...)` / `reg.Auth(...)`, a route-table literal carrying a
public flag, and a route-table literal without one. A declaration with a blank
`requires_permission` is reported for the same reason a missing declaration is. A
declaration that says `public: true` is legal (the loader honours it) and is not
reported.

The reverse drift is a warning: a route declared in `plugin.yaml` that nothing in
`backend/` registers is a dead declaration.

### The build traps

| Finding | Severity | Why |
|---|---|---|
| `go.mod` has CRLF line endings | error | The packager reads it as text and stages a `.so` that cannot load, with nothing in the build output saying so. This is an LF repo |
| A dependency is at one version here and another in `src/backend/go.mod` | error | A shared dependency at two versions stages a `.so` the host refuses to open |
| `backend/main.go`'s `Version:` disagrees with `plugin.yaml` | error | The loader asserts that the manifest, the install directory name and the `.so` symbol all agree, and a source edit moves only two of the three |
| No `icons/plugin.svg` | error | The packager treats a missing brand icon as fatal, and an entry with none renders an empty tile in the store |
| `capabilities.net_bind` or `capabilities.fs_write` is true | error | Both are hard-vetoed by the loader; the plugin will never boot |
| A file under `frontend/src` is newer than `frontend/<slug>.umd.cjs` | warning | The committed bundle is copied verbatim, so an edit under `frontend/src` ships nothing until `npm run build` runs in that plugin. It is a warning rather than an error because a checkout does not preserve modification times |

### The manifest

`schema` must be `mcharbor.plugin/v1`, the `slug` must equal the directory name, and the
`version` must be present and safe in a filename. A missing `mcharbor:` range is a warning:
nothing then states which host releases the plugin needs.

Every `adds_permissions` key must be `<category>.<action>` in lowercase with underscores.
A hyphen or a colon is refused at install time, which is what left an entire plugin family
uninstallable until it was found by hand. The `scope` must be one of `global`, `env` or
`stack`, and defaults to `global` when omitted; anything else, including the host catalog's
word `environment`, is refused at install time and by `mcharbor plugin dev validate`. The host
translates `env` to its own `environment` scope when the key reaches the role editor, elevation
requests and the OpenAPI document, so a plugin never writes `environment`, `owner` or `account`.

### Capability declarations

Each `contributes.capabilities` entry is checked with the same rules the host applies when it
loads the plugin, and a failure is an `error` under the `capabilities` check. The host drops a
refused declaration whole and only logs it, so this is the one place an author sees it before
install. The rules are the six known kinds, exactly the fields each kind uses, lowercase token
values, pages under the plugin's own `/plugins/<slug>`, routes the manifest declares with the
right method (`GET` for `estate-source`, `DELETE` for `backup-row-actions`, a `public: true`
`POST` for `login-endpoint`), permissions from the manifest's own `adds_permissions` or
`consumes_permissions`, and no `kind` and key pair declared twice. See
`development/contracts/capabilities.md`.

## In CI

```bash
mcharbor plugin dev validate --repo "$PWD"
```

No key, no server, no network. It exits `1` on any error, so it gates a build.

## What it deliberately does not do

- **It does not deliver a built plugin to a running instance.** A rebuilt `.so` reaches a
  container only through `catalog.sh build` plus a force-recreate, and a bundled plugin's
  version bump takes effect only through the Plugin Store's **Update**, which is the one
  path that moves the database row, the install directory, the manifest and the symbol
  together. `plugin dev build` prints the next step.
- **It does not bump a version.** Changing a plugin's version in source breaks it until the
  install is upgraded to match, so it is never a side effect of a build.
- **It does not reimplement the build.** `build`, `package` and `sign` drive
  `scripts/development/catalog.sh`, `scripts/development/build-plugin.sh` and the host's own
  `pluginsigner`, so there is exactly one definition of how a plugin is built and signed.
