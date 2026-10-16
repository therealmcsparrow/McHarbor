// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import type { ContainerInfo } from '@core/types/docker';
import { Badge } from '@resources/components/ui/Badge';
import { textColumn, dateColumn, boolColumn, listColumn } from '@resources/layout/table-page/optional-columns';
import {
  ContainerHealthCell,
  ContainerNameCell,
  ContainerStackCell,
  ContainerUpdateCell,
  type ContainerUpdateResult,
} from './container-column-cells';
import {
  firstPublicPort,
  healthSortValue,
  ipSortKey,
  stackSortValue,
  updateSortValue,
  uptimeSeconds,
} from './container-sort-values';
import { ContainerActionsCell } from '../components/ContainerActionsCell';
import { STATE_VARIANTS, parseUptime, formatPorts, getContainerIP, getAutoUpdate } from '../components/container-utils';

type UseContainerColumnsProps = {
  action: { mutate: (vars: { id: string; action: string }) => void };
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRename: (c: ContainerInfo) => void;
  onMove: (c: ContainerInfo) => void;
  onRecreate: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
  onTakeOver: (c: ContainerInfo) => void;
  updateResults?: Map<string, ContainerUpdateResult>;
};

export function useContainerColumns({
  action,
  onTerminal,
  onLogs,
  onRename,
  onMove,
  onRecreate,
  onRemove,
  onTakeOver,
  updateResults,
}: UseContainerColumnsProps) {
  const { t } = useTranslation('containers');
  const group = t('columns.groupContainer');

  return useMemo<ColumnDef<ContainerInfo, unknown>[]>(
    () => [
      {
        accessorKey: 'Names',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => <ContainerNameCell container={row.original} />,
      },
      {
        accessorKey: 'Image',
        header: t('columns.image'),
        meta: { flex: true },
        cell: ({ row }) => <span className="block truncate text-muted-foreground">{row.original.Image}</span>,
      },
      {
        accessorKey: 'State',
        header: t('columns.state'),
        size: 75,
        cell: ({ row }) => (
          <Badge variant={STATE_VARIANTS[row.original.State] ?? 'secondary'}>{row.original.State}</Badge>
        ),
      },
      {
        id: 'health',
        header: () => <span className="w-full text-center">{t('columns.health')}</span>,
        size: 70,
        meta: { label: t('columns.health'), sortValue: healthSortValue },
        cell: ({ row }) => <ContainerHealthCell container={row.original} />,
      },
      {
        id: 'uptime',
        header: t('columns.uptime'),
        size: 80,
        meta: { sortValue: (c: ContainerInfo) => uptimeSeconds(c.Status) },
        cell: ({ row }) => (
          <span className="truncate text-xs text-muted-foreground">{parseUptime(row.original.Status)}</span>
        ),
      },
      {
        id: 'ip',
        header: t('columns.ip'),
        size: 100,
        meta: { sortValue: ipSortKey },
        cell: ({ row }) => (
          <span className="truncate text-xs text-muted-foreground">{getContainerIP(row.original)}</span>
        ),
      },
      {
        id: 'ports',
        header: t('columns.ports'),
        size: 100,
        meta: { sortValue: (c: ContainerInfo) => firstPublicPort(c.Ports) },
        cell: ({ row }) => (
          <span className="truncate text-xs text-muted-foreground">{formatPorts(row.original.Ports)}</span>
        ),
      },
      {
        id: 'autoUpdate',
        header: t('columns.auto'),
        size: 50,
        meta: { sortValue: getAutoUpdate },
        cell: ({ row }) => {
          const enabled = getAutoUpdate(row.original);
          return enabled ? (
            <Badge variant="success" className="px-1.5 py-0.5 text-[10px]">
              {t('columns.autoOn')}
            </Badge>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          );
        },
      },
      {
        id: 'stack',
        header: t('columns.stack'),
        size: 85,
        meta: { sortValue: stackSortValue },
        cell: ({ row }) => <ContainerStackCell container={row.original} onTakeOver={onTakeOver} />,
      },
      {
        id: 'update',
        header: () => <span className="w-full text-center">{t('columns.update')}</span>,
        size: 65,
        meta: {
          label: t('columns.update'),
          sortValue: (c: ContainerInfo) => updateSortValue(updateResults?.get(c.Id)),
        },
        cell: ({ row }) => <ContainerUpdateCell result={updateResults?.get(row.original.Id)} />,
      },
      textColumn<ContainerInfo>(
        'imageId',
        t('columns.imageId'),
        (r) => r.ImageID?.replace('sha256:', '').slice(0, 12),
        { group },
      ),
      textColumn<ContainerInfo>('command', t('columns.command'), (r) => r.Command, { group, width: 'xl' }),
      dateColumn<ContainerInfo>(
        'created',
        t('columns.created'),
        (r) => (r.Created ? new Date(r.Created * 1000).toISOString() : null),
        { group },
      ),
      listColumn<ContainerInfo>(
        'labels',
        t('columns.labels'),
        (r) => Object.entries(r.Labels ?? {}).map(([k, v]) => `${k}=${v}`),
        { group, width: 'xl' },
      ),
      listColumn<ContainerInfo>(
        'networks',
        t('columns.networks'),
        (r) => Object.keys(r.NetworkSettings?.Networks ?? {}),
        { group: t('columns.groupNetwork') },
      ),
      listColumn<ContainerInfo>(
        'volumes',
        t('columns.volumes'),
        (r) => (r.Mounts ?? []).map((m) => m.Name || m.Source).filter(Boolean),
        { group: t('columns.groupStorage') },
      ),
      textColumn<ContainerInfo>('stackService', t('columns.stackService'), (r) => r.StackService, {
        group: t('columns.groupStack'),
      }),
      boolColumn<ContainerInfo>('protected', t('columns.protected'), (r) => r.Protected, { group }),
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        size: 155,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => (
          <ContainerActionsCell
            container={row.original}
            onAction={action.mutate}
            onTerminal={onTerminal}
            onLogs={onLogs}
            onRename={onRename}
            onMove={onMove}
            onRecreate={onRecreate}
            onRemove={onRemove}
          />
        ),
      },
    ],
    [action, t, group, onTerminal, onLogs, onRename, onMove, onRecreate, onRemove, onTakeOver, updateResults],
  );
}
