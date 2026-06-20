// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';
import { Spinner } from '@resources/components/ui/Spinner';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { AgentTokenDialog } from '../components/AgentTokenDialog';
import { EnvironmentActivityTab } from '../components/EnvironmentActivityTab';
import { EnvironmentAutomationTab } from '../components/EnvironmentAutomationTab';
import { EnvironmentDetailHeader } from '../components/EnvironmentDetailHeader';
import { EnvironmentOverviewPanel } from '../components/EnvironmentOverviewPanel';
import { EnvironmentRetentionTab } from '../components/EnvironmentRetentionTab';
import { useEnvironmentDetailState, type AutoUpdateDay } from '../hooks/useEnvironmentDetailState';
import { useGlobalDiskThresholdPercent } from '../hooks/useGlobalDiskThreshold';
import { useRegenerateToken, useUpdateEnvironment } from '../hooks/useEnvironmentActions';
import { useEnvironment, useEnvironmentHostMetrics, useEnvironmentMetrics } from '../hooks/useEnvironments';
import { EnvironmentDockerTab } from '../components/EnvironmentDockerTab';
import { EnvironmentHostLogsTab } from '../components/EnvironmentHostLogsTab';
import { EnvironmentHostTab } from '../components/EnvironmentHostTab';
import { EnvironmentHostTerminalTab } from '../components/EnvironmentHostTerminalTab';
import { EnvironmentProcessesTab } from '../components/EnvironmentProcessesTab';
import { normalizeTimezone } from '../timezones';

