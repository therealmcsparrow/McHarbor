// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import {
  IconHeartFilled,
  IconArrowsTransferUp,
  IconCircleArrowUp,
  IconCircleCheck,
  IconAlertCircle,
  IconLock,
} from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { ContainerIcon } from '../components/ContainerIcon';
import { ContainerActionsCell } from '../components/ContainerActionsCell';
import {
  STATE_VARIANTS,
  HEALTH_COLORS,
  parseHealth,
  parseUptime,
  formatPorts,
  getContainerIP,
  getStackName,
  getAutoUpdate,
} from '../components/container-utils';

type UseContainerColumnsProps = {
  action: { mutate: (vars: { id: string; action: string }) => void };
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRename: (c: ContainerInfo) => void;
  onMove: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
  onTakeOver: (c: ContainerInfo) => void;
  updateResults?: Map<string, { containerId: string; updateAvailable: boolean; error?: string }>;
};

export function useContainerColumns({
  action,
  onTerminal,
  onLogs,
  onRename,
  onMove,
  onRemove,
  onTakeOver,
  updateResults,
}: UseContainerColumnsProps) {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');

  return useMemo<ColumnDef<ContainerInfo, unknown>[]>(
    () => [
      {
        accessorKey: 'Names',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-x-2">
            <ContainerIcon image={row.original.Image} />
            <span className="truncate font-medium">
              {row.original.Names?.[0]?.replace(/^\//, '') ?? '-'}
            </span>
            {isProtectedContainer(row.original) && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <IconLock className="size-3.5 shrink-0 text-muted-foreground" />
                </TooltipTrigger>
                <TooltipContent>{tc('actions.locked')}</TooltipContent>
              </Tooltip>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'Image',
        header: t('columns.image'),
        meta: { flex: true },
        cell: ({ row }) => (
          <span className="block truncate text-muted-foreground">{row.original.Image}</span>
        ),
      },
      {
        accessorKey: 'State',
        header: t('columns.state'),
        size: 75,
        cell: ({ row }) => (
          <Badge variant={STATE_VARIANTS[row.original.State] ?? 'secondary'}>
            {row.original.State}
          </Badge>
        ),
      },
      {
        id: 'health',
        header: () => <span className="w-full text-center">{t('columns.health')}</span>,
        size: 70,
        cell: ({ row }) => {
          const health = parseHealth(row.original.Status);
          const color = health ? (HEALTH_COLORS[health] ?? 'text-muted-foreground') : 'text-muted-foreground/40';
          const label = health ? t(`health.${health}`) : t('health.noHealthcheck');
          return (
            <div className="flex justify-center">
              <Tooltip>
                <TooltipTrigger asChild>
                  <IconHeartFilled className={`h-4 w-4 ${color}`} />
                </TooltipTrigger>
                <TooltipContent>{label}</TooltipContent>
              </Tooltip>
            </div>
          );
        },
      },
      {
        id: 'uptime',
        header: t('columns.uptime'),
        size: 80,
        cell: ({ row }) => (
          <span className="truncate text-xs text-muted-foreground">
            {parseUptime(row.original.Status)}
          </span>
        ),
      },
      {
        id: 'ip',
        header: t('columns.ip'),
        size: 100,
        cell: ({ row }) => (
          <span className="truncate text-xs text-muted-foreground">
            {getContainerIP(row.original)}
          </span>
        ),
      },
      {
        id: 'ports',
        header: t('columns.ports'),
        size: 100,
        cell: ({ row }) => (
          <span className="truncate text-xs text-muted-foreground">{formatPorts(row.original.Ports)}</span>
        ),
      },
      {
        id: 'autoUpdate',
        header: t('columns.auto'),
        size: 50,
        cell: ({ row }) => {
          const enabled = getAutoUpdate(row.original);
          return enabled ? (
            <Badge variant="success" className="text-[10px] px-1.5 py-0.5">
              on
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
        cell: ({ row }) => {
          const stack = getStackName(row.original);
          return stack ? (
            <Badge variant="default" className="text-[10px] px-1.5 py-0.5 truncate max-w-full">
              {stack}
            </Badge>
          ) : (
            <div className="flex items-center gap-1">
              <Badge variant="secondary" className="text-[10px] px-1.5 py-0.5">
                {t('badges.standalone')}
              </Badge>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="size-5"
                    aria-label={t('actions.takeOver')}
                    onClick={(event) => {
                      event.stopPropagation();
                      onTakeOver(row.original);
                    }}
                  >
                    <IconArrowsTransferUp className="size-3 text-violet-400" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t('actions.takeOver')}</TooltipContent>
              </Tooltip>
            </div>
          );
        },
      },
      {
        id: 'update',
        header: () => <span className="w-full text-center">{t('columns.update')}</span>,
        size: 65,
        cell: ({ row }) => {
          if (!updateResults) return <span className="text-xs text-muted-foreground">-</span>;
          const result = updateResults.get(row.original.Id);
          if (!result) return <span className="text-xs text-muted-foreground">-</span>;
          if (result.error) {
            return (
              <div className="flex justify-center">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <IconAlertCircle className="size-4 text-amber-500" />
                  </TooltipTrigger>
                  <TooltipContent>{t('updates.checkFailed')}</TooltipContent>
                </Tooltip>
              </div>
            );
          }
          return (
            <div className="flex justify-center">
              <Tooltip>
                <TooltipTrigger asChild>
                  {result.updateAvailable ? (
                    <IconCircleArrowUp className="size-4 text-emerald-400" />
                  ) : (
                    <IconCircleCheck className="size-4 text-muted-foreground/50" />
                  )}
                </TooltipTrigger>
                <TooltipContent>
                  {result.updateAvailable ? t('updates.available') : t('updates.upToDate')}
                </TooltipContent>
              </Tooltip>
            </div>
          );
        },
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        size: 155,
        cell: ({ row }) => (
          <ContainerActionsCell
            container={row.original}
            onAction={action.mutate}
            onTerminal={onTerminal}
            onLogs={onLogs}
            onRename={onRename}
            onMove={onMove}
            onRemove={onRemove}
          />
        ),
      },
    ],
    [action, t, tc, onTerminal, onLogs, onRename, onMove, onRemove, onTakeOver, updateResults]
  );
}
