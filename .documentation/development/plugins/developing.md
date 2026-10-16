# Developing a Plugin

Writing one from scratch: the folder, the manifest, the Go side, the frontend bundle, and
the loop you work in. [Architecture](./architecture.md) covers what the host does with the
result; this page is the author's view.

Every shipped plugin also has a **Developer** page of its own, with its real adapter
registration, its Go file layout and its manifest table. Reading the one closest to what
you are building is usually faster than starting from the skeleton.

## Prerequisites

- Go matching the host's toolchain. The version is in `docker/Dockerfile`; a mismatch is
  rejected at load, not at build.
- Linux. `-buildmode=plugin` does not exist on Windows.
- Docker and Compose, because the reliable way to run a rebuilt in-tree plugin is to
  rebuild the image.

## Scaffold it

```bash
mcharbor plugin dev new <slug>
```

That writes a minimal, working plugin (a no-op adapter, one config field, and lifecycle
audit events) into `src/plugins/Plugins/<slug>/`, rewriting the slug in the manifest, the
Go module path, the exported `Version` symbol, the frontend bundle name and the npm package
name. The template lives inside the CLI binary, so there is no template directory in the
tree to fall out of date.

The slug must start with one of the families the packager accepts: `addon-`,
`ai-provider-`, `comm-`, `identity-`, `logging-`, `scanner-`, `snapshot-`, `storage-`.

Do not start a plugin by copying a sibling. A copy drags in the sibling's slug, routes,
permission keys and table prefix, and plugins share no code with one another.

```
src/plugins/Plugins/<slug>/
  plugin.yaml          # the manifest — the source of truth
  go.mod               # replace directive pointing at ../../../backend
  backend/
    main.go            # exports `var Plugin plugin.Plugin`
    <something>.go     # the client that does the work
  frontend/            # optional UMD bundle
  i18n/<lang>/<ns>.json
  icons/
  migrations/NNNN_*.sql
```

The slug has to match the directory name, and `go.mod`'s replace directive has to resolve
to the host's `src/backend`. That is what makes the plugin and the host share one module
graph.

## The manifest

```yaml
schema: "mcharbor.plugin/v1"

slug:     "comm-example"
name:     "Example"
version:  "2026.09.01"
author:   "You <you@example.com>"
license:  "Apache-2.0"
homepage: "https://example.com/plugin"
description: "One sentence that shows up on the catalog card."

mcharbor: ">=2026.09.01"
go:       ">=1.25"

permissions:
  - notification.adapter.register
  - encryption.handle.read
  - audit.emit

adds_permissions:
  - key:      "channels:use-comm-example"
    scope:    "global"
    category: "Communications"
    label:    "Send alerts through Example channels"
    description: "Required to route workflow, alert, or backup events to an Example channel."

contributes:
  routes: []
  widgets: []
  nodes: []
  i18n_namespaces:
    - "comm-example"
  forms:
    - resource: "channel"
      kind:     "config_form"
      key:      "example"

signature: ""
checksum:  ""
```

### Validation the installer applies

| Field | Rule |
|---|---|
| `schema` | Must be `mcharbor.plugin/v1` |
| `slug` | `^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`, and must match the directory |
| `name`, `author` | Required |
| `version` | `N.N.N`, optionally with `-suffix`. Bundled plugins carry the host's calendar version, such as `2026.09.01` |
| `mcharbor` | A semver range, such as `>=2026.09.01` |
| `permissions[]` | Every entry must be a known scope token |
| `adds_permissions[].key` | Unique within the manifest; cannot shadow a host or another plugin's key |

`MCHARBOR_PLUGIN_RELAXED_SCHEMA` relaxes the scope-token check and unknown properties. It
is a development switch.

### Scope tokens

These are the capabilities a manifest may ask for. Anything else fails validation.

| Group | Tokens |
|---|---|
| Docker | `docker.read`, `docker.write`, `docker.socket.read`, `docker.socket.write` |
| Kubernetes | `k8s.read`, `k8s.write` |
| Audit | `audit.read`, `audit.write`, `audit.emit` |
| Secrets and encryption | `secrets.read`, `secrets.write`, `encryption.handle.read` |
| Identity | `identity.read`, `identity.write`, `identity.issue_session`, `identity.adapter.register` |
| Environments | `env.read`, `env.write` |
| AI | `ai.invoke`, `ai.adapter.register` |
| Host surfaces | `rbac.permission.register`, `menu.sidebar.add`, `i18n.namespace.add`, `dashboard.widget.add`, `db.migrations.run` |
| Adapter families | `notification.adapter.register`, `storage.adapter.register`, `snapshot.adapter.register`, `email.adapter.register`, `logging.adapter.register`, `scanner.adapter.register`, `reporting.adapter.register`, `alerting.adapter.register` |
| Agent | `agent.rpc` |

