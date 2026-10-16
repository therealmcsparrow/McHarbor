# Plugin CLI commands

A plugin can contribute commands to `mcharbor`. Its capabilities then reach the three places
an operator works (the interface, the API and the terminal) instead of only the first.

This page is for whoever writes a plugin. For using the commands, see
[CLI → Plugin commands](../../cli/README.md). For the local authoring toolchain (`plugin dev
new`, `build`, `validate`), see [Authoring CLI](../../development/plugins/authoring-cli/README.md), which is a
different thing: it runs on your machine and talks to no server.

## What a command is

A declaration, never a handler. The plugin says "there is a command at this path, it calls
this route, it needs this permission, render these columns"; the CLI builds the request,
confirms it, renders the answer and picks the exit code.

That split is deliberate. It means a plugin cannot invent its own confirmation rules for a
destructive command, cannot advertise a weaker permission than its route enforces, and cannot
ship its own HTTP client. It also means the command and the route share one route string, so
renaming a route without updating the command is a compile error in the plugin rather than a
404 an operator discovers.

## The interface

```go
// Optional. A plugin with no commands implements nothing.
func (p *MyPlugin) RegisterCLI(reg plugin.CLIRegistry) error {
    if err := reg.Root(plugin.CLIRoot{
        Name:    "widget",
        Short:   "Widgets on this host",
        Aliases: []string{"widgets"},
    }); err != nil {
        return err
    }

    return reg.Add(plugin.CLICommand{
        Path:      []string{"list"},
        Kind:      plugin.CLIList,
        Method:    http.MethodGet,
        Route:     "/api/widgets",
        Needs:     "widget.view",
        EnvScoped: true,
        Columns: []plugin.CLIColumn{
            {Header: "ID", Path: "id"},
            {Header: "NAME", Path: "name"},
            {Header: "STATE", Path: "state"},
            {Header: "AGE", Path: "createdAt", Format: "age"},
        },
        Short: "List widgets",
    })
}
```

`Root` is called once, before any `Add`. The loader type-asserts for `CLIProvider` after
`RegisterTabs`, so a plugin built before this existed keeps loading unchanged.

## The seven interaction kinds

| Kind | What the CLI does | Requires |
|---|---|---|
| `CLIList` | Renders an array or a paginated envelope through `Columns` as table, json, yaml or tsv | at least one column |
| `CLIGet` | Renders one object | — |
| `CLIAction` | Confirms by `Risk`, mutates, reports in the past tense | — |
| `CLIText` | Writes the response body to stdout verbatim, with no parsing | — |
| `CLIUpload` | Streams `multipart/form-data` with byte progress on stderr, exempt from `--timeout` | exactly one flag of type `FlagFile` |
| `CLIDownload` | Streams the body to `--output-file` or stdout, refusing a terminal | no columns |
| `CLIAttach` | Opens a guest console: a ticket POST, then a WebSocket | a `GuestKind` |

`CLIUpload` has no timeout because an image upload is minutes, and a 60-second default would
abort every real one. `CLIDownload` refuses to write binary output to a terminal; `-f -`
forces stdout for a pipe.

`CLIAttach` is the one kind whose `Route` is not the plugin's own. The console is a core
route pair (`POST /api/compute/console/ticket` then
`GET /api/compute/console/{ticket}`), and the ticket exchange belongs to the CLI. A plugin
declares `GuestKind` (`vm` or `ct`) and the CLI does the rest; it cannot point `attach` at a
route of its own.

There is no `sse` kind. Nothing needs one yet, and adding it later is a constant, an executor
and a CLI release.

## Flags, arguments and bodies

`Args` are positional, in order, and fill the route's `{placeholders}`. A command with
`Args: []string{"id"}` and `Route: "/api/widgets/{id}"` is invoked as
`mcharbor widget get <id>`, and its usage line shows the argument.

A `CLIFlag`'s `Target` decides where its value lands:

- `TargetQuery`: appended as the query parameter named by `Field`.
- `TargetBody`: set at `Field`, read as a **dotted JSON path**, so
  `Field: "spec.memory.mb"` builds the nested object without the plugin shipping a template.
- `TargetPath`: fills the placeholder named by `Field`.

`Body` says where a mutating command's body comes from: `BodyNone`, `BodyFlags` (assembled
from the body-target flags), `BodyFile` (`--data @file`, or `-` for stdin) or `BodyBoth`
(the file as the base, flags overriding fields in it).

`EnvScoped: true` appends `?env=` from `--env` or the context. Set it on every command whose
route reads an environment, and check the handler rather than guessing.

## Root naming, and the names you cannot have

