// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { TablePage } from '@resources/layout/TablePage';
import { PERM } from '@core/auth/permissionKeys';
import { type ClusterInstance, type ClusterStatus, useDrain, useFailover } from '../hooks/useCluster';
import { type NodeAction, useClusterInstanceColumns } from './ClusterInstancesColumns';
import { ClusterInstancesFilters } from './ClusterInstancesFilters';
import { useClusterInstanceFilters } from './useClusterInstanceFilters';

type Pending = { kind: NodeAction; node: ClusterInstance };

type ClusterInstancesTableProps = {
  status: ClusterStatus;
  refresh?: { onRefresh: () => void; isFetching: boolean };
};

export function ClusterInstancesTable({ status: statusValue, refresh }: ClusterInstancesTableProps) {
  const { t } = useTranslation('enterprise');
  const failover = useFailover();
  const drain = useDrain();
  const [pending, setPending] = useState<Pending | null>(null);
  const conflict = (statusValue.activeConflict?.length ?? 0) > 0;
  const activePassive = statusValue.mode === 'active-passive';
  const mutation = pending?.kind === 'failover' ? failover : drain;
  const filters = useClusterInstanceFilters(statusValue.instances, statusValue.nodeId);

  const onAction = useCallback((kind: NodeAction, node: ClusterInstance) => setPending({ kind, node }), []);
  const columns = useClusterInstanceColumns({ conflict, activePassive, drifted: filters.drifted, onAction });

  const closeDialog = () => {
    setPending(null);
    failover.reset();
    drain.reset();
  };
  const confirmAction = () => {
    if (!pending || mutation.isPending) return;
    const done = { onSuccess: () => setPending(null) };
    if (pending.kind === 'failover') failover.mutate(pending.node.id, done);
    else drain.mutate({ nodeId: pending.node.id, drain: pending.kind === 'drain' }, done);
  };

  return (
    <>
      <TablePage
        id="enterprise-ha-nodes"
        header={false}
        title={t('ha.nodesTitle')}
        search={{
          ...filters.search,
          placeholder: t('ha.searchPlaceholder'),
          fields: (node) => [
            node.id,
            node.hostname,
            node.version,
            node.build,
            node.advertiseUrl,
            t(`ha.role.${node.haRole}`),
            t(`ha.state.${node.state}`),
          ],
        }}
        savedFilters={filters.savedFilters}
        facets={filters.facets}
        filters={<ClusterInstancesFilters {...filters.role} />}
        toolbar={{
          columns: true,
          density: true,
          refresh,
          export: { filename: 'mcharbor-ha-nodes' },
          chips: filters.chips,
        }}
        columns={columns}
        data={filters.rows}
        empty={{ title: t('ha.empty') }}
        getRowId={(node) => node.id}
        getRowTestId={(node) => `ha-node-${node.id}`}
      />
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(isOpen) => !isOpen && closeDialog()}
        title={pending ? t(`ha.confirm.${pending.kind}.title`, { node: pending.node.id }) : ''}
        description={pending ? t(`ha.confirm.${pending.kind}.description`, { node: pending.node.id }) : ''}
        confirmLabel={pending ? t(`ha.confirm.${pending.kind}.confirm`) : undefined}
        variant={pending?.kind === 'resume' ? 'default' : 'caution'}
        loading={mutation.isPending}
        permission={PERM.CLUSTER_MANAGE}
        onConfirm={confirmAction}
      >
        {mutation.error && <Alert variant="destructive">{mutation.error.message}</Alert>}
      </ConfirmDialog>
    </>
  );
}
