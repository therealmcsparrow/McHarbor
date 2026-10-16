// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFile, IconFilePlus, IconFolder, IconLink, IconPencil, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatDate } from '@resources/utils/format';
import { formatBytes } from '../format';
import { COMPUTE_PERM } from '../permissions';
import type { PoolFile } from '../types';

interface PoolFileTableProps {
  entries: PoolFile[];
  onOpenDir: (path: string) => void;
  onEdit: (entry: PoolFile) => void;
  onRename: (entry: PoolFile) => void;
  onDelete: (entry: PoolFile) => void;
}

export function PoolFileTable({ entries, onOpenDir, onEdit, onRename, onDelete }: PoolFileTableProps) {
  const { t } = useTranslation('compute');

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('pools.files.column.name')}</TableHead>
          <TableHead>{t('pools.files.column.size')}</TableHead>
          <TableHead>{t('pools.files.column.mode')}</TableHead>
          <TableHead>{t('pools.files.column.modified')}</TableHead>
          <TableHead className="text-right">{t('pools.files.column.actions')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {entries.map((entry) => (
          <TableRow key={entry.path}>
            <TableCell>
              <div className="flex min-w-0 items-center gap-2">
                {entry.isSymlink ? (
                  <IconLink className="size-4 shrink-0 text-muted-foreground" />
                ) : entry.isDir ? (
                  <IconFolder className="size-4 shrink-0 text-primary" />
                ) : (
                  <IconFile className="size-4 shrink-0 text-muted-foreground" />
                )}
                {entry.isDir ? (
                  <Button variant="ghost" size="sm" onClick={() => onOpenDir(entry.path)}>
                    {entry.name}
                  </Button>
                ) : (
                  <span className="truncate">{entry.name}</span>
                )}
                {entry.linkTarget && (
                  <span className="truncate font-mono text-xs text-muted-foreground">→ {entry.linkTarget}</span>
                )}
              </div>
            </TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">
              {entry.isDir ? '—' : formatBytes(entry.sizeBytes ?? 0)}
            </TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">{entry.mode ?? '—'}</TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {entry.modifiedAt ? formatDate(entry.modifiedAt) : '—'}
            </TableCell>
            <TableCell>
              <div className="flex items-center justify-end gap-1">
                {!entry.isDir && !entry.isSymlink && (
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('pools.files.edit')}
                    permission={COMPUTE_PERM.MANAGE}
                    onClick={() => onEdit(entry)}
                  >
                    <IconPencil className="size-4" />
                  </Button>
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('pools.files.rename')}
                  permission={COMPUTE_PERM.MANAGE}
                  onClick={() => onRename(entry)}
                >
                  <IconFilePlus className="size-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('pools.files.delete')}
                  permission={COMPUTE_PERM.DELETE}
                  onClick={() => onDelete(entry)}
                >
                  <IconTrash className="size-4" />
                </Button>
              </div>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
