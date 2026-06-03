// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { cn } from '@resources/utils/cn';
import type { DirectTransferTestResult } from '../types';

type AgentDirectTransferTestResultProps = {
  result: DirectTransferTestResult;
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

function receiverDiagnosticKey(result: DirectTransferTestResult) {
  const expectedMarker = result.receiver?.agentMarker;
  const responderMarker = result.responderMarker || result.diagnostic?.responderMarker;
  if (expectedMarker && responderMarker && expectedMarker !== responderMarker) {
    return 'agent.directTransferTest.interpretation.wrongResponder';
  }
  if (!result.diagnostic) {
    return 'agent.directTransferTest.interpretation.noDiagnostic';
  }
  if (!result.diagnostic.receiverExists) {
    return 'agent.directTransferTest.interpretation.receiverMissing';
  }
  if (!result.diagnostic.bearerPresent) {
    return 'agent.directTransferTest.interpretation.bearerMissing';
  }
  if (!result.diagnostic.tokenMatched) {
    return 'agent.directTransferTest.interpretation.tokenMismatch';
  }
  return 'agent.directTransferTest.interpretation.genericFailure';
}

export function AgentDirectTransferTestResult({ result }: AgentDirectTransferTestResultProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const phaseLabel = result.phase && t(`agent.directTransferTest.phases.${result.phase}`, result.phase);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={result.success ? 'success' : 'destructive'}>
          {result.success ? t('agent.directTransferTest.success') : t('agent.directTransferTest.failed')}
        </Badge>
        {phaseLabel && <Badge variant="outline">{phaseLabel}</Badge>}
      </div>

      <div className="grid gap-2">
        <ResultRow label={t('agent.directTransferTest.source')} value={result.sourceName} />
        <ResultRow label={t('agent.directTransferTest.sourceVersion')} value={result.sourceVersion} />
        <ResultRow label={t('agent.directTransferTest.target')} value={result.targetName} />
        <ResultRow label={t('agent.directTransferTest.targetVersion')} value={result.targetVersion} />
        <ResultRow label={t('agent.directTransferTest.statusCode')} value={result.statusCode} />
        <ResultRow
          label={t('agent.directTransferTest.duration')}
          value={t('agent.directTransferTest.durationValue', { duration: result.durationMs })}
        />
        <ResultRow label={t('agent.directTransferTest.targetTransferUrl')} value={result.targetTransferUrl} mono />
        <ResultRow label={t('agent.directTransferTest.probeUrl')} value={result.probeUrl} mono />
        <ResultRow label={t('agent.directTransferTest.responderMarker')} value={result.responderMarker} mono />
        <ResultRow label={t('agent.directTransferTest.error')} value={result.error} />
      </div>

      {!result.success && (
        <div className="rounded-lg border border-border bg-background/40 p-3">
          <p className="text-xs font-medium text-muted-foreground">
            {t('agent.directTransferTest.interpretationTitle')}
          </p>
          <p className="mt-1 text-sm text-foreground">{t(receiverDiagnosticKey(result))}</p>
        </div>
      )}

      {result.receiver && (
        <div className="space-y-2 pt-2">
          <h4 className="text-sm font-medium text-foreground">
            {t('agent.directTransferTest.receiverTitle')}
          </h4>
          <div className="grid gap-2">
            <ResultRow label={t('agent.directTransferTest.receiverTransferId')} value={result.receiver.transferId} mono />
            <ResultRow label={t('agent.directTransferTest.receiverKind')} value={result.receiver.kind} />
            <ResultRow label={t('agent.directTransferTest.receiverExpiresAt')} value={result.receiver.expiresAt} mono />
            <ResultRow label={t('agent.directTransferTest.tokenFingerprint')} value={result.receiver.tokenFingerprint} mono />
            <ResultRow label={t('agent.directTransferTest.receiverAgentMarker')} value={result.receiver.agentMarker} mono />
          </div>
        </div>
      )}

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
            <ResultRow label={t('agent.directTransferTest.receiverKind')} value={result.diagnostic.receiverKind} />
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
            <ResultRow label={t('agent.directTransferTest.remoteAddr')} value={result.diagnostic.remoteAddr} mono />
            <ResultRow
              label={t('agent.directTransferTest.responderMarker')}
              value={result.diagnostic.responderMarker}
              mono
            />
          </div>
        </div>
      )}
    </div>
  );
}
