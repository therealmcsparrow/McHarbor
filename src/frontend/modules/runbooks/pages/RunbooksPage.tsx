// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { IconLayoutGrid, IconLayoutList, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { TablePage } from '@resources/layout/TablePage';
import { describeCron } from '@resources/utils/schedule';
import { PERM } from '@core/auth/permissionKeys';
import { useEnvironmentStore } from '@resources/stores/environment';
import { RunbookEditorDialog } from '../components/RunbookEditorDialog';
import { RunbookCardGrid } from '../components/RunbookCardGrid';
import { useRunbookColumns } from '../hooks/useRunbookColumns';
import { useAttachLabel } from '../hooks/useAttachLabel';
import { useDeleteRunbook, useRunbooks } from '@resources/hooks/useRunbooks';
import { useRunbooksViewStore } from '../stores/runbooks-view';
import type { Runbook } from '@resources/types/runbooks';

export default function RunbooksPage() {
  const { t } = useTranslation('runbooks');
  const { t: tCommon } = useTranslation('common');
  const attachLabel = useAttachLabel();
  const envId = useEnvironmentStore((s) => s.currentId);
  const runbooks = useRunbooks(envId);
  const remove = useDeleteRunbook();
  const { viewMode, setViewMode } = useRunbooksViewStore();
  const [pending, setPending] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [params, setParams] = useSearchParams();
  const editId = params.get('edit');
  const creating = params.get('new') !== null;

  const openEditor = (next: { edit?: string; create?: boolean }) =>
    setParams((current) => {
      const updated = new URLSearchParams(current);
      updated.delete('edit');
      updated.delete('new');
      if (next.edit) updated.set('edit', next.edit);
      if (next.create) updated.set('new', '1');
      return updated;
    });

  // The readable schedule is searchable but the raw cron is not: nobody types
  // "0 7 * * *" into a search box, and the description is what the row shows.
  const searchFields = (runbook: Runbook) => [
    runbook.name,
    runbook.description,
    attachLabel(runbook.attachKind).label,
    runbook.automation?.cron ? describeCron(runbook.automation.cron, tCommon) : null,
  ];

  const all = useMemo(() => runbooks.data ?? [], [runbooks.data]);
  const rows = query
    ? all.filter((runbook) => matchesSearchFields(query, searchMode, searchFields(runbook)).matched)
    : all;

  const columns = useRunbookColumns({
    onEdit: (id) => openEditor({ edit: id }),
    onRemove: setPending,
  });

  return (
    <>
      <TablePage
        id="runbooks"
        title={t('list.title')}
        description={t('list.description')}
        actions={
          <>
            <Button permission={PERM.RUNBOOKS_MANAGE} onClick={() => openEditor({ create: true })}>
              <IconPlus className="size-4" aria-hidden="true" />
              {t('list.create')}
            </Button>
            <div className="h-6 w-px bg-border" />
            <div className="flex items-center rounded-lg border border-border">
              <Button
                variant={viewMode === 'table' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('table')}
                aria-label={t('tableView')}
              >
                <IconLayoutList className="size-4" aria-hidden="true" />
              </Button>
              <Button
                variant={viewMode === 'card' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('card')}
                aria-label={t('cardView')}
              >
                <IconLayoutGrid className="size-4" aria-hidden="true" />
              </Button>
            </div>
          </>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('list.searchPlaceholder'),
          fields: searchFields,
        }}
        toolbar={{
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: {
            onRefresh: () => void runbooks.refetch(),
            isFetching: runbooks.isFetching,
          },
          export: { filename: 'mcharbor-runbooks' },
        }}
        columns={columns}
        data={rows}
        loading={runbooks.isLoading}
        empty={{
          title: t('list.emptyTitle'),
          description: t('list.emptyDescription'),
        }}
        getRowId={(runbook) => runbook.id}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <RunbookCardGrid runbooks={rows} onEdit={(id) => openEditor({ edit: id })} onRemove={setPending} />
            </div>
          )
        }
      />

      <RunbookEditorDialog open={creating || editId !== null} runbookId={editId} onClose={() => openEditor({})} />

      <MutationConfirmDialog
        open={pending !== null}
        onOpenChange={(isOpen) => setPending(isOpen ? pending : null)}
        title={t('list.deleteTitle')}
        description={t('list.deleteDescription')}
        confirmLabel={t('list.delete')}
        permission={PERM.RUNBOOKS_MANAGE}
        mutation={remove}
        variables={pending ?? ''}
      />
    </>
  );
}
