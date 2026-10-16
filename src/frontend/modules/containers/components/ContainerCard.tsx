// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';
import type { BulkContainerMetric } from '../hooks/useContainersBulkStats';
import { Card, CardContent } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes } from '@resources/utils/format';
import { ContainerIcon } from './ContainerIcon';
import { ContainerCardActions } from './ContainerCardActions';
import { MiniChart } from './MiniChart';
import { useStatsHistory } from '../hooks/useStatsHistory';
import { STATE_VARIANTS, getContainerWebUrl, getContainerIP, getPublicPorts } from './container-utils';
import { CHART_COLORS } from '@resources/theme/chart-colors';

type ContainerCardProps = {
  container: ContainerInfo;
  stats?: BulkContainerMetric;
  highlighted?: boolean;
  onAction: (id: string, action: string) => void;
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRename: (c: ContainerInfo) => void;
  onMove: (c: ContainerInfo) => void;
  onRecreate: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
  onClick: (c: ContainerInfo) => void;
};

export function ContainerCard({
  container: c,
  stats,
  highlighted = false,
  onAction,
  onTerminal,
  onLogs,
  onRename,
  onMove,
  onRecreate,
  onRemove,
  onClick,
}: ContainerCardProps) {
  const { t } = useTranslation('containers');
  const itemName = c.Names?.[0]?.replace(/^\//, '') ?? c.Id.slice(0, 12);
  const isRunning = c.State === 'running';
  const locked = isProtectedContainer(c);
  const webUrl = isRunning ? getContainerWebUrl(c.Ports) : null;
  const ip = getContainerIP(c);
  const ports = getPublicPorts(c.Ports);
  const historyData = useStatsHistory(stats);

  return (
    <Card
      className={[
        'cursor-pointer transition-[background-color,border-color,box-shadow] duration-300 hover:border-primary/40',
        highlighted ? 'border-warning/60 bg-warning/5 ring-1 ring-warning/55 shadow-lg shadow-warning/20' : '',
      ].join(' ')}
      onClick={() => onClick(c)}
    >
      <CardContent className="flex-1 space-y-3 p-4">
        {/* Header: icon + name + state */}
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <ContainerIcon image={c.Image} className="size-5" />
            <span className="truncate text-sm font-medium">{itemName}</span>
            {locked && (
              <Badge variant="secondary" className="shrink-0 gap-1 px-1.5 py-0 text-[9px]">
                <IconLock className="size-3" />
                {t('actions.locked', { ns: 'common' })}
              </Badge>
            )}
          </div>
          <Badge variant={STATE_VARIANTS[c.State] ?? 'secondary'} className="shrink-0 px-2 py-0.5 text-[10px]">
            {c.State}
          </Badge>
        </div>

        {/* Image */}
        <p className="truncate text-xs text-muted-foreground">{c.Image}</p>

        {/* IP + Ports badges */}
        {(ip !== '-' || ports.length > 0) && (
          <div className="space-y-1.5">
            {ip !== '-' && (
              <div>
                <Badge variant="outline" className="px-1.5 py-0.5 font-mono text-[10px]">
                  {ip}
                </Badge>
              </div>
            )}
            {ports.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {ports.map((p) => (
                  <Badge key={p} variant="secondary" className="px-1.5 py-0.5 font-mono text-[10px]">
                    {p}
                  </Badge>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Stats sparklines (running only) */}
        {isRunning && stats && (
          <div className="grid grid-cols-2 gap-x-3 gap-y-2">
            <MiniChart
              label={t('card.cpu')}
              value={`${stats.cpuPercent.toFixed(1)}%`}
              data={historyData.cpu}
              color={CHART_COLORS.cpu}
            />
            <MiniChart
              label={t('card.memory')}
              value={formatBytes(stats.memUsage)}
              data={historyData.mem}
              color={CHART_COLORS.memory}
            />
            <MiniChart
              label={t('card.netRx')}
              value={formatBytes(stats.netRx)}
              data={historyData.netRx}
              color={CHART_COLORS.networkRx}
            />
            <MiniChart
              label={t('card.netTx')}
              value={formatBytes(stats.netTx)}
              data={historyData.netTx}
              color={CHART_COLORS.networkTx}
            />
          </div>
        )}
      </CardContent>

      <ContainerCardActions
        container={c}
        locked={locked}
        webUrl={webUrl}
        onAction={onAction}
        onTerminal={onTerminal}
        onLogs={onLogs}
        onRename={onRename}
        onMove={onMove}
        onRecreate={onRecreate}
        onRemove={onRemove}
      />
    </Card>
  );
}
