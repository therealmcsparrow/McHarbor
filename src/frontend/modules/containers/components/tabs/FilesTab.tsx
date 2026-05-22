// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconFolder,
  IconFile,
  IconFolderOpen,
  IconChevronRight,
  IconArrowUp,
  IconDatabase,
  IconUpload,
  IconFilePlus,
  IconFolderPlus,
  IconPencil,
  IconDownload,
  IconTrash,
  IconLock,
  IconEye,
  IconEdit,
} from '@tabler/icons-react';
import { Spinner } from '@resources/components/ui/Spinner';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { formatBytes } from '@resources/utils/format';
import { useContainerFiles, useDeleteFile } from '../../hooks/useContainerFiles';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { FileEntry, ContainerMount } from '@core/types/docker';
import { QuickAccessCards } from './QuickAccessCards';
import { FileViewerDialog } from '../FileViewerDialog';
import { FileEditorDialog } from '../FileEditorDialog';
import { CreateFileDialog } from '../CreateFileDialog';
import { RenameDialog } from '../RenameDialog';
import { ChmodDialog } from '../ChmodDialog';
import { UploadDialog } from '../UploadDialog';

type FilesTabProps = {
  containerId: string;
  isRunning: boolean;
  mounts: ContainerMount[];
};

type DialogState =
  | { type: 'none' }
  | { type: 'view'; entry: FileEntry }
  | { type: 'edit'; entry: FileEntry }
  | { type: 'create'; mode: 'file' | 'folder' }
  | { type: 'rename'; entry: FileEntry }
  | { type: 'chmod'; entry: FileEntry }
  | { type: 'upload' }
  | { type: 'delete'; entry: FileEntry };

function FileRow({
  entry,
  onNavigate,
  isMount,
  mountLabel,
  onAction,
}: {
  entry: FileEntry;
  onNavigate: (path: string) => void;
  isMount: boolean;
  mountLabel: string;
  onAction: (action: string, entry: FileEntry) => void;
}) {
  const { t } = useTranslation('containers');
  const maxSize = 1024 * 1024 * 100;
  const barWidth = entry.isDir ? 0 : Math.min((entry.size / maxSize) * 100, 100);

  return (
    <div
      className={`group flex items-center gap-3 border-b border-border px-3 py-2 last:border-0 ${entry.isDir ? 'cursor-pointer hover:bg-muted/50' : 'hover:bg-muted/30'}`}
      onClick={() => entry.isDir && onNavigate(entry.path)}
    >
      <div className="shrink-0 text-muted-foreground">
        {entry.isDir ? (
          isMount ? (
            <IconDatabase className="h-4 w-4 text-blue-400" />
          ) : (
            <IconFolder className="h-4 w-4 text-yellow-500" />
          )
        ) : (
          <IconFile className="h-4 w-4" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <span
          className={`font-mono text-xs text-foreground ${!entry.isDir ? 'cursor-pointer hover:underline' : ''}`}
          onClick={(e) => {
            if (!entry.isDir) {
              e.stopPropagation();
              onAction('view', entry);
            }
          }}
        >
          {entry.name}
        </span>
        {isMount && (
          <Badge variant="outline" className="ml-2 px-1 py-0 text-[10px] text-blue-400 border-blue-400/30">
            {mountLabel}
          </Badge>
        )}
        {entry.linkTarget && (
          <span className="ml-2 font-mono text-xs text-muted-foreground">
            -&gt; {entry.linkTarget}
          </span>
        )}
      </div>
      <div className="hidden w-24 sm:block">
        {!entry.isDir && entry.size > 0 && (
          <div className="h-1.5 w-full rounded-full bg-muted">
            <div
              className="h-1.5 rounded-full bg-primary/60"
              style={{ width: `${barWidth}%` }}
            />
          </div>
        )}
      </div>
      <div className="w-20 text-right font-mono text-xs text-muted-foreground">
        {entry.isDir ? '-' : formatBytes(entry.size)}
      </div>
      <div className="w-20 text-right font-mono text-xs text-muted-foreground">
        {entry.mode}
      </div>
      {/* Action buttons */}
      <div className="flex w-24 items-center justify-end gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
        {!entry.isDir && (
          <>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  aria-label={t('files.view')}
                  onClick={(e) => { e.stopPropagation(); onAction('view', entry); }}
                >
                  <IconEye className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('files.view')}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  aria-label={t('files.edit')}
                  onClick={(e) => { e.stopPropagation(); onAction('edit', entry); }}
                >
                  <IconEdit className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('files.edit')}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  aria-label={t('files.download')}
                  onClick={(e) => { e.stopPropagation(); onAction('download', entry); }}
                >
                  <IconDownload className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('files.download')}</TooltipContent>
            </Tooltip>
          </>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              aria-label={t('files.rename')}
              onClick={(e) => { e.stopPropagation(); onAction('rename', entry); }}
            >
              <IconPencil className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('files.rename')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              aria-label={t('files.changePermissions')}
              onClick={(e) => { e.stopPropagation(); onAction('chmod', entry); }}
            >
              <IconLock className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('files.changePermissions')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 text-destructive"
              aria-label={t('files.delete')}
              onClick={(e) => { e.stopPropagation(); onAction('delete', entry); }}
            >
              <IconTrash className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('files.delete')}</TooltipContent>
        </Tooltip>
      </div>
      {entry.isDir && (
        <IconChevronRight className="h-3 w-3 text-muted-foreground opacity-0 group-hover:opacity-100" />
      )}
    </div>
  );
}

