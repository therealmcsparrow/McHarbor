// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { LinkContainerDialog } from '@resources/components/LinkContainerDialog';
import { TakeOverDialog } from '@resources/components/TakeOverDialog';
import type { ContainerInspect } from '@core/types/docker';
import { MoveContainerDialog } from './MoveContainerDialog';
import { RecreateConfirmDialog } from './RecreateConfirmDialog';
import { RemoveContainerDialog } from './RemoveContainerDialog';
import { RenameContainerDialog } from './RenameContainerDialog';

type ContainerDetailDialogsProps = {
  container: ContainerInspect;
  containerName: string;
  confirmKill: boolean;
  recreateConfirmOpen: boolean;
  removeDialogOpen: boolean;
  moveDialogOpen: boolean;
  renameDialogOpen: boolean;
  takeOverOpen: boolean;
  relinkOpen: boolean;
  linkedStackName: string | null;
  actionPending: boolean;
  editSaving: boolean;
  changedFields: string[];
  onConfirmKillChange: (open: boolean) => void;
  onRecreateConfirmChange: (open: boolean) => void;
  onRemoveDialogChange: (open: boolean) => void;
  onMoveDialogChange: (open: boolean) => void;
  onRenameDialogChange: (open: boolean) => void;
  onTakeOverChange: (open: boolean) => void;
  onRelinkChange: (open: boolean) => void;
  onKill: () => void;
  onConfirmRecreate: () => void;
  onRemoveSuccess: () => void;
};

export function ContainerDetailDialogs({
  container,
  containerName,
  confirmKill,
  recreateConfirmOpen,
  removeDialogOpen,
  moveDialogOpen,
  renameDialogOpen,
  takeOverOpen,
  relinkOpen,
  linkedStackName,
  actionPending,
  editSaving,
  changedFields,
  onConfirmKillChange,
  onRecreateConfirmChange,
  onRemoveDialogChange,
  onMoveDialogChange,
  onRenameDialogChange,
  onTakeOverChange,
  onRelinkChange,
  onKill,
  onConfirmRecreate,
  onRemoveSuccess,
}: ContainerDetailDialogsProps) {
  const { t } = useTranslation('containers');

  return (
    <>
      <ConfirmDialog
        open={confirmKill}
        onOpenChange={onConfirmKillChange}
        title={t('confirm.killTitle')}
        description={t('confirm.killDescription')}
        confirmLabel={t('actions.kill')}
        onConfirm={onKill}
        loading={actionPending}
      />

      <RecreateConfirmDialog
        open={recreateConfirmOpen}
        onOpenChange={onRecreateConfirmChange}
        changedFields={changedFields}
        loading={editSaving}
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
