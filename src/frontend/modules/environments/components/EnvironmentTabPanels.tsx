// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PluginTabPanel } from '@core/plugins/PluginTabPanel';
import { parsePluginTabId, type PluginTabId } from '@core/plugins/usePluginPageTabs';
import { EnvironmentHostUpdatesTab } from '@resources/components/host/EnvironmentHostUpdatesTab';
import type { HostMetrics } from '@core/types/docker';
import type {
  AutoUpdateDay,
  EnvironmentDetailTab,
  useEnvironmentDetailState,
} from '../hooks/useEnvironmentDetailState';
import type { DashboardStats, EnvironmentInfo } from '../hooks/useEnvironments';
import { EnvironmentAccessTab } from './EnvironmentAccessTab';
import { EnvironmentActivityTab } from './EnvironmentActivityTab';
import { EnvironmentAutomationTab } from './EnvironmentAutomationTab';
import { EnvironmentDockerTab } from './EnvironmentDockerTab';
import { EnvironmentHostLogsTab } from './EnvironmentHostLogsTab';
import { EnvironmentHostTab } from './EnvironmentHostTab';
import { EnvironmentHostTerminalTab } from './EnvironmentHostTerminalTab';
import { EnvironmentOverviewPanel } from './EnvironmentOverviewPanel';
import { EnvironmentProcessesTab } from './EnvironmentProcessesTab';
import { EnvironmentRetentionTab } from './EnvironmentRetentionTab';
import { EnvironmentServicesTab } from './EnvironmentServicesTab';

type DetailState = ReturnType<typeof useEnvironmentDetailState>;

type EnvironmentTabPanelsProps = {
  env: EnvironmentInfo;
  state: DetailState;
  activeTab: EnvironmentDetailTab;
  pluginTab: PluginTabId | null;
  nativeAgent: boolean;
  stats?: DashboardStats;
  hostMetrics?: HostMetrics;
  globalDiskThresholdPercent: number;
  isSaving: boolean;
  onRegenerateToken: () => void;
  isRegenerating: boolean;
  onMigrate: () => void;
  onCancelMigration?: () => void;
  cancelMigrationPending: boolean;
};

