// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconChevronRight } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { StatusBadge, CONTAINER_STATUS } from '@resources/components/ui/StatusBadge';
import { StatusError } from '@resources/components/ui/StatusError';
import { Spinner } from '@resources/components/ui/Spinner';
import type { FleetEntry } from '../hooks/useFleet';

const COUNT_KEYS = ['running', 'stopped', 'restarting', 'other'] as const;

function containerName(names: string[]): string {
  return (names[0] ?? '').replace(/^[/]/, '');
}

export function FleetCard({ entry }: { entry: FleetEntry }) {
  const { t } = useTranslation('oncall');
  const { environment, counts, containers } = entry;
  const problems = containers.filter((container) => String(container.State) !== 'running');
  const health = !environment.isActive ? 'inactive' : entry.error ? 'unreachable' : 'reachable';
  const variant = health === 'reachable' ? 'success' : health === 'unreachable' ? 'destructive' : 'secondary';

  return (
    <li className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-center gap-2">
        <h2 className="min-w-0 flex-1 truncate font-medium">{environment.name}</h2>
        {entry.isLoading ? <Spinner size="sm" /> : <Badge variant={variant}>{t(`fleet.${health}`)}</Badge>}
        <StatusError error={entry.error} />
      </div>
      {environment.isActive && !entry.error && !entry.isLoading && (
        <dl className="mt-3 grid grid-cols-4 gap-2 text-center">
          {COUNT_KEYS.map((key) => (
            <div key={key} className="rounded-md bg-muted/40 py-2">
              <dt className="text-xs text-muted-foreground">{t(`fleet.${key}`)}</dt>
              <dd className="text-lg font-semibold tabular-nums">{counts[key]}</dd>
            </div>
          ))}
        </dl>
      )}
      {problems.length > 0 && (
        <div className="mt-3">
          <h3 className="mb-1 text-xs font-medium text-muted-foreground uppercase">{t('fleet.problemsTitle')}</h3>
          <ul className="divide-y divide-border">
            {problems.map((container) => (
              <li key={container.Id}>
                <Link
                  to={`/oncall/containers/${encodeURIComponent(environment.id)}/${encodeURIComponent(container.Id)}`}
                  className="flex min-h-11 items-center gap-2 py-1"
                >
                  <span className="min-w-0 flex-1 truncate text-sm">{containerName(container.Names)}</span>
                  <StatusBadge status={String(container.State)} map={CONTAINER_STATUS} />
                  <IconChevronRight className="size-4 text-muted-foreground" aria-hidden="true" />
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </li>
  );
}
