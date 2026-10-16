// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The per-key request history, paginated and filtered server-side.
//
// Server-side because the table exists precisely for keys with a lot of
// traffic; pulling every row to filter in the browser would defeat the
// point and would put the whole history in memory to answer one question.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { TablePage } from '@resources/layout/TablePage';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import { useAPIKeyUsageEvents } from '../hooks/useAPIKeyUsage';
import { useAPIKeyUsageColumns } from './apiKeyUsageColumns';

type APIKeyUsageEventsTableProps = {
  apiKeyId: string;
};

const PER_PAGE = 25;

export function APIKeyUsageEventsTable({ apiKeyId }: APIKeyUsageEventsTableProps) {
  const { t } = useTranslation('security');
  const [page, setPage] = useState(1);
  const [method, setMethod] = useState('');
  const [statusClass, setStatusClass] = useState('');
  const [authOutcome, setAuthOutcome] = useState('');

  const { data: environments = [] } = useEnvironmentList();
  const environmentName = useMemo(
    () => (id: string | null) => {
      if (!id) return t('apiKeys.usage.noEnvironment');
      return environments.find((environment) => environment.id === id)?.name ?? id;
    },
    [environments, t],
  );

  const { data, isLoading, isFetching, isError, error, refetch } = useAPIKeyUsageEvents(apiKeyId, {
    page,
    perPage: PER_PAGE,
    method: method || undefined,
    statusClass: statusClass || undefined,
    authOutcome: authOutcome || undefined,
  });

  const columns = useAPIKeyUsageColumns(environmentName);

  const events = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = data?.total_pages ?? 1;
  const from = total === 0 ? 0 : (page - 1) * PER_PAGE + 1;
  const to = Math.min(page * PER_PAGE, total);

  const resetTo = (setter: (value: string) => void) => (value: string) => {
    setter(value);
    setPage(1);
  };

  const methodOptions = [
    { value: '', label: t('apiKeys.usage.allMethods') },
    ...['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map((verb) => ({ value: verb, label: verb })),
  ];
  const statusOptions = [
    { value: '', label: t('apiKeys.usage.allStatuses') },
    { value: '2xx', label: '2xx' },
    { value: '3xx', label: '3xx' },
    { value: '4xx', label: '4xx' },
    { value: '5xx', label: '5xx' },
  ];
  const outcomeOptions = [
    { value: '', label: t('apiKeys.usage.allOutcomes') },
    ...(['accepted', 'revoked', 'expired', 'owner_invalid'] as const).map((outcome) => ({
      value: outcome,
      label: t(`apiKeys.usage.outcome.${outcome}`),
    })),
  ];

  return (
    <TablePage
      id="api-key-usage-events"
      header={false}
      title={t('apiKeys.usage.historyTitle')}
      savedFilters={{
        state: { method, statusClass, authOutcome },
        onRestore: (s) => {
          setMethod(typeof s.method === 'string' ? s.method : '');
          setStatusClass(typeof s.statusClass === 'string' ? s.statusClass : '');
          setAuthOutcome(typeof s.authOutcome === 'string' ? s.authOutcome : '');
          setPage(1);
        },
        pristine: !method && !statusClass && !authOutcome,
        defaults: { method: '', statusClass: '', authOutcome: '' },
      }}
      filters={
        <div className="flex flex-wrap gap-2">
          <Select
            value={method}
            onChange={resetTo(setMethod)}
            options={methodOptions}
            searchable={false}
            ariaLabel={t('apiKeys.usage.method')}
            className="w-40"
          />
          <Select
            value={statusClass}
            onChange={resetTo(setStatusClass)}
            options={statusOptions}
            searchable={false}
            ariaLabel={t('apiKeys.usage.status')}
            className="w-40"
          />
          <Select
            value={authOutcome}
            onChange={resetTo(setAuthOutcome)}
            options={outcomeOptions}
            searchable={false}
            ariaLabel={t('apiKeys.usage.outcomeLabel')}
            className="w-48"
          />
        </div>
      }
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh: () => void refetch(), isFetching },
        // No export. The history carries client addresses and user agents;
        // turning it into a downloadable file is a separate decision with
        // its own privacy question, not a toolbar convenience.
      }}
      columns={columns}
      data={events}
      loading={isLoading}
      error={isError ? (error ?? new Error(t('apiKeys.usage.error'))) : undefined}
      onRetry={() => void refetch()}
      empty={{
        title: t('apiKeys.usage.noEvents'),
        description: t('apiKeys.usage.noEventsHint'),
      }}
      getRowId={(evt) => evt.id}
      pagination={{
        mode: 'server',
        page,
        totalPages,
        pageSize: PER_PAGE,
        total,
        from,
        to,
        onPageChange: setPage,
        // The page size is fixed by the request this table makes.
        onPageSizeChange: () => undefined,
        pageSizeOptions: [PER_PAGE],
      }}
    />
  );
}
