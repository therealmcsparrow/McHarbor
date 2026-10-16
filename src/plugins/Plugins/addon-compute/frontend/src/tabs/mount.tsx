// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

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
import { createPluginMutationCache } from '@resources/utils/pluginMutationCache';
import { guardedRoot } from '../mountGuard';

export type TabContext = {
  pageId: string;
  tabKey: string;
  entityId?: string;
};

export type TabEntry = {
  mount: (el: HTMLElement, ctx: TabContext) => void;
  update: (el: HTMLElement, ctx: TabContext) => void;
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
      mutationCache: createPluginMutationCache('compute'),
      defaultOptions: { queries: { refetchOnWindowFocus: false, retry: 1 } },
    });
  }
  return sharedClient;
}

type CtxStore = {
  get: () => TabContext;
  set: (ctx: TabContext) => void;
  subscribe: (listener: () => void) => () => void;
};

function createCtxStore(initial: TabContext): CtxStore {
  let current = initial;
  const listeners = new Set<() => void>();
  return {
    get: () => current,
    set: (ctx) => {
      current = ctx;
      listeners.forEach((listener) => listener());
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}

const TabCtx = createContext<CtxStore | null>(null);

export function useTabContext(): TabContext {
  const store = useContext(TabCtx);
  if (!store) throw new Error('useTabContext outside a tab mount');
  return useSyncExternalStore(store.subscribe, store.get, store.get);
}

function Providers({ store, children }: { store: CtxStore; children: ReactNode }) {
  const tree = (
    <QueryClientProvider client={queryClient()}>
      <TooltipProvider delayDuration={200}>
        <TabCtx.Provider value={store}>{children}</TabCtx.Provider>
      </TooltipProvider>
    </QueryClientProvider>
  );
  const i18n = hostI18n();
  return i18n ? <I18nextProvider i18n={i18n}>{tree}</I18nextProvider> : tree;
}

export function defineTab(Component: ComponentType): TabEntry {
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