A root name matches `^[a-z][a-z0-9-]{1,30}$`. It must not collide with a core command, and
**every core command's aliases count**: `containers`, `images`, `volumes`, `users`, `groups`,
`roles`, `api-keys`, `apps`, `catalogs`, `policies`, `elevations`, `plugins`, `networks`,
`backups`, `stacks`, `ctr` and `environment` are all reserved, as are `tui` and `ui`. The
list lives in `core/pluginruntime/cli_reserved.go` and a test in the CLI module asserts it
against the real command tree, so it cannot drift as core commands are added.

Two plugins cannot share a root. The second one to claim it is refused and told which plugin
owns it.

## The permission agreement rule

`Needs` **must equal** the `requires_permission` your `plugin.yaml` declares for that
`{method, route}` pair, and the route must be declared there at all.

This rule fails more often than any other. A plugin
route's permission comes from the **manifest**, not from the Go registration: the loader
resolves it through the manifest and, on a miss, builds the permission middleware with an
empty key, which short-circuits to a pass-through. So an undeclared non-public route is
reachable by **any authenticated user, whatever their role**, and a route in that state
cannot satisfy this rule by design.

If a command is refused for naming a route the plugin does not declare, the fix is in
`plugin.yaml`, and it is also a live authorization gap worth fixing on its own.

`mcharbor plugin dev validate <slug>` checks exactly this before you build.

## Risk, and why a plugin does not choose its own rules

`Risk` maps onto the CLI's own confirmation levels:

- `RiskSafe`: prompts for nothing. Reads, and creates.
- `RiskDisruptive`: prompts on a terminal; `--yes` satisfies it. Anything that interrupts
  something running: stop, restart, drain, scale down.
- `RiskIrreversible`: **`--yes` is refused.** The target must be named with
  `--confirm <target>` or typed at the prompt. Anything that destroys data: delete, revert,
  restore, prune.

The target of an irreversible command is its first positional argument, or the route when it
takes none. A plugin cannot loosen these rules, which is the point: the CLI documents one
guarantee about destructive commands and it has to hold for every command in the tree.

## A rejected command set never disables the plugin

Validation happens at load time, in this order: root name, permission agreement, path
uniqueness, kind coherence. A failure **drops the whole command set**, logs at WARN with the
plugin and the reason, and reports it on the loader status endpoint.

The plugin's routes, pages, widgets, nodes and adapters are untouched. A mistake in a command
declaration must never take a working plugin offline, so it does not.

The set is dropped whole rather than partially: a tree that holds half a plugin's commands is
worse than one that holds none, because the missing half looks like a capability the plugin
does not have.

## How it reaches the operator

The host serves the whole surface at `GET /api/plugins/loader/cli`, authenticated, with an
ETag. The CLI caches it per context under `<config dir>/cli-spec/<context>.json` at `0600`
and **builds its tree from the cache alone**. It makes no request, so `mcharbor --help` works
offline and costs nothing.

The cache is fetched at `mcharbor login`, at `mcharbor context use`, and on demand with
`mcharbor plugin cli refresh`. `mcharbor plugin cli list` prints the cached tree and its age;
`mcharbor plugin cli explain <root> <path>` shows which route a command calls and what it
needs. `mcharbor doctor` reports the cache's age, the server build it came from, and any
command whose kind this CLI is too old to run.

A command whose `kind` an older CLI does not recognise is **skipped with a count**, never a
hard failure. `MCHARBOR_NO_PLUGIN_CLI=1` disables the surface entirely, for a CI job that
wants the tree frozen to what the binary itself ships.

When a plugin is uninstalled its rows go with it, so its root disappears from the next spec
every operator fetches. Until they refresh, their cached tree still offers commands that
now 404. `doctor` says so.

## What goes wrong

| Symptom | Cause |
|---|---|
| The root is missing entirely, plugin otherwise fine | The command set was refused. Check the loader status endpoint for the reason, or the WARN line naming the plugin. |
| "names route X, which this plugin does not declare" | The route is not in `plugin.yaml`'s `contributes.routes`. |
| "needs A but route X enforces B" | `Needs` and the manifest's `requires_permission` disagree. The manifest wins. |
| "a list command needs at least one column" | A `CLIList` with no `Columns` has nothing to render. |
| "root name X is a core command" | See the reserved list above; aliases count. |
| The commands exist but every call 404s | The cached spec is older than the server. `mcharbor plugin cli refresh`. |
| A command is absent but its siblings work | This CLI does not know its `kind`. `mcharbor doctor` reports the count. |

## Related

- [CLI](../../cli/README.md): using the commands, and the permission keys per group.
- [Authoring CLI](../../development/plugins/authoring-cli/README.md): the local toolchain, including `validate`.
- [Plugin architecture](../../development/plugins/architecture.md): where `RegisterCLI` sits among the hooks.
