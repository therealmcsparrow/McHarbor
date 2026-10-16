// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ClustersPage — the connection admin surface.
//
// New with the migration. Kubernetes clusters used to be rows on the host's
// Environments page with an orchestrator type of "kubernetes", which is what
// kept `client-go` in the host binary and five encrypted columns on a table
// that otherwise describes Docker hosts. They are the plugin's own records
// now, so the plugin owns the screen that manages them.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconPlus, IconStar, IconStarFilled, IconTrash } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { ClusterExportButton } from '../components/ClusterExportButton';
import { ClusterFormDialog } from '../components/ClusterFormDialog';
import { useClusters, useDeleteCluster, useUpdateCluster, type Cluster } from '../hooks/useClusters';
import { KUBE_PERM } from '../permissions';
import { usePluginNav } from '../nav-context';

export function ClustersPage() {
  const { t } = useTranslation('kubernetes');
  const { data: clusters = [], isLoading, isFetching, refetch } = useClusters();
  const update = useUpdateCluster();
  const remove = useDeleteCluster();

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [editing, setEditing] = useState<Cluster | null>(null);
  const [creating, setCreating] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<Cluster | null>(null);

  // The Ctrl+K palette's add-cluster action lands here as `?action=create`; the dialog is
  // open for as long as the param is, and closing it drops the param.
  const nav = usePluginNav();
  const canManage = usePermissionGate(KUBE_PERM.CLUSTERS_MANAGE).allowed;
  const createRequested = new URLSearchParams(nav.search).get('action') === 'create' && canManage;

  const columns = useMemo<ColumnDef<Cluster, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('clusters.columns.name'),
        meta: { flex: true },
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium">{row.original.name}</span>
            {row.original.isDefault && (
              <Badge variant="default" className="shrink-0 px-1.5 py-0 text-[10px]">
                {t('clusters.default')}
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'serverUrl',
        header: t('clusters.columns.server'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">
            {row.original.serverUrl || t('clusters.fromKubeconfig')}
          </span>
        ),
      },
      {
        accessorKey: 'namespace',
        header: t('clusters.columns.namespace'),
        size: 160,
        cell: ({ row }) => <span className="text-sm">{row.original.namespace || t('values.allNamespaces')}</span>,
      },
      {
        id: 'credential',
        header: t('clusters.columns.credential'),
        size: 140,
        meta: {
          sortValue: (c: Cluster) => (c.hasKubeconfig ? 'kubeconfig' : c.hasToken ? 'token' : 'none'),
        },
        cell: ({ row }) => (
          <Badge variant="secondary">
            {row.original.hasKubeconfig
              ? t('clusters.credential.kubeconfig')
              : row.original.hasToken
                ? t('clusters.credential.token')
                : t('clusters.credential.none')}
          </Badge>
        ),
      },
      {
        accessorKey: 'isActive',
        header: t('clusters.columns.state'),
        size: 110,
        cell: ({ row }) => (
          <Badge variant={row.original.isActive ? 'success' : 'secondary'}>
            {row.original.isActive ? t('clusters.active') : t('clusters.inactive')}
          </Badge>
        ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        size: 120,
        enableSorting: false,
        meta: { width: 'sm', locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        cell: ({ row }) => {
          const cluster = row.original;
          return (
            <div className="flex items-center justify-end gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('clusters.actions.makeDefault')}
                permission={KUBE_PERM.CLUSTERS_MANAGE}
                disabled={cluster.isDefault}
                onClick={(e) => {
                  e.stopPropagation();
                  update.mutate({ id: cluster.id, isDefault: true });
                }}
              >
                {cluster.isDefault ? (
                  <IconStarFilled className="size-4 text-warning" />
                ) : (
                  <IconStar className="size-4" />
                )}
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('clusters.actions.remove')}
                permission={KUBE_PERM.CLUSTERS_MANAGE}
                onClick={(e) => {
                  e.stopPropagation();
                  setRemoveTarget(cluster);
                }}
              >
                <IconTrash className="size-4" />
              </Button>
            </div>
          );
        },
      },
    ],
    [t, update],
  );

  return (
    <>
      <TablePage
        id="kubernetes-clusters"
        title={t('clusters.title')}
        description={t('clusters.description', { count: clusters.length })}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {/* Uninstalling the plugin drops these rows, so the way out ships
                beside the way in. */}
            <ClusterExportButton clusters={clusters} />
            <Button size="sm" permission={KUBE_PERM.CLUSTERS_MANAGE} onClick={() => setCreating(true)}>
              <IconPlus className="size-4" />
              {t('clusters.actions.add')}
            </Button>
          </div>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('clusters.searchPlaceholder'),
          fields: (c: Cluster) => [c.name, c.serverUrl, c.namespace],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-kubernetes-clusters' },
        }}
        columns={columns}
        data={clusters}
        loading={isLoading}
        empty={{ title: t('clusters.empty'), description: t('clusters.emptyDescription') }}
        getRowId={(row) => row.id}
        onRowClick={(row) => setEditing(row)}
      />

      <ClusterFormDialog
        open={creating || createRequested || editing !== null}
        cluster={editing}
        onOpenChange={(nextOpen) => {
          if (nextOpen) return;
          setCreating(false);
          setEditing(null);
          if (createRequested) nav.navigate(nav.path, { replace: true });
        }}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(nextOpen) => !nextOpen && setRemoveTarget(null)}
        title={t('clusters.confirmRemove.title')}
        description={t('clusters.confirmRemove.description', { name: removeTarget?.name ?? '' })}
        confirmLabel={t('clusters.actions.remove')}
        loading={remove.isPending}
        onConfirm={() => {
          if (removeTarget) remove.mutate(removeTarget.id);
          setRemoveTarget(null);
        }}
      />
    </>
  );
}
