// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import type { ContainerActivitySeverity, ContainerActivitySource } from '../hooks/useContainerActivity';

export type ActivityTimeRange = '1h' | '6h' | '24h' | '7d' | 'custom' | 'all';

type ContainerActivityFiltersProps = {
  source: ContainerActivitySource;
  severity: ContainerActivitySeverity | 'all';
  timeRange: ActivityTimeRange;
  customFrom: string;
  customTo: string;
  onSourceChange: (source: ContainerActivitySource) => void;
  onSeverityChange: (severity: ContainerActivitySeverity | 'all') => void;
  onTimeRangeChange: (range: ActivityTimeRange) => void;
  onCustomFromChange: (value: string) => void;
  onCustomToChange: (value: string) => void;
};

export function ContainerActivityFilters(props: ContainerActivityFiltersProps) {
  const { t } = useTranslation('containers');
  const sourceOptions = ['all', 'logs', 'history', 'changes', 'processes'].map((value) => ({
    value,
    label: t(`activity.source.${value}`),
  }));
  const severityOptions = ['all', 'debug', 'info', 'warn', 'error'].map((value) => ({
    value,
    label: t(`activity.severity.${value}`),
  }));
  const rangeOptions = ['1h', '6h', '24h', '7d', 'custom', 'all'].map((value) => ({
    value,
    label: t(`activity.time.${value}`),
  }));

  return (
    <div className="mc-glass grid gap-3 rounded-lg p-4 sm:grid-cols-3">
      <Select
        value={props.source}
        onChange={(value) => props.onSourceChange(value as ContainerActivitySource)}
        options={sourceOptions}
        searchable={false}
        ariaLabel={t('activity.sourceLabel')}
      />
      <Select
        value={props.severity}
        onChange={(value) => props.onSeverityChange(value as ContainerActivitySeverity | 'all')}
        options={severityOptions}
        searchable={false}
        ariaLabel={t('activity.severityLabel')}
      />
      <Select
        value={props.timeRange}
        onChange={(value) => props.onTimeRangeChange(value as ActivityTimeRange)}
        options={rangeOptions}
        searchable={false}
        ariaLabel={t('activity.timeLabel')}
      />
      {props.timeRange === 'custom' ? (
        <div className="grid gap-3 sm:col-span-3 sm:grid-cols-2">
          <Input
            type="datetime-local"
            value={props.customFrom}
            onChange={(changeEvent) => props.onCustomFromChange(changeEvent.target.value)}
            aria-label={t('activity.customFrom')}
            variant="outline"
          />
          <Input
            type="datetime-local"
            value={props.customTo}
            onChange={(evt) => props.onCustomToChange(evt.target.value)}
            aria-label={t('activity.customTo')}
            variant="outline"
          />
        </div>
      ) : null}
    </div>
  );
}
