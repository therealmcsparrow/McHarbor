// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, type ComponentType } from 'react';
import type { WidgetCatalogDefinition } from '@resources/hooks/useWidgetCatalog';
import { resolvePluginIcon } from '@resources/layout/pluginIcons';
import { createPluginWidgetComponent, isPluginWidgetEntry } from '@core/plugins/PluginWidgetMount';
import type { WidgetProps } from './catalog';
import type { WidgetDefinition } from './registry';

export const DOWNLOADED_PREFIX = 'downloaded:';

declare global {
  interface Window {
    [key: `McharborWidget_${string}`]: unknown;
  }
}

export function downloadedWidgetGlobal(key: string): `McharborWidget_${string}` {
  return `McharborWidget_${key.replace(/-/g, '_')}`;
}

/**
 * The browser never judges a signature itself: it trusts only the server's
 * `verified` flag, and the bundle route re-verifies before serving. Anything
 * else — a built-in, a plugin state row, a tampered install — gets no script tag.
 */
export function isLoadableDownloaded(def: WidgetCatalogDefinition): boolean {
  return def.source === 'downloaded' && def.verified === true && /^[a-z0-9][a-z0-9-]*$/.test(def.key);
}

const inflight = new Map<string, Promise<unknown>>();

export function loadDownloadedBundle(key: string): Promise<unknown> {
  const existing = inflight.get(key);
  if (existing) return existing;
  const p = new Promise<unknown>((resolve, reject) => {
    const script = document.createElement('script');
    script.src = `/api/widgets/${encodeURIComponent(key)}/bundle`;
    script.async = true;
    script.onload = () => resolve(window[downloadedWidgetGlobal(key)]);
    script.onerror = () => {
      inflight.delete(key);
      reject(new Error(`failed to load widget bundle ${key}`));
    };
    document.head.appendChild(script);
  });
  inflight.set(key, p);
  return p;
}

function lazyDownloadedComponent(key: string): ComponentType<WidgetProps> {
  return lazy(async () => {
    const entry = await loadDownloadedBundle(key);
    if (!isPluginWidgetEntry(entry)) {
      inflight.delete(key);
      throw new Error(`widget bundle ${key} does not export { mount, unmount, update }`);
    }
    return { default: createPluginWidgetComponent('downloaded', key, entry) };
  });
}

/** Returns null for anything that must not load, so callers cannot register it by accident. */
export function toDownloadedDefinition(def: WidgetCatalogDefinition): WidgetDefinition | null {
  if (!isLoadableDownloaded(def)) return null;
  return {
    id: `${DOWNLOADED_PREFIX}${def.key}`,
    labelKey: `widgets.${def.key}.label`,
    descriptionKey: `widgets.${def.key}.description`,
    category: 'plugin',
    // The sidebar icon registry holds Tabler icons but types them as the wider IconType.
    icon: resolvePluginIcon(def.icon) as WidgetDefinition['icon'],
    defaultSize: def.defaultSize,
    minSize: def.minSize,
    component: lazyDownloadedComponent(def.key),
  };
}
