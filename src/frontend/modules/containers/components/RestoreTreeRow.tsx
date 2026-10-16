// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight, IconFile, IconFolder, IconFolderOpen } from '@tabler/icons-react';
import { Box } from '@resources/components/ui/Layout';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { Text as BodyText } from '@resources/components/ui/Typography';
import { cssVars } from '@resources/utils/cssVars';
import type { ContainerBackupRestoreOption } from '@resources/hooks/useContainerBackups';
import {
  computeConfigIndeterminate,
  computeIndeterminate,
  coveredByFolder,
  type RestoreFileTreeKind,
} from './restore-tree-keys';

type TreeRowProps = {
  entry: ContainerBackupRestoreOption;
  mountTarget: string;
  depth: number;
  selected: string[];
  cache: Map<string, ContainerBackupRestoreOption[]>;
  expanded: Set<string>;
  onToggle: (entry: ContainerBackupRestoreOption, checked: boolean) => void;
  onExpand: (entry: ContainerBackupRestoreOption) => void;
  formatSize: (size: number) => string;
  kind: RestoreFileTreeKind;
  // renderChildren supplies the nested branch for an expanded folder.
  // Passing it in keeps this module free of an import back to the
  // branch, so the two files do not form an import cycle.
  renderChildren: (entry: ContainerBackupRestoreOption) => ReactNode;
};

// TreeRow renders one file or folder line of the restore tree, with its
// tri-state checkbox and (for an expanded folder) the nested branch.
export function TreeRow({
  entry,
  mountTarget,
  depth,
  selected,
  cache,
  expanded,
  onToggle,
  onExpand,
  formatSize,
  kind,
  renderChildren,
}: TreeRowProps) {
  const { t } = useTranslation('containers');
  const isFolder = entry.type === 'folder';
  const isFile = entry.type === 'file';
  const isExpanded = expanded.has(entry.path ?? '');
  const indent = depth * 12;

  // Config rows use the literal `entry.key` as the wire key
  // (`config:Config.Env`, `config:Config.Env[FOO]`,
  // `config:Config.WorkingDir`). Mount/filesystem rows synthesize
  // `folder:<mount>:<path>` / `file:<mount>:<path>`.
  const folderWireKey = kind === 'config' ? entry.key : `folder:${mountTarget}:${entry.path}`;
  const fileWireKey = kind === 'config' ? entry.key : `file:${mountTarget}:${entry.path}`;

  const folderChecked = useMemo(() => {
    if (!isFolder) return false;
    return selected.includes(folderWireKey);
  }, [selected, isFolder, folderWireKey]);

  const folderIndeterminate = useMemo(() => {
    if (!isFolder) return false;
    if (selected.includes(folderWireKey)) return false;
    if (kind === 'config') {
      return computeConfigIndeterminate(entry, selected, cache);
    }
    return computeIndeterminate(entry, mountTarget, selected, cache);
  }, [selected, isFolder, folderWireKey, kind, entry, mountTarget, cache]);

  const fileChecked = useMemo(() => {
    if (!isFile) return false;
    return selected.includes(fileWireKey);
  }, [selected, isFile, fileWireKey]);

  // Under a ticked folder the row is already part of the restore.
  const covered = kind !== 'config' && coveredByFolder(entry.path, mountTarget, selected);

  const checkedState: boolean | 'indeterminate' = covered
    ? true
    : isFolder
      ? folderIndeterminate
        ? 'indeterminate'
        : folderChecked
      : fileChecked;

  const handleCheck = (value: boolean | 'indeterminate') => {
    onToggle(entry, value === true);
  };

  const handleExpand = () => {
    onExpand(entry);
  };

  const labelKind = isFolder ? t('backups.restoreFolderLabel') : t('backups.restoreFileLabel');
  const ariaLabel = t('backups.restoreTreeEntryLabel', {
    label: entry.label,
    kind: labelKind,
  });

  return (
    <Box>
      <Box
        className="flex items-center gap-2 rounded-md px-2 py-1 pl-(--indent) transition-colors hover:bg-muted/40"
        style={cssVars({ indent: `${indent + 8}px` })}
      >
        {isFolder ? (
          <PlainButton
            type="button"
            onClick={handleExpand}
            aria-label={
              isExpanded ? t('nav.collapseSection', { ns: 'common' }) : t('nav.expandSection', { ns: 'common' })
            }
            aria-expanded={isExpanded}
            className="inline-flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground"
          >
            {isExpanded ? (
              <IconChevronDown className="size-4" aria-hidden="true" />
            ) : (
              <IconChevronRight className="size-4" aria-hidden="true" />
            )}
          </PlainButton>
        ) : (
          <Box className="size-5 shrink-0" />
        )}
        {isFolder ? (
          isExpanded ? (
            <IconFolderOpen className="size-4 shrink-0 text-warning" aria-hidden="true" />
          ) : (
            <IconFolder className="size-4 shrink-0 text-warning" aria-hidden="true" />
          )
        ) : (
          <IconFile className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        )}
        <span className="flex shrink-0" title={covered ? t('backups.restoreIncludedByFolder') : undefined}>
          <Checkbox
            checked={checkedState}
            disabled={covered}
            onCheckedChange={handleCheck}
            aria-label={ariaLabel}
            className="shrink-0"
          />
        </span>
        <BodyText size="sm" className="min-w-0 flex-1 truncate">
          {entry.label}
        </BodyText>
        {isFile && typeof entry.size === 'number' && entry.size > 0 && (
          <BodyText size="sm" variant="muted" className="shrink-0">
            {formatSize(entry.size)}
          </BodyText>
        )}
      </Box>
      {isFolder && isExpanded && renderChildren(entry)}
    </Box>
  );
}
