// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useNavigate } from 'react-router';
import {
  useRelinkAllContainerBackups,
  type ContainerBackupPlan,
  type ContainerBackupRun,
} from '@resources/hooks/useContainerBackups';
import { BackupPlansTable, BackupRunsTable } from './backups-page-tables';
import { bulkDownloadRuns, triggerBrowserDownload } from './backups-page-helpers';
import { useBackupsPageData } from '../hooks/useBackupsPageData';
import BackupLogsTab from '../components/BackupLogsTab';
import { BackupOverviewTab } from '../components/BackupOverviewTab';
import { BackupSettingsTab } from '../components/BackupSettingsTab';
import { BackupsPageDialogs } from '../components/BackupsPageDialogs';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { BackupsPageHead } from '../components/BackupsPageHead';
import { BackupsTabs } from '../components/BackupsTabs';
import { useUrlSearchState } from '@resources/hooks/useUrlSearchState';
import {
  BACKUPS_GROUPS,
  backupsTabIDs,
  backupsTabIcons,
  backupsTabLabelKeys,
  groupForBackupsTab,
  type BackupsGroupId,
  type BackupsTabId,
} from '../components/backups-tab-groups';

export default function BackupsPage() {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const navigate = useNavigate();
  const envId = useEnvironmentStore((s) => s.currentId);
  const setCurrentId = useEnvironmentStore((s) => s.setCurrentId);
  const environments = useEnvironmentStore((s) => s.environments);
  const [tab, setTab] = useTabFromUrl<BackupsTabId>(backupsTabIDs, 'overview');
  const [scopeEnvId, setScopeEnvId] = useUrlSearchState<string>('', {
    param: 'scope',
  });
  const data = useBackupsPageData(scopeEnvId);
  const relinkAll = useRelinkAllContainerBackups();
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);
  const [editTarget, setEditTarget] = useState<ContainerBackupPlan | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ContainerBackupPlan | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<ContainerBackupRun | null>(null);
  const [deleteRunTarget, setDeleteRunTarget] = useState<ContainerBackupRun | null>(null);
  const [bulkDeleteRunTargets, setBulkDeleteRunTargets] = useState<ContainerBackupRun[]>([]);
  const [bulkDeleteTargets, setBulkDeleteTargets] = useState<ContainerBackupPlan[]>([]);

  // Leaving Activity and coming back should land where you were, not always
  // on Runs — the same rule the Security page's Management group follows.
  const lastActivityTab = useRef<BackupsTabId>('runs');
  useEffect(() => {
    if (groupForBackupsTab(tab) === 'activity') {
      lastActivityTab.current = tab;
    }
  }, [tab]);
  const entryTabFor = useCallback(
    (group: BackupsGroupId): BackupsTabId =>
      group === 'activity' ? lastActivityTab.current : BACKUPS_GROUPS[group][0]!,
    [],
  );

  const tabCommands = useMemo(
    () =>
      backupsTabIDs.map((tabId) => ({
        id: tabId,
        label: t(backupsTabLabelKeys[tabId]),
        icon: backupsTabIcons[tabId],
      })),
    [t],
  );

  useTabCommands({
    pageId: 'backups',
    pageLabel: tc('nav.backups'),
    tabs: tabCommands,
    onTabChange: setTab,
    keywords: ['backup', 'backups'],
  });

  // openContainer switches the global environment to the run /
  // plan's owning environment, then navigates to the container
  // detail page. We can't just link to `/containers/{id}`
  // because the ContainerDetailPage reads the active env from
  // useEnvironmentStore (via the `/containers/{id}` route + the
  // `useContainer` hook). On the Backups page with "All
  // environments" selected, the global env may not match the
  // row's env — switching first ensures the detail page queries
  // the right orchestrator.
  function openContainer(environmentId: string, containerId: string) {
    if (environmentId && environmentId !== envId) {
      setCurrentId(environmentId);
    }
    void navigate(`/containers/${containerId}`);
  }

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <HeaderBreadcrumb
        crumbs={[{ label: tc('nav.backups'), to: '/backups' }, { label: t(backupsTabLabelKeys[tab]) }]}
      />

      <BackupsPageHead
        tab={tab}
        scopeEnvId={scopeEnvId}
        onScopeChange={setScopeEnvId}
        environmentName={environments.find((e) => e.id === scopeEnvId)?.name ?? tc('header.allEnvironments')}
        onRelinkAll={() => relinkAll.mutate()}
        relinkPending={relinkAll.isPending}
      />

      <Card className="flex-none overflow-visible">
        <BackupsTabs
          activeTab={tab}
          onTabChange={setTab}
          entryTabFor={entryTabFor}
          ariaLabel={t('backups.sectionsAriaLabel')}
          onReloadTab={data.reloadTab}
        />
      </Card>

      {/* Bare canvas: every destination brings its own surface — a
          TablePage, `mc-glass` panels, stat cards. A wrapper here would
          nest one card inside another.

          No scroller of its own: the route declares `scroll: "page"`, so
          PageShell is the one scroller on the screen (and gives the page
          the same centred measure Security and Settings get). */}
      <div
        id="backups-detail-panel"
        role="tabpanel"
        aria-labelledby={`backups-tab-${tab}`}
        className="flex min-w-0 flex-col gap-4"
      >
        {tab === 'overview' && (
          <BackupOverviewTab
            runs={data.runRows}
            plans={data.planRows}
            loading={data.overviewLoading}
            onNavigate={setTab}
          />
        )}
        {tab === 'runs' && (
          <BackupRunsTable
            runs={data.runRows}
            loading={data.runsLoading}
            envNameById={data.envNameById}
            onRestore={(run) => setRestoreTarget(run)}
            onDelete={(run) => setDeleteRunTarget(run)}
            onDownload={triggerBrowserDownload}
            onOpenContainer={openContainer}
            onBulkDownload={bulkDownloadRuns}
            onBulkDelete={(runs) => setBulkDeleteRunTargets(runs)}
          />
        )}
        {tab === 'logs' && <BackupLogsTab envId={scopeEnvId} />}
        {tab === 'plans' && (
          <BackupPlansTable
            plans={data.planRows}
            loading={data.plansLoading}
            envNameById={data.envNameById}
            onEdit={(plan) => setEditTarget(plan)}
            onDelete={(plan) => setDeleteTarget(plan)}
            onOpenContainer={openContainer}
            onBulkDelete={(plans) => setBulkDeleteTargets(plans)}
          />
        )}
        {tab === 'settings' && <BackupSettingsTab />}
      </div>

      <BackupsPageDialogs
        restoreTarget={restoreTarget}
        onCloseRestore={() => setRestoreTarget(null)}
        deleteRunTarget={deleteRunTarget}
        onCloseDeleteRun={() => setDeleteRunTarget(null)}
        bulkRunTargets={bulkDeleteRunTargets}
        onCloseBulkRuns={() => setBulkDeleteRunTargets([])}
        editTarget={editTarget}
        onCloseEdit={() => setEditTarget(null)}
        deletePlanTarget={deleteTarget}
        onCloseDeletePlan={() => setDeleteTarget(null)}
        bulkPlanTargets={bulkDeleteTargets}
        onCloseBulkPlans={() => setBulkDeleteTargets([])}
      />
    </div>
  );
}
