// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ClusterSuggestedActions — the overview's opening band, built to the
// container overview's `overview.suggested` section.
//
// The container overview opens by answering "what should I do next?" before it
// answers "what is this?": a stopped container is offered Start, an unhealthy
// one is offered Logs and Restart, a healthy one is offered Terminal. The
// cluster overview used to open with an identity card — four grey facts about
// the API server — which is the least actionable thing on the page.
//
// The set is chosen in `kubernetes-insights.ts` from what the cluster is
// actually reporting, so a cluster with failing pods leads with Inspect pods
// and a quiet one leads with the workload list.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconBox, IconServer, IconSettings, IconTopologyStar3 } from '@tabler/icons-react';
import type { TablerIcon } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { resourcePath } from '../format';
import { usePluginNavigate } from '../nav-context';
import { KUBE_PERM } from '../permissions';
import type { KubernetesSummary } from '../types';
import { suggestedActions, type SuggestedActionId } from './kubernetes-insights';

const ACTION_ICONS: Record<SuggestedActionId, TablerIcon> = {
  inspectPods: IconBox,
  reviewEvents: IconAlertTriangle,
  viewNodes: IconServer,
  viewWorkloads: IconTopologyStar3,
  manageClusters: IconSettings,
};

/**
 * Labels as literal `t('…')` calls rather than an interpolated key: the i18n
 * CI gate reads literals, and a template key is invisible to it.
 */
function actionLabel(id: SuggestedActionId, t: (key: string) => string): string {
  switch (id) {
    case 'inspectPods':
      return t('overview.suggested.inspectPods');
    case 'reviewEvents':
      return t('overview.suggested.reviewEvents');
    case 'viewNodes':
      return t('overview.suggested.viewNodes');
    case 'viewWorkloads':
      return t('overview.suggested.viewWorkloads');
    case 'manageClusters':
      return t('overview.suggested.manageClusters');
  }
}

export function ClusterSuggestedActions({ summary }: { summary: KubernetesSummary }) {
  const { t } = useTranslation('kubernetes');
  const navigate = usePluginNavigate();
  const actions = suggestedActions(summary, KUBE_PERM.CLUSTERS_MANAGE);

  return (
    <section className="mc-glass rounded-lg p-5">
      <div className="mb-4">
        <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('overview.suggested.title')}
        </h3>
        <p className="mt-1 text-sm text-muted-foreground">{t('overview.suggested.description')}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {actions.map((action) => {
          const Icon = ACTION_ICONS[action.id];
          return (
            <Button
              key={action.id}
              type="button"
              variant={action.primary ? 'default' : 'outline'}
              size="sm"
              className="gap-1.5"
              permission={action.permission}
              onClick={() => navigate(resourcePath(action.to))}
            >
              <Icon className="size-4" />
              {actionLabel(action.id, t)}
            </Button>
          );
        })}
      </div>
    </section>
  );
}
