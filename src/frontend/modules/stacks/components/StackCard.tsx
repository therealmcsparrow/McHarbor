// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import type { StackInfo } from '@resources/hooks/useStacks';
import { isProtectedStack } from '@core/utils/protection';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import { ContainerIcon } from '@resources/components/ContainerIcon';
import { Card, CardContent } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes } from '@resources/utils/format';
import { StackCardActions } from './StackCardActions';
import { aggregateStats, useStatsHistory, MiniChart } from './StackCardStats';
import { CHART_COLORS } from '@resources/theme/chart-colors';

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

export function StackCard({ stack: s, statsMap, onAction, onEdit, onLogs, onRemove, onTakeOver }: StackCardProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const isRunning = s.status === 'running' || s.status === 'partial';
  const locked = isProtectedStack(s);
  const running = s.services.filter((svc) => svc.status === 'running').length;
  const aggregated = aggregateStats(s.services, statsMap);
  const statsHistory = useStatsHistory(aggregated);
  const lockedLabel = tc('actions.locked');

  return (
    <Card className="transition-colors hover:border-primary/40">
      <CardContent className="flex-1 space-y-3 p-4">
        {/* Header */}
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            {s.services[0] && <ContainerIcon image={s.services[0].image} className="size-5" />}
            <span className="truncate text-sm font-medium">{s.name}</span>
            {locked && (
              <Badge variant="secondary" className="shrink-0 gap-1 px-1.5 py-0 text-[9px]">
                <IconLock className="size-3" />
                {lockedLabel}
              </Badge>
            )}
            <Badge variant={s.type === 'managed' ? 'default' : 'outline'} className="shrink-0 px-1.5 py-0 text-[9px]">
              {s.type === 'managed' ? t('badges.managed') : t('badges.discovered')}
            </Badge>
          </div>
          <Badge variant={STATUS_VARIANTS[s.status] ?? 'secondary'} className="shrink-0 px-2 py-0.5 text-[10px]">
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
              <Badge variant={svc.status === 'running' ? 'success' : 'destructive'} className="px-1.5 py-0 text-[9px]">
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
              data={statsHistory.cpu}
              color={CHART_COLORS.cpu}
            />
            <MiniChart
              label={t('stats.memory')}
              value={formatBytes(aggregated.memUsage)}
              data={statsHistory.mem}
              color={CHART_COLORS.memory}
            />
            <MiniChart
              label={t('stats.netRx')}
              value={formatBytes(aggregated.netRx)}
              data={statsHistory.netRx}
              color={CHART_COLORS.networkRx}
            />
            <MiniChart
              label={t('stats.netTx')}
              value={formatBytes(aggregated.netTx)}
              data={statsHistory.netTx}
              color={CHART_COLORS.networkTx}
            />
          </div>
        )}

        {s.description && <p className="truncate text-xs text-muted-foreground">{s.description}</p>}
      </CardContent>

      <StackCardActions
        stack={s}
        locked={locked}
        isRunning={isRunning}
        onAction={onAction}
        onEdit={onEdit}
        onLogs={onLogs}
        onRemove={onRemove}
        onTakeOver={onTakeOver}
      />
    </Card>
  );
}
