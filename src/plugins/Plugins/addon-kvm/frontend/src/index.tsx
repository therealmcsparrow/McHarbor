// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type Root } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import type { ComponentProps } from 'react';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { App } from './App';
import { defineWidget } from './widgets/mount';
import { SummaryWidget } from './widgets/SummaryWidget';
import { ResourcesWidget } from './widgets/ResourcesWidget';
import { ProtectionWidget } from './widgets/ProtectionWidget';
import { ImportsWidget } from './widgets/ImportsWidget';
import { createPluginMutationCache } from '@resources/utils/pluginMutationCache';
import { guardedRoot } from './mountGuard';

// The UMD entry point.
//
// The host loads this bundle once and mounts it for the whole
// `/plugins/addon-kvm/*` subtree, so moving between the two sidebar entries
// arrives through `onNavigate` rather than as a remount. Keeping the React root
// alive across those navigations is what preserves the console session — a
// remount would drop the RFB connection and make the operator reconnect every
// time they looked at another tab.

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
 * This bundle compiles its own copy of `react-i18next`, so the module instance
 * the host wired up with `initReactI18next` is not the one `useTranslation`
 * reaches here — without a provider every `t()` echoes its key back. The host
 * publishes its live i18next instance on `window.McharborHost.i18n`; binding
 * to it gives the plugin the `vm` namespace the loader already merged
 * in, plus the user's current language and later language switches.
 */
// Typed against what `I18nextProvider` accepts rather than against `i18next`'s
// own `i18n`: the plugin resolves its own copy of i18next while the provider's
// declaration resolves the host's, and the two structurally-identical types do
// not unify. Taking the prop type straight off the component sidesteps the
// duplicate-declaration mismatch without an `any` or a cast.
type HostI18n = ComponentProps<typeof I18nextProvider>['i18n'];

function hostI18n(): HostI18n | null {
  return (window as Window & { McharborHost?: { i18n?: HostI18n } }).McharborHost?.i18n ?? null;
}

const listeners = new Set<(ctx: PluginNavContext) => void>();
let latest: PluginNavContext = {
  slug: 'vm',
  path: '',
  splat: '',
  search: '',
  navigate: () => {},
};

function subscribe(listener: (ctx: PluginNavContext) => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function mount(el: HTMLElement, ctx?: PluginNavContext) {
  if (ctx) latest = ctx;

  queryClient = new QueryClient({
    mutationCache: createPluginMutationCache('kvm'),
    defaultOptions: {
      queries: {
        // The guest list already polls on a 10s interval; a focus refetch on top
        // of that is a second round trip to every agent for no new information.
        refetchOnWindowFocus: false,
        retry: 1,
      },
    },
  });

  // Radix builds its provider contexts with no default value, so a `<Tooltip>`
  // with no `TooltipProvider` above it throws rather than degrading. The host
  // mounts one, but this bundle carries its own React and its own Radix copy —
  // that provider is invisible from in here, and the shared action buttons in
  // the tables use tooltips on every row.
  const tree = (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider delayDuration={200}>
        <App initial={latest} subscribe={subscribe} />
      </TooltipProvider>
    </QueryClientProvider>
  );

  const i18n = hostI18n();
  root = guardedRoot(el);
  // Without the host instance the plugin still renders — it just falls back to
  // showing the raw keys rather than blanking the page.
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

// Dashboard widgets, keyed as the backend registers them (RegisterWidgets in
// backend/main.go). The host reads `widgets[key]` and drives the mount
// contract from `widgets/mount.tsx`.
export const widgets = {
  'vm-summary': defineWidget(SummaryWidget),
  'vm-resources': defineWidget(ResourcesWidget),
  'vm-protection': defineWidget(ProtectionWidget),
  'vm-imports': defineWidget(ImportsWidget),
};

export default { mount, unmount, onNavigate, widgets };
