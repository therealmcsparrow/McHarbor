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
import { useEnvironmentDetailState } from '../hooks/useEnvironmentDetailState';
import { useRegenerateToken, useUpdateEnvironment } from '../hooks/useEnvironmentActions';
import { useEnvironment, useEnvironmentHostMetrics, useEnvironmentMetrics } from '../hooks/useEnvironments';
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

  const showSaveAction = state.activeTab === 'activity' || state.activeTab === 'automation';
  const saveDisabled =
    updateEnvironment.isPending ||
    (state.activeTab === 'activity'
      ? !state.activityIsDirty
      : state.activeTab === 'automation'
        ? !state.automationIsDirty
        : true);
  const saveLabel = updateEnvironment.isPending ? tc('actions.saving') : tc('actions.save');

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
                  ? () =>
                      updateEnvironment.mutate({
                        id: env.id,
                        data:
                          state.activeTab === 'activity'
                            ? {
                                trackContainerEventsEnabled: state.trackContainerEventsEnabled,
                                collectContainerMetricsEnabled: state.collectContainerMetricsEnabled,
                                highlightContainerChangesEnabled: state.highlightContainerChangesEnabled,
                                dockerDiskUsageNotificationsEnabled: state.dockerDiskUsageNotificationsEnabled,
                                dockerDiskUsageThresholdPercent: state.normalizedThreshold,
                              }
                            : {
                                scheduledUpdateCheckEnabled: state.scheduledUpdateCheckEnabled,
                                automaticImagePruningEnabled: state.automaticImagePruningEnabled,
                                timezone: normalizeTimezone(state.timezone),
                              },
                      })
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
            isSaving={updateEnvironment.isPending}
            onTrackContainerEventsChange={state.setTrackContainerEventsEnabled}
            onCollectContainerMetricsChange={state.setCollectContainerMetricsEnabled}
            onHighlightContainerChangesChange={state.setHighlightContainerChangesEnabled}
            onDockerDiskUsageNotificationsChange={state.setDockerDiskUsageNotificationsEnabled}
            onDockerDiskUsageThresholdChange={state.setDockerDiskUsageThresholdPercent}
          />
        </TabsContent>
        <TabsContent value="automation">
          <EnvironmentAutomationTab
            env={env}
            scheduledUpdateCheckEnabled={state.scheduledUpdateCheckEnabled}
            automaticImagePruningEnabled={state.automaticImagePruningEnabled}
            timezone={state.timezone}
            isSaving={updateEnvironment.isPending}
            onScheduledUpdateCheckChange={state.setScheduledUpdateCheckEnabled}
            onAutomaticImagePruningChange={state.setAutomaticImagePruningEnabled}
            onTimezoneChange={state.setTimezone}
          />
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
