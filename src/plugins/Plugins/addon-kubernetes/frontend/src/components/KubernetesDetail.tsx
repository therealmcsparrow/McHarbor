// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// KubernetesDetail — a pod or a deployment, read the way the app reads every
// other resource.
//
// It used to be a "Back to pods" button, one card of `InfoRow`s, and then the
// containers and conditions arrays dumped through `JSON.stringify(…, null, 2)`
// into a `<pre>`. Nothing else in the product shows a user raw JSON as its
// primary presentation: a container's mounts, a network's IPAM entries and a
// stack's services are all tables. The shapes here are perfectly regular —
// `ContainerInfo` is name/image/state/ready/restartCount and
// `ConditionSummary` is type/status/message, both fixed in the plugin's
// `model.go` — so there was never anything a table could not carry.
//
// The frame is the container/network detail frame: hero card with the tab rail
// inside it, then one concern at a time in the panel below.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBox, IconInfoCircle, IconListCheck } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { list } from '../format';
import type { Row } from '../types';
import type { ActionsKind } from './KubernetesActionsCell';
import { ConditionsPanel, ContainersPanel } from './KubernetesDetailPanels';
import { badgeVariant } from './kubernetes-facets';
import { KubernetesDetailHeader, type KubernetesDetailActions } from './KubernetesDetailHeader';

type DetailTabId = 'overview' | 'containers' | 'conditions';

const TAB_ICONS: Record<DetailTabId, SubNavItem<DetailTabId>['icon']> = {
  overview: IconInfoCircle,
  containers: IconBox,
  conditions: IconListCheck,
};

/** Values worth a badge rather than a bare word, matching the table cells. */
const BADGED_FIELDS = new Set(['status', 'type']);

type KubernetesDetailProps = {
  kind: ActionsKind;
  namespace: string;
  name: string;
  data?: Row;
  loading: boolean;
  actions: KubernetesDetailActions;
};

export function KubernetesDetail({ kind, namespace, name: itemName, data, loading, actions }: KubernetesDetailProps) {
  const { t } = useTranslation('kubernetes');
  const [activeTab, setActiveTab] = useState<DetailTabId>('overview');

  const containers = Array.isArray(data?.containers) ? (data.containers as Row[]) : [];
  const conditions = Array.isArray(data?.conditions) ? (data.conditions as Row[]) : [];

  // A deployment reports no containers, so it gets no Containers tab rather
  // than an empty one — a rail entry that is always empty teaches the reader
  // to distrust the rail.
  const tabs = useMemo<SubNavItem<DetailTabId>[]>(() => {
    const items: SubNavItem<DetailTabId>[] = [
      { id: 'overview', label: t('detail.tabs.overview'), icon: TAB_ICONS.overview },
    ];
    if (kind === 'pods') {
      items.push({
        id: 'containers',
        label: t('detail.tabs.containers'),
        icon: TAB_ICONS.containers,
        adornment: containers.length > 0 ? <CountBadge count={containers.length} /> : undefined,
      });
    }
    items.push({
      id: 'conditions',
      label: t('detail.tabs.conditions'),
      icon: TAB_ICONS.conditions,
      adornment: conditions.length > 0 ? <CountBadge count={conditions.length} /> : undefined,
    });
    return items;
  }, [conditions.length, containers.length, kind, t]);

  if (loading) {
    return (
      <div className="space-y-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <div className="grid gap-4 lg:grid-cols-2">
          <CardSkeleton lines={6} />
          <CardSkeleton lines={6} />
        </div>
      </div>
    );
  }

  if (!data) {
    return <ErrorState variant="card" title={t('errors.detailUnavailable')} description={t('errors.detailMessage')} />;
  }

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <Card className="flex-none overflow-visible">
        <div className="px-5 pt-4 pb-3">
          <KubernetesDetailHeader kind={kind} name={itemName} namespace={namespace} data={data} actions={actions} />
        </div>
        <div className="border-t border-border bg-transparent px-3 py-2">
          <SubNav
            items={tabs}
            value={activeTab}
            onChange={setActiveTab}
            ariaLabel={t('detail.tabs.ariaLabel')}
            idPrefix="kubernetes-detail-tab"
            controls="kubernetes-detail-panel"
          />
        </div>
      </Card>

      <div
        id="kubernetes-detail-panel"
        role="tabpanel"
        aria-labelledby={`kubernetes-detail-tab-${activeTab}`}
        className="flex min-h-0 flex-col"
      >
        {activeTab === 'overview' && <OverviewPanel data={data} />}
        {activeTab === 'containers' && <ContainersPanel rows={containers} />}
        {activeTab === 'conditions' && <ConditionsPanel rows={conditions} />}
      </div>
    </div>
  );
}

/** Column keys are whatever fields the API row carries, so an unknown one shows as its own name. */
function columnLabel(t: (key: string, options?: Record<string, unknown>) => string, key: string): string {
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: keys are the API row's own field names
  return t(`columns.${key}`, { defaultValue: key });
}

function CountBadge({ count }: { count: number }) {
  return (
    <Badge variant="secondary" className="ml-0.5 px-1.5 py-0 text-[10px]">
      {count}
    </Badge>
  );
}

/** Every scalar the API returned, stated once. Arrays own their own tab. */
function OverviewPanel({ data }: { data: Row }) {
  const { t } = useTranslation('kubernetes');
  const scalars = Object.entries(data).filter(([, value]) => !Array.isArray(value));

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t('sections.details')}</CardTitle>
      </CardHeader>
      <CardContent className="px-4 py-1 md:px-5">
        {scalars.map(([key, value]) => (
          <InfoRow key={key} label={columnLabel(t, key)} variant="spread">
            {BADGED_FIELDS.has(key) ? <Badge variant={badgeVariant(String(value))}>{list(value)}</Badge> : list(value)}
          </InfoRow>
        ))}
      </CardContent>
    </Card>
  );
}
