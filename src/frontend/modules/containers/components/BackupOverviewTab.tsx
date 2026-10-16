// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArchive, IconChartAreaLine, IconDatabaseExport, IconPercentage, IconRestore } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { Spinner } from '@resources/components/ui/Spinner';
import { splitBytes } from '@resources/utils/format';
import { BACKUP_ACTIVITY_DAYS, BACKUP_METRICS_WINDOW_DAYS, useBackupMetrics } from '../hooks/useBackupMetrics';
import type { ContainerBackupPlan, ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { EncryptionStatusPanel, RecentFailuresPanel, UpcomingRunsPanel } from './BackupOverviewPanels';
import type { BackupsTabId } from './backups-tab-groups';

type BackupOverviewTabProps = {
  runs: ContainerBackupRun[] | undefined;
  plans: ContainerBackupPlan[] | undefined;
  loading: boolean;
  onNavigate: (tab: BackupsTabId) => void;
};

/**
 * Backups → Overview: is my data protected, and did anything break?
 *
 * Every number is derived from the run and plan lists the page already
 * loads (see `useBackupMetrics`), so the tab adds no request of its own and
 * can never disagree with the table one click away.
 */
export function BackupOverviewTab({ runs, plans, loading, onNavigate }: BackupOverviewTabProps) {
  const { t, i18n } = useTranslation('containers');
  const metrics = useBackupMetrics(runs, plans, i18n.language);
  const written = splitBytes(metrics.writtenBytes);
  const average = splitBytes(metrics.averageBytes);
  const windowDays = BACKUP_METRICS_WINDOW_DAYS;

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  if (!metrics.hasData) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-16 text-center">
        <IconArchive className="size-10 text-muted-foreground" />
        <p className="mt-3 text-sm font-medium text-foreground">{t('backups.overview.emptyTitle')}</p>
        <p className="mt-1 max-w-md text-xs text-muted-foreground">{t('backups.overview.emptyBody')}</p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Encryption first: whether archives are protected at all frames how
          to read every number under it. */}
      <EncryptionStatusPanel onNavigate={onNavigate} />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          title={t('backups.overview.backupsTitle', { days: windowDays })}
          value={metrics.backups.total}
          icon={<IconArchive className="size-5" />}
          description={t('backups.overview.outcomeSummary', {
            succeeded: metrics.backups.succeeded,
            failed: metrics.backups.failed,
          })}
        />
        <StatCard
          title={t('backups.overview.successRateTitle')}
          value={metrics.successRate === null ? '—' : metrics.successRate}
          unit={metrics.successRate === null ? undefined : '%'}
          icon={<IconPercentage className="size-5" />}
          description={t('backups.overview.successRateHint', {
            finished: metrics.finishedBackups,
          })}
        />
        <StatCard
          title={t('backups.overview.restoresTitle', { days: windowDays })}
          value={metrics.restores.total}
          icon={<IconRestore className="size-5" />}
          description={t('backups.overview.outcomeSummary', {
            succeeded: metrics.restores.succeeded,
            failed: metrics.restores.failed,
          })}
        />
        <StatCard
          title={t('backups.overview.writtenTitle', { days: windowDays })}
          value={written.value}
          unit={written.unit}
          icon={<IconDatabaseExport className="size-5" />}
          description={t('backups.overview.writtenHint', {
            size: `${average.value} ${average.unit}`,
          })}
        />
      </div>

      <section className="mc-glass rounded-lg p-4">
        <header className="mb-1 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <IconChartAreaLine className="size-4 text-muted-foreground" />
            <h3 className="text-sm font-semibold text-foreground">{t('backups.overview.activityTitle')}</h3>
          </div>
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <span aria-hidden="true" className="size-2 rounded-full bg-primary" />
              {t('backups.overview.legendSucceeded')}
            </span>
            <span className="flex items-center gap-1.5">
              <span aria-hidden="true" className="size-2 rounded-full bg-destructive" />
              {t('backups.overview.legendFailed')}
            </span>
          </div>
        </header>
        <p className="mb-2 text-xs text-muted-foreground">
          {t('backups.overview.activityHint', { days: BACKUP_ACTIVITY_DAYS })}
        </p>
        <div className="flex h-48 flex-col">
          <StatsAreaChart
            data={metrics.activity}
            dataKey="succeeded"
            secondaryDataKey="failed"
            xAxisKey="label"
            color="var(--primary)"
            secondaryColor="var(--destructive)"
            label={t('backups.overview.legendSucceeded')}
            secondaryLabel={t('backups.overview.legendFailed')}
            formatValue={(value) => String(Math.round(value))}
          />
        </div>
      </section>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <UpcomingRunsPanel plans={metrics.upcoming} onNavigate={onNavigate} />
        <RecentFailuresPanel runs={metrics.failures} onNavigate={onNavigate} />
      </div>
    </div>
  );
}
