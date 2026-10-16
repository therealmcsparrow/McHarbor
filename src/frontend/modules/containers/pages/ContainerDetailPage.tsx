// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useCallback, useLayoutEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useContainerStackLink } from '@resources/hooks/useStackLinks';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { PageBody } from '@resources/layout/PageBody';
import { PageSplit } from '@resources/layout/PageShell';
import { cn } from '@resources/utils/cn';
import { ContainerSidebar } from '../components/ContainerSidebar';
import { ContainerDetailTabPanels } from '../components/ContainerDetailTabPanels';
import { ContainerDetailDialogs } from '../components/ContainerDetailDialogs';
import { ContainerDetailLoading, ContainerDetailNotFound } from '../components/ContainerDetailStates';
import { useContainer } from '../hooks/useContainers';
import { useContainerEdit } from '../hooks/useContainerEdit';
import { useContainerStats } from '../hooks/useContainerStats';
import { useContainerDetailCommands } from '../hooks/useContainerDetailCommands';
import { useContainerDetailEnvironment } from '../hooks/useContainerDetailEnvironment';
import { useContainerDetailScroll } from '../hooks/useContainerDetailScroll';
import { useContainerDetailDialogs, useContainerDetailPermissions } from '../hooks/useContainerDetailState';
import { useContainerDetailTabs } from '../hooks/useContainerDetailTabs';
import { useContainerSiblings } from '../hooks/useContainerSiblings';
import { ContainerDetailHero } from './ContainerDetailHero';

/** Keys the page content on environment and container so editing, unlock and dialog state reset on switch. */
export default function ContainerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const currentEnvironmentId = useEnvironmentStore((state) => state.currentId);
  return <ContainerDetailPageContent key={`${currentEnvironmentId ?? ''}|${id ?? ''}`} />;
}

