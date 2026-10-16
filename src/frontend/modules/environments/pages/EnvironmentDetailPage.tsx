// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useLayoutEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { EnvironmentDetailDialogs } from '../components/EnvironmentDetailDialogs';
import { EnvironmentDetailHeader } from '../components/EnvironmentDetailHeader';
import { EnvironmentDetailSkeleton } from '../components/EnvironmentDetailSkeleton';
import { EnvironmentDetailTabs } from '../components/EnvironmentDetailTabs';
import { EnvironmentTabPanels } from '../components/EnvironmentTabPanels';
import { useEnvironmentDetailState } from '../hooks/useEnvironmentDetailState';
import { useEnvironmentDetailNav } from '../hooks/useEnvironmentDetailNav';
import { useEnvironmentSiblings } from '../hooks/useEnvironmentSiblings';
import { useCancelMigration } from '../hooks/useEnvironmentMigration';
import { useGlobalDiskThresholdPercent } from '../hooks/useGlobalDiskThreshold';
import {
  deriveEnvironmentStatus,
  useRegenerateToken,
  useTestEnvironment,
  useUpdateEnvironment,
} from '../hooks/useEnvironmentActions';
import { useEnvironment, useEnvironmentHostMetrics, useEnvironmentMetrics } from '../hooks/useEnvironments';
import { buildSavePayload, isSaveTab, isTabDirty } from './environment-detail-save';

export default function EnvironmentDetailPage() {
  const { t } = useTranslation('environments');
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: env, isLoading: envLoading } = useEnvironment(id);
  const siblings = useEnvironmentSiblings();
  const cancelMigration = useCancelMigration();
  const [migrateOpen, setMigrateOpen] = useState(false);
  const { data: stats } = useEnvironmentMetrics(id, env?.collectContainerMetricsEnabled ?? true);
  const { data: hostMetrics } = useEnvironmentHostMetrics(id);
  const regenToken = useRegenerateToken();
  const testEnv = useTestEnvironment();
  const updateEnvironment = useUpdateEnvironment();
  const globalDiskThreshold = useGlobalDiskThresholdPercent();
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  const state = useEnvironmentDetailState(env);
  const [testOpen, setTestOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const nav = useEnvironmentDetailNav(id, env);
  const { activeTab, pluginTab } = nav;

  // AppLayout drops the shell padding while the header slot is claimed; the
  // page puts it back itself, exactly as the container detail page does.
  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  if (envLoading) {
    return <EnvironmentDetailSkeleton />;
  }

  if (!env) {
    return (
      <div>
        <HeaderBreadcrumb crumbs={[{ label: t('title'), to: '/environments' }, { label: t('detail.notFound') }]} />
        <ErrorState
          variant="page"
          title={t('detail.notFound')}
          description={t('detail.notFoundDescription')}
          primaryAction={{
            label: t('detail.backToEnvironments'),
            onClick: () => navigate('/environments'),
          }}
        />
      </div>
    );
  }

  const connectionStatus = deriveEnvironmentStatus(env);
  const showSaveAction = isSaveTab(activeTab);
  const saveDisabled = updateEnvironment.isPending || !(showSaveAction && isTabDirty(activeTab, state));
  const saveLabel = updateEnvironment.isPending
    ? t('actions.saving', { ns: 'common' })
    : t('actions.save', { ns: 'common' });

  const isConsoleTab = activeTab === 'terminal' || activeTab === 'logs';

  const handleRegenerateToken = () =>
    regenToken.mutate(id, {
      onSuccess: (data) => {
        state.setRegeneratedToken(data.token);
        state.setRegeneratedPin(data.caFingerprint ?? '');
        state.setTokenDialogOpen(true);
      },
    });

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 p-6">
      <HeaderBreadcrumb
        crumbs={[{ label: t('title'), to: '/environments' }, { label: env.name }]}
        siblings={siblings}
        currentId={env.id}
      />

      {/* The page head (identity and actions) is portalled into the shell's
          band by EnvironmentDetailHeader; this card holds the group tabs. */}
      <EnvironmentDetailHeader
        env={env}
        status={connectionStatus}
        onTestConnection={() => {
          setTestOpen(true);
          testEnv.mutate(env.id);
        }}
        testPending={testEnv.isPending}
        onRegenerateToken={handleRegenerateToken}
        regeneratePending={regenToken.isPending}
        onEdit={() => setEditOpen(true)}
        saveLabel={showSaveAction ? saveLabel : undefined}
        onSave={
          showSaveAction
            ? () =>
                updateEnvironment.mutate({
                  id: env.id,
                  data: buildSavePayload(activeTab, state),
                })
            : undefined
        }
        saveDisabled={saveDisabled}
        savePending={updateEnvironment.isPending}
      />
      <Card className="flex-none overflow-visible">
        <EnvironmentDetailTabs
          activeTab={activeTab}
          onTabChange={nav.setActiveTab}
          pluginTabs={nav.pluginTabs}
          activePluginTab={pluginTab}
          onPluginTabChange={nav.setPluginTab}
          labelFor={(tab) => t(`detail.tabs.${tab}`)}
          groupLabelFor={(key) => t(key)}
          ariaLabel={t('detail.title')}
          hiddenTabs={nav.hiddenTabs}
        />
      </Card>

      <div className="flex min-h-0 flex-1 flex-col">
        <div
          id="environment-detail-panel"
          role="tabpanel"
          aria-labelledby={`environment-tab-${activeTab}`}
          tabIndex={0}
          className={`flex min-h-0 flex-1 flex-col ${isConsoleTab ? 'overflow-hidden' : 'overflow-y-auto'}`}
        >
          <EnvironmentTabPanels
            env={env}
            state={state}
            activeTab={activeTab}
            pluginTab={pluginTab}
            nativeAgent={nav.nativeAgent}
            stats={stats}
            hostMetrics={hostMetrics}
            globalDiskThresholdPercent={globalDiskThreshold.data ?? 80}
            isSaving={updateEnvironment.isPending}
            onRegenerateToken={handleRegenerateToken}
            isRegenerating={regenToken.isPending}
            onMigrate={() => setMigrateOpen(true)}
            onCancelMigration={env.pendingConnectionType ? () => cancelMigration.mutate(env.id) : undefined}
            cancelMigrationPending={cancelMigration.isPending}
          />
        </div>
      </div>

      <EnvironmentDetailDialogs
        env={env}
        state={state}
        testEnv={testEnv}
        migrateOpen={migrateOpen}
        onMigrateOpenChange={setMigrateOpen}
        editOpen={editOpen}
        onEditOpenChange={setEditOpen}
        testOpen={testOpen}
        onTestOpenChange={setTestOpen}
      />
    </div>
  );
}
