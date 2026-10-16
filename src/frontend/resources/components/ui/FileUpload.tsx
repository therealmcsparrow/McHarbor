// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState, type ChangeEvent, type DragEvent as ReactDragEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconUpload } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { FileChip, formatFileSize, validateUpload, type FileState } from './FileUpload.parts';

type FileUploadProps = {
  accept?: string;
  multiple?: boolean;
  maxSize?: number;
  maxFiles?: number;
  disabled?: boolean;
  label?: ReactNode;
  description?: ReactNode;
  buttonText?: string;
  removeLabel?: string;
  onChange: (files: File[]) => void;
  className?: string;
};

export function FileUpload({
  accept,
  multiple = false,
  maxSize,
  maxFiles,
  disabled = false,
  label,
  description,
  buttonText,
  removeLabel,
  onChange,
  className,
}: FileUploadProps) {
  const { t } = useTranslation('common');
  const [files, setFiles] = useState<FileState[]>([]);
  const [isDragging, setIsDragging] = useState(false);

  const validateFile = useCallback((file: File) => validateUpload(file, accept, maxSize), [accept, maxSize]);

  const processFiles = useCallback(
    (fileList: File[]) => {
      if (maxFiles && fileList.length > maxFiles) {
        onChange([]);
        setFiles([]);
        return;
      }

      const newFiles: FileState[] = fileList.map((file) => {
        const validation = validateFile(file);
        return {
          file,
          id: Math.random().toString(36).slice(2),
          status: validation.valid ? 'complete' : 'error',
          error: validation.error,
        };
      });

      const validFiles = newFiles.filter((f) => f.status === 'complete').map((f) => f.file);
      setFiles((prev) => [...prev, ...newFiles].slice(0, maxFiles));
      onChange(validFiles);
    },
    [maxFiles, onChange, validateFile],
  );

  const handleFileChange = useCallback(
    (evt: ChangeEvent<HTMLInputElement>) => {
      if (disabled) return;
      const fileList = Array.from(evt.target.files || []);
      processFiles(fileList);
      evt.target.value = '';
    },
    [disabled, processFiles],
  );

  const handleDrop = useCallback(
    (evt: ReactDragEvent<HTMLDivElement>) => {
      evt.preventDefault();
      setIsDragging(false);
      if (disabled) return;
      const fileList = Array.from(evt.dataTransfer.files);
      processFiles(fileList);
    },
    [disabled, processFiles],
  );

  const handleRemove = useCallback(
    (id: string) => {
      setFiles((prev) => {
        const newFiles = prev.filter((f) => f.id !== id);
        onChange(newFiles.filter((f) => f.status === 'complete').map((f) => f.file));
        return newFiles;
      });
    },
    [onChange],
  );

  const handleDragEnter = useCallback((evt: ReactDragEvent<HTMLDivElement>) => {
    evt.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((evt: ReactDragEvent<HTMLDivElement>) => {
    evt.preventDefault();
    setIsDragging(false);
  }, []);

  const handleDragOver = useCallback((evt: ReactDragEvent<HTMLDivElement>) => {
    evt.preventDefault();
  }, []);

  return (
    <div className={cn('space-y-3', className)}>
      {label && <label className="text-sm font-medium text-foreground">{label}</label>}
      {description && <p className="text-xs text-muted-foreground">{description}</p>}

      <div
        role="presentation"
        className={cn(
          'flex flex-col items-center justify-center rounded-xl border-2 border-dashed transition-colors',
          'min-h-[120px] cursor-pointer bg-muted/30 p-6',
          'focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-1 focus-within:ring-offset-background focus-within:outline-none',
          isDragging
            ? 'border-solid border-primary bg-primary/5'
            : 'border-border hover:border-primary/30 hover:bg-primary/5',
          disabled && 'cursor-not-allowed opacity-50',
        )}
        onDragEnter={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDragOver={handleDragOver}
        onDrop={handleDrop}
      >
        <input
          type="file"
          accept={accept}
          multiple={multiple}
          disabled={disabled}
          onChange={handleFileChange}
          className="hidden"
          id="file-upload-input"
        />

        <IconUpload
          className={cn('mb-3 size-8 text-muted-foreground', isDragging && 'text-primary')}
          aria-hidden="true"
        />

        <div className="text-center">
          <label
            htmlFor="file-upload-input"
            className={cn('cursor-pointer text-sm font-medium text-foreground', disabled && 'cursor-not-allowed')}
          >
            {isDragging ? t('fileUpload.dropHere') : (buttonText ?? t('fileUpload.browse'))}
          </label>
          <p className="mt-1 text-xs text-muted-foreground">
            {accept ? t('fileUpload.accepted', { accept }) : t('fileUpload.dragAndDrop')}
            {maxSize && ` | ${t('fileUpload.maxSize', { size: formatFileSize(maxSize) })}`}
            {maxFiles && ` | ${t('fileUpload.maxFiles', { count: maxFiles })}`}
          </p>
        </div>
      </div>

      {files.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {files.map((fileState) => (
            <FileChip
              key={fileState.id}
              fileState={fileState}
              disabled={disabled}
              removeLabel={removeLabel ?? t('fileUpload.remove')}
              onRemove={handleRemove}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export type { FileUploadProps };