export function FilesTab({ containerId, isRunning, mounts }: FilesTabProps) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);
  const [currentPath, setCurrentPath] = useState('/');
  const [dialog, setDialog] = useState<DialogState>({ type: 'none' });
  const { data: files = [], isLoading } = useContainerFiles(containerId, currentPath, isRunning);
  const deleteMutation = useDeleteFile(containerId);

  const mountPaths = new Set(mounts.map((m) => m.Destination.replace(/\/$/, '')));
  const isMountPoint = (entry: FileEntry) =>
    mountPaths.has(entry.path.replace(/\/$/, ''));

  const navigateTo = (path: string) => {
    setCurrentPath(path.endsWith('/') ? path : path + '/');
  };

  const navigateUp = () => {
    if (currentPath === '/') return;
    const parts = currentPath.replace(/\/$/, '').split('/');
    parts.pop();
    const parent = parts.join('/') || '/';
    setCurrentPath(parent.endsWith('/') ? parent : parent + '/');
  };

  const handleAction = (action: string, entry: FileEntry) => {
    switch (action) {
      case 'view':
        setDialog({ type: 'view', entry });
        break;
      case 'edit':
        setDialog({ type: 'edit', entry });
        break;
      case 'download': {
        const params = new URLSearchParams({ path: entry.path, download: 'true' });
        if (envId) params.set('env', envId);
        window.open(`/api/containers/${containerId}/files/content?${params}`, '_blank');
        break;
      }
      case 'rename':
        setDialog({ type: 'rename', entry });
        break;
      case 'chmod':
        setDialog({ type: 'chmod', entry });
        break;
      case 'delete':
        setDialog({ type: 'delete', entry });
        break;
    }
  };

  const handleDelete = () => {
    if (dialog.type !== 'delete') return;
    deleteMutation.mutate(
      { path: dialog.entry.path, recursive: dialog.entry.isDir },
      { onSuccess: () => setDialog({ type: 'none' }) }
    );
  };

  const closeDialog = () => setDialog({ type: 'none' });

  if (!isRunning) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-card text-muted-foreground">
        <IconFolderOpen className="mr-2 h-5 w-5" />
        {t('files.mustBeRunning')}
      </div>
    );
  }

  const sortedFiles = [...files].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  return (
    <div className="space-y-4">
      <QuickAccessCards mounts={mounts} currentPath={currentPath} onNavigate={navigateTo} />

      {/* File browser */}
      <div className="space-y-3">
        {/* Toolbar */}
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm" onClick={navigateUp} disabled={currentPath === '/'}>
            <IconArrowUp className="h-4 w-4" />
          </Button>
          <div className="flex-1 rounded-md bg-muted px-3 py-1.5 font-mono text-xs text-foreground">
            {currentPath}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDialog({ type: 'upload' })}
          >
            <IconUpload className="mr-1.5 h-4 w-4" />
            {t('files.upload')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDialog({ type: 'create', mode: 'file' })}
          >
            <IconFilePlus className="mr-1.5 h-4 w-4" />
            {t('files.newFile')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDialog({ type: 'create', mode: 'folder' })}
          >
            <IconFolderPlus className="mr-1.5 h-4 w-4" />
            {t('files.newFolder')}
          </Button>
        </div>

        <div className="flex max-h-[60vh] flex-col rounded-lg border border-border bg-card">
          {/* Sticky header */}
          <div className="sticky top-0 z-10 flex items-center gap-3 border-b border-border bg-card px-3 py-2 text-xs font-medium text-muted-foreground">
            <div className="shrink-0 w-4" />
            <div className="min-w-0 flex-1">{t('files.name')}</div>
            <div className="hidden w-24 sm:block">{t('files.usage')}</div>
            <div className="w-20 text-right">{t('files.size')}</div>
            <div className="w-20 text-right">{t('files.mode')}</div>
            <div className="w-24 text-right">{t('files.actions')}</div>
            <div className="w-3" />
          </div>

          {/* Scrollable file list */}
          <div className="flex-1 overflow-y-auto">
            {isLoading ? (
              <div className="flex h-32 items-center justify-center">
                <Spinner size="md" />
              </div>
            ) : sortedFiles.length > 0 ? (
              sortedFiles.map((entry) => (
                <FileRow
                  key={entry.path}
                  entry={entry}
                  onNavigate={navigateTo}
                  isMount={isMountPoint(entry)}
                  mountLabel={t('files.mount')}
                  onAction={handleAction}
                />
              ))
            ) : (
              <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
                {t('files.emptyDirectory')}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Dialogs */}
      {dialog.type === 'view' && (
        <FileViewerDialog
          open
          onOpenChange={(open) => !open && closeDialog()}
          containerId={containerId}
          filePath={dialog.entry.path}
          onEdit={() => setDialog({ type: 'edit', entry: dialog.entry })}
        />
      )}
      {dialog.type === 'edit' && (
        <FileEditorDialog
          open
          onOpenChange={(open) => !open && closeDialog()}
          containerId={containerId}
          filePath={dialog.entry.path}
        />
      )}
      {dialog.type === 'create' && (
        <CreateFileDialog
          open
          onOpenChange={(open) => !open && closeDialog()}
          containerId={containerId}
          currentPath={currentPath}
          mode={dialog.mode}
        />
      )}
      {dialog.type === 'rename' && (
        <RenameDialog
          open
          onOpenChange={(open) => !open && closeDialog()}
          containerId={containerId}
          filePath={dialog.entry.path}
          currentName={dialog.entry.name}
        />
      )}
      {dialog.type === 'chmod' && (
        <ChmodDialog
          open
          onOpenChange={(open) => !open && closeDialog()}
          containerId={containerId}
          filePath={dialog.entry.path}
          currentMode={dialog.entry.mode}
        />
      )}
      {dialog.type === 'upload' && (
        <UploadDialog
          open
          onOpenChange={(open) => !open && closeDialog()}
          containerId={containerId}
          currentPath={currentPath}
        />
      )}
      {dialog.type === 'delete' && (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && closeDialog()}
          title={t('files.delete')}
          description={
            dialog.entry.isDir
              ? t('files.confirmDeleteDir', { name: dialog.entry.name })
              : t('files.confirmDelete', { name: dialog.entry.name })
          }
          confirmLabel={t('files.delete')}
          loading={deleteMutation.isPending}
          onConfirm={handleDelete}
        />
      )}
    </div>
  );
}
