// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconLock } from '@tabler/icons-react';
import { StatusBadge, STACK_STATUS } from '@resources/components/ui/StatusBadge';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { ContainerIcon } from '@resources/components/ContainerIcon';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import { textColumn, dateColumn, boolColumn, listColumn } from '@resources/layout/table-page/optional-columns';
import { isProtectedStack } from '@core/utils/protection';
import type { StackInfo } from '@resources/hooks/useStacks';
import type { StackUpdateResult } from '../hooks/useStackUpdates';
import { StackRowActions, StackUpdateCell } from './StackTableCells';

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
  const { t: tc } = useTranslation('common');
  const lockedLabel = tc('actions.locked');
  const navigate = useNavigate();
  const { data: environments } = useEnvironmentList();
  const envNameById = useMemo(() => new Map((environments ?? []).map((env) => [env.id, env.name])), [environments]);
  const group = t('columns.groupStack');

  return useMemo<ColumnDef<StackInfo, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('columns.name'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            {row.original.services[0] && <ContainerIcon image={row.original.services[0].image} />}
            <Button
              type="button"
              variant="link"
              className="h-auto p-0 font-medium text-foreground underline-offset-4 hover:text-primary"
              onClick={() => navigate(`/stacks/${encodeURIComponent(row.original.name)}`)}
            >
              {row.original.name}
            </Button>
            {isProtectedStack(row.original) && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <IconLock className="size-3.5 shrink-0 text-muted-foreground" />
                </TooltipTrigger>
                <TooltipContent>{lockedLabel}</TooltipContent>
              </Tooltip>
            )}
            <Badge variant={row.original.type === 'managed' ? 'default' : 'outline'} className="px-1.5 py-0 text-[9px]">
              {row.original.type === 'managed' ? t('badges.managed') : t('badges.discovered')}
            </Badge>
          </div>
        ),
      },
      {
        accessorKey: 'status',
        header: t('columns.status'),
        size: 90,
        cell: ({ row }) => <StatusBadge status={row.original.status} map={STACK_STATUS} />,
      },
      {
        id: 'services',
        header: t('columns.services'),
        size: 85,
        meta: {
          sortValue: (s: StackInfo) => (s.services ?? []).filter((svc) => svc.status === 'running').length,
        },
        cell: ({ row }) => {
          const svcs = row.original.services ?? [];
          const running = svcs.filter((svc) => svc.status === 'running').length;
          return (
            <span className="text-xs text-muted-foreground tabular-nums">
              {running}/{svcs.length}
            </span>
          );
        },
      },
      {
        id: 'images',
        header: t('columns.images'),
        meta: {
          sortValue: (s: StackInfo) => [...new Set(s.services.map((svc) => svc.image))].join(', '),
        },
        cell: ({ row }) => {
          const images = [...new Set(row.original.services.map((svc) => svc.image))];
          return (
            <div className="flex flex-wrap gap-1">
              {images.slice(0, 3).map((img) => (
                <Badge key={img} variant="secondary" className="max-w-[140px] truncate px-1.5 py-0 text-[9px]">
                  {img.split(':')[0]?.split('/').pop()}
                </Badge>
              ))}
              {images.length > 3 && (
                <Badge variant="secondary" className="px-1.5 py-0 text-[9px]">
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
          <span className="block truncate text-xs text-muted-foreground">{row.original.description || '-'}</span>
        ),
      },
      {
        id: 'update',
        header: t('updates.column'),
        size: 70,
        meta: {
          sortValue: (s: StackInfo) => {
            const result = updateResults?.get(s.name);
            return result ? result.updateAvailable : null;
          },
        },
        cell: ({ row }) => {
          const result = updateResults?.get(row.original.name);
          return result ? <StackUpdateCell result={result} /> : null;
        },
      },
      textColumn<StackInfo>('stackId', t('columns.id'), (r) => r.id, { group }),
      textColumn<StackInfo>('type', t('columns.type'), (r) => r.type, { group, width: 'sm' }),
      textColumn<StackInfo>('orchestrator', t('columns.orchestrator'), (r) => r.orchestrator, { group, width: 'sm' }),
      dateColumn<StackInfo>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<StackInfo>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
      boolColumn<StackInfo>('protected', t('columns.protected'), (r) => r.protected, { group }),
      textColumn<StackInfo>('environment', t('columns.environment'), (r) => envNameById.get(r.environmentId ?? ''), {
        group: t('columns.groupEnvironment'),
      }),
      textColumn<StackInfo>('owner', t('columns.owner'), (r) => r.ownerName, { group: t('columns.groupOwner') }),
      listColumn<StackInfo>(
        'containerIds',
        t('columns.containerIds'),
        (r) => r.services.map((s) => s.containerId?.slice(0, 12)).filter(Boolean) as string[],
        { group: t('columns.groupContainers') },
      ),
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        size: 160,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => (
          <StackRowActions
            stack={row.original}
            onEdit={onEdit}
            onLogs={onLogs}
            onRemove={onRemove}
            onTakeOver={onTakeOver}
            onAction={onAction}
          />
        ),
      },
    ],
    [envNameById, group, navigate, onAction, onEdit, onLogs, onRemove, onTakeOver, t, lockedLabel, updateResults],
  );
}
