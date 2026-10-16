// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconAlertTriangle, IconCheck, IconCircle, IconClock, IconHelp } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { cn } from '@resources/utils/cn';
import type { AIProviderTestStep } from '../hooks/useAIProviderConnections';

const STATUS_ICON: Record<AIProviderTestStep['status'], typeof IconCheck> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<AIProviderTestStep['status'], string> = {
  pass: 'text-success',
  fail: 'text-destructive',
  warn: 'text-warning',
  skip: 'text-muted-foreground',
};

const STATUS_BG: Record<AIProviderTestStep['status'], string> = {
  pass: 'bg-success/10 border-success/30',
  fail: 'bg-destructive/10 border-destructive/30',
  warn: 'bg-warning/10 border-warning/30',
  skip: 'bg-muted/30 border-border',
};

type AIProviderTestReportProps = {
  overallStatus: string;
  duration?: string;
  steps: AIProviderTestStep[];
};

/** The per-step report an AI provider connection test produces. */
export function AIProviderTestReport({ overallStatus, duration, steps }: AIProviderTestReportProps) {
  const { t } = useTranslation(['settings', 'common']);

  const overallVariant: 'default' | 'destructive' | 'secondary' =
    overallStatus === 'pass' ? 'default' : overallStatus === 'fail' ? 'destructive' : 'secondary';

  const overallLabel: string =
    overallStatus === 'pass'
      ? t('aiProviders.test.resultPass')
      : overallStatus === 'fail'
        ? t('aiProviders.test.resultFail')
        : overallStatus === 'warn'
          ? t('aiProviders.test.resultWarn')
          : overallStatus === 'running'
            ? t('aiProviders.test.running')
            : t('aiProviders.test.resultSkip');

  return (
    <output data-testid="ai-test-result" className="block space-y-3" aria-live="polite">
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t('aiProviders.test.overallStatus')}
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
        <ul className="space-y-2" data-testid="ai-test-steps">
          {steps.map((step) => {
            const StepIcon = STATUS_ICON[step.status];
            return (
              <li
                key={step.name}
                className={cn('rounded-lg border p-3 text-sm', STATUS_BG[step.status])}
                data-testid={`ai-test-step-${step.name}`}
              >
                <div className="flex items-start gap-3">
                  <StepIcon className={cn('mt-0.5 size-4 shrink-0', STATUS_COLOR[step.status])} aria-hidden="true" />
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium text-foreground">
                        {t(`aiProviders.test.steps.${step.name}`, {
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
                          {t(`aiProviders.test.status.${step.status}`)}
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
          {t('aiProviders.test.noSteps')}
        </div>
      )}
    </output>
  );
}
