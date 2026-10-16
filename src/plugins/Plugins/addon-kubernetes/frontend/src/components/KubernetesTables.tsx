// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Eleven tabs, one table. The column set is data — `tableConfig` names which
// keys each tab shows — so the TanStack column defs are generated from it
// rather than written out eleven times.
//
// Each tab passes its own TablePage `id` (`kubernetes-pods`, `kubernetes-nodes`,
// …). Sharing one id would make hiding "namespace" on Pods also hide it on
// Services, since column visibility is stored per id.
//
// Everything a containers row can do, a Kubernetes row now does too: a chip
// band of state facets above the table (`kubernetes-facets.ts`), the whole row
// as the click target into the detail rather than the name cell alone, and a
// selection bar with the bulk actions the tab actually supports.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconRotate, IconTopologyStar3, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { TablePage } from '@resources/layout/TablePage';
import type { BatchAction, TableColumnMeta } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { list, resourcePath } from '../format';
import { PluginLink, usePluginNavigate } from '../nav-context';
import type { KubernetesSummary, Row } from '../types';
import type { ResourceRef } from '../hooks/useKubernetesActions';
import { KubernetesActionsCell, type ActionsKind } from './KubernetesActionsCell';
import { ALL_FACET, badgeVariant, buildFacets, matchesFacet } from './kubernetes-facets';

export type TableId =
  | 'pods'
  | 'deployments'
  | 'services'
  | 'namespaces'
  | 'nodes'
  | 'events'
  | 'config'
  | 'storage'
  | 'networking'
  | 'workloads'
  | 'capabilities';

type TableConfig = { titleKey: string; rows: (summary: KubernetesSummary) => Row[]; cols: string[] };

const tableConfig: Record<TableId, TableConfig> = {
  pods: {
    titleKey: 'tabs.pods',
    rows: (s) => s.pods,
    cols: ['name', 'namespace', 'status', 'ready', 'restarts', 'node', 'age'],
  },
  deployments: {
    titleKey: 'tabs.deployments',
    rows: (s) => s.deployments,
    cols: ['name', 'namespace', 'ready', 'upToDate', 'available', 'images', 'age'],
  },
  services: {
    titleKey: 'tabs.services',
    rows: (s) => s.services,
    cols: ['name', 'namespace', 'type', 'clusterIP', 'externalIP', 'ports', 'age'],
  },
  namespaces: { titleKey: 'tabs.namespaces', rows: (s) => s.namespaces, cols: ['name', 'status', 'age'] },
  nodes: {
    titleKey: 'tabs.nodes',
    rows: (s) => s.nodes,
    cols: ['name', 'status', 'roles', 'version', 'cpu', 'memory', 'age'],
  },
  events: {
    titleKey: 'tabs.events',
    rows: (s) => s.events,
    cols: ['namespace', 'type', 'reason', 'object', 'message', 'age'],
  },
  config: { titleKey: 'tabs.config', rows: configRows, cols: ['kind', 'name', 'namespace', 'type', 'age'] },
  storage: {
    titleKey: 'tabs.storage',
    rows: storageRows,
    cols: ['kind', 'name', 'namespace', 'status', 'capacity', 'storageClass', 'claim', 'age'],
  },
  networking: {
    titleKey: 'tabs.networking',
    rows: networkingRows,
    cols: ['kind', 'name', 'namespace', 'type', 'addresses', 'ports', 'age'],
  },
  workloads: {
    titleKey: 'tabs.workloads',
    rows: workloadRows,
    cols: ['kind', 'name', 'namespace', 'ready', 'target', 'replicas', 'age'],
  },
  capabilities: { titleKey: 'tabs.capabilities', rows: (s) => s.capabilities, cols: ['name', 'status', 'description'] },
};

export type KubernetesTableActions = {
  onScale: (ref: ResourceRef) => void;
  onRestart: (ref: ResourceRef) => void;
  onDelete: (ref: ResourceRef, kind: ActionsKind) => void;
  onLogs: (ref: ResourceRef) => void;
  /** The selection bar's destructive action. Confirms once for the whole set. */
  onDeleteMany: (refs: ResourceRef[], kind: ActionsKind) => void;
  /** Rolling-restarts every selected deployment. */
  onRestartMany: (refs: ResourceRef[]) => void;
};

