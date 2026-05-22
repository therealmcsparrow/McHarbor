// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { IconUpload, IconFile } from '@tabler/icons-react';
import { formatBytes } from '@resources/utils/format';
import { useUploadFile } from '../hooks/useContainerFiles';

type UploadDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  currentPath: string;
};

export function UploadDialog({
  open,
  onOpenChange,
  containerId,
  currentPath,
}: UploadDialogProps) {
  const { t } = useTranslation('containers');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const uploadMutation = useUploadFile(containerId);

  const handleFileSelect = (file: File) => {
    setSelectedFile(file);
  };

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    const file = e.dataTransfer.files[0];
    if (file) handleFileSelect(file);
  }, []);

  const handleUpload = () => {
    if (!selectedFile) return;
    uploadMutation.mutate(
      { path: currentPath, file: selectedFile },
      {
        onSuccess: () => {
          setSelectedFile(null);
          onOpenChange(false);
        },
      }
    );
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) setSelectedFile(null);
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('files.upload')}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 p-4">
          <div className="text-xs text-muted-foreground font-mono">{currentPath}</div>
          <div
            className={`flex min-h-[120px] cursor-pointer flex-col items-center justify-center rounded-lg border-2 border-dashed transition-colors ${
              isDragging
                ? 'border-primary bg-primary/5'
                : 'border-border hover:border-muted-foreground'
            }`}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => inputRef.current?.click()}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => e.key === 'Enter' && inputRef.current?.click()}
          >
            {selectedFile ? (
              <div className="flex items-center gap-2 text-sm text-foreground">
                <IconFile className="h-5 w-5 text-muted-foreground" />
                <span>{selectedFile.name}</span>
                <span className="text-muted-foreground">({formatBytes(selectedFile.size)})</span>
              </div>
            ) : (
              <div className="flex flex-col items-center gap-1.5 text-muted-foreground">
                <IconUpload className="h-6 w-6" />
                <span className="text-sm">{t('files.dropOrClick')}</span>
              </div>
            )}
          </div>
          <input
            ref={inputRef}
            type="file"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) handleFileSelect(file);
            }}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t('edit.cancelChanges')}
          </Button>
          <Button onClick={handleUpload} disabled={!selectedFile || uploadMutation.isPending}>
            {uploadMutation.isPending ? t('files.uploading') : t('files.upload')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
