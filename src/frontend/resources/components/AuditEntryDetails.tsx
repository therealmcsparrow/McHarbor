// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';

export type AuditEntry = {
  id: string;
  userId: string | null;
  username: string | null;
  action: string;
  severity: AuditSeverity;
  entityType: string | null;
  entityId: string | null;
  entityName: string | null;
  details: string | null;
  stateBefore?: Record<string, unknown> | unknown[] | string | number | boolean | null;
  stateAfter?: Record<string, unknown> | unknown[] | string | number | boolean | null;
  changedKeys?: string[];
  ipAddress: string | null;
  environmentId: string | null;
  environmentName: string | null;
  apiKeyId?: string | null;
  apiKeyName?: string | null;
  timestamp: string;
  createdAt: string;
  updatedAt: string;
};

export type AuditSeverity = 'info' | 'success' | 'warning' | 'error';

export function DetailRow({ label, value }: { label: string; value: string | null | undefined }) {
  if (!value) return null;
  return (
    <div className="flex gap-3 py-1.5 text-sm">
      <span className="w-32 shrink-0 text-muted-foreground">{label}</span>
      <span className="font-mono break-all text-foreground">{value}</span>
    </div>
  );
}

type AuditEntryDetailsProps = {
  entry: AuditEntry;
  envName: (id: string | null, storedName?: string | null) => string;
};

export function AuditEntryDetails({ entry, envName }: AuditEntryDetailsProps) {
  const { t } = useTranslation('common');
  const hasDiff = Boolean(
    entry.changedKeys?.length || entry.stateBefore !== undefined || entry.stateAfter !== undefined,
  );

  return (
    <div className="space-y-4 px-4 py-3">
      <div className="divide-y divide-border">
        <DetailRow label={t('audit.columnTimestamp')} value={entry.timestamp} />
        <DetailRow label={t('audit.columnUser')} value={entry.username} />
        <DetailRow label={t('audit.columnApiKey')} value={entry.apiKeyName ?? entry.apiKeyId} />
        <DetailRow label={t('audit.columnAction')} value={entry.action} />
        <div className="flex gap-3 py-1.5 text-sm">
          <span className="w-32 shrink-0 text-muted-foreground">{t('audit.columnSeverity')}</span>
          <Badge variant={SEVERITY_VARIANTS[entry.severity] ?? 'secondary'}>
            {t(`audit.severity.${entry.severity}`)}
          </Badge>
        </div>
        <DetailRow label={t('audit.columnEntity')} value={entry.entityType} />
        <DetailRow label={t('audit.columnEntityId')} value={entry.entityId} />
        <DetailRow label={t('audit.columnEntityName')} value={entry.entityName} />
        <DetailRow label={t('audit.columnEnvironment')} value={envName(entry.environmentId, entry.environmentName)} />
        <DetailRow label={t('audit.columnIpAddress')} value={entry.ipAddress} />
        <DetailRow label={t('audit.columnDetails')} value={entry.details} />
      </div>
      {hasDiff && (
        <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
          <div>
            <div className="text-sm font-medium text-foreground">{t('audit.diffTitle')}</div>
            <div className="mt-1 text-xs text-muted-foreground">
              {entry.changedKeys?.length ? entry.changedKeys.join(', ') : t('audit.diffNoKeys')}
            </div>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <DiffPanel title={t('audit.stateBefore')} value={entry.stateBefore} />
            <DiffPanel title={t('audit.stateAfter')} value={entry.stateAfter} />
          </div>
        </div>
      )}
    </div>
  );
}

function DiffPanel({ title, value }: { title: string; value: unknown }) {
  return (
    <div className="min-w-0 rounded-md border border-border bg-background/60 p-2">
      <div className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</div>
      <pre className="max-h-72 overflow-auto text-xs break-words whitespace-pre-wrap text-foreground">
        {value === undefined ? '-' : JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

export const ACTION_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'default' | 'secondary'> = {
  create: 'success',
  update: 'warning',
  delete: 'destructive',
  start: 'success',
  stop: 'destructive',
  login: 'default',
  logout: 'secondary',
  assign: 'success',
  unassign: 'warning',
  revoke: 'destructive',
};

export const SEVERITY_VARIANTS: Record<AuditSeverity, 'success' | 'destructive' | 'warning' | 'default'> = {
  info: 'default',
  success: 'success',
  warning: 'warning',
  error: 'destructive',
};
