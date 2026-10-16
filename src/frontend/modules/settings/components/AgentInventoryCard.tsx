// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCloudUpload } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { TablePage } from '@resources/layout/TablePage';
import type { BatchAction } from '@resources/layout/table-page/types';
import {
  agentPlatform,
  inFlight,
  useAgentInventory,
  useUpdateAgents,
  type AgentRow,
} from '@resources/hooks/useAgentInventory';
import { useAgentInventoryColumns } from './AgentInventoryColumns';
import { AgentInventoryFilters } from './AgentInventoryFilters';
import { AgentUpdateConfirmDialog, type AgentUpdateTarget } from './AgentUpdateConfirmDialog';
import { useAgentInventoryFilters } from './useAgentInventoryFilters';

export function AgentInventoryCard() {
  const { t } = useTranslation('settings');
  const { data, isPending, isFetching, error, refetch } = useAgentInventory();
  const bulkUpdate = useUpdateAgents();
  const [target, setTarget] = useState<AgentUpdateTarget | null>(null);

  const agents = useMemo(() => data?.agents ?? [], [data]);
  const excluded = useMemo(() => new Set(data?.excludedEnvIds ?? []), [data]);
  const sweeping = Boolean(data?.updating) || agents.some(inFlight);
  const filters = useAgentInventoryFilters(agents, excluded);

  const onUpdate = useCallback((agent: AgentRow) => setTarget({ envIds: [agent.envId], name: agent.name }), []);
  const columns = useAgentInventoryColumns({ excluded, sweeping, onUpdate });

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: t('batch.updateSelected', { ns: 'common' }),
        icon: IconCloudUpload,
        confirm: true,
        permission: PERM.ENVIRONMENTS_MANAGE,
        onClick: (rows) => bulkUpdate.mutate((rows as AgentRow[]).map((agent) => agent.envId)),
      },
    ],
    [bulkUpdate, t],
  );

  const busy = bulkUpdate.isPending || sweeping;

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4 min-w-0">
        <h3 className="text-sm font-medium text-foreground">{t('about.agents.title')}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {data?.targetBuild
            ? t('about.agents.subtitleBuild', { version: data.targetVersion || '—', build: data.targetBuild })
            : t('about.agents.subtitle', { version: data?.targetVersion || '—' })}
        </p>
      </div>

      <TablePage
        id="settings-agent-inventory"
        header={false}
        title={t('about.agents.title')}
        search={{
          ...filters.search,
          placeholder: t('about.agents.searchPlaceholder'),
          fields: (agent) => [
            agent.name,
            agent.hostname,
            agent.remoteIp,
            agent.version,
            agent.build,
            agent.installKind,
            agentPlatform(agent),
          ],
        }}
        savedFilters={filters.savedFilters}
        facets={filters.facets}
        filters={<AgentInventoryFilters {...filters.selects} />}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-agents' },
          chips: filters.chips,
          actions: (
            <Button
              size="sm"
              permission={PERM.ENVIRONMENTS_MANAGE}
              disabled={agents.length === 0 || busy}
              onClick={() => setTarget({ envIds: [] })}
            >
              {busy ? (
                <Spinner className="mr-1.5 h-4 w-4" />
              ) : (
                <IconCloudUpload className="mr-1.5 h-4 w-4" aria-hidden="true" />
              )}
              {t('about.agents.updateAll')}
            </Button>
          ),
        }}
        columns={columns}
        data={filters.rows}
        loading={isPending}
        error={error ?? undefined}
        onRetry={() => void refetch()}
        empty={{ title: t('about.agents.empty') }}
        selectable
        batchActions={batchActions}
        isRowDisabled={(agent) => !agent.updatable || inFlight(agent) || busy}
        getRowId={(agent) => agent.envId}
        getRowTestId={(agent) => `agent-row-${agent.envId}`}
      />

      <AgentUpdateConfirmDialog target={target} onOpenChange={(nextOpen) => !nextOpen && setTarget(null)} />
    </div>
  );
}
