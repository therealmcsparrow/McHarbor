// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes, formatDateCustom } from '@resources/utils/format';
import { capacityTone } from '@resources/utils/capacity';
import type { CapacitySubject } from '@resources/hooks/useCapacity';

export function capacityKindKey(kind: CapacitySubject['subjectKind']) {
  return kind === 'storage_location' ? 'capacity.kind.storageLocation' : 'capacity.kind.rootfs';
}

type CapacitySentenceProps = {
  subject: CapacitySubject;
  compact?: boolean;
};

export function CapacitySentence({ subject, compact = false }: CapacitySentenceProps) {
  const { t } = useTranslation('host');
  const tone = capacityTone(subject.usedPercent);

  if (!subject.red) {
    return (
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">
          {t(`capacity.refusal.${subject.refusal || 'no_fit'}`, {
            required: subject.requiredDays,
            observed: subject.observedDays,
          })}
        </p>
        {!compact && (
          <p className="text-xs text-muted-foreground">
            {t('capacity.usedOfTotal', {
              percent: subject.usedPercent,
              used: formatBytes(subject.usedBytes),
              total: formatBytes(subject.totalBytes),
            })}
          </p>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <p className="text-sm font-medium text-foreground">
        {t('capacity.sentence', {
          subject: subject.subjectName || subject.subjectId,
          date: formatDateCustom(subject.red.at, { day: 'numeric', month: 'long' }),
          days: subject.red.days,
          ceiling: subject.red.percent,
        })}
      </p>
      {!compact && (
        <p className="text-xs text-muted-foreground">
          {t('capacity.evidence', {
            percent: subject.usedPercent,
            used: formatBytes(subject.usedBytes),
            total: formatBytes(subject.totalBytes),
            rate: subject.percentPerDay,
            r2: subject.r2,
            observed: subject.observedDays,
          })}
        </p>
      )}
      <Badge variant={tone === 'destructive' ? 'destructive' : tone === 'warning' ? 'warning' : 'info'}>
        {t('capacity.usedBadge', { percent: subject.usedPercent })}
      </Badge>
    </div>
  );
}
