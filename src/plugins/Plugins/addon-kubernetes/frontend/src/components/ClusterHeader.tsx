// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ClusterHeader — the details section at the top of the browser.
//
// `PageHeader` portals into `#header-slot`, which is the topbar. So passing
// `<ClusterSelect />` to its `actions` did not put the picker at the top of
// the page — it put it in the global chrome, beside the app's own environment
// switcher, which is the arrangement `ContainerDetailHeader` and
// `NetworkDetailHeader` were both written to get away from: the topbar ends up
// carrying page-specific controls, and the page opens with nothing that says
// what you are looking at.
//
// The cluster's identity and its controls now sit in a card at the top of the
// content, and the topbar keeps only the title, which sits in the same place
// on every page. The group's tab rail goes inside this card, the way
// `NetworkDetailTabs` sits inside the network hero, so the card reads as one
// object: this is the cluster, this is its state, these are its faces.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh, IconTopologyStar3 } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { cn } from '@resources/utils/cn';
import { useSelectedCluster } from '../cluster-store';
import { useClusters } from '../hooks/useClusters';
import { KUBE_PERM } from '../permissions';
import type { KubernetesSummary } from '../types';
import { ClusterSelect } from './ClusterSelect';

type ClusterHeaderProps = {
  /** Absent while the first summary is in flight. */
  summary?: KubernetesSummary;
  onRefresh: () => void;
  isFetching: boolean;
  /**
   * False on a table tab, where the control lives in the table toolbar with
   * every other list page's refresh. Two refresh buttons on one screen is one
   * too many.
   */
  showRefresh: boolean;
  /** The group's tab rail, rendered inside the card. */
  tabs?: ReactNode;
};

export function ClusterHeader({ summary, onRefresh, isFetching, showRefresh, tabs }: ClusterHeaderProps) {
  const { t } = useTranslation('kubernetes');
  const selected = useSelectedCluster();
  const { data: clusters = [] } = useClusters();

  // The picker shows the name too, but the heading is what the reader's eye
  // lands on, and a page whose <h1> is a generic noun tells them nothing about
  // which of their three clusters they are about to delete a pod from.
  const cluster = clusters.find((entry) => entry.id === selected);
  const itemName = cluster?.name ?? t('cluster.label');
  const connected = summary?.connected ?? false;

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconTopologyStar3 className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {itemName}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StatusIndicator
              tone={connected ? 'success' : 'danger'}
              label={connected ? t('badges.connected') : t('badges.disconnected')}
              size="sm"
            />
            {summary?.version && (
              <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                {summary.version}
              </span>
            )}
            {/* Node and workload counts belong to the panels and tiles
                  below, which have room to label them. The hero carries only
                  what identifies the cluster and whether it is answering. */}
            <Badge variant="default" className="px-1.5 py-0 text-[10px]">
              {summary?.namespaceScope || t('values.allNamespaces')}
            </Badge>
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex flex-wrap items-center gap-2">
          <ClusterSelect />
          {showRefresh && (
            <Button variant="outline" size="sm" permission={KUBE_PERM.VIEW} onClick={onRefresh} disabled={isFetching}>
              <IconRefresh className={cn('size-4', isFetching && 'animate-spin')} />
              {t('actions.refresh')}
            </Button>
          )}
        </div>
      </PageHeadActions>

      {tabs && (
        <Card className="flex-none overflow-visible">
          <div className="px-3 py-2">{tabs}</div>
        </Card>
      )}
    </>
  );
}
