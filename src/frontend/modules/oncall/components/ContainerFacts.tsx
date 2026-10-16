// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { formatDateCustom } from '@resources/utils/format';
import type { ContainerInspect } from '@core/types/docker';
import { Badge } from '@resources/components/ui/Badge';
import { StatusError } from '@resources/components/ui/StatusError';

function isSet(value: string | undefined): value is string {
  return typeof value === 'string' && value !== '' && !value.startsWith('0001-01-01');
}

export function ContainerFacts({ container }: { container: ContainerInspect }) {
  const { t } = useTranslation('oncall');
  const format = (value: string) => formatDateCustom(value, { dateStyle: 'short', timeStyle: 'medium' });
  const { State } = container;
  const rows: Array<[string, string]> = [
    [t('container.exitCode'), String(State.ExitCode)],
    [t('container.restarts'), String(container.RestartCount)],
  ];
  if (isSet(State.StartedAt)) rows.push([t('container.startedAt'), format(State.StartedAt)]);
  if (isSet(State.FinishedAt)) rows.push([t('container.finishedAt'), format(State.FinishedAt)]);
  const checks = [...(State.Health?.Log ?? [])].reverse().slice(0, 5);

  return (
    <div className="flex flex-col gap-3">
      <dl className="grid grid-cols-2 gap-2">
        {rows.map(([label, value]) => (
          <div key={label} className="rounded-md bg-muted/40 p-2">
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="font-medium tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
      {State.OOMKilled && <Badge variant="destructive">{t('container.oomKilled')}</Badge>}
      {State.Error && (
        <div className="flex items-center gap-2 text-sm text-destructive">
          <span>{t('container.error')}</span>
          <StatusError error={State.Error} />
        </div>
      )}
      <div>
        <h2 className="mb-1 text-sm font-medium">
          {t('container.health')}
          {State.Health && (
            <Badge variant="secondary" className="ml-2">
              {State.Health.Status}
            </Badge>
          )}
        </h2>
        {checks.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('container.noHealth')}</p>
        ) : (
          <ul aria-label={t('container.healthLog')} className="divide-y divide-border">
            {checks.map((check) => (
              <li key={check.Start} className="flex min-h-9 items-center gap-2 text-sm">
                <time dateTime={check.Start} className="flex-1 text-muted-foreground">
                  {format(check.Start)}
                </time>
                <Badge variant={check.ExitCode === 0 ? 'success' : 'destructive'}>{check.ExitCode}</Badge>
                {check.ExitCode !== 0 && <StatusError error={check.Output} />}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
