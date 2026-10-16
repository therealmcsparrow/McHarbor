// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentProps } from 'react';
import { type Root } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { App } from './App';
import { defineWidget } from './widgets/mount';
import { RecentReportsWidget } from './widgets/RecentReportsWidget';
import { ReportSchedulesWidget } from './widgets/ReportSchedulesWidget';
import { createPluginMutationCache } from '@resources/utils/pluginMutationCache';
import { guardedRoot } from './mountGuard';

// The UMD entry point.
//
// The host loads this bundle once and mounts it for the whole
// `/plugins/addon-reporting/*` subtree, so navigation between the plugin's
// three sidebar entries arrives through `onNavigate` rather than as a
// remount. Keeping the React root alive across those navigations is what
// preserves the query cache — and means clicking Library after generating
// a report does not refetch everything.

/**
 * PluginNavContext is the shape the host passes on mount and on every
 * in-plugin navigation. Mirrored from
 * `src/frontend/core/plugins/PluginPageHost.tsx`.
 */
export type PluginNavContext = {
  slug: string;
  path: string;
  splat: string;
  search: string;
  navigate: (to: string, opts?: { replace?: boolean }) => void;
};

let root: Root | null = null;
let queryClient: QueryClient | null = null;

/**
 * This bundle compiles its own copy of `react-i18next`, so the module
 * instance the host wired up with `initReactI18next` is not the one
 * `useTranslation` reaches here — without a provider every `t()` echoes
 * its key back. The host publishes its live i18next instance on
 * `window.McharborHost.i18n` (see `core/i18n/i18n.ts`); binding to it
 * gives the plugin the `reporting` namespace the loader already merged
 * in, plus the user's current language and later language switches.
 */
type HostI18n = ComponentProps<typeof I18nextProvider>['i18n'];

function hostI18n(): HostI18n | null {
  return (window as Window & { McharborHost?: { i18n?: HostI18n } }).McharborHost?.i18n ?? null;
}

// Listeners let App follow host navigation without the bundle being torn
// down and rebuilt.
const listeners = new Set<(ctx: PluginNavContext) => void>();
let latest: PluginNavContext = { slug: 'reporting', path: '', splat: '', search: '', navigate: () => {} };

function subscribe(listener: (ctx: PluginNavContext) => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function mount(el: HTMLElement, ctx?: PluginNavContext) {
  if (ctx) latest = ctx;

  queryClient = new QueryClient({
    mutationCache: createPluginMutationCache('reporting'),
    defaultOptions: {
      queries: {
        // A report is a fixed record; refetching one on window focus
        // would be pure noise.
        refetchOnWindowFocus: false,
        retry: 1,
      },
    },
  });

  const i18n = hostI18n();
  // Radix builds its provider contexts with no default value, so a
  // `<Tooltip>` with no `TooltipProvider` above it throws rather than
  // degrading. The host mounts one in `main.tsx`, but this bundle carries
  // its own React and its own Radix copy — that provider is invisible from
  // in here. Without this wrapper the library's action cells crash the
  // plugin root the moment a row renders, which is why the page looked
  // fine while it was empty and blanked as soon as a report existed.
  const tree = (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider delayDuration={200}>
        <App initial={latest} subscribe={subscribe} />
      </TooltipProvider>
    </QueryClientProvider>
  );

  root = guardedRoot(el);
  // Without the host instance the plugin still renders — it just falls
  // back to showing the raw keys rather than blanking the page.
  root.render(i18n ? <I18nextProvider i18n={i18n}>{tree}</I18nextProvider> : tree);
}

export function onNavigate(ctx: PluginNavContext) {
  latest = ctx;
  listeners.forEach((listener) => listener(ctx));
}

export function unmount() {
  listeners.clear();
  root?.unmount();
  queryClient?.clear();
  root = null;
  queryClient = null;
}

// Dashboard widgets, keyed as backend/main.go RegisterWidgets declares them.
// The host reads `widgets[key]` and drives the mount contract in widgets/mount.tsx.
export const widgets = {
  'recent-reports': defineWidget(RecentReportsWidget),
  'report-schedules': defineWidget(ReportSchedulesWidget),
};

export default { mount, unmount, onNavigate, widgets };
