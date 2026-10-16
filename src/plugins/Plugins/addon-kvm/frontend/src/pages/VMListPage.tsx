// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TablePage } from '@resources/layout/TablePage';
import { matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useDiagnose, useVMLifecycle, useVMs } from '../hooks/useVMs';
import { useKvmView } from '../hooks/useKvmView';
import { CardGrid } from '../components/CardGrid';
import { VMCard } from '../components/VMCard';
import { useVMColumns } from '../components/VMColumns';
import { ViewToggle } from '../components/ViewToggle';
import { ImportVMDialog } from '../components/ImportVMDialog';
import { ImportProgressChip } from '../components/ImportProgressChip';
import { DeleteVMDialog } from '../components/DeleteVMDialog';
import { DiagnosePanel } from '../components/DiagnosePanel';
import { ImportsTable } from './ImportsTable';
import type { VMAction } from '../components/VMActions';
import type { VM } from '../types';
import { VMListToolbarActions } from './VMListToolbarActions';
import { FACETS, matchesFacet, type StateFacet } from './vmFacets';
import { VMListTabs, type ListTab } from './VMListTabs';
import { DisplayProfilesTable } from './DisplayProfilesTable';

interface VMListPageProps {
  templates: boolean;
  environments: { id: string; name: string }[];
  onOpen: (id: string) => void;
  onCreate: () => void;
  importRequested?: boolean;
  onImportRequestDone?: () => void;
}

export function VMListPage({
  templates,
  environments,
  onOpen,
  onCreate,
  importRequested = false,
  onImportRequestDone,
}: VMListPageProps) {
  const { t } = useTranslation('vm');
  const { viewMode, setViewMode } = useKvmView(templates ? 'templates' : 'machines');

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [facet, setFacet] = useState<StateFacet>('all');
  const [importing, setImporting] = useState(false);
  const [tab, setTab] = useState<ListTab>('machines');

  const { data, isLoading, isFetching, isError, refetch } = useVMs(undefined, templates);
  const lifecycle = useVMLifecycle();
  const diagnose = useDiagnose();
  const [deleting, setDeleting] = useState<VM | null>(null);

  const hostName = useCallback(
    (envId: string) => environments.find((env) => env.id === envId)?.name ?? envId,
    [environments],
  );

  const all = useMemo(() => data ?? [], [data]);

  const guests = useMemo(
    () => (templates || facet === 'all' ? all : all.filter((vm) => matchesFacet(vm, facet))),
    [all, facet, templates],
  );

  const searchFields = useCallback(
    (vm: VM) => [
      vm.name,
      hostName(vm.environmentId),
      vm.guestOsVariant,
      vm.guestReportedIps?.[0],
      t(`state.${vm.state}`),
    ],
    [hostName, t],
  );

  const cardGuests = useMemo(() => {
    if (!query.trim()) return guests;
    return guests.filter((vm) => matchesSearchFields(query, searchMode, searchFields(vm)).matched);
  }, [guests, query, searchMode, searchFields]);

  const runAction = useCallback((id: string, action: VMAction) => lifecycle.mutate({ id, action }), [lifecycle]);

  const runDiagnose = useCallback((vm: VM) => diagnose.mutate(vm.id), [diagnose]);

  const columns = useVMColumns({
    hostName,
    busy: lifecycle.isPending,
    diagnosePending: diagnose.isPending,
    onOpen,
    onAction: runAction,
    onDelete: setDeleting,
    onDiagnose: runDiagnose,
  });

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  const emptyTitle = templates ? t('list.emptyTemplates.title') : t('list.empty.title');

  return (
    <>
      {importing || importRequested ? (
        <ImportVMDialog
          environments={environments}
          onClose={() => {
            setImporting(false);
            onImportRequestDone?.();
          }}
          onImported={(id) => {
            setImporting(false);
            onOpen(id);
          }}
        />
      ) : null}
      {templates ? null : <ImportProgressChip onReopen={() => setImporting(true)} />}
      {(diagnose.isPending || diagnose.data) && (
        <DiagnosePanel pending={diagnose.isPending} result={diagnose.data} onDismiss={() => diagnose.reset()} />
      )}
      {deleting && (
        <DeleteVMDialog vm={deleting} onClose={() => setDeleting(null)} onDeleted={() => setDeleting(null)} />
      )}
      {templates ? null : <VMListTabs tab={tab} onChange={setTab} />}

      {tab === 'profiles' && !templates ? (
        <DisplayProfilesTable />
      ) : tab === 'conversion' && !templates ? (
        <ImportsTable onConvert={() => setImporting(true)} />
      ) : (
        <TablePage
          id={templates ? 'kvm-templates' : 'kvm-machines'}
          title={templates ? t('nav.templates') : t('list.title')}
          description={t('description')}
          search={{
            value: query,
            onChange: setQuery,
            mode: searchMode,
            onModeChange: setSearchMode,
            placeholder: t('list.searchPlaceholder'),
            fields: searchFields,
          }}
          facets={
            templates
              ? undefined
              : {
                  items: FACETS.map((id) => ({
                    id,
                    label: t(`list.filter.${id === 'all' ? 'all' : id}`),
                    count: id === 'all' ? all.length : all.filter((vm) => matchesFacet(vm, id)).length,
                  })),
                  value: facet,
                  onChange: (next) => setFacet(next as StateFacet),
                }
          }
          actions={<ViewToggle viewMode={viewMode} setViewMode={setViewMode} />}
          toolbar={{
            actions: (
              <VMListToolbarActions templates={templates} onImport={() => setImporting(true)} onCreate={onCreate} />
            ),
            columns: viewMode === 'table',
            density: viewMode === 'table',
            refresh: { onRefresh: () => void refetch(), isFetching },
            export: { filename: templates ? 'mcharbor-vm-templates' : 'mcharbor-vms' },
          }}
          columns={columns}
          data={guests}
          loading={isLoading}
          empty={{
            title: emptyTitle,
            description: templates ? t('list.emptyTemplates.body') : t('list.empty.body'),
          }}
          getRowId={(vm) => vm.id}
          getRowClassName={(vm) => (vm.hostAvailable ? undefined : 'opacity-60')}
          body={
            viewMode === 'table' ? undefined : (
              <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
                <CardGrid
                  count={cardGuests.length}
                  isLoading={isLoading}
                  filtered={guests.length > 0}
                  emptyLabel={emptyTitle}
                >
                  {cardGuests.map((vm) => (
                    <VMCard
                      key={vm.id}
                      vm={vm}
                      hostName={hostName(vm.environmentId)}
                      busy={lifecycle.isPending}
                      diagnosePending={diagnose.isPending}
                      onOpen={onOpen}
                      onAction={(action) => runAction(vm.id, action)}
                      onDelete={setDeleting}
                      onDiagnose={runDiagnose}
                    />
                  ))}
                </CardGrid>
              </div>
            )
          }
        />
      )}
    </>
  );
}
