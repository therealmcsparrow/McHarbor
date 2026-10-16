# Development Workflow

How to run McHarbor from source, what lives where under `src/`, and how to build every artifact
with the helpers in `scripts/development/`.

## Quick start

1. Backend: `cd src/backend && GOTOOLCHAIN=auto GOWORK=off go run ./main.go`, which serves `:8705`.
2. Frontend: `cd src/frontend && npm install && npm run dev`, which serves `:8173` and proxies
   `/api` to `:8705`.
3. Type-check: `cd src/frontend && npx tsc -b`.
4. Run the tests for what you changed (see [Testing](#testing)).

## Toolchain

| Tool | Version | Used for |
|---|---|---|
| Go | as `src/backend/go.mod` states (1.27.2) | `src/backend`, `src/agent`, `src/cli`, `src/terraform-provider-mcharbor`, plugins |
| Node | 26 | `src/frontend` |
| Docker | Docker Desktop (WSL2) or Docker Engine | the agent's Linux build and plugin builds, both of which run in containers |
| Bash | Git Bash on Windows | `scripts/development/*.sh` |

Put `GOWORK=off` on every Go command in `src/backend`, `src/agent` and `src/cli`, so the plugin
workspace stays out of the build. If your local Go is older than `go.mod`, prefix
`GOTOOLCHAIN=auto` and Go fetches the right toolchain.

## What is in `src/`

| Path | Holds |
|---|---|
| `src/backend/` | The Go server (chi router, SQLite through `modernc.org/sqlite`, no CGO) |
| `src/backend/core/` | Shared infrastructure: `config`, `db` and its migrations, `agent`, `auth`, `docker`, `kubernetes`, `i18n`, `middleware`, `encryption`, `response`, `router`, `rbac`, `apispec`, `plugin`, `pluginruntime` |
| `src/backend/modules/` | About 35 feature folders, each `handler.go` / `service.go` (or `store.go`) / `model.go` / `routes.go` |
| `src/backend/api/` | The committed `openapi.json` and the route and architecture contract tests |
| `src/frontend/` | React 19, Vite, TanStack Query, Zustand, Tailwind 4, Radix |
| `src/frontend/core/` | `api`, `auth`, `i18n` and its `locales/`, `router`, `plugins` |
| `src/frontend/resources/` | Shared UI (`@resources`): `components/ui`, `layout`, `hooks`, `stores`, `utils`. See [Frontend](../frontend/README.md) |
| `src/frontend/modules/` | About 19 feature folders, each `pages/`, `components/`, `hooks/` |
| `src/agent/` | The remote agent: its own Go module, which connects outbound over WebSocket |
| `src/cli/` | The `mcharbor` command-line client |
| `src/terraform-provider-mcharbor/` | The Terraform provider |
| `src/plugins/Plugins/<slug>/` | One folder per in-tree plugin: `plugin.yaml`, `backend/`, `frontend/`, `i18n/`, `migrations/` |

### Conventions that hold across `src/`

- **Backend.** A handler parses, validates, authorises, calls the service, audits and responds.
  SQL lives in `service.go` or `store.go`. Responses are `{ success, data?, error?, code? }`, and
  an error reaches the client as an i18n code, never as `err.Error()`. Docker and Kubernetes
  calls get a 30 s timeout.
- **Module boundaries.** `modules/X` never imports `modules/Y`, backend or frontend. Shared code
  moves to `core/` or `@resources`.
- **Frontend.** Strict TypeScript. `<Button>` from `@resources/components/ui/Button`, never a raw
  `<button>`. OKLCH tokens, never raw colours. No inline `style`. Every user-facing string goes
  through `t()` with a named namespace, and the key exists in `en`, `nl`, `de`, `es`, `fr` and
  `pt`.
- **Permissions.** A new capability gets a key in `src/backend/core/rbac/`, a
  `RequirePermission` gate in its `routes.go`, the same key in
  `src/frontend/core/auth/permissionKeys.ts`, and a disabled-with-tooltip control in the UI.
- **Every source file** starts with the two-line copyright header.

## Building

| Artifact | Command | Output |
|---|---|---|
| The remote agent (linux/amd64, static, seccomp) | `bash scripts/development/build-agent.sh` | `agent/mcharbor-agent-<os>-<arch>` |
| The agent for darwin and windows too | `bash scripts/development/build-agent.sh --cross` | same |
| The CLI for every platform | `bash scripts/development/build-cli.sh` (`--only os/arch`, `--out DIR`) | `cli/`, archives plus checksums |
| The native Linux payload | `bash scripts/development/build-mcharbor.sh [out]` | `dist/payload/` |
| One plugin | `bash scripts/development/build-plugin.sh <slug> <catalog-url>` | `scripts/development/output/` |
| Every plugin, signed and indexed | `bash scripts/development/build-plugins.sh` | same |
| The plugin catalog index | `bash scripts/development/build-plugin-catalog-index.sh` | `output/index.json` |
| The frontend | `cd src/frontend && npm run build` | `src/frontend/dist/` |

Every build is stamped with the repository's version and build number. A Linux agent cannot be
built with `CGO_ENABLED=0`, because runc's `nsenter` is C. That is why `build-agent.sh` builds
the Linux target in a container.

### Plugins: `catalog.sh`

`scripts/development/catalog.sh` wraps the three plugin scripts with visible progress:

```sh
cd scripts/development
./catalog.sh status              # current, stale or missing, per plugin
./catalog.sh build               # build what is stale or missing
./catalog.sh build --all         # build everything
./catalog.sh build addon-kvm     # build just these
./catalog.sh index               # regenerate index.json only
```

`build` runs several plugins at once. Each plugin writes its own log to
`mnt/plugin-catalog/.build-logs/<slug>.log`, a result is printed as each one finishes, and
`index.json` is written once at the end.

| Option | Default | What it does |
|---|---|---|
| `-j`, `--jobs <n>` | `6`, or `MCHARBOR_CATALOG_JOBS` | How many plugins build at the same time |
| `--timeout <min>` | `10`, or `MCHARBOR_CATALOG_TIMEOUT_MIN` | Stops one plugin's build after this many minutes and counts it as failed. `0` means no limit |
| `--no-cache` | off | Builds without the BuildKit cache. A cache record BuildKit never finished makes every cached build hang |
| `--keep-going` | off | Keeps launching builds after a failure. Without it, no new build starts after the first failure, and the builds already running are allowed to finish |
| `--index-anyway` | off | Writes `index.json` even when a plugin failed |
| `--no-cache` | off | Builds without the BuildKit cache |

The time limit exists because BuildKit occasionally leaves a build step unfinished for hours. With
a limit, one stalled plugin fails on its own, marked `timed out` in the result line, and the
rest of the sweep carries on. Lower `-j` on a machine with little memory, because every job is a
separate packager build.

Run `status` first. A **stale** entry is one whose catalog build predates the host's, and the
host will refuse to repair from it. `scripts/development/plugins.list` names the plugins the
sweep builds. The full reference is `scripts/development/README-plugins.md`.

A rebuilt plugin reaches a running install only when the catalog has a tarball at the plugin's
version and the plugin is then updated from the Plugin Store. A new `.so` next to an old install
fails the version-agreement check. See [Contract tests → Plugins](../contracts/tests.md#plugins).

### What a rebuild does not reach

- **A native remote agent.** It self-updates only when the version rises, and only from a
  published release. Copy the binary from `agents/` to the host yourself.
  `scripts/development/install-native-agent.sh` installs one.
- **Installed plugins, after an edit under `src/backend/core/plugin/**`.** Build IDs change and
  every installed `.so` refuses to load until it is rebuilt and reinstalled.

## Local dev data

`scripts/development/` also carries helpers for a running dev container's database:

| Script | Does |
|---|---|
| `add-dev-storage-and-catalogs.sh` | Registers `/mnt/backup` as a `local` storage location, and `/mnt/app-catalog` and `/mnt/plugin-catalog` as catalogs. `--dry-run` previews. |
| `remove-dev-storage-and-catalogs.sh` | Removes the same three rows, with `PRAGMA foreign_keys` on so backup plans cascade. `--yes` skips the prompt. |
| `dev-mounts-common.sh` | Shared settings for the two scripts above |
| `publish-agent-catalog.mjs` | Copies a built agent set from `agents/` into `agents/catalog/<version>/<build>/` with its `SHA256SUMS`, for McHarbor to serve as an agent source. An unsigned set is refused until `SHA256SUMS.sig` exists. |
| `compact-wsl-vhdx.ps1`, `compact-win-vhdx.ps1` | Reclaim disk from the WSL2 and Docker Desktop virtual disks on Windows |

The add and remove scripts write SQL, because the API refuses to delete or disable a `local`
storage location (`ErrLocalStorageProtected`). Both are idempotent. You can override
`MCHARBOR_SERVICE`, `MCHARBOR_DB`, `MCHARBOR_DEV_BACKUP_PATH`, `MCHARBOR_DEV_APP_CATALOG_PATH`
and `MCHARBOR_DEV_PLUGIN_CATALOG_PATH`.

## Testing

```bash
cd src/backend  && GOTOOLCHAIN=auto GOWORK=off go test ./...          # backend
cd src/agent    && GOTOOLCHAIN=auto GOWORK=off go test ./...          # agent
cd src/frontend && npx vitest run && npx tsc -b                       # frontend
cd src/frontend && npm run check:a11y                                 # accessibility, needs a running instance
cd src/frontend && npm run test:visual                                # visual regression, needs a running instance
```

A new or changed route, permission, destructive control, shared primitive or piece of pure logic
ships with its tests. Frontend guards are registered in `src/frontend/test/floor.test.ts`. The
contract tests are in [Contracts](../contracts/README.md).

## Windows traps

| Trap | Fix |
|---|---|
| Files written in text mode on Windows come out CRLF | `src/` is LF, and CRLF breaks the plugin tarball build and `go.mod` reads in the packager |
| `-v "$PWD/…"` in Git Bash mis-mounts silently | Use `MSYS_NO_PATHCONV=1` and a `Z:/…` path |
| Locale JSON under `src/frontend/core/i18n/locales/` is PowerShell-formatted | Edit the lines. Never parse and re-dump a bundle. |
| `-buildmode=plugin` does not exist on Windows | Plugins build in a container through `catalog.sh` |
