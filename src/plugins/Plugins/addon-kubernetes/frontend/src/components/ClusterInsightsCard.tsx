// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ClusterInsightsCard — `ContainerInsightsCard`, for a cluster.
//
// Same section shell, same bulb, same severity badge, same "all clear" line,
// same trailing arrow that takes the reader to the tab where the problem
// lives. It is a copy in shape rather than an import because the host
// component's `onNavigate` is typed to `DetailTabId` — a container's 21 tabs —
// and its strings resolve in the `containers` namespace. Everything visual is
// held identical on purpose: a reader who has learned to scan this card on a
// container should not have to learn it again on a cluster.

import { useTranslation } from 'react-i18next';
import { IconArrowRight, IconBulb, IconCircleCheck } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { resourcePath } from '../format';
import { usePluginNavigate } from '../nav-context';
import type { KubernetesSummary } from '../types';
import { clusterInsights, type InsightSeverity } from './kubernetes-insights';

const SEVERITY_VARIANT: Record<InsightSeverity, 'destructive' | 'warning' | 'secondary'> = {
  error: 'destructive',
  warning: 'warning',
  info: 'secondary',
};

/** Literal `t('…')` calls — the i18n gate cannot see an interpolated key. */
function severityLabel(severity: InsightSeverity, t: (key: string) => string): string {
  switch (severity) {
    case 'error':
      return t('insights.severity.error');
    case 'warning':
      return t('insights.severity.warning');
    case 'info':
      return t('insights.severity.info');
  }
}

export function ClusterInsightsCard({ summary }: { summary: KubernetesSummary }) {
  const { t } = useTranslation('kubernetes');
  const navigate = usePluginNavigate();
  const insights = clusterInsights(summary);

  return (
    <section className="mc-glass rounded-lg p-5">
      <div className="mb-4 flex items-center gap-2">
        <IconBulb className="size-5 text-primary" />
        <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">{t('insights.title')}</h3>
      </div>
      {insights.length === 0 ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <IconCircleCheck className="size-4 text-primary" />
          {t('insights.clear')}
        </div>
      ) : (
        <ul className="divide-y divide-border">
          {insights.map((insight) => {
            const title = t(insight.titleKey, insight.values);
            return (
              <li key={insight.id} className="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
                <Badge variant={SEVERITY_VARIANT[insight.severity]} className="mt-0.5">
                  {severityLabel(insight.severity, t)}
                </Badge>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-foreground">{title}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{t(insight.descriptionKey, insight.values)}</p>
                </div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => navigate(resourcePath(`/${insight.target}`))}
                  aria-label={t('insights.open', { title })}
                >
                  <IconArrowRight className="size-4" />
                </Button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
