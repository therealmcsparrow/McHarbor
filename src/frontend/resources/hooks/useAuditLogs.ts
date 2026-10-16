// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useInfiniteQuery, useMutation, useQuery } from '@tanstack/react-query';
import i18n from 'i18next';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { DateRange } from '@resources/utils/date-range';
import type { SearchMode } from '@resources/utils/search-filter';
import { assertSuccess } from '@resources/utils/api-mutation';
import { downloadFile } from '@resources/utils/download';
import type { AuditEntry } from '@resources/components/AuditEntryDetails';

const PAGE_SIZE = 100;

export type AuditLogsPage = PaginatedData<AuditEntry>;

export interface AuditActionCatalogEntry {
  id: string;
  module: string;
  action: string;
  severityDefault: 'info' | 'success' | 'warning' | 'error';
  description: string;
}

export interface PluginAuditSummary {
  pluginSlug: string;
  count: number;
  lastSeen: string;
  installed: boolean;
  enabled: boolean;
  severityBreakdown: Record<'info' | 'success' | 'warning' | 'error', number>;
}

export interface AuditCoverageSummary {
  totalRoutes: number;
  requiredRoutes: number;
  mappedRoutes: number;
  unmappedRequired: number;
  coveragePercent: number;
  routes: Array<{
    id: string;
    module: string;
    method: string;
    path: string;
    handler: string;
    action: string;
    entityType: string;
    entityIdSource: string;
    auditRequired: boolean;
    rationale: string;
    eventCount: number;
  }>;
}

export function useAuditLogs(
  action: string,
  entityType: string,
  range: DateRange = { from: null, to: null },
  entityId?: string,
  enabled = true,
  includeEnvironment = true,
) {
  const envId = useEnvironmentStore((state) => state.currentId);

  const baseParams: Record<string, string> = {
    per_page: String(PAGE_SIZE),
    expand: 'diff',
  };
  if (action) baseParams.action = action;
  if (entityType) baseParams.entity_type = entityType;
  if (entityId) baseParams.entity_id = entityId;
  if (includeEnvironment && envId) baseParams.env = envId;
  if (range.from) baseParams.from = range.from.toISOString();
  if (range.to) baseParams.to = range.to.toISOString();

  // no polling: the audit log is append-only history, browsed and filtered
  // rather than watched. A timer would re-fetch every loaded page of an
  // infinite query; staleTime covers the "came back to the tab" case.
  return useInfiniteQuery({
    queryKey: [
      'audit-logs',
      envId,
      action,
      entityType,
      entityId ?? null,
      includeEnvironment,
      range.from?.toISOString() ?? null,
      range.to?.toISOString() ?? null,
    ],
    queryFn: async ({ pageParam }) => {
      const params: Record<string, string> = {
        ...baseParams,
        page: String(pageParam),
      };
      const response = await api.get<AuditLogsPage>('/audit', params);
      return assertSuccess(response);
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage) => (lastPage.page < lastPage.total_pages ? lastPage.page + 1 : undefined),
    enabled,
    staleTime: 15_000,
  });
}

export interface AuditLogsPageParams {
  page: number;
  perPage: number;
  /** Every ticked action; the API matches them as an OR set. */
  actions?: string[];
  entityType?: string;
  entityId?: string;
  range?: DateRange;
  search?: string;
  /**
   * `contains` and `exact` are matched by the API, so the reported total is the
   * real number of matches. `regex` has no portable SQL equivalent across the
   * three supported drivers and stays a client-side filter over the page.
   */
  mode?: SearchMode;
  enabled?: boolean;
  includeEnvironment?: boolean;
  actor?: string;
  severities?: string[];
}

// Server-paged twin of `useAuditLogs`. Same endpoint and same filters, but the
// caller owns the page number, so a pager can drive it instead of a scroll
// sentinel. The infinite variant stays for the container tabs that stream.
export function useAuditLogsPage({
  page,
  perPage,
  actions = [],
  entityType = '',
  entityId,
  range = { from: null, to: null },
  search = '',
  mode = 'contains',
  enabled = true,
  includeEnvironment = true,
  actor = '',
  severities = [],
}: AuditLogsPageParams) {
  const envId = useEnvironmentStore((state) => state.currentId);
  const serverSearch = mode === 'regex' ? '' : search.trim();

  // Built by hand rather than through the client's Record<string, string>
  // params: `action` repeats once per ticked action, which a plain object
  // cannot express.
  const query = new URLSearchParams();
  query.set('page', String(page));
  query.set('per_page', String(perPage));
  query.set('expand', 'diff');
  for (const action of actions) query.append('action', action);
  if (entityType) query.set('entity_type', entityType);
  if (entityId) query.set('entity_id', entityId);
  if (includeEnvironment && envId) query.set('env', envId);
  if (range.from) query.set('from', range.from.toISOString());
  if (range.to) query.set('to', range.to.toISOString());
  if (serverSearch) {
    query.set('search', serverSearch);
    query.set('search_mode', mode);
  }
  if (actor.trim()) query.set('actor', actor.trim());
  for (const severity of severities) query.append('severity', severity);
  const qs = query.toString();

  return useQuery({
    queryKey: ['audit-logs-page', qs],
    queryFn: async () => {
      const response = await api.get<AuditLogsPage>(`/audit?${qs}`);
      return assertSuccess(response);
    },
    // Keep the previous page on screen while the next one loads, so the table
    // does not collapse to the empty state on every page change.
    placeholderData: (previous) => previous,
    enabled,
    staleTime: 15_000,
  });
}

export function useAuditActions(module: string) {
  return useQuery({
    queryKey: ['audit-actions', module],
    queryFn: async () => {
      const params: Record<string, string> = {};
      if (module) params.module = module;
      const response = await api.get<AuditActionCatalogEntry[]>('/audit/actions', params);
      return assertSuccess(response);
    },
    staleTime: 60_000,
  });
}

export function usePluginAuditSummary() {
  return useQuery({
    queryKey: ['audit-plugin-summary'],
    queryFn: async () => {
      const response = await api.get<PluginAuditSummary[]>('/audit/plugins/summary');
      return assertSuccess(response);
    },
    staleTime: 30_000,
  });
}

export function useAuditCoverage() {
  return useQuery({
    queryKey: ['audit-coverage'],
    queryFn: async () => {
      const response = await api.get<AuditCoverageSummary>('/audit/coverage');
      return assertSuccess(response);
    },
    staleTime: 60_000,
  });
}

/**
 * useExportAuditCoverage saves the coverage report as a file. The route
 * answers with a JSON envelope, so it goes through fetch rather than the API
 * client, and the download is what the server audits as an export.
 */
export function useExportAuditCoverage() {
  return useMutation({
    mutationFn: () => downloadFile('/api/audit/coverage/export', 'mcharbor-audit-coverage.json'),
    meta: {
      success: i18n.t('audit.exportSuccess', { ns: 'common' }),
      error: i18n.t('audit.exportFailed', { ns: 'common' }),
    },
  });
}

export const auditAccessActions = new Set([
  'audit.accessed',
  'audit.exported',
  'audit.verified',
  'audit.verified.scheduled',
]);
