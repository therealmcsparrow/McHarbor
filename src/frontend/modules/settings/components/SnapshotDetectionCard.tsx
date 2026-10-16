// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { useSnapshotDrivers, type SnapshotDriverStatus } from '../hooks/useSnapshotConfigs';
import { DriverRow } from './SnapshotDriverRow';

interface SnapshotDetectionCardProps {
  envId: string | null;
  /** Opens the config dialog pre-filled with the chosen driver. */
  onSetUp: (driver: SnapshotDriverStatus) => void;
}

/**
 * What this environment can actually freeze.
 *
 * Snapshot support is a property of the machine holding the data, not of
 * McHarbor, so it cannot be answered from configuration. For an agent
 * environment the agent probes its own host; for a local one McHarbor
 * probes its own container. Unavailable drivers stay in the list with
 * the reason attached, because "btrfs is not installed" is the one piece
 * of information that tells an operator what to do next.
 */
export function SnapshotDetectionCard({ envId, onSetUp }: SnapshotDetectionCardProps) {
  const { t } = useTranslation('settings');
  const drivers = useSnapshotDrivers(envId);

  const rows = drivers.data ?? [];
  const originGroups = groupByOrigin(rows);
  const availableCount = rows.filter((row) => row.available).length;

  return (
    <Card>
      <CardHeader className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-lg font-semibold">{t('snapshots.detection.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('snapshots.detection.subtitle')}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => drivers.refetch()}
          disabled={!envId || drivers.isFetching}
          aria-label={t('snapshots.detection.rescan')}
        >
          {drivers.isFetching ? <Spinner className="size-4" /> : <IconRefresh className="size-4" />}
          {t('snapshots.detection.rescan')}
        </Button>
      </CardHeader>
      <CardContent>
        {!envId && <EmptyState title={t('snapshots.configs.noEnvironments')} />}

        {envId && drivers.isLoading && (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        )}

        {envId && drivers.isError && (
          <ErrorState error={drivers.error as Error} title={t('snapshots.detection.failed')} />
        )}

        {envId && !drivers.isLoading && !drivers.isError && rows.length === 0 && (
          <EmptyState title={t('snapshots.detection.empty')} description={t('snapshots.detection.emptyHint')} />
        )}

        {rows.length > 0 && (
          <>
            <p className="pb-4 text-sm text-muted-foreground">
              {availableCount > 0
                ? t('snapshots.detection.summaryAvailable', { count: availableCount })
                : t('snapshots.detection.summaryNone')}
            </p>
            {/* Grouped by origin rather than listed flat: a plugin driver and a
                driver probed on the host are different kinds of answer, and the
                hairline that separates two rows of the same kind is not enough
                to say where one kind ends. */}
            <div className="space-y-6">
              {originGroups.map(([originKey, group]) => (
                <section key={originKey}>
                  {/* One origin is the normal case, and a lone "Local host"
                      heading over the whole list says nothing the row subtitle
                      does not already say. */}
                  {originGroups.length > 1 && (
                    <h4 className="pb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                      {t(`snapshots.detection.origin.${originKey}`)}
                    </h4>
                  )}
                  {/* Spaced, not divided: ListItem is already a rounded glass
                      card, so a divider stacks the cards flush and the rows
                      read as one slab. */}
                  <div className="space-y-2">
                    {group.map((row) => (
                      <DriverRow key={`${row.origin}-${row.driver}`} row={row} onSetUp={onSetUp} />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Groups drivers by origin, keeping the server's order: a group appears where
 * its first driver did, and rows keep their order inside it. Sorting by origin
 * name instead would reshuffle a list the backend already orders by usefulness.
 */
function groupByOrigin(rows: SnapshotDriverStatus[]): [string, SnapshotDriverStatus[]][] {
  const groups = new Map<string, SnapshotDriverStatus[]>();
  for (const row of rows) {
    const existing = groups.get(row.origin);
    if (existing) {
      existing.push(row);
      continue;
    }
    groups.set(row.origin, [row]);
  }
  return [...groups.entries()];
}
