// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconChartDonut } from '@tabler/icons-react';
import { useDashboardStats } from '@modules/dashboard/hooks/useDashboardStats';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { WidgetEmpty, WidgetFrame, WidgetLoading, type Tone } from '@modules/dashboard/components/widget-kit';
import { DonutPanel } from '@modules/dashboard/components/widget-donut';

const SEGMENTS: { key: string; tone: Tone; to: string }[] = [
  { key: 'running', tone: 'success', to: '/containers?state=running' },
  { key: 'stopped', tone: 'danger', to: '/containers?state=exited' },
  { key: 'images', tone: 'info', to: '/images' },
  { key: 'volumes', tone: 'primary', to: '/volumes' },
  { key: 'networks', tone: 'warning', to: '/networks' },
];

export default function ResourceDonutWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: stats } = useDashboardStats();

  const values: Record<string, number> = {
    running: stats?.containers?.running ?? 0,
    stopped: stats?.containers?.stopped ?? 0,
    images: stats?.images ?? 0,
    volumes: stats?.volumes ?? 0,
    networks: stats?.networks ?? 0,
  };

  const segments = SEGMENTS.map((s) => ({
    key: s.key,
    label: t(`resourceDonutWidget.${s.key}`),
    value: values[s.key] ?? 0,
    tone: s.tone,
    onClick: () => navigate(s.to),
  }));
  const total = segments.reduce((sum, s) => sum + s.value, 0);

  return (
    <WidgetFrame title={t('resourceDonutWidget.title')}>
      {!stats ? (
        <WidgetLoading label={t('loading')} />
      ) : total === 0 ? (
        <WidgetEmpty
          icon={<IconChartDonut className="size-5" aria-hidden="true" />}
          title={t('resourceDonutWidget.noResources')}
        />
      ) : (
        <DonutPanel segments={segments} centerValue={total} centerLabel={t('resourceDonutWidget.total')} />
      )}
    </WidgetFrame>
  );
}
