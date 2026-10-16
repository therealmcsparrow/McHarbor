// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// KubernetesOverview — the cluster's two document-shaped tabs, laid out the
// way `modules/containers/components/tabs/OverviewTab.tsx` lays out a
// container.
//
// The container overview answers three questions in a fixed order, and the
// order is the design:
//
//   1. What should I do next?  → the `overview.suggested` action band
//   2. What is wrong?          → `ContainerInsightsCard`
//   3. What is this, exactly?  → a grid of `rounded-lg mc-glass p-6` panels,
//                                each an uppercase caption over `InfoRow`s
//
// The cluster overview used to answer only the third, and answered it with a
// `Card`/`CardHeader`/`CardTitle` box — a shape the container overview does not
// use — opening on an identity card of four grey facts about the API server,
// which is the least actionable thing on the page. It now opens with the same
// two bands and states its facts in the same panels.
//
// The linked stat tiles stay. They are the Swarm overview's grammar for the
// same question ("how much is in here, and take me there"), and they occupy
// the slot the container overview gives its health and uptime cards.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBox, IconServer, IconTopologyStar3, IconWorld } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { statusSoft, type StatusTone } from '@resources/theme/status-palette';
import { resourcePath } from '../format';
import { PluginLink } from '../nav-context';
import type { KubernetesSummary } from '../types';
import { ClusterInsightsCard } from './ClusterInsightsCard';
import { ClusterSuggestedActions } from './ClusterSuggestedActions';

/** Counts arrive as an open record, so a missing key reads as zero, not NaN. */
function count(summary: KubernetesSummary, key: string): number {
  return summary.counts[key] ?? 0;
}

export function KubernetesOverview({ summary }: { summary: KubernetesSummary }) {
  const { t } = useTranslation('kubernetes');
  const c = summary.counts;

  return (
    <div className="space-y-6">
      <ClusterSuggestedActions summary={summary} />
      <ClusterInsightsCard summary={summary} />

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile
          to={resourcePath('/pods')}
          icon={<IconBox className="size-5" />}
          label={t('tabs.pods')}
          value={count(summary, 'pods')}
          detail={t('stats.podsDetail', { namespaces: count(summary, 'namespaces') })}
          tone="success"
        />
        <StatTile
          to={resourcePath('/deployments')}
          icon={<IconTopologyStar3 className="size-5" />}
          label={t('tabs.deployments')}
          value={count(summary, 'deployments')}
          detail={t('stats.deploymentsDetail', { replicaSets: count(summary, 'replicaSets') })}
          tone="info"
        />
        <StatTile
          to={resourcePath('/services')}
          icon={<IconServer className="size-5" />}
          label={t('tabs.services')}
          value={count(summary, 'services')}
          detail={t('stats.servicesDetail', { endpoints: count(summary, 'endpoints') })}
          tone="info"
        />
        <StatTile
          to={resourcePath('/nodes')}
          icon={<IconWorld className="size-5" />}
          label={t('tabs.nodes')}
          value={count(summary, 'nodes')}
          detail={t('stats.nodesDetail', { events: count(summary, 'events') })}
          tone="neutral"
        />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Panel title={t('overview.general')}>
          <InfoRow label={t('fields.connected')}>
            <StatusIndicator tone="success" label={t('values.yes')} />
          </InfoRow>
          <InfoRow label={t('fields.version')}>
            <span className="font-mono text-xs">{summary.version || '-'}</span>
          </InfoRow>
          <InfoRow label={t('fields.namespaceScope')}>{summary.namespaceScope || t('values.allNamespaces')}</InfoRow>
          <InfoRow label={t('tabs.namespaces')}>{c.namespaces ?? 0}</InfoRow>
          <InfoRow label={t('fields.nodes')}>{c.nodes ?? 0}</InfoRow>
        </Panel>

        <Panel title={t('sections.workloads')}>
          <InfoRow label={t('tabs.pods')}>{c.pods ?? 0}</InfoRow>
          <InfoRow label={t('tabs.deployments')}>{c.deployments ?? 0}</InfoRow>
          <InfoRow label={t('resources.replicaSets')}>{c.replicaSets ?? 0}</InfoRow>
          <InfoRow label={t('tabs.workloads')}>{(c.statefulSets ?? 0) + (c.daemonSets ?? 0)}</InfoRow>
          <InfoRow label={t('resources.hpas')}>{c.horizontalPodAutoscalers ?? 0}</InfoRow>
        </Panel>

        <Panel title={t('sections.networking')}>
          <InfoRow label={t('tabs.services')}>{c.services ?? 0}</InfoRow>
          <InfoRow label={t('resources.endpoints')}>{c.endpoints ?? 0}</InfoRow>
          <InfoRow label={t('tabs.networking')}>{c.ingresses ?? 0}</InfoRow>
          <InfoRow label={t('resources.networkPolicies')}>{c.networkPolicies ?? 0}</InfoRow>
        </Panel>

        <Panel title={t('sections.storage')}>
          <InfoRow label={t('resources.persistentVolumes')}>{c.persistentVolumes ?? 0}</InfoRow>
          <InfoRow label={t('resources.claims')}>{c.persistentVolumeClaims ?? 0}</InfoRow>
          <InfoRow label={t('resources.storageClasses')}>{c.storageClasses ?? 0}</InfoRow>
        </Panel>
      </div>
    </div>
  );
}

