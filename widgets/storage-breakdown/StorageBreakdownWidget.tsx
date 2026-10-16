// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabaseOff } from '@tabler/icons-react';
import { useHostMetrics } from '@modules/dashboard/hooks/useHostMetrics';
import { formatBytes } from '@resources/utils/format';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { WidgetEmpty, WidgetFrame, WidgetLoading, type Tone } from '@modules/dashboard/components/widget-kit';
import { DonutPanel } from '@modules/dashboard/components/widget-donut';

const SEGMENTS: { key: 'images' | 'volumes' | 'containers' | 'buildCache'; tone: Tone }[] = [
  { key: 'images', tone: 'info' },
  { key: 'volumes', tone: 'primary' },
  { key: 'containers', tone: 'success' },
  { key: 'buildCache', tone: 'warning' },
];

export default function StorageBreakdownWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: metrics } = useHostMetrics();

  const sizes: Record<string, number> = {
    images: metrics?.disk.imagesSize ?? 0,
    volumes: metrics?.disk.volumesSize ?? 0,
    containers: metrics?.disk.containersSize ?? 0,
    buildCache: metrics?.disk.buildCacheSize ?? 0,
  };
  const segments = SEGMENTS.map((s) => ({
    key: s.key,
    label: t(`storageBreakdownWidget.${s.key}`),
    value: sizes[s.key] ?? 0,
    display: formatBytes(sizes[s.key] ?? 0),
    tone: s.tone,
  }));
  const total = segments.reduce((sum, s) => sum + s.value, 0);

  return (
    <WidgetFrame title={t('storageBreakdownWidget.title')}>
      {!metrics ? (
        <WidgetLoading label={t('loading')} />
      ) : total === 0 ? (
        <WidgetEmpty
          icon={<IconDatabaseOff className="size-5" aria-hidden="true" />}
          title={t('storageBreakdownWidget.noData')}
        />
      ) : (
        <DonutPanel
          segments={segments}
          centerValue={formatBytes(total)}
          centerLabel={t('storageBreakdownWidget.total')}
        />
      )}
    </WidgetFrame>
  );
}
