// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Dashboard widget mounting.
//
// The host renders one div per widget tile and calls `mount(el, ctx)` on the
// entry this bundle exports under `widgets[key]` — the same imperative shape
// the page uses, and for the same reason: this bundle carries its own React,
// so a component of ours cannot be rendered inside the host's tree.
//
// Every widget of this plugin shares one QueryClient. Three tiles that all
// read `/kubernetes/summary` then cost one request per interval, not three.

import {
  createContext,
  useContext,
  useSyncExternalStore,
  type ComponentProps,
  type ComponentType,
  type ReactNode,
} from 'react';
import { type Root } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { guardedRoot } from '../mountGuard';

/** Mirrors `PluginWidgetContext` in the host's `core/plugins/PluginWidgetMount.tsx`. */
export type WidgetContext = {
  slug: string;
  key: string;
  typeId: string;
  colSpan: number;
  navigate: (to: string, opts?: { replace?: boolean }) => void;
};

export type WidgetEntry = {
  mount: (el: HTMLElement, ctx: WidgetContext) => void;
  update: (el: HTMLElement, ctx: WidgetContext) => void;
  unmount: (el: HTMLElement) => void;
};

type HostI18n = ComponentProps<typeof I18nextProvider>['i18n'];

function hostI18n(): HostI18n | null {
  return (window as Window & { McharborHost?: { i18n?: HostI18n } }).McharborHost?.i18n ?? null;
}

let sharedClient: QueryClient | null = null;

function queryClient(): QueryClient {
  if (!sharedClient) {
    sharedClient = new QueryClient({
      defaultOptions: { queries: { refetchOnWindowFocus: false, retry: 1 } },
    });
  }
  return sharedClient;
}

/** A tiny external store so `update` re-renders without remounting. */
type CtxStore = {
  get: () => WidgetContext;
  set: (ctx: WidgetContext) => void;
  subscribe: (listener: () => void) => () => void;
};

function createCtxStore(initial: WidgetContext): CtxStore {
  let current = initial;
  const listeners = new Set<() => void>();
  return {
    get: () => current,
    set: (ctx) => {
      current = ctx;
      listeners.forEach((l) => l());
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}

const WidgetCtx = createContext<CtxStore | null>(null);

/** The mount context — `navigate`, `colSpan` — from inside a widget. */
export function useWidgetContext(): WidgetContext {
  const store = useContext(WidgetCtx);
  if (!store) throw new Error('useWidgetContext outside a widget mount');
  return useSyncExternalStore(store.subscribe, store.get, store.get);
}

function Providers({ store, children }: { store: CtxStore; children: ReactNode }) {
  const tree = (
    <QueryClientProvider client={queryClient()}>
      <TooltipProvider delayDuration={200}>
        <WidgetCtx.Provider value={store}>{children}</WidgetCtx.Provider>
      </TooltipProvider>
    </QueryClientProvider>
  );
  const i18n = hostI18n();
  return i18n ? <I18nextProvider i18n={i18n}>{tree}</I18nextProvider> : tree;
}

/** Wrap a widget component in the mount contract the host expects. */
export function defineWidget(Component: ComponentType): WidgetEntry {
  const mounted = new WeakMap<HTMLElement, { root: Root; store: CtxStore }>();
  return {
    mount(el, ctx) {
      const store = createCtxStore(ctx);
      const root = guardedRoot(el);
      mounted.set(el, { root, store });
      root.render(
        <Providers store={store}>
          <Component />
        </Providers>,
      );
    },
    update(el, ctx) {
      mounted.get(el)?.store.set(ctx);
    },
    unmount(el) {
      const entry = mounted.get(el);
      if (!entry) return;
      mounted.delete(el);
      entry.root.unmount();
    },
  };
}
