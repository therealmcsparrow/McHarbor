// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { useContainerFiles } from '../hooks/useContainerFiles';
import { ContainerPathNode } from './ContainerPathNode';

type ContainerPathTreeProps = {
  containerId: string;
  // Absolute container path the tree is rooted at: "/" for the container
  // filesystem layer, or the mount target for a mount.
  rootPath: string;
  selected: string[];
  onChange: (paths: string[]) => void;
};

type ContainerPathChildrenProps = {
  containerId: string;
  rootPath: string;
  path: string;
  depth: number;
  selected: string[];
  onChange: (paths: string[]) => void;
};

// Children are fetched only when a folder is opened, so browsing a deep
// tree costs one request per level actually looked at rather than a
// recursive walk of the whole container. ContainerPathNode renders this
// for its open folders, so the two files import each other; both only
// reference the other at render time, which ES modules handle fine.
export function ContainerPathChildren({
  containerId,
  rootPath,
  path,
  depth,
  selected,
  onChange,
}: ContainerPathChildrenProps) {
  const { t } = useTranslation('containers');
  const { data: entries = [], isLoading, isError, error } = useContainerFiles(containerId, path);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-1 pl-6 text-xs text-muted-foreground">
        <Spinner size="sm" />
        {t('backups.treeLoading')}
      </div>
    );
  }
  if (isError) {
    return <ErrorState variant="inline" className="ml-6" title={t('backups.treeLoadError')} error={error} />;
  }
  if (entries.length === 0) {
    return <p className="py-1 pl-6 text-xs text-muted-foreground">{t('backups.treeEmpty')}</p>;
  }

  return (
    <div>
      {entries.map((entry) => (
        <ContainerPathNode
          key={entry.path}
          containerId={containerId}
          rootPath={rootPath}
          path={entry.path}
          name={entry.name}
          isDir={entry.isDir}
          depth={depth}
          selected={selected}
          onChange={onChange}
        />
      ))}
    </div>
  );
}

// Browses the live container so a backup selection can be made before any
// archive exists. It intentionally mirrors the restore dialog's tree: the
// same shape of decision deserves the same shape of control.
export function ContainerPathTree({ containerId, rootPath, selected, onChange }: ContainerPathTreeProps) {
  const { t } = useTranslation('containers');

  return (
    <div className="space-y-2">
      <div className="max-h-64 overflow-auto rounded-lg border border-border bg-background p-2">
        <ContainerPathChildren
          containerId={containerId}
          rootPath={rootPath}
          path={rootPath}
          depth={0}
          selected={selected}
          onChange={onChange}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {selected.length === 0
          ? t('backups.treeNothingPicked')
          : t('backups.treePickedCount', { count: selected.length })}
      </p>
    </div>
  );
}
