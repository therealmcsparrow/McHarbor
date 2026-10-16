// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { ContainerDetailActions } from './ContainerDetailActions';
import { ContainerDetailIdentity } from './ContainerDetailIdentity';
import type { ContainerInspect } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';

export { getInspectWebUrl } from './container-web-url';

type ContainerDetailHeaderProps = {
  container: ContainerInspect;
  isRunning: boolean;
  isPaused: boolean;
  name: string;
  webUrl: string | null;
  editUnlocked: boolean;
  editUnlockable: boolean;
  actionsLocked: boolean;
  onToggleEditLock: () => void;
  onOpenConfigEditor: () => void;
  onRename: () => void;
  stackName?: string | null;
  onAction: (action: string) => void;
  onRecreate: () => void;
  onKill: () => void;
  onRemove: () => void;
  onMove: () => void;
  onTakeOver: () => void;
  onRelink: () => void;
};

export function ContainerDetailHeader({
  container,
  isRunning,
  isPaused,
  name: containerName,
  webUrl,
  editUnlocked,
  editUnlockable,
  actionsLocked,
  onToggleEditLock,
  onOpenConfigEditor,
  onRename,
  stackName,
  onAction,
  onRecreate,
  onKill,
  onRemove,
  onMove,
  onTakeOver,
  onRelink,
}: ContainerDetailHeaderProps) {
  const resolvedStackName = stackName ?? container.Config?.Labels?.['com.docker.compose.project'] ?? null;
  const protectedContainer = isProtectedContainer(container);
  return (
    <>
      <ContainerDetailIdentity
        container={container}
        containerName={containerName}
        stackName={resolvedStackName}
        protectedContainer={protectedContainer}
        editUnlocked={editUnlocked}
      />
      <ContainerDetailActions
        container={container}
        isRunning={isRunning}
        isPaused={isPaused}
        webUrl={webUrl}
        protectedContainer={protectedContainer}
        editUnlocked={editUnlocked}
        editUnlockable={editUnlockable}
        actionsLocked={actionsLocked}
        onToggleEditLock={onToggleEditLock}
        onOpenConfigEditor={onOpenConfigEditor}
        onRename={onRename}
        onAction={onAction}
        onRecreate={onRecreate}
        onKill={onKill}
        onRemove={onRemove}
        onMove={onMove}
        onTakeOver={onTakeOver}
        onRelink={onRelink}
      />
    </>
  );
}
