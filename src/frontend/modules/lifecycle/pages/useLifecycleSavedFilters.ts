// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { DEFAULT_DATE_RANGE_VALUE, type DateRangeFilterValue } from '@resources/components/DateRangeFilter';
import { EMPTY_DATE_RANGE, type DateRangePresetId } from '@resources/utils/date-range';
import type { TablePageSavedFilters } from '@resources/layout/table-page/types';
import type { LifecycleSeverity, LifecycleSubjectType } from '../hooks/useLifecycle';

type SavedFiltersArgs = {
  envId: string;
  setEnvId: (value: string) => void;
  subjectType: LifecycleSubjectType;
  setSubjectType: (value: LifecycleSubjectType) => void;
  severity: LifecycleSeverity | '';
  setSeverity: (value: LifecycleSeverity | '') => void;
  search: string;
  setSearch: (value: string) => void;
  dateRange: DateRangeFilterValue;
  setDateRange: (value: DateRangeFilterValue) => void;
  setPage: (page: number) => void;
};

export function useLifecycleSavedFilters({
  envId,
  setEnvId,
  subjectType,
  setSubjectType,
  severity,
  setSeverity,
  search,
  setSearch,
  dateRange,
  setDateRange,
  setPage,
}: SavedFiltersArgs): TablePageSavedFilters {
  return useMemo(
    () => ({
      state: {
        envId,
        subjectType,
        severity,
        search,
        range: dateRange.preset === 'custom' ? undefined : dateRange.preset,
      },
      onRestore: (state: Record<string, unknown>) => {
        setEnvId(typeof state.envId === 'string' ? state.envId : '');
        setSubjectType((state.subjectType as LifecycleSubjectType) ?? '');
        setSeverity((state.severity as LifecycleSeverity | '') ?? '');
        setSearch(typeof state.search === 'string' ? state.search : '');
        if (typeof state.range === 'string' && state.range !== 'custom') {
          setDateRange({ preset: state.range as DateRangePresetId, custom: EMPTY_DATE_RANGE });
        } else {
          setDateRange(DEFAULT_DATE_RANGE_VALUE);
        }
        setPage(1);
      },
      pristine: !envId && !subjectType && !severity && !search,
      defaults: { envId: '', subjectType: '', severity: '', search: '', range: DEFAULT_DATE_RANGE_VALUE.preset },
    }),
    [
      envId,
      setEnvId,
      subjectType,
      setSubjectType,
      severity,
      setSeverity,
      search,
      setSearch,
      dateRange,
      setDateRange,
      setPage,
    ],
  );
}
