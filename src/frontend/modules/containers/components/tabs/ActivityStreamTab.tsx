// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useDeferredValue, useMemo, useState } from 'react';
import { useNow } from '../../hooks/useNow';
import { useTranslation } from 'react-i18next';
import { IconActivity } from '@tabler/icons-react';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { ContainerActivityFilters, type ActivityTimeRange } from '../ContainerActivityFilters';
import { ContainerActivityList } from '../ContainerActivityList';
import {
  useContainerActivity,
  type ContainerActivitySeverity,
  type ContainerActivitySource,
} from '../../hooks/useContainerActivity';

type ActivityStreamTabProps = {
  containerId: string;
  containerImage: string;
  isRunning: boolean;
};

const RANGE_MILLISECONDS: Partial<Record<ActivityTimeRange, number>> = {
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
};

export function ActivityStreamTab(props: ActivityStreamTabProps) {
  const { t } = useTranslation('containers');
  const [source, setSource] = useState<ContainerActivitySource>('all');
  const [severity, setSeverity] = useState<ContainerActivitySeverity | 'all'>('all');
  const [timeRange, setTimeRange] = useState<ActivityTimeRange>('24h');
  const [customFrom, setCustomFrom] = useState('');
  const [customTo, setCustomTo] = useState('');
  const now = useNow();
  const deferredSource = useDeferredValue(source);
  const activity = useContainerActivity(props.containerId, props.containerImage, props.isRunning, deferredSource);

  const handleSourceChange = (nextSource: ContainerActivitySource) => {
    setSource(nextSource);
    if (nextSource === 'history' && timeRange !== 'all') {
      setTimeRange('all');
    }
  };

  const filteredItems = useMemo(() => {
    const from =
      timeRange === 'custom'
        ? Date.parse(customFrom)
        : RANGE_MILLISECONDS[timeRange]
          ? now - (RANGE_MILLISECONDS[timeRange] ?? 0)
          : Number.NEGATIVE_INFINITY;
    const to = timeRange === 'custom' && customTo ? Date.parse(customTo) : Number.POSITIVE_INFINITY;
    return activity.items.filter((item) => {
      const timestamp = Date.parse(item.timestamp);
      return (
        (severity === 'all' || item.severity === severity) &&
        (Number.isNaN(from) || timestamp >= from) &&
        (Number.isNaN(to) || timestamp <= to)
      );
    });
  }, [activity.items, customFrom, customTo, now, severity, timeRange]);

  return (
    <div className="space-y-4">
      <ContainerActivityFilters
        source={source}
        severity={severity}
        timeRange={timeRange}
        customFrom={customFrom}
        customTo={customTo}
        onSourceChange={handleSourceChange}
        onSeverityChange={setSeverity}
        onTimeRangeChange={setTimeRange}
        onCustomFromChange={setCustomFrom}
        onCustomToChange={setCustomTo}
      />
      {activity.error ? (
        <ErrorState error={activity.error} onRetry={() => activity.refetch()} />
      ) : activity.isLoading ? (
        <div className="flex h-64 items-center justify-center">
          <Spinner size="md" />
        </div>
      ) : filteredItems.length === 0 ? (
        <EmptyState
          icon={<IconActivity className="size-5" />}
          title={t('activity.emptyTitle')}
          description={t('activity.emptyDescription')}
        />
      ) : (
        <ContainerActivityList items={filteredItems.slice(0, 500)} />
      )}
    </div>
  );
}
