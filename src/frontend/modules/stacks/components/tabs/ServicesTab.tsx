// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconExternalLink } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Spinner } from '@resources/components/ui/Spinner';
import { truncateId, formatBytes } from '@resources/utils/format';
import type { ContainerInfo } from '@core/types/docker';
import { useContainersBulkStats } from '@resources/hooks/useContainersBulkStats';
import { textColumn, dateColumn, listColumn } from '@resources/layout/table-page/optional-columns';
import { useStackContainers } from '@resources/hooks/useStacks';

type ServicesTabProps = {
  stackName: string;
};

export function ServicesTab({ stackName }: ServicesTabProps) {
  const { t } = useTranslation('stacks');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const navigate = useNavigate();
  const { data: containers, isLoading } = useStackContainers(stackName);
  const { data: statsMap } = useContainersBulkStats();
  const group = t('columns.groupContainer');

  const columns = useMemo<ColumnDef<ContainerInfo, unknown>[]>(
    () => [
      {
        id: 'service',
        header: t('detail.serviceName'),
        meta: { sortValue: (c: ContainerInfo) => c.Labels['com.docker.compose.service'] ?? '' },
        cell: ({ row }) => (
          <span className="text-sm font-medium">{row.original.Labels['com.docker.compose.service'] ?? '-'}</span>
        ),
      },
      {
        id: 'containerId',
        header: t('detail.containerId'),
        size: 120,
        meta: { sortValue: (c: ContainerInfo) => c.Id },
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{truncateId(row.original.Id)}</span>
        ),
      },
      {
        id: 'status',
        header: t('detail.status'),
        size: 90,
        meta: { sortValue: (c: ContainerInfo) => c.State },
        cell: ({ row }) => (
          <Badge
            variant={row.original.State === 'running' ? 'success' : 'destructive'}
            className="px-1.5 py-0 text-[10px]"
          >
            {row.original.State}
          </Badge>
        ),
      },
      {
        id: 'image',
        header: t('detail.image'),
        meta: { sortValue: (c: ContainerInfo) => c.Image },
        cell: ({ row }) => (
          <span className="block max-w-[200px] truncate text-xs text-muted-foreground">{row.original.Image}</span>
        ),
      },
      {
        id: 'ports',
        header: t('detail.ports'),
        meta: {
          sortValue: (c: ContainerInfo) => {
            const published = (c.Ports ?? []).map((p) => p.PublicPort).filter((n): n is number => !!n);
            return published.length ? Math.min(...published) : null;
          },
        },
        cell: ({ row }) => {
          const ports = row.original.Ports ?? [];
          if (ports.length === 0) return <span className="text-xs text-muted-foreground">-</span>;
          return (
            <div className="flex flex-wrap gap-1">
              {ports
                .filter((p) => p.PublicPort)
                .slice(0, 3)
                .map((p) => (
                  <Badge
                    key={`${p.PublicPort}-${p.PrivatePort}`}
                    variant="secondary"
                    className="px-1.5 py-0 text-[9px]"
                  >
                    {p.PublicPort}:{p.PrivatePort}/{p.Type}
                  </Badge>
                ))}
            </div>
          );
        },
      },
      {
        id: 'cpu',
        header: t('detail.cpu'),
        size: 70,
        meta: { sortValue: (c: ContainerInfo) => statsMap?.get(c.Id)?.cpuPercent ?? null },
        cell: ({ row }) => {
          const stats = statsMap?.get(row.original.Id);
          if (!stats) return <span className="text-xs text-muted-foreground">-</span>;
          return <span className="text-xs tabular-nums">{stats.cpuPercent.toFixed(1)}%</span>;
        },
      },
      {
        id: 'memory',
        header: t('detail.memory'),
        size: 90,
        meta: { sortValue: (c: ContainerInfo) => statsMap?.get(c.Id)?.memUsage ?? null },
        cell: ({ row }) => {
          const stats = statsMap?.get(row.original.Id);
          if (!stats) return <span className="text-xs text-muted-foreground">-</span>;
          return <span className="text-xs tabular-nums">{formatBytes(stats.memUsage)}</span>;
        },
      },
      textColumn<ContainerInfo>('containerName', t('columns.containerName'), (r) => r.Names?.[0]?.replace(/^\//, ''), {
        group,
      }),
      textColumn<ContainerInfo>('statusText', t('columns.statusText'), (r) => r.Status, { group }),
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
        { group },
      ),
      listColumn<ContainerInfo>(
        'networks',
        t('columns.networks'),
        (r) => Object.keys(r.NetworkSettings?.Networks ?? {}),
        { group },
      ),
      listColumn<ContainerInfo>(
        'ipAddresses',
        t('columns.ipAddresses'),
        (r) =>
          Object.values(r.NetworkSettings?.Networks ?? {})
            .map((n) => n.IPAddress)
            .filter(Boolean),
        { group },
      ),
      listColumn<ContainerInfo>(
        'volumes',
        t('columns.volumes'),
        (r) => (r.Mounts ?? []).map((m) => m.Name || m.Source).filter(Boolean),
        { group },
      ),
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        size: 50,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => (
          <div className="flex items-center justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => navigate(`/containers/${row.original.Id}`)}
                  aria-label={t('detail.viewContainer')}
                >
                  <IconExternalLink className="h-3.5 w-3.5 text-muted-foreground" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('detail.viewContainer')}</TooltipContent>
            </Tooltip>
          </div>
        ),
      },
    ],
    [navigate, statsMap, t, group],
  );

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <TablePage
      id="stack-services"
      // Rendered inside the stack detail tabs, which carry their own heading.
      header={false}
      title={t('detail.services')}
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('searchPlaceholder'),
        fields: (container) => [container.Image, container.State, container.Status, ...(container.Names ?? [])],
      }}
      toolbar={{ columns: true, density: true, export: { filename: 'mcharbor-stack-services' } }}
      columns={columns}
      data={containers ?? []}
      loading={isLoading}
      empty={{ title: t('detail.noServices') }}
      getRowId={(container) => container.Id}
    />
  );
}