`i18n.namespace.add` takes an argument after a colon: `i18n.namespace.add:compute`. The
token before the colon is what is validated.

Three of these are worth thinking about before you declare them:

- `identity.issue_session` means the plugin asks the host to mint a session. No shipped
  plugin declares it any more: LDAP, the only one that verifies a password itself, now has
  the host complete the sign-in (lockout, MFA and session included) and declares
  `identity.read` and `identity.write` instead.
- `agent.rpc` sends messages to a host's agent. It exists because a plugin cannot extend
  the agent protocol (`src/agent/protocol.go` is a versioned core artifact), and it does
  nothing without an `agent-rpc` capability naming the messages. It was `compute.rpc`
  before. See [Agent RPC](../contracts/agent-rpc/README.md).
- `ai.invoke` lets a plugin call whatever AI provider the operator configured, without
  holding a key of its own. Prefer it over shipping a provider dependency.

## The Go side

Implement `Plugin`, and `AdapterContributor` when you are filling a family slot:

```go
type Plugin interface {
    Manifest() Manifest

    RegisterRoutes(Registry) error
    RegisterMenu(MenuRegistry) error
    RegisterWidgets(WidgetRegistry) error
    RegisterNodes(NodeRegistry) error
    RegisterPermissions(AccessRegistry) error

    Init(ctx context.Context, host HostContext) error
    Shutdown(ctx context.Context) error
}
```

Export exactly one symbol:

```go
var Plugin plugin.Plugin = &ExamplePlugin{
    manifest: plugin.Manifest{
        Schema:  "mcharbor.plugin/v1",
        Slug:    "comm-example",
        Name:    "Example",
        Version: "2026.09.01",
        Author:  "You <you@example.com>",
        Desc:    "…",
    },
}
```

An adapter plugin leaves the four `Register*` methods as no-ops and does its work in
`RegisterAdapters`. A feature plugin uses `RegisterRoutes`. `Registry` has three methods
matching the host's middleware groups:

| Method | Middleware |
|---|---|
| `Public(method, path, h)` | No session required |
| `Auth(method, path, h)` | Session required |
| `Protected(method, path, h)` | Session required, and the manifest's `requires_permission` is enforced |

A route registered here but not declared in `contributes.routes` is never mounted.

### The host handles

`Init(ctx, host)` receives `HostContext`. Store what you need; it is the only way into the
host.

| Handle | What it gives you |
|---|---|
| `Logger` | `Info` / `Warn` / `Error` / `Debug` |
| `DB` | Queries against your own tables |
| `Encryption` | Encrypt and decrypt values at rest, without ever holding the key |
| `DockerPool`, `AgentPool` | Per-environment clients |
| `I18n` | Translation lookup |
| `Audit` | `Emit(action, fields)`, and `EmitEvent(AuditEvent)` when you know which account asked; a row emitted through `Emit` has no actor |
| `Lifecycle` | `Record(ctx, LifecycleEvent)`: put your own objects on the Logging timeline. `nil` when the host wired no recorder |
| `RBAC` | Permission checks |
| `Identity` | Provision a user from an external identity provider |
| `Sessions` | Issue an authenticated session after external auth |
| `AI` | Call whatever provider the operator configured |
| `SnapshotRPC` | Snapshot operations through the agent transport. `nil` when no agent transport is wired; treat that as "snapshot mode unavailable" |
| `AgentRPC` | The agent message bridge. Typed `any`: assert it against a local interface. `nil` without the `agent.rpc` token and an `agent-rpc` capability |

Anything nillable should be treated as absent rather than assumed. `SnapshotRPC` being
`nil` is a supported state, not an error.

### Declaring configuration fields

Adapter specs carry a `ConfigSchema` that the host renders as a form when the plugin ships
no frontend of its own:

