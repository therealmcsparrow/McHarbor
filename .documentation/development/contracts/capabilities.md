# Capability Declarations

A **capability** is a fact a plugin states in `plugin.yaml` so the host can find it without
knowing the plugin's slug. The host never names a plugin
([host-plugin-boundary](../../../.claude/rules/host-plugin-boundary.md)). When a host page needs
something only a plugin can supply, such as the page a VM links to or the route an LDAP sign-in
posts to, it asks the capability registry which loaded plugin declares it.

Capabilities are **manifest-only**. There is no Go interface to implement and no `.so` symbol,
so adding or changing one never changes a plugin's ABI. The loader reads them from
`<install dir>/plugin.yaml` after the plugin finishes loading. The host-only package
`src/backend/core/plugincaps` validates and holds them, and no plugin links that package.

## Declaring one

```yaml
contributes:
  capabilities:
    - kind: runbook-provider
      domain: vm
    - kind: lifecycle-subject
      subject_type: vm
      page: "/plugins/addon-kvm"
  routes:
    # …
```

Each entry has a `kind` and the fields that kind uses. Any other field is refused. A `runbook-provider` may also carry `i18n_ns`, `targets` and `steps` (see [Runbook providers](#runbook-providers)).

## Kinds

| `kind` | Fields (all required) | Keyed by | Host consumer |
|---|---|---|---|
| `runbook-provider` | `domain`; optional `i18n_ns`, `targets`, `steps` | `domain` | Runbook catalog (`modules/runbooks`). The plugin declares the steps and selector lists of its domain (`vm`, `ct`, `compute`, `swarm`, `k8s` today) and serves the routes they call. |
| `backup-row-actions` | `source_kind`, `pages` {`open`, `restore`, `edit_plan`}, `routes` {`delete_run`, `delete_plan`}, `permissions` {`view`, `manage`, `delete`, `restore`} | `source_kind` | Row actions on Backups → Runs and Backups → Schedules for rows whose `sourceKind` matches and whose `sourceSlug` is the declaring plugin. |
| `estate-source` | `metric`, `path`, `environment_field` (unless `env_scoped`); optional card fields `label_key` + `i18n_ns`, `icon`, `env_scoped`, `running_field` + `running_values`, `link` | `metric` | Dashboard estate summary and environment cards. `vms` is the only metric the dashboard reads. It counts the items `GET path` returns, and the distinct values of `environment_field`. A source with `label_key` also adds a tile to each environment card (see below). |
| `lifecycle-subject` | `subject_type`, `page` | `subject_type` | Link target for lifecycle events of that subject type. |
| `document-export` | `formats`, `permission`, `generate_path`, `export_path` | — | Runbook export offers PDF and DOCX while a plugin declaring both formats is loaded and the caller holds `permission`. `generate_path` is the `POST` route that renders a report and `export_path` the `GET` route that downloads it (`{id}` is the report id); each must be a route the manifest declares. |
| `login-endpoint` | `provider_type`, `path` | `provider_type` | The sign-in form posts a directory provider's credentials to `path`. |
| `picker-source` | `target` | `target` | The workflow editor's `plugin-select` field. Exposes one of the plugin's own runbook-provider `targets[]` lists as a picker (see [Picker sources](#picker-sources)). |
| `workflow-node-alias` | `alias`, and exactly one of `node` or `channel_type` | `alias` | The workflow executor, the trigger-binding scan and the canvas. Resolves a bare action a saved canvas still carries to `plugin:<slug>:<node>`, or to the host's `send-communication` node on the plugin's own channel type (see [Workflow node aliases](#workflow-node-aliases)). |
| `email-channel` | `channel_type` | `channel_type` | The workflow `send-email` node's server field, its default-channel resolution, and `supportsEmail` on `GET /api/communication-channels/adapters` (the security email picker). Says the plugin's channel type delivers addressed email (see [Email channels](#email-channels)). |
| `console-subject` | `subject_type`, `consoles`, `permission` | `subject_type` | The host console gateway (`modules/compute_console`). Names a guest kind, the console kinds the agent may open for it (`vnc`, `spice`, `serial`, `tty`, `exec`) and the gate key, which must be in the plugin's `adds_permissions`. See [Console subjects](#console-subjects). |
| `agent-rpc` | `send`, optional `observe` | — | The agent bridge (`core/agentrpc`). Narrows the host's ceiling to the message types the plugin may send and the unsolicited frames it may observe. See [Agent RPC](./agent-rpc/README.md). |
| `reboot-drain` | `subject_type`, `routes` {`running`, `stop`} | `subject_type` | The host reboot (`modules/system`). Two plugin routes that list and stop running guests. See [Reboot drain](./reboot-drain/README.md). |
| `search-source` | `i18n_ns`, `sources` | — (one per plugin) | The Ctrl+K palette (`GlobalSearch`). Lists the palette searches while open, one group per source. See [Search palette](./search/README.md). |
| `search-action` | `i18n_ns`, `actions` | — (one per plugin) | The Ctrl+K palette's Actions group. Navigation-only entries into the plugin's own pages, each gated on a key the plugin owns or consumes. See [Search palette](./search/README.md). |

Placeholders in `pages` and `routes`: `{subject}` is the backed-up object's id and `{id}` is the
run or schedule id. The host path-escapes every value.

## Runbook providers

A `runbook-provider` states which runbook steps a plugin offers and how to run them, so the
runbook engine holds no route, parameter or permission of any plugin. `domain` is required;
`i18n_ns`, `targets` and `steps` are optional, and a provider that declares none of them offers
nothing to runbooks.

```yaml
- kind: runbook-provider
  domain: vm
  i18n_ns: vm
  targets:
    - kind: vm
      label_key: runbook.targets.vm
      path: "/api/vms"
      query: {env: "$env"}
      id: id
      name: [name]
      detail: [state]
  steps:
    - key: vm.shutdown
      description_key: runbook.steps.vm_shutdown
      kind: action
      target: vm
      destructive: true
      permissions: [vm.manage]
      params:
        - {name: vm, type: target, target: vm}
        - {name: graceSeconds, type: int, min: 0, max: 3600}
      call:
        method: POST
        path: "/api/vms/{vm}/shutdown"
        body: [{json: graceSeconds, param: graceSeconds, type: int}]
```

**`i18n_ns`** is the plugin's own i18n namespace (its `i18n/<lang>/<ns>.json` file), required
whenever `steps` or `targets` is present. Each step's `description_key` is a dotted key in that namespace, and
the runbook editor resolves it there. Ship the key in all six locales.

**`targets[]`** describe the selector lists the editor fills from the plugin. A kind the host has never heard of renders like any other: the catalog answer carries each loaded kind with its `label_key` and the provider's `i18n_ns`, and the editor titles the selector with that text.

| Field | Meaning |
|---|---|
| `kind` | The selector kind. Must be the `domain` or start with `<domain>_`, so a plugin cannot claim another provider's kind. |
| `label_key` | Dotted key in `i18n_ns` naming the kind itself (`Virtual machine`), used as the selector's title. Ship it in all six locales. |
| `path` | A `GET` route the plugin declares, with no placeholders. |
| `query` | Query parameters, each from `$env` (the run's environment), `cluster` (the request's `cluster`) or `=literal`. |
| `id` or `id_join` | The JSON field holding the id, or the fields joined with `/` into a composite id (a pod is `namespace/name`). Exactly one. |
| `items` | The response field holding the list, when the list is wrapped in an object. |
| `name`, `detail` | The fields read for the label and the detail line. `name` takes the first non-empty. |
| `attach` | Optional block that lets a runbook be attached to the objects this target lists (see [Attach kinds](#attach-kinds)). |

**`steps[]`** are the actions and diagnostics.

| Field | Meaning |
|---|---|
| `key` | The step key stored in runbooks. It must start with `<domain>.` and is never renamed: a stored runbook names it. |
| `description_key` | Key in `i18n_ns`. |
| `kind` | `action` or `diagnostic`. A diagnostic is read-only, so its `call.method` must be `GET`, and only an action can be `destructive`. |
| `target` | `environment` for a step that acts on the run's environment, or one of this provider's `targets[].kind`. |
| `permissions` | Keys checked against the operator before the call. Each is in `adds_permissions` or `consumes_permissions`, and must include the `requires_permission` of the route the step calls. |
| `params[]` | `name`, `type` (`target`, `enum`, `int`, `string`, `bool`), and `target`, `options`, `min` and `max`, `required` where the type uses them. A step with a `target` other than `environment` has exactly one `target` param of that kind. |
| `call` | `method`, `path`, `query`, `body`, `pick_id`, `text`. |

In `call.path`, `{name}` is a step param, `{$env}` is the run's environment, and
`{param.namespace}` and `{param.name}` split a `namespace/name` value. Every value is
path-escaped, so a param can never add a path segment. `call.query` values are `$env`,
`=literal` or a param name. `call.body` maps a JSON field to a param and its type. `pick_id`
names the param that selects one item out of a list response, and `text` names a response
field printed as-is, for logs. The two are exclusive. `call.path` is a route the plugin declares
under `contributes.routes`, with that method.

The host calls the route in-process as the operator, so the plugin's own permission middleware
still decides. A provider that is not loaded contributes no steps: the editor does not offer
them, a runbook cannot be saved with one, and a stored step fails with
`no installed plugin serves <domain> steps`.

A step key is a contract with every runbook already stored.
`TestEveryFormerStepIsProvidedByExactlyOnePlugin` (`modules/runbooks/plugin_catalog_test.go`)
reads the list in `former_steps_test.go`, which names the keys that shipped, literally, with their target, params, permissions and call, and fails
when a manifest drops, renames or changes one. Add a new key to that list in the same change.

### Attach kinds

The runbook editor's **Attach to** list is the four host kinds (`alert_condition`, `container`,
`stack`, `environment`) plus whatever loaded plugins declare. The host holds no plugin attach
kind, so a plugin that owns attachable objects says so on the target that lists them:

```yaml
targets:
  - kind: k8s_deployment
    label_key: runbook.targets.k8s_deployment
    path: "/api/kubernetes/deployments"
    id_join: [namespace, name]
    name: [name]
    attach:
      kind: k8s_workload
      label_key: runbook.attach.k8s_workload
      subject_targets: [k8s_pod, k8s_deployment]
```

| Field | Meaning |
|---|---|
| `kind` | The attach kind stored in runbooks. A token that is the `domain` or starts with `<domain>_`, and is not one of the four host kinds. It is a contract with every runbook already stored, so it is never renamed. |
| `label_key` | Dotted key in `i18n_ns` that titles the kind in the editor, the list and the run page. Ship it in all six locales. |
| `subject_targets` | The target kinds whose steps a run for this subject may execute. Each must be a `kind` under this manifest's `targets`. Empty means the target that carries the block. |

An attach kind is declared once per manifest, and across plugins the first slug to declare it
wins. The same shared cases run in `plugincaps` and in `mcharbor plugin dev validate`.

The catalog (`GET /api/runbooks/catalog`) carries `attachKinds` for every loaded provider. A
runbook that names a kind no loaded plugin declares is shown as **Provider not installed:
&lt;kind&gt;** and cannot be saved with that kind (`err.plugin.not_installed`), so uninstalling a
plugin never deletes a runbook, and reinstalling it restores the label.

The shipped declarations are `vm` (`addon-kvm`), `ct` (`addon-lxc`), `swarm_service`
(`addon-swarm`) and `k8s_workload` (`addon-kubernetes`).

## Picker sources

A `picker-source` lets a workflow node offer a pick list of the plugin's objects without the host
naming the plugin's route. It does not describe a list of its own: `target` names one of the same
manifest's runbook-provider `targets[]`, and the list is read exactly as the runbook editor reads
that target.

```yaml
- kind: runbook-provider
  domain: vm
  i18n_ns: vm
  targets:
    - {kind: vm, label_key: runbook.targets.vm, path: "/api/vms", query: {env: "$env"}, id: id, name: [name], detail: [state]}
- kind: picker-source
  target: vm
```

The target's `GET` route must declare `requires_permission`, and that key must be one the plugin
adds or consumes. The picker is gated on that key, which is the key the plugin's own permission
middleware checks when the list is fetched. The loader endpoint serves the capability with a
resolved `picker` object: `path`, `query`, `items`, `id` or `idJoin`, `name`, `detail`,
`labelKey`, `i18nNs` and `permission`.

A node uses it from its descriptor with the `plugin-select` field type:

```go
{"key": "vm", "label": "Virtual Machine", "type": "plugin-select", "allowCustom": true,
    "source": map[string]any{"plugin": "addon-kvm", "target": "vm"}}
```

The loader pins `source.plugin` to the plugin registering the node, whatever it says, so a node
can list only through its own plugin's pickers. A `target` the manifest does not declare is
logged at registration (`workflow node field names an undeclared picker source`), and the field
then renders disabled. In the editor, `$env` in `query` is the node's environment and is left out
when the node has none. `cluster` is ignored, and `=literal` is sent as written. Items map to ids,
titles and details exactly as `parseTargets` in `modules/runbooks` maps them.

The shipped declarations are `vm` (`addon-kvm`, used by the VM trigger), `ct` (`addon-lxc`), `swarm_service` and
`swarm_node` (`addon-swarm`), and `k8s_deployment` and `k8s_pod` (`addon-kubernetes`). The
`ct-*`, `swarm-*` and `kubernetes-*` workflow nodes use them with `allowCustom`, so a typed id
still works and a workflow saved before the picker existed runs unchanged. The Kubernetes nodes
also accept a `namespace/name` value, which a Kubernetes name can never contain. A test in
`core/plugincaps/picker_nodes_test.go` fails when a node names a source its plugin does not
declare.

When the declaring plugin is not installed, not enabled or not loaded, the field stays on the
form, disabled, reading "The plugin that provides this list is not installed or not enabled."
When the operator lacks the list's permission, the field is disabled with the usual "Requires the
`<key>` permission" tooltip, and no list request is made.

## Workflow node aliases

A node that moved from the host into a plugin keeps the bare action every saved canvas stores
(`vm-trigger`, for example), while a plugin node's action is `plugin:<slug>:<key>`. A
`workflow-node-alias` bridges the two without a data migration:

```yaml
- kind: workflow-node-alias
  alias: vm-trigger
  node: vm-trigger
```

`alias` matches the node-key alphabet `^[a-z0-9][a-z0-9-]{0,62}$`, so a composite key can never
be aliased. `node` must be listed under the same manifest's `contributes.nodes`. While the plugin
is loaded, the host resolves the alias in three places: `ExecuteNode` runs the node as the
plugin's node (on a copy, so the stored canvas keeps its action), `PluginTriggerBindings` counts
the aliased placement when the plugin fires its trigger, and the canvas resolves the node's
definition to the plugin node. A built-in node always wins over an alias, because the executor's
switch and the canvas's built-in catalogue are consulted first.

With the plugin absent, the alias does not resolve. The node stays on the canvas with its stored
label and configuration and no definition, and a run treats it as an unknown action, passing the
message through. Reinstalling the plugin restores it.

### Channel-type aliases

The host once shipped one send node per messenger (`send-slack`, `send-discord`, `send-teams`,
`send-telegram`, `send-whatsapp`, `send-signal`, `send-ntfy`, `send-gotify`); all of them were
folded into the generic `send-communication` node. Each comm plugin now claims its own former
key, pointing at its channel type instead of at a node:

```yaml
- kind: workflow-node-alias
  alias: send-slack
  channel_type: slack
```

`channel_type` must be the `key` of one of the same manifest's `contributes.forms` entries with
`resource: channel`, so a plugin can only speak for the channel type it registers. While the
plugin is loaded, `ExecuteNode` runs the stored node as `send-communication` with that
`channel_type` (a `channel_type` already in the node's config wins), so an empty channel picks the
default channel of that provider, exactly as the host's old rewrite table did; the canvas shows
it as the Send Message node. With the plugin absent it degrades like any unresolved alias: shown
without a definition, passed through as an unknown action. The host's own rename table
(`modules/workflows/legacy_aliases.go`, `resources/nodes/legacyAliases.ts`) keeps only
host-to-host renames and names no provider.

## Email channels

An `email-channel` says that the channel type the plugin registers delivers addressed email (a
sender, recipients, a subject), rather than a chat post or a push notification:

```yaml
- kind: email-channel
  channel_type: smtp
```

The same `contributes.forms` rule as a channel-type alias applies. comm-smtp, comm-gmail and
comm-exchange declare it. The consumers:

- The **Email Server** field of the `send-email` node (`email-server-select`) lists the enabled
  channels whose type an active plugin declares. With none loaded it renders disabled, reading
  "No installed communication plugin can send email."
- With no server picked, the node sends through the default channel among those types, and fails
  with `no installed communication plugin can send email` when there are none, instead of
  falling back to whichever channel is the estate default. A stored `server_id` resolves by id,
  as before.
- `supportsEmail` on `GET /api/communication-channels/adapters` reads the same declaration, so the
  security module's email picker and the workflow node cannot disagree. It used to be inferred
  from the adapter's config schema (a `from_address` plus a `to`).

## Estate sources

An `estate-source` always feeds the dashboard's estate summary. When it also names a `label_key`, each environment card on the Environments page shows one tile for it, under the container counts. The host reads these fields and never names the plugin:

| Field | Meaning |
|---|---|
| `label_key` + `i18n_ns` | The tile's title, a key in the plugin's own namespace. They come together, and every other card field needs them. |
| `icon` | Required with `label_key`. One of the names in `src/contracts/plugincaps/search_icons.yaml`, the list the search palette uses. |
| `env_scoped` | When `true` the card requests `GET path?env=<environment id>` and counts every item returned; `environment_field` is then optional. Otherwise the list is requested once for all cards and the card counts the items whose `environment_field` equals its environment id. |
| `running_field` + `running_values` | Optional, together, at most eight values. The tile reads `running/total`, counting items whose field equals one of the values; without them it shows the total. |
| `link` | Optional `/plugins/<slug>` path the tile links to. `{environmentId}` is the only placeholder allowed. |

The route has to be a `GET` the manifest declares with a `requires_permission` the plugin owns, because the card requests it for every viewer. A source whose request fails (the viewer lacks the key, the plugin is disabled, the agent is offline, the environment is not of that kind) or that lists nothing for the environment is left off that card, never shown as zero. Cards only ask while the environment is active and connected, and refresh every 30 seconds.

## Console subjects

A `console-subject` tells the host's console gateway which guest kinds a plugin owns:

```yaml
- kind: console-subject
  subject_type: vm
  consoles: [vnc, serial, spice]
  permission: vm.console
```

`consoles` are agent console kinds, each listed once. `permission` must be a key the plugin adds
itself, since opening a console is root in the guest. The gateway (`POST /api/compute/console/ticket`,
then the `GET /api/compute/console/{ticket}` WebSocket) reads the guest kind, the allowed console kinds
and the gate key from these declarations and calls the agent bridge directly. `addon-kvm` declares `vm`
with `vnc`, `serial`, `spice` gated by `vm.console`; `addon-lxc` declares `ct` with `tty`, `exec`
gated by `ct.console`.

## Agent RPC and reboot drain

Both kinds have a page of their own: [Agent RPC](./agent-rpc/README.md) (what a plugin may send to
an agent, and how it calls) and [Reboot drain](./reboot-drain/README.md) (the two routes a host
reboot calls).

## Validation

`plugincaps.Parse` checks each entry against the rest of the same manifest:

- `kind` is one of the fourteen above. The fields are exactly the ones the kind uses.
- `domain`, `source_kind`, `subject_type`, `metric`, `provider_type` and each `formats` item
  match `^[a-z][a-z0-9_]{0,62}$`.
- A page (`page`, `pages.*`) is `/plugins/<own slug>` or a path under it. A plugin cannot point a
  host link at another plugin or at a host page.
- A route (`path`, `routes.*`) is a route the plugin declares under `contributes.routes`, with the
  right method. `estate-source` needs `GET`, `backup-row-actions` needs `DELETE`, `login-endpoint`
  needs a `POST` marked `public: true`, and `document-export` needs a `POST` for `generate_path`
  and a `GET` for `export_path`.
- A permission (`permission`, `permissions.*`) is in the plugin's `adds_permissions` or
  `consumes_permissions`.
- A `runbook-provider` is held to the rules in [Runbook providers](#runbook-providers): step keys carry the domain prefix, every route is one the plugin declares, every permission is one it owns, a diagnostic is a `GET`, and every placeholder, query source and body field names a param.
- A `picker-source` names a runbook-provider target of the same manifest, declared in any order,
  whose `GET` route declares a `requires_permission` the plugin owns.
- A `console-subject` lists known console kinds once each and a `permission` in `adds_permissions`.
- An `agent-rpc` lists `send` types inside the host ceiling and `observe` types the host can observe, each once.
- A `reboot-drain` names a `GET` `running` route and a `POST` `stop` route the plugin declares, with no placeholders, not public, each with a `requires_permission` the plugin adds or consumes.
- A `search-source` names `GET` routes the plugin declares, without placeholders, each with a `requires_permission` the plugin adds or consumes; so does a source's optional `fan_out` list, which is exclusive with `env_scoped`. A `search-action` names a `permission` the plugin adds or consumes. Both link only into `/plugins/<own slug>` and use icons from `src/contracts/plugincaps/search_icons.yaml` (see [Search palette](./search/README.md)).
- A `workflow-node-alias` names a bare `alias` and exactly one of a `node` listed under
  `contributes.nodes` or a `channel_type`.
- A `channel_type` (`email-channel`, channel-type alias) matches `^[a-z][a-z0-9_]{0,62}$` and is
  the `key` of a `contributes.forms` entry with `resource: channel` in the same manifest.
- One plugin declares each `kind` and key pair only once.

A declaration that fails is **dropped whole** and logged
(`plugin capabilities refused; the plugin itself is unaffected`). The plugin still loads and
works. Only the host surfaces that depend on the capability stay hidden.

The CLI applies the same rules without importing the host: `mcharbor plugin dev validate`
(`src/cli/internal/cli/plugin_validate_caps.go`) carries its own copy. The two are kept in step by
`src/contracts/plugincaps/cases.yaml`, a set of accept and refuse manifests with the message text
each refusal must contain. `core/plugincaps` (`TestParseSharedCases`) and the CLI
(`TestCapabilitiesSharedCases`) both run every case, and `TestCapabilitiesInTreeManifestsPass` runs
the CLI rules over each in-tree `plugin.yaml`. A rule changed in one place fails the other suite
until both agree; add a case there for any new rule.

## Resolution and lifecycle

- The registry holds a plugin's capabilities from a successful load until `UnloadOne`
  (uninstall, or a rescan that drops it). A failed load publishes nothing.
- When two loaded plugins claim the same key, the first slug in sort order wins, so the result
  never depends on load order.
- The frontend also treats a **disabled** plugin as absent. A disabled plugin keeps running until
  restart, but the host has always hidden its surfaces, so the capability hooks join each entry
  against the installed list's `enabled && loaded`.
- With no provider, every consumer falls back to its absent state. The runbook entries and target
  kinds are hidden, and a step already saved fails with
  `no installed plugin serves <domain> steps`. Backup rows show no actions, lifecycle links point
  nowhere, and the estate summary leaves out the VM segment. Runbook export offers JSON only, and
  a directory sign-in fails without posting the credentials. A `plugin-select` field renders
  disabled with its reason, and an aliased workflow node stays on the canvas unresolved. The
  `send-email` server field renders disabled, and a `send-email` run with no server picked fails.

## Reading them

| Route | Auth | Returns |
|---|---|---|
| `GET /api/plugins/loader/capabilities` | Signed in, no permission key, like every other loader table | Every capability of every loaded plugin, `{plugin, kind, …camelCase fields}` |
| `GET /api/plugins/loader/login-endpoints` | Public, since the login screen runs before a session exists | `[{providerType, path}]` for `login-endpoint` capabilities only |

In the browser, `@core/plugins/capabilities` provides `usePluginCapabilities(kind)`,
`usePluginCapability(kind, key)`, `usePickerSource(source)`, `resolveNodeAliasAction`,
`resolveChannelAlias`, `useEmailChannelTypes`, `apiClientPath`, `fillPath` and
`fetchLoginEndpoint`. The
loader store fetches the table once at boot, next to the menu and widgets.

In Go, a host module reads `plugincaps.Default.Provider(kind, key)` or `OfKind(kind)`, or has the
lookup injected as a narrow interface the way `runbooks.RouterGateway` does. The workflow engine
resolves aliases through `plugincaps.Default.NodeAliasAction` (replaceable with
`Service.SetNodeAliasResolver` in tests) and `ChannelNodeAlias`, and the email types through
`EmailChannelTypes` / `IsEmailChannel`.

## Adding a kind

A kind always has a host consumer. To add one:

1. Add the `Kind` constant, its entry in `kindFields`, its `discriminator` and its validation in
   `core/plugincaps/capabilities.go`. Add the name to `PluginCapabilityKind` (and `KEY_FIELD`) in
   `src/frontend/core/plugins/capabilities.ts`.
2. Cover it in `core/plugincaps/capabilities_test.go`, port the rule to `src/cli/internal/cli/plugin_validate_caps.go`, and add a case to `src/contracts/plugincaps/cases.yaml`. `TestShippedManifestsDeclareValidCapabilities`
   parses every in-tree `plugin.yaml`, so a shipped declaration that stops validating fails there.
3. Document it in the table above.

## Tests

`core/plugincaps` (parsing, validation, registry, shipped manifests),
`core/pluginruntime/capabilities_test.go` (publish on load, withdraw on a refused declaration or
on unload), `modules/runbooks/plugin_catalog_test.go` (domains resolve from the manifests and
through the registry, every former step key is still provided by exactly one manifest, a
descriptor alone makes a step runnable, every description key exists in six locales), `modules/plugin_runtime/capabilities_test.go` (the public route's slice),
and one frontend test per consumer: `core/plugins/capabilities.test.ts`,
`modules/lifecycle/components/lifecycle-format.test.ts`,
`modules/dashboard/hooks/useEstateSummary.test.tsx`,
`modules/containers/pages/plugin-backup-actions.test.tsx`,
`modules/runbooks/components/RunbookExportMenu.test.tsx`, `core/auth/useAuth.test.ts`,
`modules/workflows/components/PluginSelect.test.tsx`, `modules/workflows/nodes/resolve.test.ts` and
`resources/components/GlobalSearch.plugins.test.tsx` (the two search kinds, with `core/plugincaps/search_test.go`).
The two workflow kinds are also covered by `core/plugincaps/picker_test.go`,
`core/pluginruntime/node_picker_source_test.go` (the loader pins a field's source to its own
plugin) and `modules/workflows/node_alias_test.go` (a stored bare action executes and binds as
the plugin node). The channel kinds are covered by `core/plugincaps/channel_test.go`,
`core/notify/anyof_test.go` (an any-of-types send never falls back to another type),
`modules/workflows/channel_caps_test.go` (a stored `send-<provider>` runs on the claiming
plugin's channel; `send-email` uses only declared email channels),
`modules/communications/email_capability_test.go` and, in the browser,
`modules/workflows/components/EmailChannelSelect.test.tsx`.
