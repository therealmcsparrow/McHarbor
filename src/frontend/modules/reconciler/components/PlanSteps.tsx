// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { StatusError } from '@resources/components/ui/StatusError';
import type { PlanStep, StepResult } from '../hooks/useProposals';

type PlanStepsProps = {
  steps: PlanStep[];
  results?: StepResult[];
};

const RESULT_VARIANT = { done: 'success', failed: 'destructive', skipped: 'secondary', manual: 'secondary' } as const;

export function PlanSteps({ steps, results = [] }: PlanStepsProps) {
  const { t } = useTranslation('reconciler');
  if (steps.length === 0) return <p className="text-sm text-muted-foreground">{t('proposals.noSteps')}</p>;
  return (
    <ol className="space-y-2">
      {steps.map((step, i) => {
        const result = results[i];
        return (
          <li
            key={`${step.action}|${step.target}|${(step.fields ?? []).join(',')}`}
            className="rounded-md border border-border p-2 text-sm"
          >
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={step.action === 'manual' ? 'secondary' : 'default'}>{t(`step.${step.action}`)}</Badge>
              {step.pluginAction && <span className="text-xs">{step.pluginAction.replace(/_/g, ' ')}</span>}
              <span className="font-mono text-xs">{step.target}</span>
              {step.disruptive && <Badge variant="warning">{t('step.disruptive')}</Badge>}
              {result && (
                <span className="ml-auto flex items-center gap-1">
                  <Badge variant={RESULT_VARIANT[result.status]}>{t(`stepResult.${result.status}`)}</Badge>
                  <StatusError error={result.error} title={t('proposals.stepError')} />
                </span>
              )}
            </div>
            {step.reason && <p className="mt-1 text-xs text-muted-foreground">{t(`manual.${step.reason}`)}</p>}
            {step.detail && <p className="mt-1 text-xs text-muted-foreground">{step.detail}</p>}
            {step.fields && step.fields.length > 0 && (
              <p className="mt-1 font-mono text-xs break-all text-muted-foreground">{step.fields.join(', ')}</p>
            )}
          </li>
        );
      })}
    </ol>
  );
}
