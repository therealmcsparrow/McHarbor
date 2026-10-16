// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType } from 'react';

type BundleGlobal = Record<string, unknown> & {
  mount?: (el: HTMLElement, ctx: unknown) => unknown;
  widgets?: Record<string, ComponentType<unknown>>;
  nodes?: Record<string, ComponentType<unknown>>;
  tabs?: Record<string, ComponentType<unknown>>;
};

declare global {
  interface Window {
    [key: `McharborPlugin_${string}`]: unknown;
  }
}

const bundleCache = new Set<string>();
const inflight = new Map<string, Promise<void>>();

export function bundleGlobalKey(slug: string): `McharborPlugin_${string}` {
  return `McharborPlugin_${slug.replace(/-/g, '_')}`;
}

export function readPluginGlobal(slug: string): unknown {
  return window[bundleGlobalKey(slug)];
}

function readBundle(slug: string): BundleGlobal | null {
  return (readPluginGlobal(slug) as BundleGlobal | undefined) ?? null;
}

export function loadPluginBundle(slug: string): Promise<void> {
  const key = slug;
  if (bundleCache.has(key)) {
    if (readBundle(slug)) return Promise.resolve();
    bundleCache.delete(key);
  }
  const existing = inflight.get(key);
  if (existing) return existing;

  const p = new Promise<void>((resolve, reject) => {
    const url = `/api/plugins/loader/bundle?${new URLSearchParams({ slug }).toString()}`;
    const script = document.createElement('script');
    script.src = url;
    script.async = true;
    script.onload = () => {
      bundleCache.add(key);
      inflight.delete(key);
      resolve();
    };
    script.onerror = () => {
      inflight.delete(key);
      reject(new Error(`failed to load plugin UMD bundle for ${slug}`));
    };
    document.head.appendChild(script);
  });
  inflight.set(key, p);
  return p;
}

export async function loadPluginComponent(
  slug: string,
  namespace: 'widgets' | 'nodes' | 'tabs',
  key: string,
): Promise<ComponentType<unknown> | null> {
  await loadPluginBundle(slug);
  const bundle = readBundle(slug);
  if (!bundle) return null;
  const ns = bundle[namespace];
  if (!ns) return null;
  const ns2 = ns as Record<string, ComponentType<unknown>>;
  return ns2[key] ?? null;
}

export async function loadPluginExport(
  slug: string,
  namespace: 'widgets' | 'nodes' | 'tabs',
  key: string,
): Promise<unknown> {
  await loadPluginBundle(slug);
  const bundle = readBundle(slug);
  if (!bundle) return null;
  const ns = bundle[namespace] as Record<string, unknown> | undefined;
  if (!ns) return null;
  return ns[key] ?? null;
}

export function forgetPluginBundle(slug: string): void {
  bundleCache.delete(slug);
  inflight.delete(slug);
}
