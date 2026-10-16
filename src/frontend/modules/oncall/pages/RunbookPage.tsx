// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useParams, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { QueryState } from '../components/QueryState';
import { StaleNotice } from '@resources/components/StaleNotice';
import { useOnline } from '@resources/hooks/useOnline';
import { useResolvedRunbooks, useRunbookRun, useStartRun } from '@resources/hooks/useRunbooks';
import { RunbookRunner } from '@resources/components/runbooks/RunbookRunner';
import { stoppedAtPosition } from '@resources/types/runbooks';

export default function RunbookPage() {
  const { t } = useTranslation('runbooks');
  const { runbookId } = useParams();
  const [params] = useSearchParams();
  const online = useOnline();
  const canRead = usePermission(PERM.RUNBOOKS_VIEW);
  const [startedRunId, setStartedRunId] = useState<string | null>(null);

  const target = {
    environmentId: params.get('env') ?? undefined,
    container: params.get('container') ?? undefined,
    alertEventId: params.get('alertEventId') ?? undefined,
  };
  const resolved = useResolvedRunbooks(target, canRead);
  const runbook = resolved.runbooks.find((book) => book.id === runbookId) ?? null;
  const existing = resolved.openRuns.find((run) => run.runbookId === runbookId);
  const runId = startedRunId ?? params.get('run') ?? existing?.id ?? null;
  const run = useRunbookRun(runId ?? undefined);
  const start = useStartRun();

  function begin() {
    if (!runbook) return;
    start.mutate(
      {
        runbookId: runbook.id,
        environmentId: target.environmentId,
        subjectKind: target.container ? 'container' : runbook.attachKind,
        subjectName: target.container,
        alertEventId: target.alertEventId,
      },
      { onSuccess: (created) => setStartedRunId(created.id) },
    );
  }

  return (
    <section aria-labelledby="oncall-runbook-title">
      <h1 id="oncall-runbook-title" className="mb-3 text-lg font-semibold">
        {runbook?.name ?? t('page.title')}
      </h1>
      <StaleNotice cachedAt={resolved.cachedAt} />
      <QueryState
        isLoading={resolved.isLoading}
        error={resolved.error}
        onRetry={() => void resolved.refetch()}
        missingPermission={canRead ? null : PERM.RUNBOOKS_VIEW}
        isEmpty={!runbook}
        emptyText={t('page.notFound')}
      >
        {runbook && (
          <div className="flex flex-col gap-3">
            {runbook.description && <p className="text-sm text-muted-foreground">{runbook.description}</p>}

            {run.data && run.data.outcome !== 'in_progress' && (
              <p className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-card p-3 text-sm">
                <Badge variant="secondary">{t(`outcome.${run.data.outcome}`)}</Badge>
                {t('page.stoppedAt', { position: stoppedAtPosition(run.data), total: runbook.steps.length })}
              </p>
            )}

            {run.data && run.data.outcome === 'in_progress' ? (
              <RunbookRunner runbook={runbook} run={run.data} onFinished={() => void run.refetch()} />
            ) : (
              <>
                {!online && (
                  <output className="block rounded-md border border-warning/40 bg-warning/10 p-3 text-sm">
                    {t('run.offlineReadOnly')}
                  </output>
                )}
                {start.isError && (
                  <p
                    role="alert"
                    className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
                  >
                    {start.error.message}
                  </p>
                )}
                <ol className="flex flex-col gap-2">
                  {runbook.steps.map((step) => (
                    <li key={step.id} className="rounded-md border border-border bg-card p-3">
                      <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                        <Badge variant="outline">{step.position}</Badge>
                        {step.title}
                        <Badge variant="secondary">{t(`kind.${step.kind}`)}</Badge>
                      </p>
                      {step.body && <p className="mt-1 text-sm text-muted-foreground">{step.body}</p>}
                    </li>
                  ))}
                </ol>
                <Button
                  className="min-h-12 w-full"
                  permission={PERM.RUNBOOKS_RUN}
                  disabled={!online || start.isPending}
                  onClick={begin}
                >
                  {t('page.startRun')}
                </Button>
              </>
            )}
          </div>
        )}
      </QueryState>
    </section>
  );
}
