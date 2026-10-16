# Plugin frontend

A plugin's interface is a **UMD** bundle, not ESM, so the host can load it lazily after an
install rather than at build time. The bundle exposes one global, and the host mounts from it.

## The entry point

```tsx
// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Root } from 'react-dom/client';
import { guardedRoot } from './mountGuard';
import { MyPluginPage } from './MyPluginPage';

let root: Root | null = null;

export function mount(el: HTMLElement) {
  root = guardedRoot(el);
  root.render(<MyPluginPage />);
}

export function unmount() {
  root?.unmount();
  root = null;
}
```

`mount` and `unmount` must be exported **by name**. A default export produces a page that
renders blank with no error, which is the single most common plugin frontend bug.

**Create every root with `guardedRoot(el)`, never a bare `createRoot(el)`.** `src/mountGuard.tsx`
exports it: it calls `createRoot` with React 19's `onUncaughtError`, logs the error, and renders
"This plugin view failed to render." (the host's `common:pluginMount.failed`, read from
`window.McharborHost.i18n`) in the mount point. A plugin root is a separate React tree, so the
host's own error boundaries cannot see it, and without the guard a render crash leaves the mount
point blank. This applies to every root a plugin creates: its page, each `widgets/mount.tsx`
entry and each `tabs/mount.tsx` entry. `mcharbor plugin dev new` writes `mountGuard.tsx` and the
entry point that uses it; the bundled plugins each carry their own identical copy, because plugins
share no code (see [Isolation](./isolation-and-families.md)). The host additionally wraps the
places it mounts plugin and widget bundles in a `PluginMountBoundary`, which shows the same message
with a **Retry** button.

The host auto-creates the SPA route from the manifest's menu path and mounts the bundle
there. The page does not register itself in the host router, and must not try to.

## The build

```ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'node:path';

const slug = 'my-plugin'; // keep in sync with plugin.yaml

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.tsx'),
      formats: ['umd'],
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'react-dom/client', 'react-i18next', 'i18next'],
    },
  },
});
```

**React and i18next are externals.** The host provides them once. Bundling your own React
gives you two copies, two contexts and a page that fails in ways that look like unrelated
hook errors.

The global name is derived from the slug with hyphens replaced by underscores. It has to match
what the host looks for.

## Calling your own API

Through the shared client, never through raw `fetch`. The client attaches the session
cookie, handles the envelope and surfaces the error code.

```tsx
import { useQuery, useMutation } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useTranslation } from 'react-i18next';

export function MyPluginPage() {
  const { t } = useTranslation('my-plugin');

  const items = useQuery({
    queryKey: ['my-plugin', 'items'],
    queryFn: () => api.get('/api/my-plugin/items').then(assertSuccess),
  });

  const create = useMutation({
    mutationFn: (body: { name: string }) =>
      api.post('/api/my-plugin/items', body).then(assertSuccess),
    meta: { success: t('toast.itemCreated') },
  });

  if (items.isLoading) return <Skeleton />;
  if (items.error) return <ErrorState error={items.error} />;

  return (
    <PageLayout>
      <PageHeader title={t('page.title')} description={t('page.subtitle')} />
      {/* … */}
    </PageLayout>
  );
}
```

Server state goes through TanStack Query; client state goes through a store. Never a raw
`fetch` into `useState`.

## The rules that apply

A plugin's frontend is held to the same standard as the host's, because the user cannot tell
which is which:

- **Strict TypeScript**, no escape-hatch `any`.
- **Shared primitives** from the host's UI library: its button, dialog, table, form. Do not
  ship your own.
- **Theme tokens only**: the semantic OKLCH tokens for background, foreground, border and
  muted text. No `bg-white`, no `text-gray-500`, no raw hex; a plugin using literal colours
  looks wrong in every theme but the one it was written against.
- **No inline `style={{}}`.** Tailwind utilities, with arbitrary values where a utility does
  not exist.
- **Icon-only buttons need an accessible label**, and images need alt text.
- **Components under 200 lines.** Extract to hooks.
- **Every string through `t()`** with your namespace named explicitly.

## The four UI hook points

A plugin can mount into the host interface in four places. Each is declared in the manifest
and mirrored into a runtime contribution table the frontend reads; see
[Manifest](./manifest.md) for the declaration syntax.

### 1. Sidemenu

The host appends plugin entries to the sidebar. Entries sharing a `parent` section id render
as their own named section rather than as loose items under **Plugins**.

`required_permission` hides the link outright. Navigation is the one place in the product
where hiding beats disabling: a dead-end page teaches the user nothing.

The icon is a Tabler **component name**, looked up in a registry of imported components. An
unknown name falls back to a puzzle piece, which is what a `.svg` filename here produces.

### 2. Topbar

Two mount points: left, after the environment selector, for page-scoped tools; right, before
the global search, for always-visible actions. `visibility` is either `always` or
`permission:<key>`.

Render one button that opens something: a dropdown, a dialog, a route. The topbar is not a
place for a panel.

### 3. Context menus

Entries on a host page's overflow or right-click menu. `page` is a host-defined page id, and
an unknown id fails the install. `when` is an optional one-condition JavaScript boolean
evaluated against the page's context object in a sandbox.

### 4. Tabs on host pages

A plugin can add a tab to a page it does not own. The host renders the tab strip; the plugin
owns the body.

The tab body is exposed on the plugin's global under a `tabs` map keyed by the declared key,
and lazily imported when the tab is first activated. The host hands the page's data context
(the route parameters, for instance) to the mount function as its second argument.

The permission hides the tab **and** gates the body, so revoking it takes effect immediately
rather than at the next reload.

## Widgets in a plugin

A plugin widget is not a React component the host renders. The bundle carries its own React,
so the host mounts each tile imperatively. The exported `widgets` map is keyed by widget key,
and each value is an object with `mount`, and optionally `unmount` and `update`.

The shared helper that wraps a component into that contract also gives it a query client, the
host's i18n instance and a tooltip provider. Copy it from a plugin that already ships tiles.

See [Build a dashboard widget](../widgets/README.md) for the visual contract and the
widget-kit primitives.

## i18n

```json
// i18n/en/my-plugin.json
{
  "menu":   { "root": "My Plugin" },
  "topbar": { "open": "Open My Plugin" },
  "tabs":   { "panel": "My Plugin" },
  "page":   { "title": "My Plugin", "subtitle": "Manage items" },
  "toast":  { "itemCreated": "Item created" }
}
```

The namespace is declared in the manifest and the host registers the bundle at install. Every
locale the host ships needs a file; English is the source of truth and the rest fall back to
it until translated. Manifest labels resolve through the same namespace, so the plugin keeps
ownership of its own wording.

## When the page is blank

In order of likelihood:

1. `mount` and `unmount` are not exported by name.
2. The UMD global name does not match the slug-derived name.
3. React was bundled instead of externalised.
4. The menu path in the manifest does not match the route the host created.
5. The bundle did not rebuild; the container is serving the old one.

## Related

- [Build a plugin](./README.md): the end-to-end path
- [Manifest](./manifest.md): declaring the hook points this page mounts into
- [Build a dashboard widget](../widgets/README.md)
- [Packaging](./packaging.md): rebuilding after a frontend change
