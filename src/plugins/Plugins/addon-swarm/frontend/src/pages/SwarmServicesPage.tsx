// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { usePluginNavigate, usePluginSearchParams } from '../nav-context';
import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import { SwarmGuard } from '../components/SwarmGuard';
import { ScaleServiceDialog } from '../components/ScaleServiceDialog';
import {
  useRemoveSwarmService,
  useRestartSwarmService,
  useRollbackSwarmService,
  useSwarmServices,
} from '../hooks/useSwarmServices';
import type { SwarmService } from '../types';
import { useServiceColumns } from './useServiceColumns';

type ModeFilter = 'all' | 'replicated' | 'global';
type StateFilter = 'all' | 'converged' | 'degraded';

export default function SwarmServicesPage() {
  return (
    <SwarmGuard>
      <SwarmServices />
    </SwarmGuard>
  );
}

function SwarmServices() {
  const { t } = useTranslation('swarm');
  const navigate = usePluginNavigate();
  const [searchParams, setSearchParams] = usePluginSearchParams();
  const { data: services = [], isLoading, isFetching, refetch } = useSwarmServices();

  const restart = useRestartSwarmService();
  const rollback = useRollbackSwarmService();
  const remove = useRemoveSwarmService();

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [modeFilter, setModeFilter] = useState<ModeFilter>('all');
  const [stateFilter, setStateFilter] = useState<StateFilter>('all');
  const [scaleTarget, setScaleTarget] = useState<SwarmService | null>(null);
  const [removeTarget, setRemoveTarget] = useState<SwarmService | null>(null);

  // The stacks page links here with `?stack=<name>`, which is how a stack
  // shows its members without needing a page of its own.
  const stackFilter = searchParams.get('stack') ?? '';

  const filtered = useMemo(() => {
    return services.filter((s) => {
      if (stackFilter && s.stackName !== stackFilter) return false;
      if (modeFilter === 'replicated' && !s.mode.startsWith('replicated')) return false;
      if (modeFilter === 'global' && !s.mode.startsWith('global')) return false;
      if (stateFilter === 'converged' && s.runningTasks < s.desiredTasks) return false;
      if (stateFilter === 'degraded' && s.runningTasks >= s.desiredTasks) return false;
      return true;
    });
  }, [services, modeFilter, stateFilter, stackFilter]);

  const columns = useServiceColumns({
    restart,
    rollback,
    onScale: setScaleTarget,
    onRemove: setRemoveTarget,
  });

  return (
    <>
      <TablePage
        id="swarm-services"
        title={t('services.title')}
        description={t('services.description', { count: services.length })}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('services.searchPlaceholder'),
          fields: (s: SwarmService) => [s.name, s.image, s.stackName],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-2">
            {stackFilter && (
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  setSearchParams((current) => {
                    current.delete('stack');
                    return current;
                  })
                }
              >
                {t('services.stackFilter', { name: stackFilter })}
                <IconX className="size-3.5" />
              </Button>
            )}
            <SegmentedFilter
              value={modeFilter}
              onChange={setModeFilter}
              options={[
                { id: 'all', label: t('filters.all') },
                { id: 'replicated', label: t('serviceMode.replicated') },
                { id: 'global', label: t('serviceMode.global') },
              ]}
            />
            <SegmentedFilter
              value={stateFilter}
              onChange={setStateFilter}
              options={[
                { id: 'all', label: t('filters.all') },
                { id: 'converged', label: t('filters.converged') },
                { id: 'degraded', label: t('filters.degraded') },
              ]}
            />
          </div>
        }
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-swarm-services' },
        }}
        columns={columns}
        data={filtered}
        loading={isLoading}
        empty={{ title: t('services.empty') }}
        getRowId={(row) => row.id}
        onRowClick={(row) => navigate(`services/${row.id}`)}
      />

      <ScaleServiceDialog service={scaleTarget} onOpenChange={() => setScaleTarget(null)} />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(nextOpen) => !nextOpen && setRemoveTarget(null)}
        title={t('dialogs.removeService.title')}
        description={t('dialogs.removeService.description', { name: removeTarget?.name ?? '' })}
        loading={remove.isPending}
        onConfirm={() => {
          if (removeTarget) remove.mutate(removeTarget.id);
          setRemoveTarget(null);
        }}
      />
    </>
  );
}

/** A small segmented control, matching the networks page's usage facet. */
function SegmentedFilter<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T;
  onChange: (next: T) => void;
  options: { id: T; label: string }[];
}) {
  return (
    <div className="flex items-center gap-1 rounded-lg border border-border p-0.5">
      {options.map((option) => (
        <Button
          key={option.id}
          variant={value === option.id ? 'default' : 'ghost'}
          size="sm"
          onClick={() => onChange(option.id)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  );
}
