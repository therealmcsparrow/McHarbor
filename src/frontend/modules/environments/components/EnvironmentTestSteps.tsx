// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconAlertTriangle, IconCheck, IconHelp } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { cn } from '@resources/utils/cn';
import type { EnvironmentTestResult, EnvironmentTestStep } from '../hooks/useEnvironments';

const STATUS_ICON: Record<EnvironmentTestStep['status'], typeof IconCheck> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<EnvironmentTestStep['status'], string> = {
  pass: 'text-success',
  fail: 'text-destructive',
  warn: 'text-warning',
  skip: 'text-muted-foreground',
};

const STATUS_BG: Record<EnvironmentTestStep['status'], string> = {
  pass: 'bg-success/10 border-success/30',
  fail: 'bg-destructive/10 border-destructive/30',
  warn: 'bg-warning/10 border-warning/30',
  skip: 'bg-muted/30 border-border',
};

// Map backend step names to a translated, user-facing label.
// Falls back to the raw step name when the i18n key is missing (which
// happens for new backend steps whose translations have not yet been
// added).
const STEP_LABEL_KEYS: Record<string, string> = {
  load: 'envs.test.steps.load',
  reset: 'envs.test.steps.reset',
  connect: 'envs.test.steps.connect',
  ping: 'envs.test.steps.ping',
  version: 'envs.test.steps.version',
  persist: 'envs.test.steps.persist',
};

export function EnvironmentTestSteps({ steps }: { steps: EnvironmentTestResult['steps'] }) {
  const { t } = useTranslation('environments');

  return (
    <ul className="space-y-2" data-testid="env-test-steps">
      {steps.map((step, index) => {
        const StepIcon = STATUS_ICON[step.status];
        const labelKey = STEP_LABEL_KEYS[step.name];
        const label = labelKey ? t(labelKey) : step.name;
        return (
          <li
            key={`${step.name}-${index}`}
            className={cn('rounded-lg border p-3 text-sm', STATUS_BG[step.status])}
            data-testid={`env-test-step-${step.name}`}
          >
            <div className="flex items-start gap-3">
              <StepIcon className={cn('mt-0.5 size-4 shrink-0', STATUS_COLOR[step.status])} aria-hidden="true" />
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-medium text-foreground">{label}</span>
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Badge
                      variant={
                        step.status === 'fail' ? 'destructive' : step.status === 'warn' ? 'secondary' : 'outline'
                      }
                    >
                      {t(`envs.test.status.${step.status}`)}
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
