// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { truncateId } from '@resources/utils/format';
import type { AuditEntry } from '@resources/components/AuditEntryDetails';
import { useAuditColumns } from '../components/AuditColumns';
import { useAuditFilters, useEntityTypeOptions, useFilteredAuditEntries } from './useAuditFilters';
import { useAuditLogsPage } from '@resources/hooks/useAuditLogs';

export const AUDIT_VIEW_IDS = ['events', 'denials', 'ai', 'retention', 'coverage'] as const;
export type AuditViewId = (typeof AUDIT_VIEW_IDS)[number];

/**
 * Everything AuditPage needs that is not markup: the filter state, the page of
 * entries behind it, the derived row range, and the dialogs' open state.
 *
 * Extracted so the page component stays a layout: the query wiring and the
 * four pieces of dialog state were most of its length.
 */
export function useAuditPage() {
  const { t } = useTranslation('common');
  const environments = useEnvironmentStore((s) => s.environments);
  const filters = useAuditFilters();
  const [selected, setSelected] = useState<AuditEntry | null>(null);
  const [exportOpen, setExportOpen] = useState(false);
  const [actionsOpen, setActionsOpen] = useState(false);
  const [actionModuleFilter, setActionModuleFilter] = useState('');
  const [verifyOpen, setVerifyOpen] = useState(false);

  // Actions, entity type, range and the contains/exact search are all matched by
  // the API, so `total` below counts real matches rather than the page in hand.
  const {
    data: auditData,
    isLoading,
    isFetching,
    refetch,
  } = useAuditLogsPage({
    page: filters.page,
    perPage: filters.pageSize,
    actions: filters.serverActions,
    entityType: filters.entityTypeFilter,
    range: filters.resolvedRange,
    search: filters.query,
    mode: filters.mode,
    actor: filters.actor,
    severities: filters.severities,
  });

  const entries = useMemo(() => auditData?.items ?? [], [auditData]);
  const total = auditData?.total ?? 0;
  const totalPages = Math.max(auditData?.total_pages ?? 1, 1);
  const rangeFrom = total === 0 ? 0 : (filters.page - 1) * filters.pageSize + 1;
  const rangeTo = entries.length === 0 ? 0 : rangeFrom + entries.length - 1;

  const envName = useCallback(
    (id: string | null, storedName?: string | null) => {
      if (!id) return t('audit.local');
      if (storedName) return storedName;
      return environments.find((e) => e.id === id)?.name ?? truncateId(id);
    },
    [environments, t],
  );

  const [view, setView] = useTabFromUrl<AuditViewId>(AUDIT_VIEW_IDS, 'events', {
    param: 'view',
  });

  const entityTypeOptions = useEntityTypeOptions(entries);
  const filteredEntries = useFilteredAuditEntries(entries, filters.query, filters.mode, envName);
  const columns = useAuditColumns(envName, setSelected);

  return {
    filters,
    selected,
    setSelected,
    exportOpen,
    setExportOpen,
    actionsOpen,
    setActionsOpen,
    actionModuleFilter,
    setActionModuleFilter,
    verifyOpen,
    setVerifyOpen,
    isLoading,
    isFetching,
    refetch,
    entries,
    total,
    totalPages,
    rangeFrom,
    rangeTo,
    envName,
    view,
    setView,
    entityTypeOptions,
    filteredEntries,
    columns,
  };
}
