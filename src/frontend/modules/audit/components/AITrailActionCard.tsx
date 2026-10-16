// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Disclosure } from '@resources/components/ui/Disclosure';
import { formatDate } from '@resources/utils/format';
import type { AITrailGroup } from '../hooks/useAuditCenter';

const STEP_VARIANTS: Record<string, 'success' | 'warning' | 'destructive' | 'info' | 'secondary'> = {
  preview: 'info',
  approved: 'success',
  executed: 'success',
  denied: 'warning',
  expired: 'secondary',
  pending: 'secondary',
  failed: 'destructive',
  consent_failed: 'destructive',
  execute_failed: 'destructive',
};

const STEP_KINDS = new Set(Object.keys(STEP_VARIANTS));

function StepLabel({ kind }: { kind: string }) {
  const { t } = useTranslation('audit');
  const label = STEP_KINDS.has(kind) ? t(`ai.step.${kind}`) : kind;
  return <Badge variant={STEP_VARIANTS[kind] ?? 'secondary'}>{label}</Badge>;
}

function Payload({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <pre className="max-h-60 overflow-auto rounded-md border border-border bg-background/60 p-2 font-mono text-xs break-all whitespace-pre-wrap text-foreground">
        {value}
      </pre>
    </div>
  );
}

export function AITrailActionCard({ group }: { group: AITrailGroup }) {
  const { t } = useTranslation('audit');
  const hasPayload = Boolean(group.arguments || group.plan || group.result || group.error);

  return (
    <li className="space-y-3 rounded-md border border-border bg-background/40 px-4 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-sm text-foreground">{group.tool}</span>
        <StepLabel kind={group.status} />
        {group.username && (
          <span className="text-xs text-muted-foreground">{t('ai.by', { actor: group.username })}</span>
        )}
        <span className="ml-auto font-mono text-xs text-muted-foreground">{group.actionId}</span>
      </div>
      <ol className="flex flex-wrap items-center gap-2">
        {group.steps.map((step) => (
          <li
            key={step.auditId ?? `${step.kind}-${step.timestamp}`}
            className="flex items-center gap-1.5 text-xs text-muted-foreground"
          >
            <StepLabel kind={step.kind} />
            <span className="whitespace-nowrap">{formatDate(step.timestamp)}</span>
          </li>
        ))}
      </ol>
      {group.recordMissing && <p className="text-xs text-muted-foreground">{t('ai.recordMissing')}</p>}
      {hasPayload && (
        <Disclosure title={t('ai.payloads')} description={group.truncated ? t('ai.truncated') : undefined}>
          <div className="space-y-3">
            <Payload label={t('ai.arguments')} value={group.arguments} />
            <Payload label={t('ai.plan')} value={group.plan} />
            <Payload label={t('ai.result')} value={group.result} />
            <Payload label={t('ai.error')} value={group.error} />
          </div>
        </Disclosure>
      )}
    </li>
  );
}