```go
[]plugin.ConfigFieldDecl{
    {Key: "server_url", Type: "string", Label: "Server URL", Required: true,
     Help: "Base URL of the server.", Placeholder: "https://example.com"},
    {Key: "token", Type: "secret", Label: "Token", Required: true,
     Help: "Stored encrypted at rest."},
}
```

| Field | Notes |
|---|---|
| `Key` | The config map key your client reads |
| `Type` | `string`, `int`, `bool`, `secret` |
| `Label`, `Help`, `Placeholder` | What the operator sees |
| `LabelKey`, `HelpKey` | Dotted keys in the plugin's own i18n namespace (`config.server_url.label`, `config.server_url.help`) that translate `Label` and `Help`. Optional, and every communication plugin sets them on every field. |
| `Default`, `Min`, `Max`, `Required` | Validation and prefill |
| `Secret` | Encrypted at rest and never returned by the API |

Write `Help` as if it will be the only documentation someone reads, because on most
screens it is. The field tables on every plugin's configuration page in this section are
generated from exactly these declarations.

`Label` and `Help` stay in English: they are what the CLI, the API and any caller without the
plugin's bundle read. `LabelKey` and `HelpKey` are what the host's form renders. The host
(`SchemaFormField`) looks the key up in the owning plugin's namespace, and falls back to the
English text when the key is not in the bundle, so an untranslated key never shows as a raw
dotted path. Ship each key in all six locales. `Placeholder` has no key: placeholders are
examples (a URL, a phone number) and stay literal.

A plugin with its own frontend translates its setup guide the same way. The guide's steps are
keys (`guide.step1.body`, `guide.step2.body`, and so on) in the plugin's bundle, and the guide
component renders `t(key)` for each, so a step is translated by adding a key, never by editing
the component. Values that are code, such as a JSON payload to paste, or protocol names such as
`STARTTLS` and `CRAM-MD5`, stay literal and are listed in `.instructions/donottranslate.md`.

### Zero-config OAuth sign-in (AI providers)

An AI provider adapter can offer the operator a one-click sign-in instead of asking them to
register an OAuth app and paste a client ID. Declare `Capabilities.SupportsOAuth` and ship
the non-secret half of the device-flow config on the spec:

```go
plugin.AIAdapterSpec{
    ProviderType: "acme",
    // ...
    Capabilities: plugin.AIAdapterCapabilities{SupportsOAuth: true},
    OAuthDefaults: &plugin.OAuthDefaults{
        ClientID:  "acme-public-device-client-id",
        DeviceURL: "https://acme.example/oauth/device/code",
        TokenURL:  "https://acme.example/oauth/token",
        Scopes:    "inference.read",
    },
}
```

When the merged config is complete (client ID + both endpoints), the create form hides the
OAuth fields and the operator just clicks **Sign in**; the host fills these values in at
sign-in time via the RFC 8628 device flow, so a later plugin update reaches every existing
connection. Rules that keep this legitimate:

- **Client ID and endpoints only.** They are not secrets: they travel in the operator's
  browser during any normal OAuth login. Ship them freely.
- **Never a client secret.** A confidential client's secret is a `secret` `ConfigFieldDecl`
  stored encrypted in `plugin_config`; most device-grant clients are public and need none.
  Do not bake a secret into a bundle.
- **Only a client you registered.** Use a client the plugin author (or the operator)
  registered with the provider, obtained through the provider's sanctioned flow. McHarbor
  never reads another application's credential store and never impersonates another client
  to bypass a provider's access controls.
- **Additive.** An operator's hand-entered value always wins over the plugin default.

The same `OAuthDefaults` shape is available to other adapter families as their self-hosted
OAuth flows land; today the AI family consumes it.

## Migrations

SQL files under `migrations/`, applied once per install, in order, in a transaction.

They are confined to the plugin's own table prefix (`plugin_<slug>_*`, or the manifest's
`table_prefix`). A migration that touches a host table is refused, which is the guard that
makes uninstalling a plugin a bounded operation.

`MCHARBOR_AUTO_APPLY_PLUGIN_MIGRATIONS` applies pending migrations without asking.

## Permissions

Declaring a key is what makes it grantable:

- `adds_permissions` entries are inserted into `plugin_permissions` with
  `origin = <slug>` and merged into the catalog the role editor reads. **A key a plugin
  never declares is a key no role can be granted**, which leaves the feature reachable
  only by a wildcard role.
