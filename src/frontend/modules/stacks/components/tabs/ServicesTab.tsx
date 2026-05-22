// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { IconExternalLink } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { Button } from '@resources/components/ui/Button';
import { DataGrid } from '@resources/components/DataGrid';
import { Spinner } from '@resources/components/ui/Spinner';
import { truncateId, formatBytes } from '@resources/utils/format';
import type { ContainerInfo } from '@core/types/docker';
import { useContainersBulkStats } from '@resources/hooks/useContainersBulkStats';
import { useStackContainers } from '../../hooks/useStacks';

type ServicesTabProps = {
  stackName: string;
};

export function ServicesTab({ stackName }: ServicesTabProps) {
  const { t } = useTranslation('stacks');
  const navigate = useNavigate();
  const { data: containers, isLoading } = useStackContainers(stackName);
  const { data: statsMap } = useContainersBulkStats();

  const columns = useMemo<ColumnDef<ContainerInfo, unknown>[]>(
    () => [
      {
        id: 'service',
        header: t('detail.serviceName'),
        cell: ({ row }) => (
          <span className="font-medium text-sm">
            {row.original.Labels['com.docker.compose.service'] ?? '-'}
          </span>
        ),
      },
      {
        id: 'containerId',
        header: t('detail.containerId'),
        size: 120,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">
            {truncateId(row.original.Id)}
          </span>
        ),
      },
      {
        id: 'status',
        header: t('detail.status'),
        size: 90,
        cell: ({ row }) => (
          <Badge
            variant={row.original.State === 'running' ? 'success' : 'destructive'}
            className="text-[10px] px-1.5 py-0"
          >
            {row.original.State}
          </Badge>
        ),
      },
      {
        id: 'image',
        header: t('detail.image'),
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground truncate block max-w-[200px]">
            {row.original.Image}
          </span>
        ),
      },
      {
        id: 'ports',
        header: t('detail.ports'),
        cell: ({ row }) => {
          const ports = row.original.Ports ?? [];
          if (ports.length === 0) return <span className="text-xs text-muted-foreground">-</span>;
          return (
            <div className="flex flex-wrap gap-1">
              {ports
                .filter((p) => p.PublicPort)
                .slice(0, 3)
                .map((p) => (
                  <Badge key={`${p.PublicPort}-${p.PrivatePort}`} variant="secondary" className="text-[9px] px-1.5 py-0">
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
        cell: ({ row }) => {
          const stats = statsMap?.get(row.original.Id);
          if (!stats) return <span className="text-xs text-muted-foreground">-</span>;
          return (
            <span className="text-xs tabular-nums">{stats.cpuPercent.toFixed(1)}%</span>
          );
        },
      },
      {
        id: 'memory',
        header: t('detail.memory'),
        size: 90,
        cell: ({ row }) => {
          const stats = statsMap?.get(row.original.Id);
          if (!stats) return <span className="text-xs text-muted-foreground">-</span>;
          return (
            <span className="text-xs tabular-nums">{formatBytes(stats.memUsage)}</span>
          );
        },
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        size: 50,
        cell: ({ row }) => (
          <div className="flex items-center justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => navigate(`/containers/${row.original.Id}`)}
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
    [navigate, statsMap, t],
  );

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <DataGrid
      data={containers ?? []}
      columns={columns}
      searchKey="Image"
      searchPlaceholder={t('searchPlaceholder')}
      loading={isLoading}
      emptyMessage={t('detail.noServices')}
    />
  );
}
