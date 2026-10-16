// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFile, IconFolder, IconUpload } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { formatBytes } from '@resources/utils/format';
import { useUploadFile } from '../hooks/useContainerFiles';
import { UploadProgress } from './UploadProgress';
import { UploadSelectionList } from './UploadSelectionList';
import {
  mergeUploadSelections,
  selectionFromDataTransfer,
  selectionFromFileList,
  totalUploadSize,
  type UploadSelection,
} from './upload-dialog-utils';

type UploadDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  currentPath: string;
};

const emptySelection: UploadSelection = { files: [], directories: [] };

export function UploadDialog({ open: isOpen, onOpenChange, containerId, currentPath }: UploadDialogProps) {
  const { t } = useTranslation('containers');
  const [selection, setSelection] = useState<UploadSelection>(emptySelection);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);
  const uploadMutation = useUploadFile(containerId);
  const uploadSize = totalUploadSize(selection);
  const hasSelection = selection.files.length > 0 || selection.directories.length > 0;

  const setFolderInput = useCallback((node: HTMLInputElement | null) => {
    folderInputRef.current = node;
    node?.setAttribute('webkitdirectory', '');
    node?.setAttribute('directory', '');
  }, []);

  const clearSelection = () => setSelection({ files: [], directories: [] });

  const handleDragOver = useCallback((evt2: React.DragEvent) => {
    evt2.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((evt2: React.DragEvent) => {
    evt2.preventDefault();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback(async (evt2: React.DragEvent) => {
    evt2.preventDefault();
    setIsDragging(false);
    const droppedSelection = await selectionFromDataTransfer(evt2.dataTransfer);
    setSelection((current) => mergeUploadSelections(current, droppedSelection));
  }, []);

  const handleUpload = () => {
    if (!hasSelection) return;
    uploadMutation.mutate(
      { path: currentPath, files: selection.files, directories: selection.directories },
      {
        onSuccess: () => {
          clearSelection();
          uploadMutation.resetProgress();
          onOpenChange(false);
        },
      },
    );
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      clearSelection();
      uploadMutation.resetProgress();
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('files.upload')}</DialogTitle>
          <DialogDescription className="sr-only">{t('files.uploadDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3 p-4">
          <div className="font-mono text-xs text-muted-foreground">{currentPath}</div>
          <PlainButton
            className={`flex min-h-[140px] w-full cursor-pointer flex-col items-center justify-center rounded-lg border-2 border-dashed px-4 text-center transition-colors ${
              isDragging ? 'border-primary bg-primary/5' : 'border-border hover:border-muted-foreground'
            }`}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
          >
            {hasSelection ? (
              <span className="flex flex-col items-center gap-1.5 text-sm text-foreground">
                <span className="flex items-center gap-2">
                  <IconFile className="size-5 text-muted-foreground" />
                  <span>{t('files.uploadSelection', { count: selection.files.length })}</span>
                </span>
                <span className="text-xs text-muted-foreground">
                  {formatBytes(uploadSize)}
                  {selection.directories.length > 0
                    ? ` - ${t('files.uploadFolders', { count: selection.directories.length })}`
                    : ''}
                </span>
              </span>
            ) : (
              <span className="flex flex-col items-center gap-1.5 text-muted-foreground">
                <IconUpload className="size-6" />
                <span className="text-sm">{t('files.dropOrClick')}</span>
              </span>
            )}
          </PlainButton>

          <UploadSelectionList selection={selection} />

          {uploadMutation.isPending && <UploadProgress progress={uploadMutation.progress} />}

          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
              <IconFile className="mr-1.5 size-4" />
              {hasSelection ? t('files.addFiles') : t('files.chooseFiles')}
            </Button>
            <Button variant="outline" size="sm" onClick={() => folderInputRef.current?.click()}>
              <IconFolder className="mr-1.5 size-4" />
              {hasSelection ? t('files.addFolder') : t('files.chooseFolder')}
            </Button>
            {hasSelection && (
              <Button variant="ghost" size="sm" onClick={clearSelection}>
                {t('files.clearSelection')}
              </Button>
            )}
          </div>

          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(changeEvent) => {
              const nextSelection = selectionFromFileList(changeEvent.target.files);
              setSelection((current) => mergeUploadSelections(current, nextSelection));
              changeEvent.target.value = '';
            }}
          />
          <input
            ref={setFolderInput}
            type="file"
            multiple
            className="hidden"
            onChange={(evt) => {
              const nextSelection = selectionFromFileList(evt.target.files);
              setSelection((current) => mergeUploadSelections(current, nextSelection));
              evt.target.value = '';
            }}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t('edit.cancelChanges')}
          </Button>
          <Button onClick={handleUpload} disabled={!hasSelection || uploadMutation.isPending}>
            {uploadMutation.isPending ? t('files.uploading') : t('files.upload')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
