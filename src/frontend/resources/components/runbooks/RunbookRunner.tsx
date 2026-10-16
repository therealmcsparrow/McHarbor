// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { IconChevronLeft, IconChevronRight, IconPlayerPlay, IconSquareCheck } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { PERM } from '@core/auth/permissionKeys';
import { useOnline } from '@resources/hooks/useOnline';
import { useRecordStep } from '@resources/hooks/useRunbooks';
import { firstOpenStep, stepResultFor, type Runbook, type RunbookRun } from '@resources/types/runbooks';
import { RunbookActionDialog } from './RunbookActionDialog';
import { RunbookFinish } from './RunbookFinish';
import { RunbookStepCard } from './RunbookStepCard';

type RunbookRunnerProps = {
  runbook: Runbook;
  run: RunbookRun;
  onFinished?: () => void;
};

export function RunbookRunner({ runbook, run, onFinished }: RunbookRunnerProps) {
  const { t } = useTranslation('runbooks');
  const formatPrefs = useLocaleFormat();
  const online = useOnline();
  const steps = runbook.steps;
  const [index, setIndex] = useState(() => firstOpenStep(run, steps));
  const [finding, setFinding] = useState('');
  const [skipReason, setSkipReason] = useState('');
  const [confirming, setConfirming] = useState(false);
  const record = useRecordStep(run.id);
  const current = steps[index];

  const [findingIndex, setFindingIndex] = useState(index);
  if (findingIndex !== index) {
    setFindingIndex(index);
    setFinding('');
    setSkipReason('');
  }

  if (!current) return null;
  const step = current;
  const result = stepResultFor(run, step);
  const last = index >= steps.length - 1;

  function advance() {
    setIndex((at) => Math.min(at + 1, steps.length - 1));
  }

  function complete() {
    record.mutate({ stepId: step.id, state: 'done', finding }, { onSuccess: advance });
  }

  function skip() {
    record.mutate({ stepId: step.id, state: 'skipped', skipReason, finding }, { onSuccess: advance });
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground">
        {t('run.startedBy', { who: run.startedByName, at: formatDate(run.startedAt, formatPrefs) })}
      </p>

      {!online && (
        <output className="block rounded-md border border-warning/40 bg-warning/10 p-3 text-sm">
          {t('run.offlineReadOnly')}
        </output>
      )}

      <RunbookStepCard step={step} total={steps.length} result={result} finding={finding} onFindingChange={setFinding}>
        {record.isError && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
          >
            {record.error.message}
          </p>
        )}

        {step.kind === 'instruction' && (
          <Button
            className="min-h-12 w-full"
            permission={PERM.RUNBOOKS_RUN}
            disabled={!online || record.isPending}
            onClick={complete}
          >
            <IconSquareCheck className="size-4" aria-hidden="true" />
            {t('run.markDone')}
          </Button>
        )}

        {step.kind === 'diagnostic' && (
          <Button
            className="min-h-12 w-full"
            permission={step.requiredPermissions?.length ? step.requiredPermissions : undefined}
            disabled={!online || record.isPending}
            onClick={complete}
          >
            <IconPlayerPlay className="size-4" aria-hidden="true" />
            {t('run.runDiagnostic')}
          </Button>
        )}

        {step.kind === 'action' && (
          <>
            <Button
              variant="destructive"
              className="min-h-12 w-full"
              permission={step.requiredPermissions?.length ? step.requiredPermissions : undefined}
              disabled={!online || record.isPending}
              onClick={() => setConfirming(true)}
            >
              <IconPlayerPlay className="size-4" aria-hidden="true" />
              {t('run.runStep')}
            </Button>
            <RunbookActionDialog
              runId={run.id}
              step={step}
              finding={finding}
              open={confirming}
              onOpenChange={setConfirming}
              onSuccess={advance}
            />
          </>
        )}

        <Input
          aria-label={t('run.skipReasonLabel')}
          placeholder={t('run.skipReasonLabel')}
          value={skipReason}
          onChange={(evt) => setSkipReason(evt.target.value)}
        />
        <Button
          variant="outline"
          className="min-h-12 w-full"
          permission={PERM.RUNBOOKS_RUN}
          disabled={!online || record.isPending || skipReason.trim() === ''}
          onClick={skip}
        >
          {t('run.skipStep')}
        </Button>
      </RunbookStepCard>

      <nav aria-label={t('run.navigation')} className="flex items-center gap-2">
        <Button
          variant="ghost"
          className="min-h-12 flex-1"
          disabled={index === 0}
          onClick={() => setIndex((at) => Math.max(at - 1, 0))}
        >
          <IconChevronLeft className="size-4" aria-hidden="true" />
          {t('run.previous')}
        </Button>
        <Button variant="ghost" className="min-h-12 flex-1" disabled={last} onClick={advance}>
          {t('run.next')}
          <IconChevronRight className="size-4" aria-hidden="true" />
        </Button>
      </nav>

      {run.outcome === 'in_progress' && online && <RunbookFinish runId={run.id} onFinished={onFinished} />}
    </div>
  );
}
