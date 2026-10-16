// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { IconDatabase, IconDeviceDesktop, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import type { HostMetrics, PruneType } from '@core/types/docker';
import { cssVars } from '@resources/utils/cssVars';
import { formatBytes, splitBytes } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';

const PRUNE_TYPES: PruneType[] = ['system', 'builder', 'volumes', 'images', 'containers', 'networks'];

export function DockerDiskPanel({
  metrics,
  t,
}: {
  metrics: HostMetrics;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const { disk } = metrics;
  const split = useMemo(() => splitBytes(disk.total), [disk.total]);

  const rows = useMemo(() => {
    const base = [
      { key: 'images', label: t('detail.host.disk.images'), value: disk.imagesSize },
      { key: 'volumes', label: t('detail.host.disk.volumes'), value: disk.volumesSize },
      { key: 'containers', label: t('detail.host.disk.containers'), value: disk.containersSize },
      { key: 'buildCache', label: t('detail.host.disk.buildCache'), value: disk.buildCacheSize },
    ];
    return base.sort((a, b) => b.value - a.value);
  }, [disk, t]);

  return (
    <div className="mc-glass rounded-xl p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2 text-sm font-medium text-foreground">
          <IconDatabase className="h-4 w-4 text-muted-foreground" />
          {t('detail.host.disk.title')}
        </div>
        <div className="text-sm text-foreground tabular-nums">
          {parseFloat(split.value).toFixed(1)} <span className="text-xs text-muted-foreground">{split.unit}</span>
        </div>
      </div>
      <ul className="space-y-1.5 text-xs">
        {rows.map((r) => {
          const s = splitBytes(r.value);
          const pct = disk.total > 0 ? (r.value / disk.total) * 100 : 0;
          return (
            <li key={r.key} className="space-y-0.5">
              <div className="flex items-center justify-between text-muted-foreground">
                <span>{r.label}</span>
                <span className="text-foreground tabular-nums">
                  {parseFloat(s.value).toFixed(1)} {s.unit} · {pct.toFixed(1)}%
                </span>
              </div>
              <div className="h-1 overflow-hidden rounded-full bg-muted">
                <div className="h-full w-(--w) bg-primary/60" style={cssVars({ w: `${Math.min(100, pct)}%` })} />
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export function HostFSPanel({
  metrics,
  t,
}: {
  metrics: HostMetrics;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const { hostFs } = metrics;
  const available = hostFs.total > 0;

  return (
    <div className="mc-glass rounded-xl p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2 text-sm font-medium text-foreground">
          <IconDeviceDesktop className="h-4 w-4 text-muted-foreground" />
          {t('detail.host.hostFs.title')}
        </div>
        {available && (
          <div className="text-sm text-foreground tabular-nums">
            {formatBytes(hostFs.used)}{' '}
            <span className="text-xs text-muted-foreground">/ {formatBytes(hostFs.total)}</span>
          </div>
        )}
      </div>
      {available ? (
        <>
          <div className="mb-2 text-xs text-muted-foreground">
            {t('detail.host.hostFs.path', { path: hostFs.path })}
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-muted">
            <div
              className={hostFs.percent > 90 ? 'h-full w-(--w) bg-danger' : 'h-full w-(--w) bg-primary'}
              style={cssVars({ w: `${Math.min(100, hostFs.percent)}%` })}
            />
          </div>
          <div className="mt-1 text-right text-xs text-muted-foreground tabular-nums">{hostFs.percent.toFixed(1)}%</div>
        </>
      ) : (
        <div className="text-xs text-muted-foreground">{t('detail.host.hostFs.unavailable')}</div>
      )}
    </div>
  );
}

export function PrunePanel({
  onPrune,
  busy,
  agentLimited,
  t,
}: {
  onPrune: (target: { type: PruneType; volumes: boolean }) => void;
  busy: boolean;
  agentLimited: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  return (
    <div className="mc-glass rounded-xl p-4">
      <div className="mb-3 flex items-center gap-2 text-sm font-medium text-foreground">
        <IconTrash className="h-4 w-4 text-muted-foreground" />
        {t('detail.host.prune.title')}
      </div>
      <p className="mb-3 text-xs text-muted-foreground">
        {agentLimited ? t('detail.host.prune.agentLimited') : t('detail.host.prune.description')}
      </p>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {PRUNE_TYPES.map((type) => {
          const destructive = type === 'volumes';
          return (
            <Button
              permission={PERM.HOST_MANAGE}
              key={type}
              variant="outline"
              size="sm"
              disabled={busy || agentLimited}
              onClick={() => onPrune({ type, volumes: false })}
              className="justify-start"
            >
              <IconTrash className="mr-2 h-3.5 w-3.5 text-destructive" />
              {t(`detail.host.prune.${type}.label`)}
              {destructive && (
                <span className="ml-auto text-[10px] tracking-wide text-destructive uppercase">
                  {t('detail.host.prune.destructiveTag')}
                </span>
              )}
            </Button>
          );
        })}
        <Button
          permission={PERM.HOST_MANAGE}
          variant="destructive"
          size="sm"
          disabled={busy || agentLimited}
          onClick={() => onPrune({ type: 'system', volumes: true })}
          className="justify-start"
        >
          <IconTrash className="mr-2 h-3.5 w-3.5" />
          {t('detail.host.prune.system.withVolumesLabel')}
          <span className="ml-auto text-[10px] tracking-wide uppercase opacity-80">
            {t('detail.host.prune.destructiveTag')}
          </span>
        </Button>
      </div>
    </div>
  );
}