- `consumes_permissions` names host keys the plugin relies on; they must already exist.
- Menu entries take `required_permission`.
- Every declared route is wrapped in permission middleware automatically, so declaring
  correctly *is* enforcing.

Scope must be the narrowest that is still enforceable. `global` on a per-environment
resource is a privilege escalation waiting to happen.

Storage-adapter plugins are the documented exception: they ship no RBAC declarations,
because the host's settings permissions already gate the screens where destinations are
created.

## The frontend bundle

Optional. A UMD build the host loads at runtime, serving whatever the manifest declares in
`contributes.forms`, `widgets`, `nodes`, `menu`, `tabs`, `context-menus` and `topbar`.

Every React root the bundle creates goes through `guardedRoot(el)` from `frontend/src/mountGuard.tsx`
(React 19 `onUncaughtError`, rendering the host's `common:pluginMount.failed` message), never a bare
`createRoot`. `plugin dev new` scaffolds both files; see
[Plugin frontend](../../how-to/plugin/frontend.md#the-entry-point).

The common case for an adapter plugin is two forms:

| Kind | What it is |
|---|---|
| `config_form` | The fields, when you want more than `ConfigSchema` renders |
| `setup_guide` | The walkthrough shown beside them: where to click in the provider's console |

A `setup_guide` is worth writing for anything involving an OAuth app registration. It is
the difference between an operator finishing the setup and filing a support ticket.

## i18n

Namespaces go in `i18n/<lang>/<namespace>.json` and are declared in
`contributes.i18n_namespaces`. The host merges them at boot. English is the source; the
other locales fall back to it, so a partially translated namespace renders English rather
than a missing key.

## Build, sign, install

There are **no per-plugin Makefiles** in this repository. `src/plugins/Makefile` iterates
`Plugins/*/Makefile`, which matches nothing today, so its `build` and `sign` targets are
no-ops. Build the way the image does:

```bash
cd src/plugins/Plugins/<slug>
CGO_ENABLED=1 go build -trimpath -buildmode=plugin -o backend/<slug>.so ./backend
```

Sign for distribution:

```bash
python -m venv .venv
.venv/bin/pip install -r src/plugins/scripts/requirements-signing.txt
.venv/bin/python src/plugins/scripts/sign-plugin.py \
  --tarball dist/mcharbor-plugin-<slug>-<version>.tar.gz \
  --key signing/plugin-signing.key
```

The signature is base64 ed25519, written next to the tarball as `.sig`. Add the public
half to the host's trust list with `MCHARBOR_PLUGIN_TRUST_KEYS`; keep the private half out
of the repository.

Install from **App Store → Plugins**, or drop the tarball into a catalog directory named
by `MCHARBOR_PLUGIN_CATALOG_DIRS`.

## The developer loop

For a plugin that ships inside the image, the only reliable path is to rebuild it:

```bash
docker compose build && docker compose up -d
```

The entrypoint then copies each bundled `.so` over the installed one and re-extracts the
matching tarball payload, so the frontend bundle, i18n and icons move with it. Copying
only the `.so` leaves the previous UI rendering, which is a confusing way to lose a fix.

For contributions that are not code (a changed menu entry, a new widget key), the loader
can rescan without a restart:

```bash
curl -X POST http://localhost:8705/api/plugins/loader/reload
```

## Debugging

| Symptom | Where to look |
|---|---|
| The plugin does not load | The container log at start-up; the loader names the plugin and the reason |
| `plugin built with a different Go toolchain` | The `.so` was built by another Go version than the host. Rebuild it; the host refuses it and keeps running |
| `plugin was built with a different version of package …` | Toolchain or module-graph mismatch; rebuild against the host's |
| It loads, nothing appears | The contribution is registered in code but missing from the manifest |
| A route 404s | Check `GET /api/plugins/loader/routes` for what was actually mounted |
| The form is empty | `ConfigSchema` is empty and no `config_form` is declared |
| A secret is blank after saving | Expected: secrets are never returned; the host keeps the stored value unless you replace it |

## Related

- [Overview](../../plugins/overview.md): what a plugin is, and the lifecycle
- [Architecture](./architecture.md): the loader, the ABI check, trust keys, the loader endpoints
- [Catalog](../../plugins/catalog.md): every shipped plugin, each with its own developer page
