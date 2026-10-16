// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { formatRelative } from '../format';
import type { Container } from '../types';

export function ContainerOverviewTab({ container }: { container: Container }) {
  const { t } = useTranslation('ct');

  return (
    <>
      <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Fact label={t('detail.overview.init')} value={container.initSystem} />
        <Fact
          label={t('detail.overview.os')}
          value={
            container.osFamily
              ? `${container.osFamily} ${container.osVersion ?? ''}`.trim()
              : t('detail.overview.unknown')
          }
        />
        <Fact label={t('detail.overview.pool')} value={`${container.poolBackend} · ${container.poolTarget}`} mono />
        <Fact
          label={t('isolation.idMapping')}
          value={
            container.privileged
              ? t('isolation.privileged')
              : `${container.uidMapStart ?? 0}..${(container.uidMapStart ?? 0) + (container.uidMapCount ?? 0)}`
          }
          hint={t('isolation.idMappingHint')}
          mono
        />
        <Fact
          label={t('detail.overview.started')}
          value={container.lastStartedAt ? formatRelative(container.lastStartedAt) : t('detail.overview.neverStarted')}
        />
        <Fact
          label={t('detail.overview.processes')}
          value={container.initPid ? `PID ${container.initPid}` : '—'}
          mono
        />
      </dl>

      {/*
        The exit reason gets its own block above the last error, because it
        is the more specific of the two: the agent reads it from the unit's
        result, so an OOM kill says so rather than presenting as a generic
        failure.
      */}
      {container.exitReason && (
        <div className="mt-4 rounded-md bg-muted p-3">
          <p className="text-xs font-medium text-muted-foreground">{t('detail.overview.exitReason')}</p>
          <p className="mt-1 text-sm text-foreground">{container.exitReason}</p>
        </div>
      )}
      {container.lastError && (
        <div className="mt-2 rounded-md bg-destructive/10 p-3">
          <p className="text-xs font-medium text-muted-foreground">{t('detail.overview.lastError')}</p>
          <p className="mt-1 text-sm text-foreground">{container.lastError}</p>
        </div>
      )}
    </>
  );
}

export function Fact({
  label,
  value,
  hint,
  mono = false,
}: {
  label: string;
  value: string;
  hint?: string;
  mono?: boolean;
}) {
  const body = (
    <dd className={mono ? 'mt-0.5 font-mono text-sm text-foreground' : 'mt-0.5 text-sm text-foreground'}>{value}</dd>
  );

  return (
    <div>
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      {hint ? (
        <Tooltip>
          <TooltipTrigger asChild>{body}</TooltipTrigger>
          <TooltipContent>{hint}</TooltipContent>
        </Tooltip>
      ) : (
        body
      )}
    </div>
  );
}