// Only pods and deployments have write endpoints; every other tab renders
// without a trailing actions column.
function actionsKind(tab: TableId): ActionsKind | null {
  if (tab === 'pods' || tab === 'deployments') return tab;
  return null;
}

/** A row that a namespaced endpoint can actually address. */
function rowRef(row: Row): ResourceRef | null {
  const namespace = typeof row.namespace === 'string' ? row.namespace : '';
  const itemName = typeof row.name === 'string' ? row.name : '';
  return namespace && itemName ? { namespace, name: itemName } : null;
}

export function KubernetesTables({
  summary,
  activeTab,
  actions,
  onRefresh,
  isFetching,
}: {
  summary: KubernetesSummary;
  activeTab: TableId;
  actions: KubernetesTableActions;
  /** Re-reads the cluster summary. Lives in the toolbar, as on every other
      list page, rather than in the page header. */
  onRefresh: () => void;
  isFetching: boolean;
}) {
  const { t } = useTranslation('kubernetes');
  const navigate = usePluginNavigate();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [facet, setFacet] = useState<string>(ALL_FACET);

  const cfg = tableConfig[activeTab];
  const allRows = cfg.rows(summary);
  const kind = actionsKind(activeTab);

  // A facet is only meaningful for the tab that produced it — switching from
  // Pods (`Running`) to Storage (`Bound`) would otherwise land on a chip that
  // matches nothing and read as an empty cluster.
  const [facetTab, setFacetTab] = useState(activeTab);
  if (facetTab !== activeTab) {
    setFacetTab(activeTab);
    setFacet(ALL_FACET);
  }

  const facets = useMemo(() => buildFacets(allRows, activeTab, t('facets.all')), [activeTab, allRows, t]);

  // The chip narrows what the search returns rather than replacing it, so
  // contains / exact / regex keep working exactly as before — `TablePage`
  // applies the text query to whatever `data` it is handed.
  const rows = useMemo(
    () => (facet === ALL_FACET ? allRows : allRows.filter((row) => matchesFacet(row, activeTab, facet))),
    [activeTab, allRows, facet],
  );

  const columns = useMemo<ColumnDef<Row, unknown>[]>(() => {
    const defs: ColumnDef<Row, unknown>[] = cfg.cols.map((col) => ({
      id: col,
      accessorFn: (row: Row) => list(row[col]),
      header: t(`columns.${col}`),
      cell: ({ row }) => {
        const value = row.original[col];
        const rowName = row.original.name;
        const rowNamespace = row.original.namespace;
        if (
          col === 'name' &&
          (activeTab === 'pods' || activeTab === 'deployments') &&
          typeof rowName === 'string' &&
          typeof rowNamespace === 'string' &&
          rowName &&
          rowNamespace
        ) {
          return (
            <PluginLink
              className="font-medium text-foreground underline-offset-4 transition-colors hover:text-primary hover:underline"
              to={resourcePath(`/${activeTab}/${rowNamespace}/${rowName}`)}
              // The whole row is already the click target; the anchor stays
              // for middle-click and "copy link address", and must not fire
              // the row handler a second time.
              onClick={(evt) => evt.stopPropagation()}
            >
              {list(value)}
            </PluginLink>
          );
        }
        if (col === 'status' || col === 'type') {
          return <Badge variant={badgeVariant(String(value))}>{list(value)}</Badge>;
        }
        return <span className="break-words">{list(value)}</span>;
      },
    }));

    if (kind) {
      defs.push({
        id: 'actions',
        header: t('columns.actions'),
        enableSorting: false,
        meta: { width: 'sm', locked: true, noExport: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div
            role="presentation"
            className="flex justify-end"
            onClick={(evt) => evt.stopPropagation()}
            onKeyDown={(evt) => evt.stopPropagation()}
          >
            <KubernetesActionsCell row={row.original} kind={kind} {...actions} />
          </div>
        ),
      });
    }

    return defs;
  }, [actions, activeTab, cfg.cols, kind, t]);

  // Bulk work is offered only where a bulk endpoint exists. Restart is
  // deployments-only because a pod has no rolling restart — deleting it is the
  // restart, and that is already the destructive action beside it.
  const batchActions = useMemo<BatchAction[] | undefined>(() => {
    if (!kind) return undefined;
    const refsOf = (selected: unknown[]) =>
      (selected as Row[]).map(rowRef).filter((ref): ref is ResourceRef => ref !== null);

    const entries: BatchAction[] = [];
    if (kind === 'deployments') {
      entries.push({
        label: t('actions.restartSelected'),
        icon: IconRotate,
        onClick: (selected) => actions.onRestartMany(refsOf(selected)),
        confirm: true,
      });
    }
    entries.push({
      label: t('actions.deleteSelected'),
      icon: IconTrash,
      variant: 'destructive',
      onClick: (selected) => actions.onDeleteMany(refsOf(selected), kind),
      confirm: true,
    });
    return entries;
  }, [actions, kind, t]);

  return (
    <TablePage
      // Per-tab id: column visibility and density are stored per id, so a
      // shared one would leak Pods' hidden columns onto Services.
      id={`kubernetes-${activeTab}`}
      // The Kubernetes page renders its own tab bar and heading above this.
      header={false}
      title={t(cfg.titleKey)}
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('actions.searchIn', { kind: t(cfg.titleKey) }),
        fields: (row) => cfg.cols.map((col) => list(row[col])),
      }}
      facets={facets ? { items: facets, value: facet, onChange: setFacet } : undefined}
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh, isFetching },
        export: { filename: `mcharbor-kubernetes-${activeTab}` },
      }}
      columns={columns}
      data={rows}
      empty={{
        icon: <IconTopologyStar3 className="size-6" />,
        title: t('empty.resources'),
        description: facet === ALL_FACET ? undefined : t('empty.filtered'),
      }}
      selectable={Boolean(kind)}
      batchActions={batchActions}
      getRowId={(row) => `${list(row.kind)}-${list(row.namespace)}-${list(row.name)}`}
      onRowClick={
        kind
          ? (row) => {
              const ref = rowRef(row);
              if (ref) navigate(resourcePath(`/${activeTab}/${ref.namespace}/${ref.name}`));
            }
          : undefined
      }
    />
  );
}

