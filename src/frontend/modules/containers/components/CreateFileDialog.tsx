// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { useSaveFile, useCreateDirectory } from '../hooks/useContainerFiles';

const FOLDER_NAME_EXAMPLE = 'new-folder';
const FILE_NAME_EXAMPLE = 'file.txt';

type CreateFileDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  currentPath: string;
  mode: 'file' | 'folder';
};

export function CreateFileDialog({
  open: isOpen,
  onOpenChange,
  containerId,
  currentPath,
  mode,
}: CreateFileDialogProps) {
  const { t } = useTranslation('containers');
  const [itemName, setName] = useState('');
  const saveMutation = useSaveFile(containerId);
  const mkdirMutation = useCreateDirectory(containerId);
  const isPending = saveMutation.isPending || mkdirMutation.isPending;

  const handleSubmit = () => {
    if (!itemName.trim()) return;
    const basePath = currentPath.endsWith('/') ? currentPath : currentPath + '/';

    if (mode === 'folder') {
      mkdirMutation.mutate(basePath + itemName, {
        onSuccess: () => {
          setName('');
          onOpenChange(false);
        },
      });
    } else {
      saveMutation.mutate(
        { path: basePath + itemName, content: '' },
        {
          onSuccess: () => {
            setName('');
            onOpenChange(false);
          },
        },
      );
    }
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) setName('');
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{mode === 'folder' ? t('files.newFolder') : t('files.newFile')}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 p-4">
          <div className="space-y-1.5">
            <Label>{mode === 'folder' ? t('files.folderName') : t('files.fileName')}</Label>
            <Input
              value={itemName}
              onChange={(e) => setName(e.target.value)}
              placeholder={mode === 'folder' ? FOLDER_NAME_EXAMPLE : FILE_NAME_EXAMPLE}
              onKeyDown={(e) => e.key === 'Enter' && handleSubmit()}
            />
          </div>
          <div className="font-mono text-xs text-muted-foreground">{currentPath}</div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t('edit.cancelChanges')}
          </Button>
          <Button onClick={handleSubmit} disabled={!itemName.trim() || isPending}>
            {isPending ? t('files.saving') : t('files.toast.created')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