function ContainerDetailPageContent() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const { environmentReady } = useContainerDetailEnvironment();
  const { data: container, isLoading } = useContainer(environmentReady ? (id ?? '') : '');
  const { data: stackLink } = useContainerStackLink(environmentReady ? id : undefined);
  const [editUnlocked, setEditUnlocked] = useState(false);
  const perms = useContainerDetailPermissions(container, editUnlocked);
  const { containerName, isRunning, isPaused, actionsLocked } = perms;
  const edit = useContainerEdit(container, {
    unlockProtected: editUnlocked && perms.isMcHarborTarget,
  });
  const dialogs = useContainerDetailDialogs(container, perms);
  const stats = useContainerStats(container?.Id ?? '', isRunning);
  const tabs = useContainerDetailTabs(id, container, isRunning, isPaused);
  const { activeTab, setActiveTab, pluginTab, showBuiltIn } = tabs;
  const siblings = useContainerSiblings();
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  const { scrollContainerRef, handleEditFieldChange } = useContainerDetailScroll(edit);

  useContainerDetailCommands({
    containerId: container?.Id ?? '',
    containerName,
    isRunning,
    isPaused,
    canStart: perms.canStart,
    canRestart: perms.canRestart,
    canMove: perms.stateAllowsActions,
    actionsLocked,
    onTabChange: setActiveTab,
    onAction: dialogs.handleContainerAction,
    onStop: dialogs.openStopDialog,
    onKill: dialogs.openKillDialog,
    onMove: dialogs.openMoveDialog,
    onRename: dialogs.openRenameDialog,
  });

  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);
  const handleToggleEditLock = useCallback(() => {
    setEditUnlocked((current) => !current);
  }, []);
  if (isLoading) return <ContainerDetailLoading />;
  if (!container) {
    return <ContainerDetailNotFound onBack={() => navigate('/containers')} />;
  }
  const linkedStackName = stackLink?.stackName ?? container.Config?.Labels?.['com.docker.compose.project'] ?? null;
  const builtIn = showBuiltIn ? activeTab : null;
  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Orientation goes in the topbar, where it keeps the same coordinates
          on every page and does not scroll away underneath you. */}
      <HeaderBreadcrumb
        crumbs={[{ label: t('title'), to: '/containers' }, { label: containerName }]}
        siblings={siblings}
        currentId={container.Id}
      />

      <ContainerDetailHero
        container={container}
        stackName={linkedStackName}
        perms={perms}
        dialogs={dialogs}
        tabs={tabs}
        onToggleEditLock={handleToggleEditLock}
        onRecreate={() => {
          dialogs.setSecretFileMode(perms.containerSecretFileMode);
          edit.clearRecreateError();
          dialogs.setRecreateConfirmOpen(true);
        }}
      />

      {/* The panel and the sidebar are siblings in a row, so the sidebar's top
          edge meets the bottom of the tabs card and it runs to the bottom of
          the page from there. */}
      <PageSplit>
        <PageBody
          ref={scrollContainerRef}
          className={cn('min-w-0 gap-4', (builtIn === 'terminal' || builtIn === 'logs') && 'overflow-hidden')}
        >
          <ContainerDetailTabPanels
            id={id}
            container={container}
            stats={stats}
            activeTab={activeTab}
            pluginTab={pluginTab}
            showBuiltIn={showBuiltIn}
            isRunning={isRunning}
            protectedContainer={perms.protectedContainer}
            containerName={containerName}
            editData={edit.editData}
            onTabChange={setActiveTab}
            onFieldChange={handleEditFieldChange}
            onInlineFieldSave={perms.protectedContainer && !perms.editUnlockActive ? undefined : edit.saveResourceField}
          />
        </PageBody>
        <ContainerSidebar
          container={container}
          stats={stats}
          onNavigate={setActiveTab}
          onAction={dialogs.handleContainerAction}
          onOpenConfigEditor={() => navigate(`/containers/${container.Id}/edit`)}
          actionsLocked={actionsLocked}
        />
      </PageSplit>
      <ContainerDetailDialogs
        container={container}
        containerName={containerName}
        confirmStop={dialogs.confirmStop}
        confirmKill={dialogs.confirmKill}
        recreateConfirmOpen={dialogs.recreateConfirmOpen}
        secretFileModeOffered={perms.secretFileModeOffered}
        secretFileMode={dialogs.secretFileMode}
        removeDialogOpen={dialogs.removeDialogOpen}
        moveDialogOpen={dialogs.moveDialogOpen}
        renameDialogOpen={dialogs.renameDialogOpen}
        takeOverOpen={dialogs.takeOverOpen}
        relinkOpen={dialogs.relinkOpen}
        linkedStackName={linkedStackName}
        unlockProtectedRename={perms.editUnlockActive}
        stopKillBlocked={actionsLocked || (!isRunning && !isPaused)}
        editSaving={edit.isSaving}
        recreateError={edit.recreateError}
        changedFields={edit.changes.changedConfigFields}
        onConfirmStopChange={dialogs.setConfirmStop}
        onConfirmKillChange={dialogs.setConfirmKill}
        onRecreateConfirmChange={dialogs.setRecreateConfirmOpen}
        onSecretFileModeChange={dialogs.setSecretFileMode}
        onRemoveDialogChange={dialogs.setRemoveDialogOpen}
        onMoveDialogChange={dialogs.setMoveDialogOpen}
        onRenameDialogChange={dialogs.setRenameDialogOpen}
        onTakeOverChange={dialogs.setTakeOverOpen}
        onRelinkChange={dialogs.setRelinkOpen}
        onConfirmRecreate={() => {
          void edit
            .recreateInPlace(perms.secretFileModeOffered ? { secretFileMode: dialogs.secretFileMode } : {})
            .then(() => dialogs.setRecreateConfirmOpen(false))
            .catch(() => undefined);
        }}
        onRemoveSuccess={() => navigate('/containers')}
      />
    </div>
  );
}
