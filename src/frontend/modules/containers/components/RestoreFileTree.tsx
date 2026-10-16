// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Box } from '@resources/components/ui/Layout';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as BodyText } from '@resources/components/ui/Typography';
import { formatBytes } from '@resources/utils/format';
import type { ContainerBackupRestoreOption } from '@resources/hooks/useContainerBackups';
import { useRestoreTreeBranch } from '../hooks/useRestoreTreeBranch';
import type { RestoreFileTreeKind } from './restore-tree-keys';
import { applyEntryToggle } from './restore-tree-toggle';
import { TreeRow } from './RestoreTreeRow';

type RestoreFileTreeProps = {
  runId: string;
  // mountTarget is the mount path for `kind: "mount"` (e.g. "/data"),
  // and FILESYSTEM_PREFIX for `kind: "filesystem"`.
  mountTarget: string;
  innerPath?: string;
  secretKey: string;
  selected: string[];
  onChange: (next: string[]) => void;
  onError?: (message: string) => void;
  kind?: RestoreFileTreeKind;
};

// RestoreFileTree is a recursive tree view of files/folders inside a
// single mount subtree or the container filesystem of a backup
// archive. Folders expand on click and lazy-load their immediate
// children via the per-mount or per-filesystem listing endpoint.
// Selection is tri-state (checked / unchecked / indeterminate) for
// folders: a folder is fully checked when the folder key is in the
// selection, fully unchecked when neither the folder key nor any
// descendant key is in the selection, and indeterminate when only
// some descendants are in the selection.
export function RestoreFileTree({
  runId,
  mountTarget,
  innerPath = '',
  secretKey,
  selected,
  onChange,
  onError,
  kind = 'mount',
}: RestoreFileTreeProps) {
  return (
    <TreeBranch
      runId={runId}
      mountTarget={mountTarget}
      innerPath={innerPath}
      secretKey={secretKey}
      selected={selected}
      onChange={onChange}
      onError={onError}
      depth={0}
      kind={kind}
    />
  );
}

type TreeBranchProps = {
  runId: string;
  mountTarget: string;
  innerPath: string;
  secretKey: string;
  selected: string[];
  onChange: (next: string[]) => void;
  onError?: (message: string) => void;
  depth: number;
  kind: RestoreFileTreeKind;
};

function TreeBranch({
  runId,
  mountTarget,
  innerPath,
  secretKey,
  selected,
  onChange,
  onError,
  depth,
  kind,
}: TreeBranchProps) {
  const { t } = useTranslation('containers');
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const { query, items, cache, effectiveMountTarget } = useRestoreTreeBranch({
    runId,
    mountTarget,
    innerPath,
    secretKey,
    kind,
    onError,
  });

  const toggleEntry = (entry: ContainerBackupRestoreOption, checked: boolean) => {
    const next = applyEntryToggle({
      entry,
      checked,
      kind,
      effectiveMountTarget,
      selected,
      cache,
    });
    if (next) onChange(next);
  };

  const toggleExpand = (entry: ContainerBackupRestoreOption) => {
    if (entry.type !== 'folder') {
      // Files do not expand.
      return;
    }
    const entryPath = entry.path ?? '';
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(entryPath)) {
        next.delete(entryPath);
      } else {
        next.add(entryPath);
      }
      return next;
    });
  };

  if (query.isError) {
    return (
      <Box className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive">
        <IconAlertTriangle className="size-4 shrink-0" aria-hidden="true" />
        <BodyText size="sm">
          {[t('backups.restoreFileTreeLoadError'), query.error?.message].filter(Boolean).join(' — ')}
        </BodyText>
      </Box>
    );
  }

  if (query.isLoading && items.length === 0) {
    return (
      <Box className="flex items-center gap-2 rounded-md border border-border bg-muted/30 p-2 text-xs text-muted-foreground">
        <Spinner size="sm" />
        <BodyText size="sm">{t('backups.restoreChoicesLoading')}</BodyText>
      </Box>
    );
  }

  if (items.length === 0) {
    return (
      <BodyText size="sm" variant="muted" className="pl-2">
        {t('backups.restoreTreeEmptyMount')}
      </BodyText>
    );
  }

  return (
    <Box className="flex flex-col gap-1">
      {items.map((entry) => (
        <TreeRow
          key={entry.key}
          entry={entry}
          mountTarget={effectiveMountTarget}
          depth={depth}
          selected={selected}
          cache={cache}
          expanded={expanded}
          onToggle={toggleEntry}
          onExpand={toggleExpand}
          formatSize={(size) => formatBytes(size)}
          kind={kind}
          renderChildren={(folder) => (
            <TreeBranch
              runId={runId}
              mountTarget={effectiveMountTarget}
              // Config folders use the literal wire key as the parent
              // key (`config:Config.Env`); mount / filesystem folders
              // use the posix path.
              innerPath={kind === 'config' ? folder.key : (folder.path ?? '')}
              secretKey={secretKey}
              selected={selected}
              onChange={onChange}
              onError={onError}
              depth={depth + 1}
              kind={kind}
            />
          )}
        />
      ))}
      {query.data?.truncated && (
        <BodyText size="sm" variant="muted" className="pl-2">
          {t('backups.restoreFileTreeTruncated', {
            cap: query.data.maxChildrenCap,
          })}
        </BodyText>
      )}
    </Box>
  );
}
