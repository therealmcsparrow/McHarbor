// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { formatDate } from '@resources/utils/format';

export type ContainerEvent = {
  id: string;
  timestamp: string;
  environmentId: string | null;
  containerName: string | null;
  containerId: string;
  action: string;
  eventType: string;
  metadata: string | null;
};

export type ParsedMeta = Record<string, string>;

export function parseMeta(raw: string | null): ParsedMeta {
  if (!raw) return {};
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

export const ACTION_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  start: 'success',
  create: 'success',
  stop: 'destructive',
  die: 'destructive',
  kill: 'destructive',
  destroy: 'destructive',
  restart: 'warning',
  pause: 'warning',
  unpause: 'success',
};

function DetailRow({ label, value }: { label: string; value: string | null | undefined }) {
  if (!value) return null;
  return (
    <div className="flex gap-3 py-1.5 text-sm">
      <span className="w-32 shrink-0 text-muted-foreground">{label}</span>
      <span className="break-all font-mono text-foreground">{value}</span>
    </div>
  );
}

type EventDetailsProps = {
  event: ContainerEvent;
  envName: (id: string | null) => string;
};

export function EventDetails({ event, envName }: EventDetailsProps) {
  const { t } = useTranslation('common');
  const meta = parseMeta(event.metadata);

  return (
    <div className="px-4 py-3 space-y-4">
      <div className="divide-y divide-border">
        <DetailRow label={t('activity.detailTimestamp')} value={formatDate(event.timestamp)} />
        <DetailRow label={t('activity.detailEnvironment')} value={envName(event.environmentId)} />
        <DetailRow label={t('activity.detailEventType')} value={event.eventType} />
        <DetailRow label={t('activity.detailAction')} value={event.action} />
        <DetailRow label={t('activity.detailContainerId')} value={event.containerId} />
        <DetailRow label={t('activity.detailContainer')} value={event.containerName} />
        <DetailRow label={t('activity.detailImage')} value={meta.image} />
        <DetailRow label={t('activity.detailExitCode')} value={meta.exitCode} />
      </div>

      {Object.keys(meta).length > 0 && (
        <div>
          <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t('activity.attributes')}
          </h4>
          <div className="rounded-md border border-border bg-muted/30 p-3">
            <div className="divide-y divide-border">
              {Object.entries(meta)
                .sort(([a], [b]) => a.localeCompare(b))
                .map(([key, value]) => (
                  <div key={key} className="grid grid-cols-[140px_1fr] gap-3 py-1.5 text-xs">
                    <span className="truncate text-muted-foreground" title={key}>{key}</span>
                    <span className="break-all font-mono text-foreground min-w-0">{value}</span>
                  </div>
                ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

