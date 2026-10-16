// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { Textarea } from '@resources/components/ui/Textarea';
import { COMPUTE_PERM } from '../permissions';
import {
  useCreatePoolDir,
  useDeletePoolFile,
  usePoolFileContent,
  useRenamePoolFile,
  useSavePoolFile,
} from '../hooks/usePoolFiles';
import type { PoolFile } from '../types';

export type DialogState =
  | { type: 'none' }
  | { type: 'mkdir' }
  | { type: 'rename'; entry: PoolFile }
  | { type: 'delete'; entry: PoolFile }
  | { type: 'edit'; entry: PoolFile };

interface PoolFileDialogsProps {
  poolId: string;
  path: string;
  dialog: DialogState;
  draftName: string;
  onDraftNameChange: (value: string) => void;
  draftBody: string;
  onDraftBodyChange: (value: string) => void;
  closeDialog: () => void;
}

export function PoolFileDialogs({
  poolId,
  path,
  dialog,
  draftName,
  onDraftNameChange,
  draftBody,
  onDraftBodyChange,
  closeDialog,
}: PoolFileDialogsProps) {
  const { t } = useTranslation('compute');
  const mkdir = useCreatePoolDir(poolId);
  const rename = useRenamePoolFile(poolId);
  const remove = useDeletePoolFile(poolId);
  const save = useSavePoolFile(poolId);

  const editing = dialog.type === 'edit' ? dialog.entry : null;
  const content = usePoolFileContent(poolId, editing?.path ?? '', Boolean(editing));

  return (
    <>
      <Dialog open={dialog.type === 'mkdir'} onOpenChange={(next) => !next && closeDialog()}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('pools.files.dialog.newFolderTitle')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <Input
              value={draftName}
              placeholder={t('pools.files.dialog.namePlaceholder')}
              onChange={(ev) => onDraftNameChange(ev.target.value)}
            />
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={closeDialog}>
                {t('actions.cancel', { ns: 'common' })}
              </Button>
              <Button
                disabled={!draftName.trim() || mkdir.isPending}
                onClick={() => {
                  mkdir.mutate(
                    {
                      path: path ? `${path}/${draftName.trim()}` : draftName.trim(),
                    },
                    { onSuccess: closeDialog },
                  );
                }}
              >
                {t('actions.create', { ns: 'common' })}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={dialog.type === 'rename'} onOpenChange={(next) => !next && closeDialog()}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('pools.files.dialog.renameTitle')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <Input
              value={draftName}
              placeholder={t('pools.files.dialog.namePlaceholder')}
              onChange={(ev) => onDraftNameChange(ev.target.value)}
            />
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={closeDialog}>
                {t('actions.cancel', { ns: 'common' })}
              </Button>
              <Button
                disabled={!draftName.trim() || rename.isPending}
                onClick={() => {
                  if (dialog.type !== 'rename') return;
                  rename.mutate({ path: dialog.entry.path, newName: draftName.trim() }, { onSuccess: closeDialog });
                }}
              >
                {t('actions.save', { ns: 'common' })}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={dialog.type === 'edit'} onOpenChange={(next) => !next && closeDialog()}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editing?.name ?? ''}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            {content.isLoading ? (
              <div className="flex items-center justify-center p-6">
                <Spinner />
              </div>
            ) : content.isError ? (
              <ErrorState onRetry={() => void content.refetch()} />
            ) : (
              <Textarea
                value={draftBody || (content.data ?? '')}
                className="min-h-[360px] font-mono text-xs"
                onChange={(ev) => onDraftBodyChange(ev.target.value)}
              />
            )}
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={closeDialog}>
                {t('actions.cancel', { ns: 'common' })}
              </Button>
              <Button
                disabled={save.isPending || content.isLoading}
                permission={COMPUTE_PERM.MANAGE}
                onClick={() => {
                  if (dialog.type !== 'edit') return;
                  save.mutate(
                    {
                      path: dialog.entry.path,
                      content: draftBody || (content.data ?? ''),
                    },
                    { onSuccess: closeDialog },
                  );
                }}
              >
                {t('actions.save', { ns: 'common' })}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={dialog.type === 'delete'}
        onOpenChange={(next) => !next && closeDialog()}
        title={t('pools.files.dialog.deleteTitle')}
        description={t('pools.files.dialog.deleteDescription', {
          name: dialog.type === 'delete' ? dialog.entry.name : '',
        })}
        confirmLabel={t('actions.delete', { ns: 'common' })}
        variant="destructive"
        onConfirm={() => {
          if (dialog.type !== 'delete') return;
          remove.mutate({ path: dialog.entry.path, recursive: dialog.entry.isDir }, { onSuccess: closeDialog });
        }}
      />
    </>
  );
}
