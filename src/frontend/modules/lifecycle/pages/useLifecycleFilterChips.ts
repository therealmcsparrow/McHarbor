// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The active-filter chip row above the lifecycle table. Extracted from
// LifecycleLogPage.tsx unchanged: each chip clears its own filter and
// returns to page 1.

import { useMemo } from 'react';
import { resolveDateRange } from '@resources/utils/date-range';
import type { ActiveFilterChip } from '@resources/layout/table-page/types';
import { DEFAULT_DATE_RANGE_VALUE, type DateRangeFilterValue } from '@resources/components/DateRangeFilter';
import { DATE_RANGE_PRESETS } from '@resources/utils/date-range';
import { SEVERITIES } from '../components/lifecycle-filter-options';
import type { LifecycleSeverity, LifecycleSubjectType } from '../hooks/useLifecycle';
import { useLifecycleSubjectTypes } from '../hooks/useLifecycleSubjectTypes';

type FilterChipArgs = {
  subjectType: LifecycleSubjectType;
  setSubjectType: (value: LifecycleSubjectType) => void;
  severity: LifecycleSeverity | '';
  setSeverity: (value: LifecycleSeverity | '') => void;
  dateRange: DateRangeFilterValue;
  setDateRange: (value: DateRangeFilterValue) => void;
  setPage: (page: number) => void;
  t: (key: string) => string;
};

/** The resolved absolute window for the selected preset or custom range. */
export function useResolvedDateRange(dateRange: DateRangeFilterValue) {
  return useMemo(() => resolveDateRange(dateRange.preset, dateRange.custom), [dateRange]);
}

export function useLifecycleFilterChips({
  subjectType,
  setSubjectType,
  severity,
  setSeverity,
  dateRange,
  setDateRange,
  setPage,
  t,
}: FilterChipArgs): ActiveFilterChip[] {
  const subjectOptions = useLifecycleSubjectTypes();
  return useMemo<ActiveFilterChip[]>(() => {
    const list: ActiveFilterChip[] = [];
    if (subjectType) {
      list.push({
        id: 'subject',
        label: subjectOptions.find((o) => o.value === subjectType)?.label ?? subjectType,
        onRemove: () => {
          setSubjectType('');
          setPage(1);
        },
      });
    }
    if (severity) {
      list.push({
        id: 'severity',
        label: t(SEVERITIES.find((o) => o.value === severity)?.labelKey ?? severity),
        onRemove: () => {
          setSeverity('');
          setPage(1);
        },
      });
    }
    if (dateRange.preset !== DEFAULT_DATE_RANGE_VALUE.preset) {
      const preset = DATE_RANGE_PRESETS.find((entry) => entry.id === dateRange.preset);
      list.push({
        id: 'range',
        label: preset ? t(preset.i18nKey.replace(/^[^:]+:/, '')) : dateRange.preset,
        onRemove: () => {
          setDateRange(DEFAULT_DATE_RANGE_VALUE);
          setPage(1);
        },
      });
    }
    return list;
  }, [dateRange.preset, setDateRange, setPage, setSeverity, setSubjectType, severity, subjectOptions, subjectType, t]);
}
