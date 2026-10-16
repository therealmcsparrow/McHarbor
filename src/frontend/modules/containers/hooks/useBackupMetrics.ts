// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNow } from './useNow';
import type { ContainerBackupPlan, ContainerBackupRun } from '@resources/hooks/useContainerBackups';

/** How far back the headline numbers look. */
export const BACKUP_METRICS_WINDOW_DAYS = 30;
/** How many daily buckets the activity chart draws. */
export const BACKUP_ACTIVITY_DAYS = 14;

const DAY_MS = 24 * 60 * 60 * 1000;

export type BackupActivityPoint = {
  /** Local calendar day, `YYYY-MM-DD`. */
  day: string;
  /** Short label for the x axis, in the active locale. */
  label: string;
  succeeded: number;
  failed: number;
};

export type BackupOutcome = {
  total: number;
  succeeded: number;
  failed: number;
  running: number;
};

export type BackupMetrics = {
  backups: BackupOutcome;
  restores: BackupOutcome;
  /** Percentage of *finished* backup runs that succeeded, or null when none finished. */
  successRate: number | null;
  finishedBackups: number;
  /** Bytes written by successful backup runs inside the window. */
  writtenBytes: number;
  /** Mean archive size of those runs, 0 when there were none. */
  averageBytes: number;
  /** Containers with at least one successful backup inside the window. */
  protectedContainers: number;
  activity: BackupActivityPoint[];
  /** Enabled plans with a future run, soonest first. */
  upcoming: ContainerBackupPlan[];
  /** Most recent failed runs inside the window, newest first. */
  failures: ContainerBackupRun[];
  /** False only when there is nothing at all to show — no runs, no plans. */
  hasData: boolean;
};

function localDay(date: Date): string {
  const month = `${date.getMonth() + 1}`.padStart(2, '0');
  const day = `${date.getDate()}`.padStart(2, '0');
  return `${date.getFullYear()}-${month}-${day}`;
}

function emptyOutcome(): BackupOutcome {
  return { total: 0, succeeded: 0, failed: 0, running: 0 };
}

function tally(outcome: BackupOutcome, runStatus: ContainerBackupRun['status']) {
  outcome.total += 1;
  if (runStatus === 'success') outcome.succeeded += 1;
  else if (runStatus === 'failure') outcome.failed += 1;
  else if (runStatus === 'running') outcome.running += 1;
}

/**
 * useBackupMetrics derives the Overview tab's numbers from the run and plan
 * lists the page already holds.
 *
 * Deliberately client-side: `/container-backups/runs` returns the 1000 most
 * recent runs per environment, which is more than the 30-day window needs,
 * and the Runs tab has the same data loaded anyway. A stats endpoint would
 * be a second source of truth for numbers a reader can check against the
 * table one click away.
 */
export function useBackupMetrics(
  runs: ContainerBackupRun[] | undefined,
  plans: ContainerBackupPlan[] | undefined,
  locale: string,
): BackupMetrics {
  const now = useNow();
  return useMemo(() => {
    const allRuns = runs ?? [];
    const allPlans = plans ?? [];
    const windowStart = now - BACKUP_METRICS_WINDOW_DAYS * DAY_MS;

    const backups = emptyOutcome();
    const restores = emptyOutcome();
    let writtenBytes = 0;
    let sizedRuns = 0;
    const protectedIds = new Set<string>();
    const failures: ContainerBackupRun[] = [];

    // Buckets for the chart: one per day, oldest first, zero-filled so a
    // quiet day is drawn as a gap in the line rather than skipped.
    const dayFormat = new Intl.DateTimeFormat(locale, {
      month: 'short',
      day: 'numeric',
    });
    const activity: BackupActivityPoint[] = [];
    const byDay = new Map<string, BackupActivityPoint>();
    for (let offset = BACKUP_ACTIVITY_DAYS - 1; offset >= 0; offset -= 1) {
      const date = new Date(now - offset * DAY_MS);
      const point: BackupActivityPoint = {
        day: localDay(date),
        label: dayFormat.format(date),
        succeeded: 0,
        failed: 0,
      };
      activity.push(point);
      byDay.set(point.day, point);
    }

    for (const run of allRuns) {
      const startedAt = Date.parse(run.startedAt);
      if (Number.isNaN(startedAt) || startedAt < windowStart) continue;

      if (run.operation === 'restore') {
        tally(restores, run.status);
      } else {
        tally(backups, run.status);
        if (run.status === 'success') {
          protectedIds.add(run.containerId);
          if (run.archiveSize > 0) {
            writtenBytes += run.archiveSize;
            sizedRuns += 1;
          }
        }
      }

      if (run.status === 'failure') failures.push(run);

      const point = byDay.get(localDay(new Date(startedAt)));
      if (point && run.operation === 'backup') {
        if (run.status === 'success') point.succeeded += 1;
        else if (run.status === 'failure') point.failed += 1;
      }
    }

    const finishedBackups = backups.succeeded + backups.failed;

    failures.sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt));

    const upcoming = allPlans
      .filter((plan) => plan.enabled && !!plan.nextRunAt)
      .sort((a, b) => Date.parse(a.nextRunAt!) - Date.parse(b.nextRunAt!));

    return {
      backups,
      restores,
      successRate: finishedBackups > 0 ? Math.round((backups.succeeded / finishedBackups) * 100) : null,
      finishedBackups,
      writtenBytes,
      averageBytes: sizedRuns > 0 ? Math.round(writtenBytes / sizedRuns) : 0,
      protectedContainers: protectedIds.size,
      activity,
      upcoming: upcoming.slice(0, 4),
      failures: failures.slice(0, 4),
      hasData: allRuns.length > 0 || allPlans.length > 0,
    };
  }, [runs, plans, locale, now]);
}
