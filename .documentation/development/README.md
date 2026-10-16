# Development

Documentation for people who **change McHarbor's code**: the source under `src/` and the build
and dev helpers in `scripts/development/`. The rest of the documentation set describes what
McHarbor does. This section describes how it is built and extended.

## Pages

| Page | Covers |
|---|---|
| [Development workflow](./workflow/README.md) | Toolchain, what is where under `src/`, the conventions that hold across it, building every artifact with `scripts/development/`, local dev data, testing, Windows traps |
| [Frontend](./frontend/README.md) | What to build a screen from: the app shell and routing, `TablePage`, the UI primitives and shared composites, data hooks and error handling, permissions, i18n and styling tokens |
| [Hooks](./hooks/README.md) | Every extension hook point: the backend registries and `HostContext` a plugin receives, the adapter families, the frontend mount points and their contexts, `window.McharborHost`, and the shared React hooks |
| [Contracts](./contracts/README.md) | How to use the contracts, and every one of them listed: the `Plugin` interface and registries, the adapter families and providers, the host services, the declaration types, the browser contract, and the contract tests |
| [Plugin development](./plugins/README.md) | Developing a plugin, the runtime architecture, `mcharbor plugin dev`, and a developer page for each of the 55 shipped plugins |
| [Add-on development](./add-ons/README.md) | Developer pages for the six add-ons |
| [Workflow development](./workflows/README.md) | Custom, core and plugin workflow nodes |

## Quick start

1. Run from source: [Development workflow → Quick start](./workflow/README.md#quick-start).
2. To extend McHarbor without touching the host, pick a hook point in [Hooks](./hooks/README.md)
   and follow [Plugin development](./plugins/README.md).
3. Make the change, keeping to the conventions in
   [Development workflow](./workflow/README.md#conventions-that-hold-across-src).
4. Add the tests the change needs, and run the [contract tests](./contracts/README.md).
5. Build what you changed with `scripts/development/`.

## Elsewhere

| Where | What |
|---|---|
| [How-to](../how-to/README.md) | Task guides for extending McHarbor: plugin, permissions, widget, node, AI provider, report, API scripting |
| `scripts/development/README.md`, `README-plugins.md` | The dev-data helpers and the plugin build scripts in full |
