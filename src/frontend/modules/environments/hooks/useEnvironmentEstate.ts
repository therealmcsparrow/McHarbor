// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQueries } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { apiClientPath, fillPath, usePluginCapabilities } from '@core/plugins/capabilities';
import type { PluginCapability } from '@core/plugins/loader';
import { assertSuccess } from '@resources/utils/api-mutation';

const REFRESH_MS = 30_000;

type EstateItem = Record<string, unknown>;

export type EnvironmentEstateEntry = {
  key: string;
  labelKey: string;
  ns: string;
  icon: string;
  total: number;
  running?: number;
  link?: string;
};

type CardSource = PluginCapability & { path: string; labelKey: string; labelNs: string; icon: string };

function isCardSource(c: PluginCapability): c is CardSource {
  return !!(c.path && c.labelKey && c.labelNs && c.icon && (c.envScoped || c.environmentField));
}

/** Counts what one environment owns out of a source's list; a source that is not env scoped lists every environment's. */
function summarise(items: EstateItem[], source: CardSource, environmentId: string): EnvironmentEstateEntry | null {
  const own = source.envScoped ? items : items.filter((item) => item[source.environmentField ?? ''] === environmentId);
  if (own.length === 0) return null;
  const field = source.runningField;
  const running = field ? own.filter((item) => source.runningValues?.includes(String(item[field]))).length : undefined;
  return {
    key: `${source.plugin}/${source.metric ?? source.path}`,
    labelKey: source.labelKey,
    ns: source.labelNs,
    icon: source.icon,
    total: own.length,
    running,
    link: source.link ? fillPath(source.link, { environmentId }) : undefined,
  };
}

/**
 * What the plugins that declare an `estate-source` with a `label_key` report for one
 * environment. A source that fails (no view key, plugin down, agent offline) or lists
 * nothing for this environment is left out rather than shown as zero.
 */
export function useEnvironmentEstate(environmentId: string, enabled: boolean): EnvironmentEstateEntry[] {
  const sources = usePluginCapabilities('estate-source').filter(isCardSource);
  const results = useQueries({
    queries: sources.map((source) => ({
      // A source that is not env scoped is one shared list, so every card reuses the same request.
      queryKey: ['environment-estate', source.plugin, source.path, source.envScoped ? environmentId : 'all'],
      queryFn: () =>
        api
          .get<EstateItem[]>(apiClientPath(source.path), source.envScoped ? { env: environmentId } : undefined)
          .then(assertSuccess),
      enabled: enabled && !!environmentId,
      retry: false,
      staleTime: REFRESH_MS,
      refetchInterval: REFRESH_MS,
    })),
  });
  return sources.flatMap((source, i) => {
    const items = results[i]?.data;
    const entry = Array.isArray(items) ? summarise(items, source, environmentId) : null;
    return entry ? [entry] : [];
  });
}
