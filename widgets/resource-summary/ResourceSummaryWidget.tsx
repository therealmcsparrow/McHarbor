// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBoxMultiple, IconPhoto, IconDeviceFloppy, IconNetwork } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { useDashboardStats } from '@modules/dashboard/hooks/useDashboardStats';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

type WidgetConfig = {
  titleKey: string;
  icon: typeof IconBoxMultiple;
  getValue: (s: ReturnType<typeof useDashboardStats>['data']) => number;
  getDescriptionKey?: string;
  getDescriptionParams?: (s: ReturnType<typeof useDashboardStats>['data']) => Record<string, string | number>;
};

const CONFIG: Record<string, WidgetConfig> = {
  containers: {
    titleKey: 'resourceSummaryWidget.containers',
    icon: IconBoxMultiple,
    getValue: (s) => s?.containers?.total ?? 0,
    getDescriptionKey: 'resourceSummaryWidget.runningStoppedDescription',
    getDescriptionParams: (s) => ({
      running: s?.containers?.running ?? 0,
      stopped: s?.containers?.stopped ?? 0,
    }),
  },
  images: {
    titleKey: 'resourceSummaryWidget.images',
    icon: IconPhoto,
    getValue: (s) => s?.images ?? 0,
  },
  volumes: {
    titleKey: 'resourceSummaryWidget.volumes',
    icon: IconDeviceFloppy,
    getValue: (s) => s?.volumes ?? 0,
  },
  networks: {
    titleKey: 'resourceSummaryWidget.networks',
    icon: IconNetwork,
    getValue: (s) => s?.networks ?? 0,
  },
};

export default function ResourceSummaryWidget({ typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: stats } = useDashboardStats();
  const cfg = CONFIG[typeId];
  if (!cfg) return null;
  const Icon = cfg.icon;

  const description = cfg.getDescriptionKey
    ? t(cfg.getDescriptionKey, cfg.getDescriptionParams?.(stats))
    : undefined;

  return (
    <StatCard
      title={t(cfg.titleKey)}
      value={cfg.getValue(stats)}
      description={description}
      icon={<Icon className="h-5 w-5" />}
      className="h-full border-0"
    />
  );
}
