# How-to

Task-shaped guides for extending McHarbor. Each one takes a thing you want to build, and
walks the whole path (the files, the contract, the gates, the rebuild) rather than
describing a subsystem and leaving the assembly to you.

The rest of the documentation set answers "what is this and how does it behave". These pages
answer "how do I add one".

## Guides

| Guide | Build |
|---|---|
| [Build a plugin](./plugin/README.md) | A signed, installable extension with a Go backend, a React bundle, widgets, nodes, tables and translations |
| [Contract permissions](./permissions/README.md) | A new permission key, wired through all five layers so the capability is actually governed |
| [Build a dashboard widget](./widgets/README.md) | A tile on the dashboard, built-in or shipped inside a plugin |
| [Build a workflow node](./nodes/README.md) | A node for the workflow editor: built-in Go, sandboxed JavaScript, or plugin-contributed |
| [Add an AI provider](./ai/README.md) | A provider adapter the assistant can talk to, and the consent model around it |
| [Build a reporting plugin](./reporting/README.md) | A plugin that turns platform records into documents, using the shipped reporting add-on as the worked example |
| [Script the API](./api/README.md) | Automation against the HTTP API: credentials, pagination, streams, error handling |
| [Add an app to the catalog](./apps/README.md) | A Store app entry, generated, indexed and published to the official catalog |
| [Change an agent's environment variables](./agent/README.md) | A reconfigured remote agent (native or container) and the restart that makes it take |

## Which surface do I need?

Most "can McHarbor do X" questions resolve to one of five extension points. Pick by what the
thing actually is:

| You want | Build |
|---|---|
| A new page, with its own tables, routes and menu entry | A [plugin](./plugin/README.md) |
| A tile on the dashboard | A [widget](./widgets/README.md) |
| A step in an automation | A [workflow node](./nodes/README.md) |
| A new backup destination, notification channel, identity provider, image scanner, AI provider, snapshot driver or log forwarder | A plugin that registers the matching **adapter**; see [Plugins](../plugins/README.md) for the family you are filling |
| To read or drive McHarbor from outside | The [API](../api/README.md), scripted as in [Script the API](./api/README.md) |

A rule that saves rework: **if the feature is a plugin, the plugin owns all of it**: the
route, the handler, the SQL, the page, the strings, the permission key. Nothing belonging to
a plugin feature goes in `src/backend/modules/` or `src/frontend/modules/`. If you find
yourself adding a host module for a plugin feature, the design has gone wrong.

## Before you start

Three constraints catch everyone once.

**Go plugins are Linux-only.** `-buildmode=plugin` does not exist on Windows or macOS. Build
in the container, or on a Linux host.

**The toolchain must match.** A plugin compiled against a different Go version, or a
different build of the host's shared packages, is rejected at load rather than at build.
Rebuild the host image after changing anything shared.

**In-tree changes reach the container only through a rebuild.**

```bash
docker compose build && docker compose up -d
```

That is the last step of every guide here, and skipping it is the most common reason a change
appears to have done nothing.

## Conventions these guides assume

- **Every source file carries the copyright header**, in the comment syntax of its language.
- **Every user-facing string goes through `t(...)`**: JSX text, `aria-label`, `placeholder`,
  `title`, `alt`, toasts, dialog titles, table headers. English is the source of truth and
  the other locales fall back to it until translated; a hardcoded English string fails CI.
- **Handlers stay thin.** Parse, validate, check auth, call the service, audit, respond. No
  SQL in a handler.
- **Errors never reach the client raw.** Log the real error server-side, return a code.
- **Components stay small** and use the shared UI primitives, the theme tokens, and no inline
  styles.

Each guide names the specific rules that bite in its area.

## Related

- [API](../api/README.md): every endpoint, and the conventions above the routes
- [Plugins](../plugins/README.md): the runtime, and every plugin that ships
- [Workflows](../workflows/README.md): the engine these nodes run inside
- [Screens](../screens/README.md): where a widget, tab or menu entry ends up
- [Put guests on your network](../compute-foundation/guest-networking.md): an
  operating guide rather than an extension one, so it lives with the Compute plugin
