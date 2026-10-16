// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ContainerInspect } from '@core/types/docker';
import { Card } from '@resources/components/ui/Card';
import { ContainerDetailTabs } from '../components/ContainerDetailTabs';
import type { useContainerDetailDialogs, useContainerDetailPermissions } from '../hooks/useContainerDetailState';
import type { useContainerDetailTabs } from '../hooks/useContainerDetailTabs';
import { ContainerDetailHeader, getInspectWebUrl } from './ContainerDetailHeader';

interface ContainerDetailHeroProps {
  container: ContainerInspect;
  stackName: string | null;
  perms: ReturnType<typeof useContainerDetailPermissions>;
  dialogs: ReturnType<typeof useContainerDetailDialogs>;
  tabs: ReturnType<typeof useContainerDetailTabs>;
  onToggleEditLock: () => void;
  onRecreate: () => void;
}

export function ContainerDetailHero({
  container,
  stackName,
  perms,
  dialogs,
  tabs,
  onToggleEditLock,
  onRecreate,
}: ContainerDetailHeroProps) {
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const webUrl = perms.isRunning ? getInspectWebUrl(container.NetworkSettings?.Ports) : null;
  return (
    <div className="flex-none px-6 pt-6">
      {/* ContainerDetailHeader renders nothing here — it fills the page-head
          band above, identity on the left and the action buttons
          right-aligned — so this card is the container's navigation: the
          groups, and the rail inside the active group. */}
      <Card className="flex-none overflow-visible">
        <ContainerDetailHeader
          container={container}
          isRunning={perms.isRunning}
          isPaused={perms.isPaused}
          name={perms.containerName}
          webUrl={webUrl}
          editUnlocked={perms.editUnlockActive}
          editUnlockable={perms.isMcHarborTarget}
          actionsLocked={perms.actionPending}
          onToggleEditLock={onToggleEditLock}
          onRecreate={onRecreate}
          onOpenConfigEditor={() => navigate(`/containers/${container.Id}/edit`)}
          onRename={dialogs.openRenameDialog}
          stackName={stackName}
          onAction={dialogs.handleContainerAction}
          onKill={dialogs.openKillDialog}
          onRemove={() => dialogs.setRemoveDialogOpen(true)}
          onMove={dialogs.openMoveDialog}
          onTakeOver={() => dialogs.setTakeOverOpen(true)}
          onRelink={() => dialogs.setRelinkOpen(true)}
        />
        {/* Group tabs live inside the hero, so the card reads as one
            object: this is the container, this is its state, these are the
            faces of it. */}
        <ContainerDetailTabs
          activeTab={tabs.activeTab}
          onTabChange={tabs.setActiveTab}
          pluginTabs={tabs.pluginTabs}
          activePluginTab={tabs.pluginTab}
          onPluginTabChange={tabs.setPluginTab}
          labelFor={(tab) => t(`detail.${tab}`)}
          groupLabelFor={(key) => t(key)}
          ariaLabel={t('detail.title')}
          visibleTabs={tabs.visibleTabs}
          tabBadges={{ mounts: container.Mounts?.length ?? 0 }}
        />
      </Card>
    </div>
  );
}
