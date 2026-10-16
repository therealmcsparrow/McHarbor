// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import type { ContainerBackupInput } from '@resources/hooks/useContainerBackups';

type BackupSchedulingFieldsProps = {
  value: ContainerBackupInput;
  onChange: (patch: Partial<ContainerBackupInput>) => void;
};

export function BackupSchedulingFields({ value, onChange }: BackupSchedulingFieldsProps) {
  const { t } = useTranslation('containers');
  return (
    <>
      <ReadableScheduleField
        value={value.cron ?? ''}
        onChange={(cron) => onChange({ cron })}
        label={t('backups.scheduleReadable')}
        hint={t('backups.cronHint')}
      />
      <div>
        <Label className="mb-1 text-xs text-muted-foreground">{t('backups.logTailLines')}</Label>
        <NumberInput
          value={value.logTailLines ?? 10000}
          onChange={(logTailLines) => onChange({ logTailLines })}
          min={0}
          size="sm"
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('backups.logTailLinesHint')}</p>
      </div>
      <div className="rounded-lg border border-border bg-background p-3">
        <div className="mb-3">
          <h3 className="text-sm font-medium text-foreground">{t('backups.retentionTitle')}</h3>
          <p className="text-xs text-muted-foreground">{t('backups.retentionDescription')}</p>
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <Label className="mb-1 text-xs text-muted-foreground">{t('backups.retentionCount')}</Label>
            <NumberInput
              value={value.retentionCount}
              onChange={(retentionCount) => onChange({ retentionCount })}
              min={0}
              max={1000}
              size="sm"
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('backups.retentionCountHint')}</p>
          </div>
          <div>
            <Label className="mb-1 text-xs text-muted-foreground">{t('backups.retentionDays')}</Label>
            <NumberInput
              value={value.retentionDays}
              onChange={(retentionDays) => onChange({ retentionDays })}
              min={0}
              max={3650}
              size="sm"
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('backups.retentionDaysHint')}</p>
          </div>
        </div>
      </div>
    </>
  );
}