export function EnvironmentTabPanels({
  env,
  state,
  activeTab,
  pluginTab,
  nativeAgent,
  stats,
  hostMetrics,
  globalDiskThresholdPercent,
  isSaving,
  onRegenerateToken,
  isRegenerating,
  onMigrate,
  onCancelMigration,
  cancelMigrationPending,
}: EnvironmentTabPanelsProps) {
  const { t } = useTranslation('environments');
  const showBuiltIn = pluginTab === null;
  const parsedPlugin = pluginTab === null ? null : parsePluginTabId(pluginTab);

  return (
    <>
      {parsedPlugin && (
        <PluginTabPanel
          slug={parsedPlugin.slug}
          tabKey={parsedPlugin.tabKey}
          context={{ pageId: 'environment', tabKey: parsedPlugin.tabKey, entityId: env.id }}
        />
      )}

      {showBuiltIn && activeTab === 'overview' && (
        <EnvironmentOverviewPanel
          env={env}
          stats={stats}
          hostMetrics={hostMetrics}
          onRegenerateToken={onRegenerateToken}
          isRegenerating={isRegenerating}
          onMigrate={onMigrate}
          onCancelMigration={onCancelMigration}
          cancelMigrationPending={cancelMigrationPending}
          t={t}
        />
      )}
      {showBuiltIn && activeTab === 'activity' && (
        <EnvironmentActivityTab
          env={env}
          trackContainerEventsEnabled={state.trackContainerEventsEnabled}
          collectContainerMetricsEnabled={state.collectContainerMetricsEnabled}
          highlightContainerChangesEnabled={state.highlightContainerChangesEnabled}
          dockerDiskUsageNotificationsEnabled={state.dockerDiskUsageNotificationsEnabled}
          dockerDiskUsageThresholdPercent={state.dockerDiskUsageThresholdPercent}
          dockerDiskUsageUseGlobalDefault={state.dockerDiskUsageUseGlobalDefault}
          globalDiskThresholdPercent={globalDiskThresholdPercent}
          isSaving={isSaving}
          onTrackContainerEventsChange={state.setTrackContainerEventsEnabled}
          onCollectContainerMetricsChange={state.setCollectContainerMetricsEnabled}
          onHighlightContainerChangesChange={state.setHighlightContainerChangesEnabled}
          onDockerDiskUsageNotificationsChange={state.setDockerDiskUsageNotificationsEnabled}
          onDockerDiskUsageThresholdChange={state.setDockerDiskUsageThresholdPercent}
          onDockerDiskUsageUseGlobalDefaultChange={state.setDockerDiskUsageUseGlobalDefault}
        />
      )}
      {showBuiltIn && activeTab === 'automation' && (
        <EnvironmentAutomationTab
          scheduledUpdateCheckEnabled={state.scheduledUpdateCheckEnabled}
          automaticImagePruningEnabled={state.automaticImagePruningEnabled}
          imagePruneDanglingOnly={state.imagePruneDanglingOnly}
          timezone={state.timezone}
          isSaving={isSaving}
          onScheduledUpdateCheckChange={state.setScheduledUpdateCheckEnabled}
          onAutomaticImagePruningChange={state.setAutomaticImagePruningEnabled}
          onImagePruneDanglingOnlyChange={state.setImagePruneDanglingOnly}
          onTimezoneChange={state.setTimezone}
        />
      )}
      {showBuiltIn && activeTab === 'retention' && (
        <EnvironmentRetentionTab
          logRetentionDays={state.logRetentionDays}
          containerPruneEnabled={state.containerPruneEnabled}
          containerPruneStoppedDays={state.containerPruneStoppedDays}
          autoUpdateEnabled={state.autoUpdateEnabled}
          autoUpdateWindowStart={state.autoUpdateWindowStart}
          autoUpdateWindowEnd={state.autoUpdateWindowEnd}
          autoUpdateDaysSelected={state.autoUpdateDaysSelected}
          metricRetentionHours={state.metricRetentionHours}
          isSaving={isSaving}
          onLogRetentionChange={state.setLogRetentionDays}
          onContainerPruneEnabledChange={state.setContainerPruneEnabled}
          onContainerPruneStoppedDaysChange={state.setContainerPruneStoppedDays}
          onAutoUpdateEnabledChange={state.setAutoUpdateEnabled}
          onAutoUpdateWindowStartChange={state.setAutoUpdateWindowStart}
          onAutoUpdateWindowEndChange={state.setAutoUpdateWindowEnd}
          onAutoUpdateDayToggle={(day) => {
            const next = new Set(state.autoUpdateDaysSelected);
            if (next.has(day as AutoUpdateDay)) {
              next.delete(day as AutoUpdateDay);
            } else {
              next.add(day as AutoUpdateDay);
            }
            state.setAutoUpdateDaysSelected(next);
          }}
          onMetricRetentionChange={state.setMetricRetentionHours}
        />
      )}
      {showBuiltIn && activeTab === 'host' && <EnvironmentHostTab envId={env.id} env={env} />}
      {showBuiltIn && activeTab === 'docker' && <EnvironmentDockerTab envId={env.id} />}
      {showBuiltIn && activeTab === 'processes' && <EnvironmentProcessesTab envId={env.id} />}
      {showBuiltIn && activeTab === 'terminal' && <EnvironmentHostTerminalTab envId={env.id} />}
      {showBuiltIn && activeTab === 'logs' && <EnvironmentHostLogsTab envId={env.id} />}
      {showBuiltIn && activeTab === 'updates' && <EnvironmentHostUpdatesTab envId={env.id} />}
      {showBuiltIn && activeTab === 'access' && <EnvironmentAccessTab envId={env.id} />}
      {showBuiltIn && activeTab === 'services' && nativeAgent && <EnvironmentServicesTab envId={env.id} />}
    </>
  );
}
