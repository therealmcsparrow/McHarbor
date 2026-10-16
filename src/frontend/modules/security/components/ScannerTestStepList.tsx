// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconAlertTriangle, IconCheck, IconCircle, IconHelp } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { cn } from '@resources/utils/cn';
import type { ScannerTestStep } from '../hooks/useScanners';

const STATUS_ICON: Record<ScannerTestStep['status'], typeof IconCheck> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<ScannerTestStep['status'], string> = {
  pass: 'text-success',
  fail: 'text-destructive',
  warn: 'text-warning',
  skip: 'text-muted-foreground',
};

const STATUS_BG: Record<ScannerTestStep['status'], string> = {
  pass: 'bg-success/10 border-success/30',
  fail: 'bg-destructive/10 border-destructive/30',
  warn: 'bg-warning/10 border-warning/30',
  skip: 'bg-muted/30 border-border',
};

type ScannerTestStepListProps = {
  steps: ScannerTestStep[];
};

/**
 * The per-step rows of a scanner test report. Step names are translated
 * by key with the raw name as fallback, so a third-party scanner plugin
 * that invents its own step names still renders something readable.
 */
export function ScannerTestStepList({ steps }: ScannerTestStepListProps) {
  const { t } = useTranslation('security');

  if (steps.length === 0) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
        <IconCircle className="size-4" aria-hidden="true" />
        {t('scanning.test.noSteps')}
      </div>
    );
  }

  return (
    <ul className="space-y-2" data-testid="scan-test-steps">
      {steps.map((step, index) => {
        const StepIcon = STATUS_ICON[step.status];
        return (
          <li
            key={`${step.name}-${index}`}
            className={cn('rounded-lg border p-3 text-sm', STATUS_BG[step.status])}
            data-testid={`scan-test-step-${step.name}`}
          >
            <div className="flex items-start gap-3">
              <StepIcon className={cn('mt-0.5 size-4 shrink-0', STATUS_COLOR[step.status])} aria-hidden="true" />
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-medium text-foreground">
                    {t(`scanning.test.steps.${step.name}`, {
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
                      {t(`scanning.test.status.${step.status}`)}
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
  );
}
