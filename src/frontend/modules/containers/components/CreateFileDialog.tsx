// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { useSaveFile, useCreateDirectory } from '../hooks/useContainerFiles';

type CreateFileDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  currentPath: string;
  mode: 'file' | 'folder';
};

export function CreateFileDialog({
  open,
  onOpenChange,
  containerId,
  currentPath,
  mode,
}: CreateFileDialogProps) {
  const { t } = useTranslation('containers');
  const [name, setName] = useState('');
  const saveMutation = useSaveFile(containerId);
  const mkdirMutation = useCreateDirectory(containerId);
  const isPending = saveMutation.isPending || mkdirMutation.isPending;

  const handleSubmit = () => {
    if (!name.trim()) return;
    const basePath = currentPath.endsWith('/') ? currentPath : currentPath + '/';

    if (mode === 'folder') {
      mkdirMutation.mutate(basePath + name, {
        onSuccess: () => {
          setName('');
          onOpenChange(false);
        },
      });
    } else {
      saveMutation.mutate(
        { path: basePath + name, content: '' },
        {
          onSuccess: () => {
            setName('');
            onOpenChange(false);
          },
        }
      );
    }
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) setName('');
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {mode === 'folder' ? t('files.newFolder') : t('files.newFile')}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-3 p-4">
          <div className="space-y-1.5">
            <Label>{mode === 'folder' ? t('files.folderName') : t('files.fileName')}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={mode === 'folder' ? 'new-folder' : 'file.txt'}
              onKeyDown={(e) => e.key === 'Enter' && handleSubmit()}
              autoFocus
            />
          </div>
          <div className="text-xs text-muted-foreground font-mono">{currentPath}</div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t('edit.cancelChanges')}
          </Button>
          <Button onClick={handleSubmit} disabled={!name.trim() || isPending}>
            {isPending ? t('files.saving') : t('files.toast.created')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
