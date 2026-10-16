# Search Palette Contract

How a plugin puts its objects and its actions into the Ctrl+K palette (`GlobalSearch`). The
palette used to search five Docker kinds only. A plugin now declares the lists it wants searched
and the actions it wants offered, and the host reads them through `usePluginCapabilities`. The
host never names the plugin, its routes or its pages.

Two capability kinds make up the contract. Both are manifest-only, so they never change a
plugin's ABI.

## `search-source`

```yaml
contributes:
  capabilities:
    - kind: search-source
      i18n_ns: vm
      sources:
        - key: vm
          label_key: search.group.vms
          path: "/api/vms"
          env_scoped: true
          id: id
          name: [name]
          detail: [state, description]
          link: "/plugins/addon-kvm/{id}"
          icon: device-desktop
```

| Field | Rule |
|---|---|
| `i18n_ns` | The plugin's own namespace. Every `label_key` resolves in it. |
| `sources` | One to eight entries. One `search-source` declaration per plugin. |
| `key` | `^[a-z][a-z0-9_]{0,62}$`, unique within the declaration. |
| `label_key` | The group heading. The palette shows it as "<label> (<count>)". |
| `path` | A `GET` route under `contributes.routes`, with no `{placeholders}`, not public, and with a `requires_permission` the plugin adds or consumes. The palette is gated on that key and makes no request without it. |
| `env_scoped` | `true` fetches once per environment with `?env=<id>`, like containers. Each result carries the environment's name, and opening it switches the topbar environment first. Omitted or `false` fetches once. |
| `items` | Optional dotted path to the array inside the response's `data`. Omit it when `data` is the array. `/api/kubernetes/summary` uses `pods` and `deployments`. |
| `id` | The field that identifies an item. An item without it is skipped. |
| `name` | One to four fields, joined with a space, for the item's label. Falls back to `id`. |
| `detail` | Up to four fields, joined with ` · `, for the secondary text. |
| `link` | `/plugins/<own slug>` or a path under it, with an optional `?name=value&…` query. Each segment is a page name or a `{field}` placeholder filled from the item and path-escaped; a query value may be a literal or one `{field}` placeholder. No scheme, host, `..`, `.` or fragment. An item whose placeholders cannot all be filled is skipped. |
| `icon` | A name from `src/contracts/plugincaps/search_icons.yaml`. The palette maps each to a Tabler icon it ships. |
| `fan_out` | Optional. Repeats the source once per item of another list the plugin owns. See below. Exclusive with `env_scoped`. |

### `fan_out`

A source whose route answers for one object at a time (one Kubernetes cluster, say) declares
the list of those objects, and the palette asks once per item:

```yaml
        - key: pod
          label_key: search.group.pods
          path: "/api/kubernetes/summary"
          items: pods
          id: name
          name: [name]
          detail: [namespace, status]
          link: "/plugins/addon-kubernetes/pods/{namespace}/{name}?cluster={cluster}"
          icon: box
          fan_out:
            path: "/api/kubernetes/clusters"
            id: id
            param: cluster
            label: name
```

| Field | Rule |
|---|---|
| `path` | A `GET` route the plugin declares, with no placeholders, not public, with a `requires_permission` the plugin adds or consumes. An operator without that key gets no group at all. |
| `items` | Optional dotted path to the array in the list's `data`. |
| `id` | The field whose value is sent as `?<param>=` on each request. A row without it is skipped. |
| `param` | `^[a-z][a-z0-9_]{0,30}$`, never `env`. Every row a request returns gets this field, so the link can carry `?<param>={<param>}`. |
| `label` | Optional field appended to each row's detail, so the operator sees which object a hit came from. Falls back to the id. |

The palette lists the fan-out route once (shared by every source that names it), then requests
the source's `path` for at most 16 items (`plugincaps.MaxFanOutTargets`). The plugin's page must
read the parameter from its URL (`PluginNavContext.search`) and select that object, or a hit on
the second cluster opens on the first.

Two sources that share a route, such as the Kubernetes pods and deployments, share one request.
The palette fetches only while it is open, and refetches every 30 seconds with a 10-second
stale time. A source that fails is left out. The rest of the palette still works, and the browser
console gets one `global search: a plugin search source failed and is left out` warning per
route and environment per page load. The palette also says so: a muted line under the results reads
"1 search source is unavailable, so its results are missing." (`globalSearch.sourcesUnavailable` in
`common`, with a count), so a missing group is not mistaken for a search that found nothing. An
uninstalled, disabled or unloaded plugin has no group and no line, since it declares nothing.

## `search-action`

