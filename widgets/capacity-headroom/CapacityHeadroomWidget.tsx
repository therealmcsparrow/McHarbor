// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconGauge } from '@tabler/icons-react';
import { useCapacity, type CapacitySubject } from '@resources/hooks/useCapacity';
import { capacityTone } from '@resources/utils/capacity';
import { formatDateCustom } from '@resources/utils/format';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { WidgetEmpty, WidgetFrame, WidgetLoading } from '@modules/dashboard/components/widget-kit';

function soonestFirst(a: CapacitySubject, b: CapacitySubject) {
  const left = a.red?.days ?? Number.POSITIVE_INFINITY;
  const right = b.red?.days ?? Number.POSITIVE_INFINITY;
  if (left !== right) return left - right;
  return b.usedPercent - a.usedPercent;
}

function toneTextClass(tone: ReturnType<typeof capacityTone>): string {
  if (tone === 'destructive') return 'text-destructive';
  if (tone === 'warning') return 'text-warning';
  return 'text-muted-foreground';
}

export default function CapacityHeadroomWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data, isLoading } = useCapacity();

  const subjects = [...(data?.subjects ?? [])].sort(soonestFirst).slice(0, 4);

  return (
    <WidgetFrame title={t('capacityHeadroomWidget.title')}>
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : subjects.length === 0 ? (
        <WidgetEmpty
          icon={<IconGauge className="size-5" aria-hidden="true" />}
          title={t('capacityHeadroomWidget.noData')}
        />
      ) : (
        <ul className="space-y-2">
          {subjects.map((subject) => {
            const tone = capacityTone(subject.usedPercent);
            const toneClass = toneTextClass(tone);
            return (
              <li key={`${subject.subjectKind}:${subject.subjectId}`} className="space-y-0.5">
                <p className="truncate text-xs text-muted-foreground">{subject.subjectName || subject.subjectId}</p>
                {subject.red ? (
                  <p className={`text-sm font-medium ${toneClass}`}>
                    {t('capacityHeadroomWidget.fullOn', {
                      date: formatDateCustom(subject.red.at, { day: 'numeric', month: 'short' }),
                      days: subject.red.days,
                    })}
                  </p>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    {t('capacityHeadroomWidget.noProjection', { percent: subject.usedPercent })}
                  </p>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </WidgetFrame>
  );
}