export function KubernetesResources({ summary }: { summary: KubernetesSummary }) {
  const { t } = useTranslation('kubernetes');

  const inventory: [string, number][] = [
    [t('tabs.namespaces'), count(summary, 'namespaces')],
    [t('tabs.nodes'), count(summary, 'nodes')],
    [t('resources.configMaps'), count(summary, 'configMaps')],
    [t('resources.secrets'), count(summary, 'secrets')],
    [t('resources.serviceAccounts'), count(summary, 'serviceAccounts')],
    [t('resources.resourceQuotas'), count(summary, 'resourceQuotas')],
    [t('resources.limitRanges'), count(summary, 'limitRanges')],
    [t('tabs.events'), count(summary, 'events')],
  ];

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <Panel title={t('sections.inventory')}>
        {inventory.map(([label, value]) => (
          <InfoRow key={label} label={label}>
            {value}
          </InfoRow>
        ))}
      </Panel>

      <Panel title={t('tabs.capabilities')}>
        {summary.capabilities.length === 0 ? (
          <EmptyState variant="compact" title={t('empty.capabilities')} />
        ) : (
          summary.capabilities.map((capability) => (
            <InfoRow key={String(capability.name)} label={String(capability.name)}>
              <Badge variant="secondary">{String(capability.status)}</Badge>
            </InfoRow>
          ))
        )}
      </Panel>
    </div>
  );
}

/** The container overview's fact panel: uppercase caption over `InfoRow`s. */
function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">{title}</h3>
      {children}
    </div>
  );
}

function StatTile({
  to,
  icon,
  label,
  value,
  detail,
  tone,
}: {
  to: string;
  icon: ReactNode;
  label: string;
  value: number;
  detail: string;
  tone: StatusTone;
}) {
  // The glass surface is on the link itself rather than on a nested `Card`:
  // `.mc-glass` paints `background` from a plain class, so a Tailwind
  // `hover:bg-*` on a wrapper would be gambling on stylesheet order. A lift
  // and a label colour change owe nothing to that ordering.
  return (
    <PluginLink
      to={to}
      className="mc-glass group block rounded-xl p-4 transition-transform hover:-translate-y-0.5 md:p-5"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm text-muted-foreground transition-colors group-hover:text-foreground">
          {label}
        </span>
        <span className={`rounded-lg p-1.5 ${statusSoft(tone)}`}>{icon}</span>
      </div>
      <p className="mt-3 text-2xl font-semibold text-foreground">{value}</p>
      <p className="mt-1 truncate text-xs text-muted-foreground">{detail}</p>
    </PluginLink>
  );
}
