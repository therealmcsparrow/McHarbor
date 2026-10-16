// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconAlertTriangle, IconCheck, IconCircle, IconClock, IconHelp } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { cn } from '@resources/utils/cn';
import type { ChannelTestStep } from '../hooks/useNotificationChannels';

const STATUS_ICON: Record<ChannelTestStep['status'], typeof IconCheck> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<ChannelTestStep['status'], string> = {
  pass: 'text-success',
  fail: 'text-destructive',
  warn: 'text-warning',
  skip: 'text-muted-foreground',
};

const STATUS_BG: Record<ChannelTestStep['status'], string> = {
  pass: 'bg-success/10 border-success/30',
  fail: 'bg-destructive/10 border-destructive/30',
  warn: 'bg-warning/10 border-warning/30',
  skip: 'bg-muted/30 border-border',
};

type ChannelTestReportProps = {
  overallStatus: string;
  duration?: string;
  steps: ChannelTestStep[];
};

/**
 * The per-step report a channel test produces: an overall verdict with the
 * round-trip time, then one row per step. Same layout, status colours and
 * step list as the storage and identity test dialogs.
 */
export function ChannelTestReport({ overallStatus, duration, steps }: ChannelTestReportProps) {
  const { t } = useTranslation(['settings', 'common']);

  const overallVariant: 'default' | 'destructive' | 'secondary' =
    overallStatus === 'pass' ? 'default' : overallStatus === 'fail' ? 'destructive' : 'secondary';

  const overallLabel: string =
    overallStatus === 'pass'
      ? t('communications.test.resultPass')
      : overallStatus === 'fail'
        ? t('communications.test.resultFail')
        : overallStatus === 'warn'
          ? t('communications.test.resultWarn')
          : overallStatus === 'running'
            ? t('communications.test.running')
            : t('communications.test.resultSkip');

  return (
    <div data-testid="channel-test-result" className="space-y-3">
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t('communications.test.overallStatus')}
          </span>
          <Badge variant={overallVariant}>{overallLabel}</Badge>
        </div>
        {duration && (
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <IconClock className="size-3.5" />
            {duration}
          </span>
        )}
      </div>
      {steps.length > 0 ? (
        <ul className="space-y-2" data-testid="channel-test-steps">
          {steps.map((step) => {
            const StepIcon = STATUS_ICON[step.status];
            return (
              <li
                key={step.name}
                className={cn('rounded-lg border p-3 text-sm', STATUS_BG[step.status])}
                data-testid={`channel-test-step-${step.name}`}
              >
                <div className="flex items-start gap-3">
                  <StepIcon className={cn('mt-0.5 size-4 shrink-0', STATUS_COLOR[step.status])} aria-hidden="true" />
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium text-foreground">
                        {t(`communications.test.steps.${step.name}`, {
                          // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: test step names are emitted by the backend and can gain steps without a frontend release
                          defaultValue: step.name,
                        })}
                      </span>
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <Badge
                          variant={
                            step.status === 'fail' ? 'destructive' : step.status === 'warn' ? 'secondary' : 'outline'
                          }
                        >
                          {t(`communications.test.status.${step.status}`)}
                        </Badge>
                        {step.latency && <span className="font-mono">{step.latency}</span>}
                      </span>
                    </div>
                    {step.detail && <p className="text-xs break-words text-muted-foreground">{step.detail}</p>}
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      ) : (
        <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
          <IconCircle className="size-4" aria-hidden="true" />
          {t('communications.test.noSteps')}
        </div>
      )}
    </div>
  );
}
