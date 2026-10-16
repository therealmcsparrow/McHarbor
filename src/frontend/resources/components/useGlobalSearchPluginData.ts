// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import { useQueries } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { hasPermission, useCurrentUserPermissions } from '@core/auth/usePermission';
import { apiClientPath, usePluginCapabilities } from '@core/plugins/capabilities';
import type { PluginSearchSource } from '@core/plugins/pickerTypes';
import type { CommandEntry } from '@resources/hooks/useCommandRegistry';
import { assertSuccess } from '@resources/utils/api-mutation';
import {
  fanOutTargets,
  pluginSearchItems,
  searchIcon,
  type FanOutTarget,
  type PluginSearchItem,
  type SearchEnv,
  type SearchIconComponent,
} from './globalSearchPlugins';

export type PluginSearchGroup = {
  id: string;
  label: string;
  icon: SearchIconComponent;
  items: PluginSearchItem[];
};

type DeclaredSource = { plugin: string; source: PluginSearchSource };
type SourceScope = { env?: SearchEnv; target?: FanOutTarget };
type SourceRequest = SourceScope & { path: string };

const scopeKey = ({ env, target }: SourceScope) => `${env?.id ?? ''}|${target ? `${target.param}=${target.id}` : ''}`;
const requestKey = (path: string, scope: SourceScope) => `${path}|${scopeKey(scope)}`;

// A broken plugin route would otherwise warn on every 30s refetch for as long as the
// palette stays open, so each failing request is reported once per page load.
const reportedFailures = new Set<string>();

function reportOnce(key: string, path: string, error: unknown) {
  if (reportedFailures.has(key)) return;
  reportedFailures.add(key);
  console.warn('global search: a plugin search source failed and is left out', path, error);
}

/** Every source the active plugins declare that the operator may list, fan-out list included. */
function useDeclaredSources(): DeclaredSource[] {
  const capabilities = usePluginCapabilities('search-source');
  const { data: permissions } = useCurrentUserPermissions();
  return useMemo(
    () =>
      capabilities.flatMap((c) =>
        (c.sources ?? [])
          .filter(
            (source) =>
              hasPermission(permissions, source.permission) &&
              (!source.fanOut || hasPermission(permissions, source.fanOut.permission)),
          )
          .map((source) => ({ plugin: c.plugin, source })),
      ),
    [capabilities, permissions],
  );
}

const pluginQuery = (isOpen: boolean) => ({
  refetchInterval: isOpen ? 30_000 : (false as const),
  staleTime: 10_000,
  retry: false,
  enabled: isOpen,
});

/**
 * Declared plugin sources, fetched only while the palette is open. Sources that share a
 * route (one summary backing two groups) share its request. A fan-out source first lists
 * its targets (Kubernetes clusters, say), then requests its route once per target.
 */
export function usePluginSearchSources(envs: SearchEnv[], isOpen: boolean) {
  const { t } = useTranslation('common');
  const sources = useDeclaredSources();

  const fanOutPaths = useMemo(
    () => [...new Set(sources.flatMap(({ source }) => (source.fanOut ? [source.fanOut.path] : [])))],
    [sources],
  );
  const fanOutQueries = useQueries({
    queries: fanOutPaths.map((path) => ({
      queryKey: ['global-search-plugin-fan-out', path],
      queryFn: () => api.get<unknown>(apiClientPath(path)).then(assertSuccess),
      ...pluginQuery(isOpen),
    })),
  });
  const fanOutData = new Map(fanOutPaths.map((path, i) => [path, fanOutQueries[i]?.data]));

  const scopesOf = (source: PluginSearchSource): SourceScope[] => {
    if (source.fanOut)
      return fanOutTargets(source.fanOut, fanOutData.get(source.fanOut.path)).map((target) => ({ target }));
    return source.envScoped ? envs.map((env) => ({ env })) : [{}];
  };

  const requests: SourceRequest[] = [];
  const seen = new Set<string>();
  for (const { source } of sources) {
    for (const scope of scopesOf(source)) {
      const key = requestKey(source.path, scope);
      if (seen.has(key)) continue;
      seen.add(key);
      requests.push({ path: source.path, ...scope });
    }
  }

  const queries = useQueries({
    queries: requests.map((request) => ({
      queryKey: ['global-search-plugin', request.path, scopeKey(request)],
      queryFn: () =>
        api
          .get<unknown>(
            apiClientPath(request.path),
            request.env
              ? { env: request.env.id }
              : request.target
                ? { [request.target.param]: request.target.id }
                : undefined,
          )
          .then(assertSuccess),
      ...pluginQuery(isOpen),
    })),
  });

  useEffect(() => {
    fanOutQueries.forEach((query, i) => {
      const path = fanOutPaths[i];
      if (query.isError && path) reportOnce(`fan-out|${path}`, path, query.error);
    });
    queries.forEach((query, i) => {
      const request = requests[i];
      if (query.isError && request) reportOnce(requestKey(request.path, request), request.path, query.error);
    });
  });

  const data = new Map(requests.map((request, i) => [requestKey(request.path, request), queries[i]?.data]));
  const groups: PluginSearchGroup[] = sources.map(({ plugin, source }) => ({
    id: `${plugin}:${source.key}`,
    label: t(source.labelKey, { ns: source.i18nNs }),
    icon: searchIcon(source.icon),
    items: scopesOf(source).flatMap((scope) => {
      const response = data.get(requestKey(source.path, scope));
      return response === undefined ? [] : pluginSearchItems(source, response, scope.env, scope.target);
    }),
  }));

  const failedSources = queries.filter((q) => q.isError).length + fanOutQueries.filter((q) => q.isError).length;

  return {
    groups,
    failedSources,
    isLoading: queries.some((q) => q.isLoading) || fanOutQueries.some((q) => q.isLoading),
  };
}

/**
 * Declared plugin actions the operator holds the key for. A denied one is left out, the
 * way the palette's navigation and shell commands leave out what the operator cannot use.
 */
export function usePluginSearchActions(): CommandEntry[] {
  const { t } = useTranslation('common');
  const capabilities = usePluginCapabilities('search-action');
  const { data: permissions } = useCurrentUserPermissions();
  return useMemo(
    () =>
      capabilities.flatMap((c) =>
        (c.actions ?? [])
          .filter((action) => hasPermission(permissions, action.permission))
          .map<CommandEntry>((action) => ({
            id: `plugin:${c.plugin}:${action.key}`,
            label: t(action.labelKey, { ns: action.i18nNs }),
            icon: searchIcon(action.icon),
            group: 'actions',
            to: action.link,
            keywords: action.keywordsKey
              ? t(action.keywordsKey, { ns: action.i18nNs }).split(/\s+/).filter(Boolean)
              : [],
            priority: 30,
          })),
      ),
    [capabilities, permissions, t],
  );
}