```yaml
    - kind: search-action
      i18n_ns: vm
      actions:
        - key: vm.create
          label_key: search.action.createVm
          keywords_key: search.action.createVmKeywords
          icon: plus
          link: "/plugins/addon-kvm?action=create"
          permission: vm.manage
```

| Field | Rule |
|---|---|
| `key` | `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`, unique within the declaration. |
| `label_key` | The entry's label, in `i18n_ns`. |
| `keywords_key` | Optional. A translated, space-separated list of extra words the palette matches on. |
| `icon` | As for a source. |
| `link` | As for a source, but with no placeholders, since an action has no item. |
| `permission` | A key the plugin adds or consumes. An operator without it does not see the action, the same way the palette leaves out navigation entries and shell commands they cannot use. |

An action only navigates. No plugin code runs in the host. The entry joins the **Actions**
group next to the host's own commands.

### Receiving the action on the plugin page

A plugin bundle has no router of its own. The host passes the query string on mount and on
every navigation as `PluginNavContext.search`, so the page reads `?action=` from there. The
shipped plugins derive the dialog's open state from the parameter, then drop the parameter with
`ctx.navigate(ctx.path, { replace: true })` when the dialog closes. The page checks the same
permission with `usePermissionGate` and ignores the parameter without it.

## Shipped declarations

| Plugin | Sources (route) | Actions (`link`, `permission`) |
|---|---|---|
| `addon-kvm` | `vm` (`/api/vms`, per environment) | `vm.create` (`?action=create`, `vm.manage`), `vm.import` (`?action=import`, `vm.import`) |
| `addon-lxc` | `ct` (`/api/system-containers`, per environment) | `ct.create` (`?action=create`, `ct.manage`) |
| `addon-compute` | `host` (`/api/compute/environments`), `pool` (`/api/compute/pools`, per environment), `image` (`/api/compute/images`) | `compute.host.setup` (`?action=setup`, `compute.manage`), `compute.pool.create` (`/pools?action=create`, `compute.manage`), `compute.image.upload` (`/images?action=upload`, `compute.images.upload`) |
| `addon-kubernetes` | `deployment`, `pod` (`/api/kubernetes/summary`, server's default cluster) | `k8s.cluster.add` (`/clusters?action=create`, `kubernetes.clusters.manage`) |
| `addon-swarm` | `service` (`/api/swarm/services`, per environment) | `swarm.stack.deploy` (`/stacks?action=deploy`, `swarm_stacks.manage`) |

The Kubernetes sources list the server's default cluster, because the summary route takes the
cluster from `?cluster=` and the palette sends none. A pod in another cluster is not found until
that cluster is the default. The Swarm plugin has no "deploy service" dialog, so its action deploys
a stack.

## What goes wrong

| Symptom | Cause |
|---|---|
| The plugin loads but has no palette group or action. | The declaration was refused and logged as `plugin capabilities refused; the plugin itself is unaffected`. Run `mcharbor plugin dev validate`. |
| A group heading reads as a dotted key. | `label_key` is missing from the plugin's bundle for the current language. |
| A group never appears for one operator. | They lack the list route's `requires_permission`. |
| The palette shows "search source is unavailable". | The source's route failed for this request (a plugin error, or a fan-out target that did not answer). The other groups are unaffected; the console warning names the route. |
| A plugin page shows "This plugin view failed to render." | The plugin's React tree threw. The host's `PluginMountBoundary` shows the message with a **Retry** button, so the rest of the page keeps working. |
| A result opens the wrong host's page. | The source is not `env_scoped`, but the page reads the topbar environment. |
| A result opens on the wrong cluster (or other fan-out object). | The link does not carry `?<param>={<param>}`, or the page does not read it from `PluginNavContext.search`. |
| A fan-out source shows only the first 16 objects' hits. | That is the cap; the 17th object onward is not requested. |

## Validation and tests

`core/plugincaps/search.go` validates both kinds, and the CLI's `plugin dev validate` mirrors it
in `src/cli/internal/cli/plugin_validate_search.go`. Both run the search cases in
`src/contracts/plugincaps/cases.yaml`, and both hold their icon list to `search_icons.yaml`.
`TestShippedSearchDeclarations` pins the five plugins' declarations. In the browser,
`resources/components/GlobalSearch.plugins.test.tsx` covers per-environment listing, placeholder
links, a failing source, permission gating, the icon map, and fan-out (one request per target,
the target in the link, the cap, and the fan-out list's own gate).
`TestKubernetesSearchFansOutPerCluster` pins the Kubernetes fan-out, and
`test/plugins/k8s-cluster-link.test.tsx` that the Kubernetes pages select the cluster a link names.
