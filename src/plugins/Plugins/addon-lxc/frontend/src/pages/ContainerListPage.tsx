// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TablePage } from '@resources/layout/TablePage';
import { type SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Select } from '@resources/components/ui/Select';
import { CT_PERM } from '../permissions';
import { useContainers, useLifecycle } from '../hooks/useContainers';
import { useContainerColumns } from '../components/ContainerColumns';
import { OrphanedBackupsButton } from '../components/OrphanedBackupsDialog';
import { isLive, type Container } from '../types';

/**
 * The container list.
 *
 * Three things it does that a plain table would not:
 *
 *   - A container whose host is unreachable is greyed and its actions disabled,
 *     rather than shown with stale state and buttons that will fail.
 *   - Privilege is a column, not a detail-page fact. Whether root inside a
 *     container is root on the host is the most consequential thing about it,
 *     and it should be visible while scanning.
 *   - The memory column is usage against the container's own limit. A container
 *     at 95% of 512 MB is in trouble on a host with 128 GB free, which a
 *     host-relative figure would hide.
 */

interface ContainerListPageProps {
  templates: boolean;
  environments: { id: string; name: string }[];
  onOpen: (id: string) => void;
  onCreate: () => void;
}

export function ContainerListPage({ templates, environments, onOpen, onCreate }: ContainerListPageProps) {
  const { t } = useTranslation('ct');
  const [envId, setEnvId] = useState('');
  const [stateFilter, setStateFilter] = useState('');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const { data, isLoading, isFetching, isError, refetch } = useContainers(envId || undefined, templates);
  const lifecycle = useLifecycle();

  const hostName = useCallback(
    (environmentId: string) => environments.find((env) => env.id === environmentId)?.name ?? environmentId,
    [environments],
  );

  const containers = useMemo(() => {
    const all = data ?? [];
    switch (stateFilter) {
      case 'running':
        return all.filter((container) => isLive(container));
      case 'stopped':
        return all.filter((container) => container.state === 'stopped' || container.state === 'defined');
      case 'problem':
        // What an operator scanning for trouble means: crashed, or a host
        // nobody can reach.
        return all.filter((container) => container.state === 'crashed' || !container.hostAvailable);
      default:
        return all;
    }
  }, [data, stateFilter]);

  const searchFields = useCallback(
    (container: Container) => [
      container.name,
      hostName(container.environmentId),
      container.state,
      container.observedIps?.[0] ?? '',
    ],
    [hostName],
  );

  const columns = useContainerColumns({
    hostName,
    busy: lifecycle.isPending,
    onOpen,
    onAction: (id, action) => lifecycle.mutate({ id, action }),
  });

  if (isError) return <ErrorState onRetry={() => void refetch()} />;

  return (
    <TablePage
      id={templates ? 'lxc-templates' : 'lxc-containers'}
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
      toolbar={{
        actions: (
          <div className="flex flex-wrap items-center gap-2">
            {environments.length > 1 && (
              <div className="min-w-[10rem]">
                <Select
                  value={envId}
                  ariaLabel={t('list.columns.host')}
                  options={[
                    { value: '', label: t('list.filter.all') },
                    ...environments.map((env) => ({
                      value: env.id,
                      label: env.name,
                    })),
                  ]}
                  onChange={setEnvId}
                />
              </div>
            )}
            {!templates && (
              <div className="min-w-[10rem]">
                <Select
                  value={stateFilter}
                  ariaLabel={t('list.columns.state')}
                  options={[
                    { value: '', label: t('list.filter.all') },
                    { value: 'running', label: t('list.filter.running') },
                    { value: 'stopped', label: t('list.filter.stopped') },
                    { value: 'problem', label: t('list.filter.problem') },
                  ]}
                  onChange={setStateFilter}
                />
              </div>
            )}
            {!templates && (
              <OrphanedBackupsButton
                environments={environments}
                environmentId={envId || undefined}
                onRecreated={onOpen}
              />
            )}
            <Button size="sm" permission={CT_PERM.MANAGE} onClick={onCreate}>
              {t('actions.create')}
            </Button>
          </div>
        ),
        columns: true,
        density: true,
        refresh: { onRefresh: () => void refetch(), isFetching },
        export: {
          filename: templates ? 'mcharbor-lxc-templates' : 'mcharbor-lxc-containers',
        },
      }}
      columns={columns}
      data={containers}
      loading={isLoading}
      empty={{
        title: templates ? t('list.emptyTemplates.title') : t('list.empty.title'),
        description: templates ? t('list.emptyTemplates.body') : t('list.empty.body'),
      }}
      getRowId={(container) => container.id}
      onRowClick={(container) => onOpen(container.id)}
      getRowClassName={(container) => (container.hostAvailable ? undefined : 'opacity-60')}
    />
  );
}
