// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconAlertTriangle, IconCheck, IconCircleDashed, IconHelp } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { formatBytes } from '@resources/utils/format';
import type { CatalogTestStatus, CatalogTestStep, CatalogTestStepKey } from '../hooks/useCatalogTest';

// Step-card styling mirrors StorageLocationTestDialog so the two testers
// in Settings read as the same instrument. What differs here is that
// this one fills in live, which is why a `pending` state exists at all:
// the rows are on screen before their results are.
const STATUS_ICON: Record<Exclude<CatalogTestStatus, 'running'>, typeof IconCheck> = {
  pending: IconCircleDashed,
  ok: IconCheck,
  warn: IconAlertTriangle,
  fail: IconAlertCircle,
  skipped: IconHelp,
};

const STATUS_COLOR: Record<CatalogTestStatus, string> = {
  pending: 'text-muted-foreground',
  running: 'text-primary',
  ok: 'text-success',
  warn: 'text-warning',
  fail: 'text-destructive',
  skipped: 'text-muted-foreground',
};

const STATUS_BG: Record<CatalogTestStatus, string> = {
  pending: 'bg-muted/20 border-border',
  running: 'bg-primary/5 border-primary/30',
  ok: 'bg-success/10 border-success/30',
  warn: 'bg-warning/10 border-warning/30',
  fail: 'bg-destructive/10 border-destructive/30',
  skipped: 'bg-muted/30 border-border',
};

const STEP_BADGE: Record<CatalogTestStatus, 'outline' | 'secondary' | 'destructive'> = {
  pending: 'outline',
  running: 'outline',
  ok: 'outline',
  warn: 'secondary',
  fail: 'destructive',
  skipped: 'secondary',
};

// Byte counts cross the wire raw so the backend stays locale-agnostic;
// the human-readable form belongs on this side of it.
function formatDetailArgs(args?: Record<string, string | number>) {
  if (!args) return undefined;
  if (typeof args.bytes === 'number') {
    return { ...args, bytes: formatBytes(args.bytes) };
  }
  return args;
}

interface CatalogTestStepCardProps {
  stepKey: CatalogTestStepKey;
  step?: CatalogTestStep;
}

/** One row of the catalog test report. */
export function CatalogTestStepCard({ stepKey, step }: CatalogTestStepCardProps) {
  const { t } = useTranslation('settings');
  const stepStatus: CatalogTestStatus = step?.status ?? 'pending';
  const Icon = stepStatus === 'running' ? null : STATUS_ICON[stepStatus];

  return (
    <li className={cn('rounded-lg border p-3 text-sm', STATUS_BG[stepStatus])}>
      <div className="flex items-start gap-3">
        <span className="mt-0.5 flex size-4 shrink-0 items-center justify-center">
          {Icon ? (
            <Icon className={cn('size-4', STATUS_COLOR[stepStatus])} aria-hidden="true" />
          ) : (
            <Spinner size="sm" />
          )}
        </span>

        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex items-center justify-between gap-2">
            <span className={stepStatus === 'pending' ? 'text-muted-foreground' : 'font-medium text-foreground'}>
              {t(`catalogs.test.step.${stepKey}`)}
            </span>
            <span className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
              <Badge variant={STEP_BADGE[stepStatus]}>{t(`catalogs.test.status.${stepStatus}`)}</Badge>
              {step && step.duration_ms > 0 ? (
                <span className="font-mono">{t('catalogs.test.durationMs', { ms: step.duration_ms })}</span>
              ) : null}
            </span>
          </div>

          {step?.detail_key ? (
            <p className="text-xs break-words text-muted-foreground">
              {t(`catalogs.test.detail.${step.detail_key}`, formatDetailArgs(step.detail_args))}
            </p>
          ) : null}

          {step?.samples?.length ? (
            <p className="text-xs break-words text-muted-foreground">
              {t('catalogs.test.samples', { samples: step.samples.join(', ') })}
            </p>
          ) : null}

          {step?.problems?.length ? (
            <ul className="space-y-0.5">
              {step.problems.map((problem) => (
                <li key={`${problem.file}-${problem.reason_key}`} className="text-xs break-words text-warning">
                  {t(`catalogs.test.problem.${problem.reason_key}`, { file: problem.file })}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      </div>
    </li>
  );
}
