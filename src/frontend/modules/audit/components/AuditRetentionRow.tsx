// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { SEVERITY_VARIANTS } from '@resources/components/AuditEntryDetails';
import type { AuditSeverity } from '../hooks/useAuditCenter';

export type RetentionDraftRow = { inherit: boolean; days: number };

type AuditRetentionRowProps = {
  severity: AuditSeverity;
  row: RetentionDraftRow;
  defaultDays: number;
  maxDays: number;
  disabled: boolean;
  onChange: (next: RetentionDraftRow) => void;
};

export function AuditRetentionRow({ severity, row, defaultDays, maxDays, disabled, onChange }: AuditRetentionRowProps) {
  const { t } = useTranslation('audit');
  const severityLabel = t(`audit.severity.${severity}`, { ns: 'common' });
  const effective = row.inherit ? defaultDays : row.days;
  const inheritId = `audit-retention-inherit-${severity}`;

  return (
    <div className="flex flex-wrap items-center gap-4 rounded-md border border-border bg-background/40 px-4 py-3">
      <Badge variant={SEVERITY_VARIANTS[severity] ?? 'secondary'} className="w-20 justify-center">
        {severityLabel}
      </Badge>
      <div className="flex items-center gap-2">
        <Switch
          id={inheritId}
          checked={row.inherit}
          disabled={disabled}
          onCheckedChange={(checked) => onChange({ inherit: checked, days: row.days })}
        />
        <label htmlFor={inheritId} className="text-sm text-foreground">
          {t('retention.useDefault')}
        </label>
      </div>
      <NumberInput
        size="sm"
        className="w-32"
        value={row.inherit ? defaultDays : row.days}
        min={0}
        max={maxDays}
        disabled={disabled || row.inherit}
        aria-label={t('retention.daysLabel', { severity: severityLabel })}
        onChange={(days) => onChange({ inherit: false, days })}
      />
      <span className="ml-auto text-xs text-muted-foreground">
        {effective === 0 ? t('retention.forever') : t('retention.days', { count: effective })}
      </span>
    </div>
  );
}