function configRows(s: KubernetesSummary) {
  return [
    ...s.configMaps.map((r) => ({ ...r, kind: 'ConfigMap' })),
    ...s.secrets.map((r) => ({ ...r, kind: 'Secret' })),
    ...s.serviceAccounts.map((r) => ({ ...r, kind: 'ServiceAccount' })),
    ...s.resourceQuotas.map((r) => ({ ...r, kind: 'ResourceQuota' })),
    ...s.limitRanges.map((r) => ({ ...r, kind: 'LimitRange' })),
  ];
}

function storageRows(s: KubernetesSummary) {
  return [
    ...s.persistentVolumeClaims.map((r) => ({ ...r, kind: 'PVC' })),
    ...s.persistentVolumes.map((r) => ({ ...r, kind: 'PV' })),
    ...s.storageClasses.map((r) => ({ ...r, kind: 'StorageClass' })),
  ];
}

function networkingRows(s: KubernetesSummary) {
  return [
    ...s.endpoints.map((r) => ({ ...r, kind: 'Endpoint' })),
    ...s.ingresses.map((r) => ({ ...r, kind: 'Ingress' })),
    ...s.networkPolicies.map((r) => ({ ...r, kind: 'NetworkPolicy' })),
  ];
}

function workloadRows(s: KubernetesSummary) {
  return [
    ...s.replicaSets.map((r) => ({ ...r, kind: 'ReplicaSet' })),
    ...s.statefulSets.map((r) => ({ ...r, kind: 'StatefulSet' })),
    ...s.daemonSets.map((r) => ({ ...r, kind: 'DaemonSet' })),
    ...s.jobs.map((r) => ({ ...r, kind: 'Job' })),
    ...s.cronJobs.map((r) => ({ ...r, kind: 'CronJob' })),
    ...s.horizontalPodAutoscalers.map((r) => ({ ...r, kind: 'HPA' })),
  ];
}
