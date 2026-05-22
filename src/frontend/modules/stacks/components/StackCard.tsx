// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
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
} from '@tabler/icons-react';
import type { StackInfo } from '../hooks/useStacks';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import { ContainerIcon } from '@resources/components/ContainerIcon';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes } from '@resources/utils/format';
import { ActionButton } from './ActionButton';
import { aggregateStats, useStatsHistory, MiniChart } from './StackCardStats';

const STATUS_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  running: 'success',
  stopped: 'destructive',
  partial: 'warning',
};

type StackCardProps = {
  stack: StackInfo;
  statsMap?: Map<string, BulkContainerMetric>;
  onAction: (name: string, action: string) => void;
  onEdit: (s: StackInfo) => void;
  onLogs: (s: StackInfo) => void;
  onRemove: (s: StackInfo) => void;
  onTakeOver?: (s: StackInfo) => void;
};

export function StackCard({
  stack: s,
  statsMap,
  onAction,
  onEdit,
  onLogs,
  onRemove,
  onTakeOver,
}: StackCardProps) {
  const { t } = useTranslation('stacks');
  const navigate = useNavigate();
  const isRunning = s.status === 'running' || s.status === 'partial';
  const running = s.services.filter((svc) => svc.status === 'running').length;
  const aggregated = aggregateStats(s.services, statsMap);
  const history = useStatsHistory(aggregated);

  return (
    <Card className="transition-colors hover:border-primary/40">
      <CardContent className="flex-1 space-y-3 p-4">
        {/* Header */}
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            {s.services[0] && (
              <ContainerIcon image={s.services[0].image} className="size-5" />
            )}
            <span className="truncate font-medium text-sm">{s.name}</span>
            <Badge
              variant={s.type === 'managed' ? 'default' : 'outline'}
              className="shrink-0 text-[9px] px-1.5 py-0"
            >
              {s.type === 'managed' ? t('badges.managed') : t('badges.discovered')}
            </Badge>
          </div>
          <Badge
            variant={STATUS_VARIANTS[s.status] ?? 'secondary'}
            className="shrink-0 text-[10px] px-2 py-0.5"
          >
            {s.status}
          </Badge>
        </div>

        {/* Services */}
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {t('card.servicesRunning', { running, total: s.services.length })}
          </span>
        </div>

        {/* Service list */}
        <div className="space-y-1">
          {s.services.map((svc) => (
            <div key={svc.name} className="flex items-center justify-between text-xs">
              <span className="truncate text-muted-foreground">{svc.name}</span>
              <Badge
                variant={svc.status === 'running' ? 'success' : 'destructive'}
                className="text-[9px] px-1.5 py-0"
              >
                {svc.status}
              </Badge>
            </div>
          ))}
        </div>

        {/* Stats sparklines */}
        {isRunning && aggregated && (
          <div className="grid grid-cols-2 gap-x-3 gap-y-2">
            <MiniChart
              label={t('stats.cpu')}
              value={`${aggregated.cpuPercent.toFixed(1)}%`}
              data={history.cpu}
              color="hsl(142 71% 45%)"
            />
            <MiniChart
              label={t('stats.memory')}
              value={formatBytes(aggregated.memUsage)}
              data={history.mem}
              color="hsl(217 91% 60%)"
            />
            <MiniChart
              label={t('stats.netRx')}
              value={formatBytes(aggregated.netRx)}
              data={history.netRx}
              color="hsl(280 68% 60%)"
            />
            <MiniChart
              label={t('stats.netTx')}
              value={formatBytes(aggregated.netTx)}
              data={history.netTx}
              color="hsl(30 80% 55%)"
            />
          </div>
        )}

        {s.description && (
          <p className="truncate text-xs text-muted-foreground">{s.description}</p>
        )}
      </CardContent>

      {/* Actions */}
      <CardFooter className="gap-1 px-3 py-2" onClick={(e) => e.stopPropagation()}>
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
        {s.type === 'discovered' && onTakeOver && (
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
        <div className="flex-1" />
        <ActionButton
          label={t('actions.remove')}
          onClick={() => onRemove(s)}
          icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
        />
      </CardFooter>
    </Card>
  );
}
