// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import type { DirectTransferTestResult } from '../types';

type AgentDirectTransferTestDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  result: DirectTransferTestResult | null;
  requestError: string;
};

function ResultRow({
  label,
  value,
  mono,
}: {
  label: string;
  value?: string | number | boolean;
  mono?: boolean;
}) {
  if (value === undefined || value === '') return null;
  return (
    <div className="grid gap-1 rounded-lg border border-border/60 bg-background/40 px-3 py-2 sm:grid-cols-[10rem_1fr]">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <span className={cn('break-all text-sm text-foreground', mono && 'font-mono text-xs')}>
        {String(value)}
      </span>
    </div>
  );
}

export function AgentDirectTransferTestDialog({
  open,
  onOpenChange,
  pending,
  result,
  requestError,
}: AgentDirectTransferTestDialogProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const phaseLabel =
    result?.phase && t(`agent.directTransferTest.phases.${result.phase}`, result.phase);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('agent.directTransferTest.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('agent.directTransferTest.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          {pending ? (
            <div className="flex min-h-40 flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
              <Spinner />
              <span>{t('agent.directTransferTest.testing')}</span>
            </div>
          ) : requestError ? (
            <div className="rounded-lg border border-border bg-background/40 p-4 text-sm text-foreground">
              <Badge variant="destructive">{t('agent.directTransferTest.failed')}</Badge>
              <p className="mt-3 break-words text-muted-foreground">{requestError}</p>
            </div>
          ) : result ? (
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={result.success ? 'success' : 'destructive'}>
                  {result.success
                    ? t('agent.directTransferTest.success')
                    : t('agent.directTransferTest.failed')}
                </Badge>
                {phaseLabel && <Badge variant="outline">{phaseLabel}</Badge>}
              </div>

              <div className="grid gap-2">
                <ResultRow label={t('agent.directTransferTest.source')} value={result.sourceName} />
                <ResultRow
                  label={t('agent.directTransferTest.sourceVersion')}
                  value={result.sourceVersion}
                />
                <ResultRow label={t('agent.directTransferTest.target')} value={result.targetName} />
                <ResultRow
                  label={t('agent.directTransferTest.targetVersion')}
                  value={result.targetVersion}
                />
                <ResultRow
                  label={t('agent.directTransferTest.statusCode')}
                  value={result.statusCode}
                />
                <ResultRow
                  label={t('agent.directTransferTest.duration')}
                  value={t('agent.directTransferTest.durationValue', {
                    duration: result.durationMs,
                  })}
                />
                <ResultRow
                  label={t('agent.directTransferTest.targetTransferUrl')}
                  value={result.targetTransferUrl}
                  mono
                />
                <ResultRow
                  label={t('agent.directTransferTest.probeUrl')}
                  value={result.probeUrl}
                  mono
                />
                <ResultRow label={t('agent.directTransferTest.error')} value={result.error} />
              </div>

              {result.diagnostic && (
                <div className="space-y-2 pt-2">
                  <h4 className="text-sm font-medium text-foreground">
                    {t('agent.directTransferTest.diagnosticTitle')}
                  </h4>
                  <div className="grid gap-2">
                    <ResultRow
                      label={t('agent.directTransferTest.receiverExists')}
                      value={result.diagnostic.receiverExists ? tc('yes') : tc('no')}
                    />
                    <ResultRow
                      label={t('agent.directTransferTest.receiverExpired')}
                      value={result.diagnostic.receiverExpired ? tc('yes') : tc('no')}
                    />
                    <ResultRow
                      label={t('agent.directTransferTest.receiverKind')}
                      value={result.diagnostic.receiverKind}
                    />
                    <ResultRow
                      label={t('agent.directTransferTest.kindMatched')}
                      value={result.diagnostic.kindMatched ? tc('yes') : tc('no')}
                    />
                    <ResultRow
                      label={t('agent.directTransferTest.bearerPresent')}
                      value={result.diagnostic.bearerPresent ? tc('yes') : tc('no')}
                    />
                    <ResultRow
                      label={t('agent.directTransferTest.tokenMatched')}
                      value={result.diagnostic.tokenMatched ? tc('yes') : tc('no')}
                    />
                    <ResultRow
                      label={t('agent.directTransferTest.remoteAddr')}
                      value={result.diagnostic.remoteAddr}
                      mono
                    />
                  </div>
                </div>
              )}
            </div>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">{tc('actions.close')}</Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
