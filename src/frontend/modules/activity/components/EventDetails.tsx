// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { formatDate } from '@resources/utils/format';
import { Hint } from '@resources/components/ui/Hint';

export type ContainerEvent = {
  id: string;
  timestamp: string;
  environmentId: string | null;
  environmentName?: string | null;
  containerName: string | null;
  containerId: string;
  action: string;
  eventType: string;
  metadata: string | null;
  createdAt?: string;
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
      <span className="font-mono break-all text-foreground">{value}</span>
    </div>
  );
}

type EventDetailsProps = {
  event: ContainerEvent;
  envName: (id: string | null) => string;
};

export function EventDetails({ event: evt, envName }: EventDetailsProps) {
  const { t } = useTranslation('common');
  const meta = parseMeta(evt.metadata);

  return (
    <div className="space-y-4 px-4 py-3">
      <div className="divide-y divide-border">
        <DetailRow label={t('activity.detailTimestamp')} value={formatDate(evt.timestamp)} />
        <DetailRow label={t('activity.detailEnvironment')} value={envName(evt.environmentId)} />
        <DetailRow label={t('activity.detailEventType')} value={evt.eventType} />
        <DetailRow label={t('activity.detailAction')} value={evt.action} />
        <DetailRow label={t('activity.detailContainerId')} value={evt.containerId} />
        <DetailRow label={t('activity.detailContainer')} value={evt.containerName} />
        <DetailRow label={t('activity.detailImage')} value={meta.image} />
        <DetailRow label={t('activity.detailExitCode')} value={meta.exitCode} />
      </div>

      {Object.keys(meta).length > 0 && (
        <div>
          <h4 className="mb-2 text-xs font-medium tracking-wider text-muted-foreground uppercase">
            {t('activity.attributes')}
          </h4>
          <div className="rounded-md border border-border bg-muted/30 p-3">
            <div className="divide-y divide-border">
              {Object.entries(meta)
                .sort(([a], [b]) => a.localeCompare(b))
                .map(([key, value]) => (
                  <div key={key} className="grid grid-cols-[140px_1fr] gap-3 py-1.5 text-xs">
                    <Hint text={key}>
                      <span className="truncate text-muted-foreground">{key}</span>
                    </Hint>
                    <span className="min-w-0 font-mono break-all text-foreground">{value}</span>
                  </div>
                ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
