// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { LinkContainerDialog } from '@resources/components/LinkContainerDialog';
import { TakeOverDialog } from '@resources/components/TakeOverDialog';
import type { ContainerInspect } from '@core/types/docker';
import { ContainerActionDialog } from './ContainerActionDialog';
import { MoveContainerDialog } from './MoveContainerDialog';
import { RecreateConfirmDialog } from './RecreateConfirmDialog';
import { RemoveContainerDialog } from './RemoveContainerDialog';
import { RenameContainerDialog } from './RenameContainerDialog';

type ContainerDetailDialogsProps = {
  container: ContainerInspect;
  containerName: string;
  confirmStop: boolean;
  confirmKill: boolean;
  recreateConfirmOpen: boolean;
  secretFileModeOffered: boolean;
  secretFileMode: boolean;
  removeDialogOpen: boolean;
  moveDialogOpen: boolean;
  renameDialogOpen: boolean;
  takeOverOpen: boolean;
  relinkOpen: boolean;
  linkedStackName: string | null;
  unlockProtectedRename?: boolean;
  stopKillBlocked: boolean;
  editSaving: boolean;
  recreateError: string | null;
  changedFields: string[];
  onConfirmStopChange: (open: boolean) => void;
  onConfirmKillChange: (open: boolean) => void;
  onRecreateConfirmChange: (open: boolean) => void;
  onSecretFileModeChange: (value: boolean) => void;
  onRemoveDialogChange: (open: boolean) => void;
  onMoveDialogChange: (open: boolean) => void;
  onRenameDialogChange: (open: boolean) => void;
  onTakeOverChange: (open: boolean) => void;
  onRelinkChange: (open: boolean) => void;
  onConfirmRecreate: () => void;
  onRemoveSuccess: () => void;
};

export function ContainerDetailDialogs({
  container,
  containerName,
  confirmStop,
  confirmKill,
  recreateConfirmOpen,
  secretFileModeOffered,
  secretFileMode,
  removeDialogOpen,
  moveDialogOpen,
  renameDialogOpen,
  takeOverOpen,
  relinkOpen,
  linkedStackName,
  unlockProtectedRename = false,
  stopKillBlocked,
  editSaving,
  recreateError,
  changedFields,
  onConfirmStopChange,
  onConfirmKillChange,
  onRecreateConfirmChange,
  onSecretFileModeChange,
  onRemoveDialogChange,
  onMoveDialogChange,
  onRenameDialogChange,
  onTakeOverChange,
  onRelinkChange,
  onConfirmRecreate,
  onRemoveSuccess,
}: ContainerDetailDialogsProps) {
  return (
    <>
      <ContainerActionDialog
        containerId={container.Id}
        action="stop"
        open={confirmStop}
        blocked={stopKillBlocked}
        onOpenChange={onConfirmStopChange}
      />

      <ContainerActionDialog
        containerId={container.Id}
        action="kill"
        open={confirmKill}
        blocked={stopKillBlocked}
        onOpenChange={onConfirmKillChange}
      />

      <RecreateConfirmDialog
        open={recreateConfirmOpen}
        onOpenChange={onRecreateConfirmChange}
        changedFields={changedFields}
        loading={editSaving}
        error={recreateError}
        secretFileModeOffered={secretFileModeOffered}
        secretFileMode={secretFileMode}
        onSecretFileModeChange={onSecretFileModeChange}
        onConfirm={onConfirmRecreate}
      />

      <RemoveContainerDialog
        container={{
          id: container.Id,
          name: containerName,
          image: container.Config?.Image ?? '',
          imageId: container.Image ?? '',
          stackName: linkedStackName,
        }}
        open={removeDialogOpen}
        onOpenChange={onRemoveDialogChange}
        onSuccess={onRemoveSuccess}
      />

      <MoveContainerDialog
        container={{
          id: container.Id,
          name: containerName,
          image: container.Config?.Image ?? '',
          stackName: linkedStackName,
        }}
        open={moveDialogOpen}
        onOpenChange={onMoveDialogChange}
      />

      <RenameContainerDialog
        container={{
          id: container.Id,
          name: containerName,
        }}
        open={renameDialogOpen}
        onOpenChange={onRenameDialogChange}
        unlockProtected={unlockProtectedRename}
      />

      <TakeOverDialog
        open={takeOverOpen}
        onOpenChange={onTakeOverChange}
        containerId={container.Id}
        containerName={containerName}
      />

      <LinkContainerDialog
        open={relinkOpen}
        onOpenChange={onRelinkChange}
        initialContainerId={container.Id}
        initialStackName={linkedStackName ?? ''}
        fixedContainer
      />
    </>
  );
}
