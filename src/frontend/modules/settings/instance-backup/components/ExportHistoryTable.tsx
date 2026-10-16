// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TablePage } from '@resources/layout/TablePage';
import { Select } from '@resources/components/ui/Select';
import type { SearchMode } from '@resources/utils/search-filter';
import type { InstanceExport } from '../hooks/useInstanceBackups';
import { useExportHistoryColumns, type ExportRowActions } from './ExportHistoryColumns';
import { EXPORT_TRIGGERS, FACET_ALL, exportStatusFacets, filterExports } from './instance-backup-facets';

type ExportHistoryTableProps = ExportRowActions & {
  exports: InstanceExport[];
  loading: boolean;
  isFetching: boolean;
  onRefresh: () => void;
};

export function ExportHistoryTable({ exports, loading, isFetching, onRefresh, ...actions }: ExportHistoryTableProps) {
  const { t } = useTranslation('instancebackup');
  const { t: tc } = useTranslation('common');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [currentStatus, setStatus] = useState<string>(FACET_ALL);
  const [trigger, setTrigger] = useState<string>(FACET_ALL);

  const columns = useExportHistoryColumns(actions);
  const facets = useMemo(() => exportStatusFacets(exports, t, tc), [exports, t, tc]);
  const visible = useMemo(() => filterExports(exports, currentStatus, trigger), [exports, currentStatus, trigger]);

  const chips =
    trigger === FACET_ALL
      ? []
      : [{ id: 'trigger', label: t(`trigger.${trigger}`), onRemove: () => setTrigger(FACET_ALL) }];

  return (
    <TablePage
      id="instance-backup-exports"
      header={false}
      title={t('panel.title')}
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('table.search'),
        fields: (e) => [e.note, e.createdBy, e.fileName, e.keyId, t(`status.${e.status}`), e.trigger],
      }}
      facets={{ items: facets, value: currentStatus, onChange: setStatus }}
      filters={
        <Select
          value={trigger}
          onChange={setTrigger}
          options={[
            { value: FACET_ALL, label: t('trigger.all') },
            ...EXPORT_TRIGGERS.map((value) => ({ value, label: t(`trigger.${value}`) })),
          ]}
          ariaLabel={t('table.trigger')}
          className="w-44"
        />
      }
      savedFilters={{
        state: { query, mode: searchMode, status: currentStatus, trigger },
        onRestore: (s) => {
          setQuery(typeof s.query === 'string' ? s.query : '');
          if (s.mode === 'contains' || s.mode === 'exact' || s.mode === 'regex') setSearchMode(s.mode);
          setStatus(typeof s.status === 'string' ? s.status : FACET_ALL);
          setTrigger(typeof s.trigger === 'string' ? s.trigger : FACET_ALL);
        },
        pristine: query === '' && currentStatus === FACET_ALL && trigger === FACET_ALL,
        defaults: { query: '', mode: 'contains', status: FACET_ALL, trigger: FACET_ALL },
      }}
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh, isFetching },
        export: { filename: 'mcharbor-instance-exports' },
        chips: {
          items: chips,
          onClear: () => {
            setStatus(FACET_ALL);
            setTrigger(FACET_ALL);
          },
        },
      }}
      columns={columns}
      data={visible}
      loading={loading}
      empty={{ title: t('empty.title'), description: t('empty.description') }}
      getRowId={(e) => e.id}
      getRowTestId={(e) => `export-row-${e.id}`}
    />
  );
}