export default function EnvironmentDetailPage() {
  const { t } = useTranslation('environments');
  const { t: tc } = useTranslation('common');
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: env, isLoading: envLoading } = useEnvironment(id);
  const { data: stats } = useEnvironmentMetrics(id, env?.collectContainerMetricsEnabled ?? true);
  const { data: hostMetrics } = useEnvironmentHostMetrics(id);
  const regenToken = useRegenerateToken();
  const updateEnvironment = useUpdateEnvironment();
  const globalDiskThreshold = useGlobalDiskThresholdPercent();
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  const state = useEnvironmentDetailState(env);

  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  if (envLoading) {
    return <div className="flex h-64 items-center justify-center"><Spinner size="lg" /></div>;
  }

  if (!env) {
    return (
      <div className="space-y-6 p-5">
        {document.getElementById('header-slot')
          ? createPortal(
              <EnvironmentDetailHeader title={t('detail.notFound')} backLabel={t('detail.back')} onBack={() => navigate('/environments')} />,
              document.getElementById('header-slot')!,
            )
          : null}
        <div className="rounded-lg border border-border bg-card p-6">
          <p className="text-sm text-muted-foreground">{t('detail.notFoundDescription')}</p>
        </div>
      </div>
    );
  }

  const showSaveAction = state.activeTab === 'activity' || state.activeTab === 'automation' || state.activeTab === 'retention';
  const saveDisabled =
    updateEnvironment.isPending ||
    (state.activeTab === 'activity'
      ? !state.activityIsDirty
      : state.activeTab === 'automation'
        ? !state.automationIsDirty
        : state.activeTab === 'retention'
          ? !state.retentionIsDirty
          : true);
  const saveLabel = updateEnvironment.isPending ? tc('actions.saving') : tc('actions.save');

  const activeTabPayload = (() => {
    if (state.activeTab === 'activity') {
      return {
        trackContainerEventsEnabled: state.trackContainerEventsEnabled,
        collectContainerMetricsEnabled: state.collectContainerMetricsEnabled,
        highlightContainerChangesEnabled: state.highlightContainerChangesEnabled,
        dockerDiskUsageNotificationsEnabled: state.dockerDiskUsageNotificationsEnabled,
        dockerDiskUsageThresholdPercent: state.normalizedThreshold,
        dockerDiskUsageUseGlobalDefault: state.dockerDiskUsageUseGlobalDefault,
      };
    }
    if (state.activeTab === 'automation') {
      return {
        scheduledUpdateCheckEnabled: state.scheduledUpdateCheckEnabled,
        automaticImagePruningEnabled: state.automaticImagePruningEnabled,
        imagePruneDanglingOnly: state.imagePruneDanglingOnly,
        timezone: normalizeTimezone(state.timezone),
      };
    }
    return {
      logRetentionDays: state.normalizedLogRetention,
      containerPruneEnabled: state.containerPruneEnabled,
      containerPruneStoppedDays: state.normalizedContainerPruneDays,
      metricRetentionHours: state.normalizedMetricHours,
    };
  })();

  return (
    <div className="space-y-6 p-5">
      {document.getElementById('header-slot')
        ? createPortal(
            <EnvironmentDetailHeader
              title={env.name}
              description={t('detail.connectionDescription', { type: env.connectionType.toUpperCase() })}
              backLabel={t('detail.back')}
              onBack={() => navigate('/environments')}
              saveLabel={showSaveAction ? saveLabel : undefined}
              onSave={
                showSaveAction
                  ? () => updateEnvironment.mutate({ id: env.id, data: activeTabPayload })
                  : undefined
              }
              saveDisabled={saveDisabled}
              savePending={updateEnvironment.isPending}
            />,
            document.getElementById('header-slot')!,
          )
        : null}

      <Tabs value={state.activeTab} onValueChange={(value) => state.setActiveTab(value as typeof state.activeTab)} className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview">{t('detail.tabs.overview')}</TabsTrigger>
          <TabsTrigger value="activity">{t('detail.tabs.activity')}</TabsTrigger>
          <TabsTrigger value="automation">{t('detail.tabs.automation')}</TabsTrigger>
          <TabsTrigger value="retention">{t('detail.tabs.retention')}</TabsTrigger>
          <TabsTrigger value="host">{t('detail.tabs.host')}</TabsTrigger>
          <TabsTrigger value="docker">{t('detail.tabs.docker')}</TabsTrigger>
          <TabsTrigger value="processes">{t('detail.tabs.processes')}</TabsTrigger>
          <TabsTrigger value="terminal">{t('detail.tabs.terminal')}</TabsTrigger>
          <TabsTrigger value="logs">{t('detail.tabs.logs')}</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          <EnvironmentOverviewPanel
            env={env}
            stats={stats}
            hostMetrics={hostMetrics}
            onRegenerateToken={() =>
              regenToken.mutate(id, {
                onSuccess: (data) => {
                  state.setRegeneratedToken(data.token);
                  state.setTokenDialogOpen(true);
                },
              })
            }
            isRegenerating={regenToken.isPending}
            t={t}
          />
        </TabsContent>
        <TabsContent value="activity">
          <EnvironmentActivityTab
            env={env}
            trackContainerEventsEnabled={state.trackContainerEventsEnabled}
            collectContainerMetricsEnabled={state.collectContainerMetricsEnabled}
            highlightContainerChangesEnabled={state.highlightContainerChangesEnabled}
            dockerDiskUsageNotificationsEnabled={state.dockerDiskUsageNotificationsEnabled}
            dockerDiskUsageThresholdPercent={state.dockerDiskUsageThresholdPercent}
            dockerDiskUsageUseGlobalDefault={state.dockerDiskUsageUseGlobalDefault}
            globalDiskThresholdPercent={globalDiskThreshold.data ?? 80}
            isSaving={updateEnvironment.isPending}
            onTrackContainerEventsChange={state.setTrackContainerEventsEnabled}
            onCollectContainerMetricsChange={state.setCollectContainerMetricsEnabled}
            onHighlightContainerChangesChange={state.setHighlightContainerChangesEnabled}
            onDockerDiskUsageNotificationsChange={state.setDockerDiskUsageNotificationsEnabled}
            onDockerDiskUsageThresholdChange={state.setDockerDiskUsageThresholdPercent}
            onDockerDiskUsageUseGlobalDefaultChange={state.setDockerDiskUsageUseGlobalDefault}
          />
        </TabsContent>
        <TabsContent value="automation">
          <EnvironmentAutomationTab
            env={env}
            scheduledUpdateCheckEnabled={state.scheduledUpdateCheckEnabled}
            automaticImagePruningEnabled={state.automaticImagePruningEnabled}
            imagePruneDanglingOnly={state.imagePruneDanglingOnly}
            timezone={state.timezone}
            isSaving={updateEnvironment.isPending}
            onScheduledUpdateCheckChange={state.setScheduledUpdateCheckEnabled}
            onAutomaticImagePruningChange={state.setAutomaticImagePruningEnabled}
            onImagePruneDanglingOnlyChange={state.setImagePruneDanglingOnly}
            onTimezoneChange={state.setTimezone}
          />
        </TabsContent>
        <TabsContent value="retention">
          <EnvironmentRetentionTab
            env={env}
            logRetentionDays={state.logRetentionDays}
            containerPruneEnabled={state.containerPruneEnabled}
            containerPruneStoppedDays={state.containerPruneStoppedDays}
            autoUpdateEnabled={state.autoUpdateEnabled}
            autoUpdateWindowStart={state.autoUpdateWindowStart}
            autoUpdateWindowEnd={state.autoUpdateWindowEnd}
            autoUpdateDaysSelected={state.autoUpdateDaysSelected}
            metricRetentionHours={state.metricRetentionHours}
            isSaving={updateEnvironment.isPending}
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
        </TabsContent>
        <TabsContent value="host">
          <EnvironmentHostTab envId={env.id} env={env} />
        </TabsContent>
        <TabsContent value="docker">
          <EnvironmentDockerTab envId={env.id} />
        </TabsContent>
        <TabsContent value="processes">
          <EnvironmentProcessesTab envId={env.id} />
        </TabsContent>
        <TabsContent value="terminal">
          <EnvironmentHostTerminalTab envId={env.id} />
        </TabsContent>
        <TabsContent value="logs">
          <EnvironmentHostLogsTab envId={env.id} />
        </TabsContent>
      </Tabs>

      <AgentTokenDialog
        open={state.tokenDialogOpen}
        onOpenChange={state.setTokenDialogOpen}
        token={state.regeneratedToken}
        serverUrl={`${window.location.protocol}//${window.location.host}`}
      />
    </div>
  );
}
