// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconServerBolt } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useOverview } from '../hooks/useCompute';
import { useWidgetContext } from './mount';

/**
 * Which hosts can run guests, and what is stopping the ones that cannot.
 *
 * The second half is the useful half. A tile that only counted ready hosts
 * would be a number nobody acts on; naming the reason — offline, too old, a
 * missing kernel feature — is what turns it into something with a next step.
 */
export function HostsWidget() {
  const { t } = useTranslation('compute');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useOverview();

  if (isLoading) {
    return <Skeleton className="h-full w-full" />;
  }

  const hosts = (data?.environments ?? []).filter((entry) => entry.computeEnabled);

  if (hosts.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconServerBolt className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('overview.empty.title')}</p>
      </div>
    );
  }

  const ready = hosts.filter((entry) => entry.available && entry.setupComplete).length;

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-foreground">{ready}</span>
        <span className="text-sm text-muted-foreground">
          / {hosts.length} {t('overview.setupComplete').toLowerCase()}
        </span>
      </div>

      <ul className="min-h-0 flex-1 space-y-1.5 overflow-y-auto">
        {hosts.map((entry) => (
          <li key={entry.environmentId}>
            <Button
              type="button"
              variant="ghost"
              size="lg"
              onClick={() => navigate('/plugins/addon-compute')}
              className="flex w-full items-center justify-between gap-2 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
            >
              <span className="min-w-0 truncate text-sm text-foreground">{entry.name || entry.environmentId}</span>
              <HostState entry={entry} />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** One host's state, saying why rather than only that. */
function HostState({ entry }: { entry: { available: boolean; setupComplete: boolean; unavailableReason?: string } }) {
  const { t } = useTranslation('compute');

  if (!entry.available) {
    const label =
      entry.unavailableReason === 'agent_too_old'
        ? t('agent.tooOld.title')
        : entry.unavailableReason === 'not_agent_environment'
          ? t('agent.required.title')
          : t('agent.offline.title');
    return (
      <Badge variant="warning" className="shrink-0">
        {label}
      </Badge>
    );
  }

  if (!entry.setupComplete) {
    return (
      <Badge variant="outline" className="shrink-0">
        {t('overview.setupIncomplete')}
      </Badge>
    );
  }

  return (
    <Badge variant="success" className="shrink-0">
      {t('overview.setupComplete')}
    </Badge>
  );
}
