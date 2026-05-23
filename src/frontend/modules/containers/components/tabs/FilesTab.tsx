// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconFolderOpen,
  IconArrowUp,
  IconUpload,
  IconFilePlus,
  IconFolderPlus,
} from '@tabler/icons-react';
import { Spinner } from '@resources/components/ui/Spinner';
import { Button } from '@resources/components/ui/Button';
import { useContainerFiles, useDeleteFile } from '../../hooks/useContainerFiles';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { FileEntry, ContainerMount } from '@core/types/docker';
import { QuickAccessCards } from './QuickAccessCards';
import { FilesTabDialogs } from './FilesTabDialogs';
import { FilesTabRow } from './FilesTabRow';
import type { DialogState } from './files-tab-types';

type FilesTabProps = {
  containerId: string;
  isRunning: boolean;
  mounts: ContainerMount[];
};

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

      <div className="space-y-3">
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
          <div className="sticky top-0 z-10 flex items-center gap-3 border-b border-border bg-card px-3 py-2 text-xs font-medium text-muted-foreground">
            <div className="shrink-0 w-4" />
            <div className="min-w-0 flex-1">{t('files.name')}</div>
            <div className="hidden w-24 sm:block">{t('files.usage')}</div>
            <div className="w-20 text-right">{t('files.size')}</div>
            <div className="w-20 text-right">{t('files.mode')}</div>
            <div className="w-24 text-right">{t('files.actions')}</div>
            <div className="w-3" />
          </div>

          <div className="flex-1 overflow-y-auto">
            {isLoading ? (
              <div className="flex h-32 items-center justify-center">
                <Spinner size="md" />
              </div>
            ) : sortedFiles.length > 0 ? (
              sortedFiles.map((entry) => (
                <FilesTabRow
                  key={entry.path}
                  entry={entry}
                  isMount={isMountPoint(entry)}
                  mountLabel={t('files.mount')}
                  onAction={handleAction}
                  onNavigate={navigateTo}
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

      <FilesTabDialogs
        containerId={containerId}
        currentPath={currentPath}
        deletePending={deleteMutation.isPending}
        dialog={dialog}
        onClose={closeDialog}
        onConfirmDelete={handleDelete}
        onEditFromViewer={(entry) => setDialog({ type: 'edit', entry })}
      />
    </div>
  );
}
