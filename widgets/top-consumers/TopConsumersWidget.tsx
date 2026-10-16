// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconChartBarOff } from '@tabler/icons-react';
import { useContainersBulkStats } from '@modules/containers/hooks/useContainers';
import { formatBytes } from '@resources/utils/format';
import { cn } from '@resources/utils/cn';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import {
  TONE_SOLID,
  WidgetBadge,
  WidgetEmpty,
  WidgetFrame,
  WidgetLoading,
  useFittedRows,
} from '@modules/dashboard/components/widget-kit';

/** Bars are shorter than list rows: name and value share one line. */
const BAR_ROW_HEIGHT = 34;

export default function TopConsumersWidget({ typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: statsMap, isLoading } = useContainersBulkStats();
  const isCpu = typeId === 'top-cpu-consumers';
  const { ref, capacity } = useFittedRows(BAR_ROW_HEIGHT);

  const rows = useMemo(() => {
    if (!statsMap) return [];
    return Array.from(statsMap.entries())
      .map(([id, m]) => ({
        id,
        name: m.name.replace(/^\//, ''),
        value: isCpu ? m.cpuPercent : m.memUsage,
      }))
      .filter((r) => r.value > 0)
      .sort((a, b) => b.value - a.value);
  }, [statsMap, isCpu]);

  const max = rows[0]?.value ?? 0;
  const total = rows.reduce((sum, r) => sum + r.value, 0);
  const shown = rows.slice(0, capacity);
  const format = (v: number) => (isCpu ? `${v.toFixed(1)}%` : formatBytes(v));
  const tone = isCpu ? 'info' : 'primary';

  return (
    <WidgetFrame
      title={isCpu ? t('topConsumersWidget.cpuTitle') : t('topConsumersWidget.memoryTitle')}
      badge={
        rows.length > 0 ? (
          <WidgetBadge>{t('topConsumersWidget.total', { value: format(total) })}</WidgetBadge>
        ) : undefined
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : rows.length === 0 ? (
        <WidgetEmpty
          icon={<IconChartBarOff className="size-5" aria-hidden="true" />}
          title={t('topConsumersWidget.noData')}
        />
      ) : (
        <div ref={ref} className="min-h-0 flex-1 overflow-hidden px-4 pb-2">
          <ul className="flex flex-col">
            {shown.map((r) => (
              <li key={r.id}>
                <PlainButton
                  type="button"
                  onClick={() => navigate(`/containers/${r.id}`)}
                  className="flex h-[34px] w-full flex-col justify-center gap-1 rounded-md px-1 text-left transition-colors hover:bg-glass-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  <span className="flex items-center justify-between gap-2 text-xs">
                    <span className="truncate text-foreground">{r.name}</span>
                    <span className="shrink-0 font-medium text-foreground tabular-nums">{format(r.value)}</span>
                  </span>
                  <span className="block h-1.5 w-full overflow-hidden rounded-full bg-glass-2" aria-hidden="true">
                    <span
                      className={cn('block h-full w-(--w) rounded-full', TONE_SOLID[tone])}
                      // Width is the reading itself; it travels as a CSS variable the class reads.
                      style={cssVars({ w: `${max > 0 ? Math.max(2, Math.round((r.value / max) * 100)) : 0}%` })}
                    />
                  </span>
                </PlainButton>
              </li>
            ))}
          </ul>
        </div>
      )}
    </WidgetFrame>
  );
}
