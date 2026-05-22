// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconTrash,
  IconFileText,
  IconPencil,
  IconArrowDown,
  IconEye,
  IconArrowsTransferUp,
  IconCircleArrowUp,
  IconCircleCheck,
  IconAlertCircle,
} from '@tabler/icons-react';
import { StatusBadge, STACK_STATUS } from '@resources/components/ui/StatusBadge';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { ContainerIcon } from '@resources/components/ContainerIcon';
import type { StackInfo } from '../hooks/useStacks';
import type { StackUpdateResult } from '../hooks/useStackUpdates';
import { ActionButton } from './ActionButton';

type UseStackTableColumnsParams = {
  onEdit: (s: StackInfo) => void;
  onLogs: (s: StackInfo) => void;
  onRemove: (s: StackInfo) => void;
  onTakeOver: (s: StackInfo) => void;
  onAction: (name: string, action: string) => void;
  updateResults?: Map<string, StackUpdateResult>;
};

export function useStackTableColumns({
  onEdit,
  onLogs,
  onRemove,
  onTakeOver,
  onAction,
  updateResults,
}: UseStackTableColumnsParams): ColumnDef<StackInfo, unknown>[] {
  const { t } = useTranslation('stacks');
  const navigate = useNavigate();

  return [
    {
      accessorKey: 'name',
      header: t('columns.name'),
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          {row.original.services[0] && (
            <ContainerIcon image={row.original.services[0].image} />
          )}
          <span className="font-medium">{row.original.name}</span>
          <Badge
            variant={row.original.type === 'managed' ? 'default' : 'outline'}
            className="text-[9px] px-1.5 py-0"
          >
            {row.original.type === 'managed' ? t('badges.managed') : t('badges.discovered')}
          </Badge>
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: t('columns.status'),
      size: 90,
      cell: ({ row }) => (
        <StatusBadge status={row.original.status} map={STACK_STATUS} />
      ),
    },
    {
      id: 'services',
      header: t('columns.services'),
      size: 85,
      cell: ({ row }) => {
        const svcs = row.original.services ?? [];
        const running = svcs.filter((svc) => svc.status === 'running').length;
        return (
          <span className="text-xs tabular-nums text-muted-foreground">
            {running}/{svcs.length}
          </span>
        );
      },
    },
    {
      id: 'images',
      header: t('columns.images'),
      cell: ({ row }) => {
        const images = [...new Set(row.original.services.map((svc) => svc.image))];
        return (
          <div className="flex flex-wrap gap-1">
            {images.slice(0, 3).map((img) => (
              <Badge key={img} variant="secondary" className="text-[9px] px-1.5 py-0 truncate max-w-[140px]">
                {img.split(':')[0]?.split('/').pop()}
              </Badge>
            ))}
            {images.length > 3 && (
              <Badge variant="secondary" className="text-[9px] px-1.5 py-0">
                +{images.length - 3}
              </Badge>
            )}
          </div>
        );
      },
    },
    {
      accessorKey: 'description',
      header: t('columns.description'),
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground truncate block">
          {row.original.description || '-'}
        </span>
      ),
    },
    {
      id: 'update',
      header: t('updates.column'),
      size: 70,
      cell: ({ row }) => {
        if (!updateResults) return null;
        const result = updateResults.get(row.original.name);
        if (!result) return null;

        const hasError = result.services.some((s) => s.error);

        if (result.updateAvailable) {
          return (
            <Tooltip>
              <TooltipTrigger asChild>
                <IconCircleArrowUp className="size-4 text-emerald-500" />
              </TooltipTrigger>
              <TooltipContent>{t('updates.updateAvailable')}</TooltipContent>
            </Tooltip>
          );
        }
        if (hasError) {
          return (
            <Tooltip>
              <TooltipTrigger asChild>
                <IconAlertCircle className="size-4 text-amber-500" />
              </TooltipTrigger>
              <TooltipContent>{t('updates.checkFailed')}</TooltipContent>
            </Tooltip>
          );
        }
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconCircleCheck className="size-4 text-muted-foreground" />
            </TooltipTrigger>
            <TooltipContent>{t('updates.upToDate')}</TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      id: 'actions',
      header: () => <span className="ml-auto">{t('columns.actions')}</span>,
      size: 180,
      cell: ({ row }) => {
        const s = row.original;
        const isRunning = s.status === 'running' || s.status === 'partial';
        return (
          <div className="flex items-center justify-end">
            <ActionButton
              label={t('actions.view')}
              onClick={() => navigate(`/stacks/${s.name}`)}
              icon={<IconEye className="h-3.5 w-3.5 text-foreground" />}
            />
            {s.type === 'managed' && (
              <ActionButton
                label={t('actions.edit')}
                onClick={() => onEdit(s)}
                icon={<IconPencil className="h-3.5 w-3.5 text-primary" />}
              />
            )}
            {s.type === 'discovered' && (
              <ActionButton
                label={t('takeOver.adopt')}
                onClick={() => onTakeOver(s)}
                icon={<IconArrowsTransferUp className="h-3.5 w-3.5 text-violet-400" />}
              />
            )}
            <ActionButton
              label={t('actions.logs')}
              onClick={() => onLogs(s)}
              icon={<IconFileText className="h-3.5 w-3.5 text-cyan-400" />}
            />
            {isRunning ? (
              <>
                <ActionButton
                  label={t('actions.restart')}
                  onClick={() => onAction(s.name, 'restart')}
                  icon={<IconRotate className="h-3.5 w-3.5 text-blue-400" />}
                />
                <ActionButton
                  label={t('actions.stop')}
                  onClick={() => onAction(s.name, 'stop')}
                  icon={<IconPlayerStop className="h-3.5 w-3.5 text-amber-500" />}
                />
              </>
            ) : (
              s.type === 'managed' && (
                <ActionButton
                  label={t('actions.up')}
                  onClick={() => onAction(s.name, 'up')}
                  icon={<IconPlayerPlay className="h-3.5 w-3.5 text-emerald-500" />}
                />
              )
            )}
            <ActionButton
              label={t('actions.down')}
              onClick={() => onAction(s.name, 'down')}
              icon={<IconArrowDown className="h-3.5 w-3.5 text-orange-400" />}
            />
            <ActionButton
              label={t('actions.remove')}
              onClick={() => onRemove(s)}
              icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
            />
          </div>
        );
      },
    },
  ];
}
