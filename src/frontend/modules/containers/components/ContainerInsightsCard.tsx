// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowRight, IconBulb, IconCircleCheck } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { DetailTabId } from './ContainerDetailTabs';
import type { ContainerInsight } from '../hooks/useContainerInsights';

type ContainerInsightsCardProps = {
  insights: ContainerInsight[];
  onNavigate: (tab: DetailTabId) => void;
};

export function ContainerInsightsCard({ insights, onNavigate }: ContainerInsightsCardProps) {
  const { t } = useTranslation('containers');
  return (
    <section className="p-4">
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
          {insights.map((insight) => (
            <li key={insight.id} className="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
              <Badge
                variant={
                  insight.severity === 'error'
                    ? 'destructive'
                    : insight.severity === 'warning'
                      ? 'warning'
                      : 'secondary'
                }
                className="mt-0.5"
              >
                {t(`insights.severity.${insight.severity}`)}
              </Badge>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-foreground">{t(insight.titleKey, insight.values)}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">{t(insight.descriptionKey, insight.values)}</p>
              </div>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => onNavigate(insight.target)}
                aria-label={t('insights.open', {
                  title: t(insight.titleKey, insight.values),
                })}
              >
                <IconArrowRight className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
