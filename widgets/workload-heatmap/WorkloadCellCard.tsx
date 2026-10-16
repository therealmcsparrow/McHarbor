// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowRight } from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { Button } from '@resources/components/ui/Button';
import { TONE_SOFT, TONE_SOLID, type Tone } from '@modules/dashboard/components/widget-kit';
import { cn } from '@resources/utils/cn';
import { formatDate } from '@resources/utils/format';

/**
 * The card behind a heatmap cell.
 *
 * One cell is ~10px, so everything a person needs to decide "is this the
 * container I was looking for" has to live here: what it is, what state it
 * is in, where it came from, and how to reach it. Ports and IP are the two
 * fields people actually squint at a fleet grid for.
 */
export function WorkloadCellCard({
  container,
  tone,
  onOpen,
}: {
  container: ContainerInfo;
  tone: Tone;
  onOpen: () => void;
}) {
  const { t } = useTranslation('dashboard');

  const title = containerName(container);
  const ports = formatPorts(container);
  const ip = primaryIP(container);
  const created = formatDate(new Date(container.Created * 1000).toISOString());

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start gap-2">
        <span className={cn('mt-1.5 size-2 shrink-0 rounded-full', TONE_SOLID[tone])} aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold text-foreground" title={title}>
            {title}
          </p>
          <p className="truncate text-xs text-muted-foreground" title={container.Image}>
            {container.Image}
          </p>
        </div>
        <span className={cn('shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium capitalize', TONE_SOFT[tone])}>
          {t(`workloadHeatmapWidget.state.${container.State}`)}
        </span>
      </div>

      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs">
        <Row label={t('workloadHeatmapWidget.field.status')} value={container.Status} />
        {container.StackName && (
          <Row
            label={t('workloadHeatmapWidget.field.stack')}
            value={container.StackService ? `${container.StackName} · ${container.StackService}` : container.StackName}
          />
        )}
        <Row label={t('workloadHeatmapWidget.field.ports')} value={ports ?? t('workloadHeatmapWidget.none')} />
        <Row label={t('workloadHeatmapWidget.field.ip')} value={ip ?? t('workloadHeatmapWidget.none')} mono />
        <Row label={t('workloadHeatmapWidget.field.created')} value={created} />
        <Row label={t('workloadHeatmapWidget.field.id')} value={container.Id.slice(0, 12)} mono />
      </dl>

      <Button variant="outline" size="sm" className="w-full" onClick={onOpen}>
        {t('workloadHeatmapWidget.open')}
        <IconArrowRight className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn('min-w-0 truncate text-right text-foreground', mono && 'font-mono')} title={value}>
        {value}
      </dd>
    </>
  );
}

/** Docker prefixes every name with a slash; nobody wants to read it. */
export function containerName(container: ContainerInfo): string {
  const raw = container.Names?.[0] ?? container.Id.slice(0, 12);
  return raw.startsWith('/') ? raw.slice(1) : raw;
}

/**
 * Published ports only — the internal-only ones are noise here, and a
 * container with thirty exposed ports must not push the card off screen.
 */
function formatPorts(container: ContainerInfo): string | null {
  const published = (container.Ports ?? []).filter((p) => p.PublicPort);
  if (published.length === 0) return null;
  const shown = published
    .slice(0, 3)
    .map((p) => `${p.PublicPort}:${p.PrivatePort}/${p.Type}`)
    .join(', ');
  return published.length > 3 ? `${shown} +${published.length - 3}` : shown;
}

function primaryIP(container: ContainerInfo): string | null {
  const networks = container.NetworkSettings?.Networks ?? {};
  for (const net of Object.values(networks)) {
    if (net?.IPAddress) return net.IPAddress;
  }
  return null;
}
