// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { formatRelative } from '../format';
import type { SnapshotRun } from '../types';

export function ScheduleRunHistory({ runs }: { runs: SnapshotRun[] }) {
  const { t } = useTranslation('vm');

  return (
    <section className="space-y-2">
      <h4 className="text-sm font-medium text-foreground">{t('schedules.history')}</h4>
      <ul className="space-y-1">
        {runs.slice(0, 10).map((entry) => (
          <li key={entry.id} className="flex flex-wrap items-center gap-2 text-xs">
            <Badge
              variant={
                entry.status === 'success' ? 'default' : entry.status === 'failure' ? 'destructive' : 'secondary'
              }
            >
              {t(`schedules.status.${entry.status}`)}
            </Badge>
            <span className="text-muted-foreground">{formatRelative(entry.startedAt)}</span>
            {entry.snapshotName ? <span className="font-mono text-muted-foreground">{entry.snapshotName}</span> : null}
            {entry.pruned > 0 ? (
              <span className="text-muted-foreground">{t('schedules.pruned', { count: entry.pruned })}</span>
            ) : null}
            {entry.error ? <span className="text-destructive">{entry.error}</span> : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
