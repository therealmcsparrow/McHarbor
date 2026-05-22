// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { formatDate } from '@resources/utils/format';

export type AuditEntry = {
  id: string;
  userId: string | null;
  username: string | null;
  action: string;
  entityType: string | null;
  entityId: string | null;
  entityName: string | null;
  details: string | null;
  ipAddress: string | null;
  environmentId: string | null;
  timestamp: string;
  createdAt: string;
  updatedAt: string;
};

export function DetailRow({ label, value }: { label: string; value: string | null | undefined }) {
  if (!value) return null;
  return (
    <div className="flex gap-3 py-1.5 text-sm">
      <span className="w-32 shrink-0 text-muted-foreground">{label}</span>
      <span className="break-all font-mono text-foreground">{value}</span>
    </div>
  );
}

type AuditEntryDetailsProps = {
  entry: AuditEntry;
  envName: (id: string | null) => string;
};

export function AuditEntryDetails({ entry, envName }: AuditEntryDetailsProps) {
  const { t } = useTranslation('common');

  return (
    <div className="px-4 py-3 space-y-4">
      <div className="divide-y divide-border">
        <DetailRow label={t('audit.columnTimestamp')} value={formatDate(entry.timestamp)} />
        <DetailRow label={t('audit.columnUser')} value={entry.username} />
        <DetailRow label={t('audit.columnAction')} value={entry.action} />
        <DetailRow label={t('audit.columnEntity')} value={entry.entityType} />
        <DetailRow label="ID" value={entry.entityId} />
        <DetailRow label={t('audit.columnEntity') + ' Name'} value={entry.entityName} />
        <DetailRow label={t('audit.columnEnvironment')} value={envName(entry.environmentId)} />
        <DetailRow label={t('audit.columnIpAddress')} value={entry.ipAddress} />
        <DetailRow label={t('audit.columnDetails')} value={entry.details} />
      </div>
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

