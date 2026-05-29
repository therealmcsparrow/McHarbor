// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconTrash,
  IconExternalLink,
  IconTerminal2,
  IconFileText,
  IconLock,
} from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';
import type { BulkContainerMetric } from '../hooks/useContainersBulkStats';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes } from '@resources/utils/format';
import { ContainerIcon } from './ContainerIcon';
import { ActionButton } from './ActionButton';
import { MiniChart } from './MiniChart';
import { useStatsHistory } from '../hooks/useStatsHistory';
import {
  STATE_VARIANTS,
  getContainerWebUrl,
  getContainerIP,
  getPublicPorts,
} from './container-utils';

type ContainerCardProps = {
  container: ContainerInfo;
  stats?: BulkContainerMetric;
  highlighted?: boolean;
  onAction: (id: string, action: string) => void;
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
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
  onRemove,
  onClick,
}: ContainerCardProps) {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const name = c.Names?.[0]?.replace(/^\//, '') ?? c.Id.slice(0, 12);
  const isRunning = c.State === 'running';
  const locked = isProtectedContainer(c);
  const webUrl = isRunning ? getContainerWebUrl(c.Ports) : null;
  const ip = getContainerIP(c);
  const ports = getPublicPorts(c.Ports);
  const history = useStatsHistory(stats);

  return (
    <Card
      className={[
        'cursor-pointer transition-[background-color,border-color,box-shadow] duration-300 hover:border-primary/40',
        highlighted
          ? 'border-amber-400/60 bg-amber-500/5 shadow-[0_0_0_1px_rgba(251,191,36,0.55),0_0_24px_rgba(251,191,36,0.18)]'
          : '',
      ].join(' ')}
      onClick={() => onClick(c)}
    >
      <CardContent className="flex-1 space-y-3 p-4">
        {/* Header: icon + name + state */}
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <ContainerIcon image={c.Image} className="size-5" />
            <span className="truncate font-medium text-sm">{name}</span>
            {locked && (
              <Badge variant="secondary" className="shrink-0 gap-1 text-[9px] px-1.5 py-0">
                <IconLock className="size-3" />
                {tc('actions.locked')}
              </Badge>
            )}
          </div>
          <Badge
            variant={STATE_VARIANTS[c.State] ?? 'secondary'}
            className="shrink-0 text-[10px] px-2 py-0.5"
          >
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
                <Badge variant="outline" className="text-[10px] px-1.5 py-0.5 font-mono">
                  {ip}
                </Badge>
              </div>
            )}
            {ports.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {ports.map((p) => (
                  <Badge key={p} variant="secondary" className="text-[10px] px-1.5 py-0.5 font-mono">
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
              data={history.cpu}
              color="hsl(142 71% 45%)"
            />
            <MiniChart
              label={t('card.memory')}
              value={formatBytes(stats.memUsage)}
              data={history.mem}
              color="hsl(217 91% 60%)"
            />
            <MiniChart
              label={t('card.netRx')}
              value={formatBytes(stats.netRx)}
              data={history.netRx}
              color="hsl(280 68% 60%)"
            />
            <MiniChart
              label={t('card.netTx')}
              value={formatBytes(stats.netTx)}
              data={history.netTx}
              color="hsl(30 80% 55%)"
            />
          </div>
        )}
      </CardContent>

      {/* Actions */}
      <CardFooter className="gap-1 px-3 py-2" onClick={(e) => e.stopPropagation()}>
        {isRunning ? (
          <ActionButton
            label={t('actions.stop')}
            onClick={() => onAction(c.Id, 'stop')}
            disabled={locked}
            icon={<IconPlayerStop className="h-3.5 w-3.5 text-amber-500" />}
          />
        ) : (
          <ActionButton
            label={t('actions.start')}
            onClick={() => onAction(c.Id, 'start')}
            disabled={locked}
            icon={<IconPlayerPlay className="h-3.5 w-3.5 text-emerald-500" />}
          />
        )}
        <ActionButton
          label={t('actions.restart')}
          onClick={() => onAction(c.Id, 'restart')}
          disabled={locked}
          icon={<IconRotate className="h-3.5 w-3.5 text-blue-400" />}
        />
        {isRunning && (
          <ActionButton
            label={t('actions.terminal')}
            onClick={() => onTerminal(c)}
            disabled={locked}
            icon={<IconTerminal2 className="h-3.5 w-3.5 text-violet-400" />}
          />
        )}
        <ActionButton
          label={t('actions.logs')}
          onClick={() => onLogs(c)}
          icon={<IconFileText className="h-3.5 w-3.5 text-cyan-400" />}
        />
        {webUrl && (
          <ActionButton
            label={t('actions.openWebsite')}
            onClick={() => window.open(webUrl, '_blank', 'noopener,noreferrer')}
            icon={<IconExternalLink className="h-3.5 w-3.5 text-primary" />}
          />
        )}
        <div className="flex-1" />
        <ActionButton
          label={t('actions.remove')}
          onClick={() => onRemove(c)}
          disabled={locked}
          icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
        />
      </CardFooter>
    </Card>
  );
}
