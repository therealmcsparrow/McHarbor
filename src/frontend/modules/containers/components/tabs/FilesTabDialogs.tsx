// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { ChmodDialog } from '../ChmodDialog';
import { CreateFileDialog } from '../CreateFileDialog';
import { FileEditorDialog } from '../FileEditorDialog';
import { FileViewerDialog } from '../FileViewerDialog';
import { RenameDialog } from '../RenameDialog';
import { UploadDialog } from '../UploadDialog';
import type { DialogState } from './files-tab-types';

type FilesTabDialogsProps = {
  containerId: string;
  currentPath: string;
  deletePending: boolean;
  dialog: DialogState;
  onClose: () => void;
  onConfirmDelete: () => void;
  onEditFromViewer: (entry: Extract<DialogState, { type: 'view' }>['entry']) => void;
};

export function FilesTabDialogs({
  containerId,
  currentPath,
  deletePending,
  dialog,
  onClose,
  onConfirmDelete,
  onEditFromViewer,
}: FilesTabDialogsProps) {
  const { t } = useTranslation('containers');

  return (
    <>
      {dialog.type === 'view' && (
        <FileViewerDialog
          open
          onOpenChange={(open) => !open && onClose()}
          containerId={containerId}
          filePath={dialog.entry.path}
          onEdit={() => onEditFromViewer(dialog.entry)}
        />
      )}
      {dialog.type === 'edit' && (
        <FileEditorDialog
          open
          onOpenChange={(open) => !open && onClose()}
          containerId={containerId}
          filePath={dialog.entry.path}
        />
      )}
      {dialog.type === 'create' && (
        <CreateFileDialog
          open
          onOpenChange={(open) => !open && onClose()}
          containerId={containerId}
          currentPath={currentPath}
          mode={dialog.mode}
        />
      )}
      {dialog.type === 'rename' && (
        <RenameDialog
          open
          onOpenChange={(open) => !open && onClose()}
          containerId={containerId}
          filePath={dialog.entry.path}
          currentName={dialog.entry.name}
        />
      )}
      {dialog.type === 'chmod' && (
        <ChmodDialog
          open
          onOpenChange={(open) => !open && onClose()}
          containerId={containerId}
          filePath={dialog.entry.path}
          currentMode={dialog.entry.mode}
        />
      )}
      {dialog.type === 'upload' && (
        <UploadDialog
          open
          onOpenChange={(open) => !open && onClose()}
          containerId={containerId}
          currentPath={currentPath}
        />
      )}
      {dialog.type === 'delete' && (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && onClose()}
          title={t('files.delete')}
          description={
            dialog.entry.isDir
              ? t('files.confirmDeleteDir', { name: dialog.entry.name })
              : t('files.confirmDelete', { name: dialog.entry.name })
          }
          confirmLabel={t('files.delete')}
          loading={deletePending}
          onConfirm={onConfirmDelete}
        />
      )}
    </>
  );
}
