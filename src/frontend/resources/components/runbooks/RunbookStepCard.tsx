// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { StatusError } from '@resources/components/ui/StatusError';
import { Textarea } from '@resources/components/ui/Textarea';
import type { RunbookStep, RunbookStepResult } from '@resources/types/runbooks';

type RunbookStepCardProps = {
  step: RunbookStep;
  total: number;
  result: RunbookStepResult | null;
  finding: string;
  onFindingChange: (value: string) => void;
  children: React.ReactNode;
};

function stateVariant(state: RunbookStepResult['state']): 'success' | 'secondary' | 'destructive' {
  if (state === 'failed') return 'destructive';
  if (state === 'skipped') return 'secondary';
  return 'success';
}

export function RunbookStepCard({ step, total, result, finding, onFindingChange, children }: RunbookStepCardProps) {
  const { t } = useTranslation('runbooks');
  const findingId = `runbook-finding-${step.id}`;

  return (
    <article className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
      <header className="flex flex-wrap items-center gap-2">
        <Badge variant="outline">{t('run.stepCounter', { position: step.position, total })}</Badge>
        <Badge variant="secondary">{t(`kind.${step.kind}`)}</Badge>
        {!step.required && <Badge variant="outline">{t('run.optional')}</Badge>}
        {result && (
          <span className="ml-auto flex items-center gap-1">
            <Badge variant={stateVariant(result.state)}>{t(`state.${result.state}`)}</Badge>
            <StatusError
              error={result.lastError}
              title={t('run.stepErrorTitle')}
              description={t('run.stepErrorDescription')}
            />
          </span>
        )}
      </header>

      <h2 className="text-base font-semibold text-foreground">{step.title}</h2>
      {step.body && <p className="text-sm break-words whitespace-pre-wrap text-muted-foreground">{step.body}</p>}

      {step.kind !== 'instruction' && step.requiredPermissions && step.requiredPermissions.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('run.stepNeeds', { permission: step.requiredPermissions.join(', ') })}
        </p>
      )}

      {result?.output && (
        <section aria-label={t('run.output')}>
          <h3 className="mb-1 text-xs font-medium text-muted-foreground">{t('run.output')}</h3>
          <pre className="max-h-64 overflow-auto rounded-md border border-border bg-muted p-3 text-xs break-words whitespace-pre-wrap text-foreground">
            {result.output}
          </pre>
        </section>
      )}

      {result?.skipReason && (
        <p className="text-sm text-muted-foreground">{t('run.skippedBecause', { reason: result.skipReason })}</p>
      )}

      <div>
        <label htmlFor={findingId} className="mb-1 block text-xs font-medium text-muted-foreground">
          {t('run.findingLabel')}
        </label>
        <Textarea
          id={findingId}
          rows={2}
          value={finding}
          placeholder={t('run.findingPlaceholder')}
          onChange={(evt) => onFindingChange(evt.target.value)}
        />
      </div>

      <div className="flex flex-col gap-2">{children}</div>
    </article>
  );
}
