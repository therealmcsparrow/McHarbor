// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowUpRight, IconCheck, IconHandOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Textarea } from '@resources/components/ui/Textarea';
import { PERM } from '@core/auth/permissionKeys';
import { useFinishRun } from '@resources/hooks/useRunbooks';
import type { RunOutcome } from '@resources/types/runbooks';

type RunbookFinishProps = {
  runId: string;
  onFinished?: (outcome: RunOutcome) => void;
};

export function RunbookFinish({ runId, onFinished }: RunbookFinishProps) {
  const { t } = useTranslation('runbooks');
  const [note, setNote] = useState('');
  const finish = useFinishRun(runId);

  function closeFn(outcome: 'completed' | 'abandoned' | 'escalated') {
    finish.mutate({ outcome, note }, { onSuccess: () => onFinished?.(outcome) });
  }

  return (
    <section
      aria-labelledby="runbook-finish-title"
      className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4"
    >
      <h2 id="runbook-finish-title" className="text-base font-semibold text-foreground">
        {t('finish.title')}
      </h2>
      <p className="text-sm text-muted-foreground">{t('finish.description')}</p>
      <div>
        <label htmlFor="runbook-finish-note" className="mb-1 block text-xs font-medium text-muted-foreground">
          {t('finish.noteLabel')}
        </label>
        <Textarea
          id="runbook-finish-note"
          rows={3}
          value={note}
          placeholder={t('finish.notePlaceholder')}
          onChange={(evt) => setNote(evt.target.value)}
        />
      </div>
      {finish.isError && (
        <p
          role="alert"
          className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
        >
          {finish.error.message}
        </p>
      )}
      <div className="flex flex-col gap-2">
        <Button
          className="min-h-12 w-full"
          permission={PERM.RUNBOOKS_RUN}
          disabled={finish.isPending}
          onClick={() => closeFn('completed')}
        >
          <IconCheck className="size-4" aria-hidden="true" />
          {t('finish.complete')}
        </Button>
        <Button
          variant="caution"
          className="min-h-12 w-full"
          permission={PERM.RUNBOOKS_RUN}
          disabled={finish.isPending}
          onClick={() => closeFn('escalated')}
        >
          <IconArrowUpRight className="size-4" aria-hidden="true" />
          {t('finish.escalate')}
        </Button>
        <Button
          variant="destructive"
          className="min-h-12 w-full"
          permission={PERM.RUNBOOKS_RUN}
          disabled={finish.isPending}
          onClick={() => closeFn('abandoned')}
        >
          <IconHandOff className="size-4" aria-hidden="true" />
          {t('finish.abandon')}
        </Button>
      </div>
    </section>
  );
}
