// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCheck, IconFile, IconX } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from './Button';

export type FileState = {
  file: File;
  id: string;
  status: 'uploading' | 'complete' | 'error';
  error?: string;
};

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
}

export function validateUpload(
  file: File,
  accept: string | undefined,
  maxSize: number | undefined,
): { valid: boolean; error?: string } {
  if (maxSize && file.size > maxSize) {
    return {
      valid: false,
      error: `File size exceeds maximum of ${formatFileSize(maxSize)}`,
    };
  }
  if (accept) {
    const allowedTypes = accept.split(',').map((t) => t.trim());
    if (
      !allowedTypes.some((type) => {
        if (type.endsWith('/*')) {
          return file.type.startsWith(type.slice(0, -2));
        }
        return file.type === type || file.name.endsWith(type.slice(1));
      })
    ) {
      return { valid: false, error: 'File type not allowed' };
    }
  }
  return { valid: true };
}

type FileChipProps = {
  fileState: FileState;
  disabled: boolean;
  removeLabel: string;
  onRemove: (id: string) => void;
};

export function FileChip({ fileState, disabled, removeLabel, onRemove }: FileChipProps) {
  return (
    <div
      className={cn(
        'mc-glass inline-flex items-center gap-2 rounded-lg px-3 py-2 text-sm',
        fileState.status === 'error' && 'border-destructive bg-destructive/5',
      )}
    >
      <IconFile className="size-4 text-muted-foreground" aria-hidden="true" />
      <span className="max-w-[150px] truncate">{fileState.file.name}</span>
      <span className="text-xs text-muted-foreground">{formatFileSize(fileState.file.size)}</span>
      {fileState.status === 'error' && <span className="text-xs text-destructive">{fileState.error}</span>}
      {fileState.status === 'complete' && <IconCheck className="size-4 text-success" aria-hidden="true" />}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={() => onRemove(fileState.id)}
        disabled={disabled}
        aria-label={removeLabel}
      >
        <IconX className="size-3" aria-hidden="true" />
      </Button>
    </div>
  );
}
