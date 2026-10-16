# Build a dashboard widget

A widget is one tile on the dashboard. There are three ways to ship one, and they have
different contracts:

| Kind | Lives in | Loaded as |
|---|---|---|
| **Built-in** | The repository's root `widgets/` catalog | A lazily imported React component |
| **Plugin** | A plugin's `widgets/` folder | An imperatively mounted object, because the plugin bundle carries its own React |
| **Downloaded** | A signed entry in a local catalog's `widgets/` folder | The same imperatively mounted object, loaded only when its signature verifies. See [Ship a downloaded widget](./downloaded/README.md) |

Build a built-in widget when the data is the host's. Build a plugin widget when the data
belongs to the plugin.

## Built-in widgets

### Layout

```
widgets/<widget-key>/
├── <WidgetName>Widget.tsx
└── i18n/
    ├── en.json
    ├── nl.json
    ├── de.json
    ├── es.json
    ├── fr.json
    └── pt.json
```

Widgets are lazily imported by key, and the i18n bundles are auto-loaded by glob and deep-
merged into the dashboard namespace. Creating the folder is most of the registration.

### Steps

1. **Choose a category**: `resources`, `host`, `metrics` or `monitoring`.
2. **Choose an icon** from the Tabler set.
3. **Create the folder** with the component and all six locale files.
4. **Register it**: add the key to the widget type union, add the definition to the registry,
   and add the lazy import.
5. **Add it to the backend's built-in definitions**, so a fresh install seeds it.

### Categories

| Category | For |
|---|---|
| `resources` | Counts: containers, images, volumes, networks |
| `host` | Host system facts: CPU, memory, Docker version, disk |
| `metrics` | Live charts |
| `monitoring` | Lists and feeds |

### Sizes

Grid units, not pixels. Pick the pattern that matches the shape of the content:

| Pattern | Default | Minimum | For |
|---|---|---|---|
| Stat card | 3 × 1 | 2 × 1 | One value and a description |
| Chart | 3 × 2 | 2 × 2 | A chart with a couple of figures |
| List or feed | 4 × 3 | 3 × 2 | Tables, feeds, anything scrolling |

The minimum matters more than the default: it is what stops a user resizing your widget into
something illegible.

### The definition

Each widget definition carries its id, the i18n keys for the picker label and description, its
category, its icon, its lazily-imported component, its default size and its minimum size. The
component receives the current column span and its type id.

### Strings

Widget body text comes from the widget's own key path; the picker label and description come
from the widget key under the shared widgets path. All six locales get the key in the same
change, English real and the rest falling back until translated. A hardcoded English string in
a widget fails CI like anywhere else.

### Writing the body

- Fill the tile. A widget that renders a fixed-height block inside a resizable tile looks
  broken at every size but one.
- Use the widget-kit primitives (the frame, the fitted row helper, the tile, the segment bar,
  the empty state) so your widget looks like the built-in ones and behaves the same when it
  has nothing to show.
- Handle three states: loading, empty and error. Empty is the one most often forgotten and the
  one users see first on a fresh install.
- Server data goes through TanStack Query, scoped to the selected environment where the data
  is per-environment.
- Theme tokens only, no inline styles, and an accessible label on any icon-only control.

## Plugin widgets

A plugin widget is **not** a React component the host renders. The plugin bundle carries its
own React, so the host mounts each tile imperatively.

```
backend/main.go            RegisterWidgets — one descriptor per tile
frontend/src/widgets/      mount helper + one component per tile
frontend/src/index.tsx     export const widgets = { '<key>': defineWidget(Component) }
plugin.yaml                contributes.widgets: ["<key>"]
i18n/<lang>/<ns>.json      widgets.<camelKey>.{label,description,title,…}
```

### The mount contract

Each entry in the exported `widgets` map is an object with `mount(el, ctx)`, and optionally
`unmount(el)` and `update(el, ctx)`. The shared helper wraps an ordinary component into that
shape and gives it a query client, the host's i18n instance and a tooltip provider. Copy it
from a plugin that already ships tiles rather than writing it again.

### The context object

`ctx` carries the host's navigate function, the current column span, and the plugin slug,
widget key and type id. Read it with the provided hook.

**Use the host's navigate for click-throughs.** Setting the browser location reloads the whole
application and loses the user's place; the host router does not.

### The descriptor

Registered on the backend, one per tile. Title and description are i18n keys in
`namespace:path` form. Width and height are the default grid size, and the minimum width and
height bound resizing. The icon is a Tabler component name from the host's icon registry, not
a filename.

The host stores the row and lists the widget under the picker's **Plugins** category, keyed by
plugin and widget key.

### Data

Through the shared API client, scoped with the host's environment bridge where the plugin's
API is per-environment. Not raw `fetch`: the client attaches the session cookie and handles
the envelope.

## Where widget definitions live at runtime

Definitions are also stored on the backend under the data directory and served by
`GET /api/widgets/definitions`. Built-in widgets are seeded at startup; downloaded and
plugin-contributed ones persist across container updates. The relevant endpoints are in
[Dashboard, audit, alerts and notifications](../../api/endpoints/monitoring.md).

## After the change

```bash
docker compose build && docker compose up -d
```

A built-in widget also needs the frontend to type-check and build; a plugin widget needs the
plugin rebuilt and reinstalled. See [Packaging](../plugin/packaging.md).

## Related

- [Build a plugin](../plugin/README.md)
- [Plugin frontend](../plugin/frontend.md): the bundle these tiles ship in
- [Dashboard screen](../../screens/dashboard/README.md): where they end up
- [Monitoring endpoints](../../api/endpoints/monitoring.md): the widget API
