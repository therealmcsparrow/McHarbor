// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { usePluginNavigate } from '../nav-context';
import { useTranslation } from 'react-i18next';
import { TablePage } from '@resources/layout/TablePage';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import { SwarmGuard } from '../components/SwarmGuard';
import { useRemoveSwarmNode, useSetNodeAvailability, useSetNodeRole, useSwarmNodes } from '../hooks/useSwarmNodes';
import type { SwarmNode } from '../types';
import { useNodeColumns } from './useNodeColumns';

export default function SwarmNodesPage() {
  return (
    <SwarmGuard>
      <SwarmNodes />
    </SwarmGuard>
  );
}

function SwarmNodes() {
  const { t } = useTranslation('swarm');
  const navigate = usePluginNavigate();
  const { data: nodes = [], isLoading, isFetching, refetch } = useSwarmNodes();
  const setAvailability = useSetNodeAvailability();
  const setRole = useSetNodeRole();
  const remove = useRemoveSwarmNode();

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [removeTarget, setRemoveTarget] = useState<SwarmNode | null>(null);

  const columns = useNodeColumns({ setAvailability, setRole, onRemove: setRemoveTarget });

  return (
    <>
      <TablePage
        id="swarm-nodes"
        title={t('nodes.title')}
        description={t('nodes.description', { count: nodes.length })}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('nodes.searchPlaceholder'),
          fields: (n: SwarmNode) => [n.hostname, n.id, n.role, n.state, n.addr],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-swarm-nodes' },
        }}
        columns={columns}
        data={nodes}
        loading={isLoading}
        empty={{ title: t('nodes.empty') }}
        getRowId={(row) => row.id}
        onRowClick={(row) => navigate(`nodes/${row.id}`)}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(nextOpen) => !nextOpen && setRemoveTarget(null)}
        title={t('dialogs.removeNode.title')}
        description={t('dialogs.removeNode.description', {
          name: removeTarget?.hostname ?? '',
        })}
        loading={remove.isPending}
        onConfirm={() => {
          if (removeTarget) remove.mutate({ id: removeTarget.id });
          setRemoveTarget(null);
        }}
      />
    </>
  );
}
