// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconBraces,
  IconDatabaseExport,
  IconFileText,
  IconPlayerPlay,
  IconRefresh,
  IconSettings,
  IconTerminal,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PageSidebarContent } from '@resources/layout/PageSidebar';
import type { ContainerInspect } from '@core/types/docker';
import type { UseContainerStatsResult } from '../hooks/useContainerStats';
import { useContainerInsights } from '../hooks/useContainerInsights';
import { ContainerInsightsCard } from './ContainerInsightsCard';
import { ContainerAutohealCard } from './ContainerAutohealCard';
import type { DetailTabId } from './ContainerDetailTabs';

type ContainerSidebarProps = {
  container: ContainerInspect;
  stats?: UseContainerStatsResult;
  onNavigate: (tab: DetailTabId) => void;
  onAction: (action: string) => void;
  onOpenConfigEditor: () => void;
  actionsLocked: boolean;
};

export function ContainerSidebar({
  container,
  stats,
  onNavigate,
  onAction,
  onOpenConfigEditor,
  actionsLocked,
}: ContainerSidebarProps) {
  const { t } = useTranslation('containers');
  const state = container.State?.Status ?? 'unknown';
  const isRunning = state === 'running';
  const isUnhealthy = container.State?.Health?.Status === 'unhealthy';
  const canStart = state === 'created' || state === 'exited';
  const insights = useContainerInsights(container, stats);

  const suggestedActions = !isRunning
    ? [
        {
          key: 'start',
          icon: IconPlayerPlay,
          label: t('overview.suggested.start'),
          onClick: () => onAction('start'),
          disabled: actionsLocked || !canStart,
        },
        {
          key: 'logs',
          icon: IconFileText,
          label: t('overview.suggested.viewLogs'),
          onClick: () => onNavigate('logs'),
          disabled: false,
        },
        {
          key: 'edit',
          icon: IconSettings,
          label: t('overview.suggested.editConfig'),
          onClick: onOpenConfigEditor,
          disabled: actionsLocked,
        },
      ]
    : isUnhealthy
      ? [
          {
            key: 'logs',
            icon: IconFileText,
            label: t('overview.suggested.viewLogs'),
            onClick: () => onNavigate('logs'),
            disabled: false,
          },
          {
            key: 'restart',
            icon: IconRefresh,
            label: t('overview.suggested.restart'),
            onClick: () => onAction('restart'),
            disabled: actionsLocked,
          },
          {
            key: 'inspect',
            icon: IconBraces,
            label: t('overview.suggested.inspect'),
            onClick: () => onNavigate('inspect'),
            disabled: false,
          },
          {
            key: 'backups',
            icon: IconDatabaseExport,
            label: t('overview.suggested.viewBackups'),
            onClick: () => onNavigate('backups'),
            disabled: false,
          },
        ]
      : [
          {
            key: 'logs',
            icon: IconFileText,
            label: t('overview.suggested.viewLogs'),
            onClick: () => onNavigate('logs'),
            disabled: false,
          },
          {
            key: 'terminal',
            icon: IconTerminal,
            label: t('overview.suggested.openTerminal'),
            onClick: () => onNavigate('terminal'),
            disabled: false,
          },
          {
            key: 'edit',
            icon: IconSettings,
            label: t('overview.suggested.editConfig'),
            onClick: onOpenConfigEditor,
            disabled: actionsLocked,
          },
          {
            key: 'backups',
            icon: IconDatabaseExport,
            label: t('overview.suggested.viewBackups'),
            onClick: () => onNavigate('backups'),
            disabled: false,
          },
        ];

  return (
    <PageSidebarContent>
      <div className="flex flex-1 flex-col divide-y divide-border">
        <section className="flex-none p-4">
          <div className="mb-4">
            <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
              {t('overview.suggested.title')}
            </h3>
            <p className="mt-1 text-sm text-muted-foreground">{t('overview.suggested.description')}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {suggestedActions.map((action) => {
              const Icon = action.icon;
              return (
                <Button
                  key={action.key}
                  type="button"
                  variant={action.key === 'start' ? 'default' : 'outline'}
                  size="sm"
                  className="gap-1.5"
                  onClick={action.onClick}
                  disabled={action.disabled}
                >
                  <Icon className="size-4" />
                  {action.label}
                </Button>
              );
            })}
          </div>
        </section>

        <div className="flex-1">
          <ContainerInsightsCard insights={insights} onNavigate={onNavigate} />
        </div>

        <div className="flex-none">
          <ContainerAutohealCard containerId={container.Id} />
        </div>
      </div>
    </PageSidebarContent>
  );
}
