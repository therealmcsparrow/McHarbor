// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The lifecycle log's own filter controls — the left half of TablePage's
// second row.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { DateRangeFilter, type DateRangeFilterValue } from '@resources/components/DateRangeFilter';
import type { LifecycleSeverity, LifecycleSubjectType } from '../hooks/useLifecycle';
import { useLifecycleSubjectTypes } from '../hooks/useLifecycleSubjectTypes';
import { SEVERITIES } from './lifecycle-filter-options';

type LifecycleFilterControlsProps = {
  subjectType: LifecycleSubjectType;
  onSubjectTypeChange: (value: LifecycleSubjectType) => void;
  severity: LifecycleSeverity | '';
  onSeverityChange: (value: LifecycleSeverity | '') => void;
  dateRange: DateRangeFilterValue;
  onDateRangeChange: (value: DateRangeFilterValue) => void;
};

export function LifecycleFilterControls({
  subjectType,
  onSubjectTypeChange,
  severity,
  onSeverityChange,
  dateRange,
  onDateRangeChange,
}: LifecycleFilterControlsProps) {
  const { t } = useTranslation('settings');
  const subjectOptions = useLifecycleSubjectTypes();

  return (
    <>
      <DateRangeFilter value={dateRange} onChange={onDateRangeChange} />
      <Select
        value={subjectType}
        onChange={(v) => onSubjectTypeChange(v as LifecycleSubjectType)}
        options={subjectOptions}
        ariaLabel={t('lifecycle.subjectTypeLabel')}
        className="w-44"
      />
      <Select
        value={severity}
        onChange={(v) => onSeverityChange(v as LifecycleSeverity | '')}
        options={SEVERITIES.map((opt) => ({ value: opt.value, label: t(opt.labelKey) }))}
        ariaLabel={t('lifecycle.severityLabel')}
        className="w-40"
      />
    </>
  );
}
