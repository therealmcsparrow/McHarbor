// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { cssVars } from '@resources/utils/cssVars';
import { useTranslation } from 'react-i18next';
import type { KeyboardEvent as ReactKeyboardEvent } from 'react';
import {
  IconChevronRight,
  IconDatabase,
  IconDownload,
  IconEdit,
  IconEye,
  IconFile,
  IconFolder,
  IconLock,
  IconPencil,
  IconTrash,
} from '@tabler/icons-react';
import { ActionButton } from '@resources/components/ui/ActionButton';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes } from '@resources/utils/format';
import type { FileEntry } from '@core/types/docker';
import { PERM } from '@core/auth/permissionKeys';

const BUTTON_ROLE = 'button';

type FilesTabRowProps = {
  entry: FileEntry;
  isMount: boolean;
  mountLabel: string;
  onAction: (action: string, entry: FileEntry) => void;
  onNavigate: (path: string) => void;
  readOnly?: boolean;
  writePermission?: string | string[];
};

export function FilesTabRow({
  entry,
  isMount,
  mountLabel,
  onAction,
  onNavigate,
  readOnly = false,
  writePermission = PERM.CONTAINERS_MANAGE,
}: FilesTabRowProps) {
  const { t } = useTranslation('containers');
  const maxSize = 1024 * 1024 * 100;
  const barWidth = entry.isDir ? 0 : Math.min((entry.size / maxSize) * 100, 100);

  return (
    <div
      className={`group flex items-center gap-3 border-b border-border px-3 py-2 last:border-0 ${entry.isDir ? 'cursor-pointer hover:bg-muted/50' : 'hover:bg-muted/30'}`}
      {...(entry.isDir
        ? {
            role: BUTTON_ROLE,
            tabIndex: 0,
            onClick: () => onNavigate(entry.path),
            onKeyDown: (keyEvent: ReactKeyboardEvent<HTMLDivElement>) => {
              if (keyEvent.key === 'Enter' || keyEvent.key === ' ') {
                keyEvent.preventDefault();
                onNavigate(entry.path);
              }
            },
          }
        : {})}
    >
      <div className="shrink-0 text-muted-foreground">
        {entry.isDir ? (
          isMount ? (
            <IconDatabase className="h-4 w-4 text-info" />
          ) : (
            <IconFolder className="h-4 w-4 text-warning" />
          )
        ) : (
          <IconFile className="h-4 w-4" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        {entry.isDir ? (
          <span className="font-mono text-xs text-foreground">{entry.name}</span>
        ) : (
          <PlainButton
            className="font-mono text-xs text-foreground hover:underline"
            onClick={(clickEvent) => {
              clickEvent.stopPropagation();
              onAction('view', entry);
            }}
          >
            {entry.name}
          </PlainButton>
        )}
        {isMount && (
          <Badge variant="outline" className="ml-2 border-info/30 px-1 py-0 text-[10px] text-info">
            {mountLabel}
          </Badge>
        )}
        {entry.linkTarget && (
          <span className="ml-2 font-mono text-xs text-muted-foreground">-&gt; {entry.linkTarget}</span>
        )}
      </div>
      <div className="hidden w-24 shrink-0 sm:block">
        {!entry.isDir && entry.size > 0 && (
          <div className="h-1.5 w-full rounded-full bg-muted">
            <div className="h-1.5 w-(--w) rounded-full bg-primary/60" style={cssVars({ w: `${barWidth}%` })} />
          </div>
        )}
      </div>
      <div className="w-20 shrink-0 text-right font-mono text-xs text-muted-foreground">
        {entry.isDir ? '-' : formatBytes(entry.size)}
      </div>
      <div className="w-20 shrink-0 text-right font-mono text-xs text-muted-foreground">{entry.mode}</div>
      <div className="flex w-48 shrink-0 items-center justify-end gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
        {!entry.isDir && (
          <>
            <ActionButton
              icon={<IconEye className="h-3.5 w-3.5" />}
              label={t('files.view')}
              onClick={() => onAction('view', entry)}
            />
            <ActionButton
              icon={<IconEdit className="h-3.5 w-3.5" />}
              label={t('files.edit')}
              permission={writePermission}
              onClick={() => onAction('edit', entry)}
              disabled={readOnly}
            />
            <ActionButton
              icon={<IconDownload className="h-3.5 w-3.5" />}
              label={t('files.download')}
              onClick={() => onAction('download', entry)}
            />
          </>
        )}
        <ActionButton
          icon={<IconPencil className="h-3.5 w-3.5" />}
          label={t('files.rename')}
          permission={writePermission}
          onClick={() => onAction('rename', entry)}
          disabled={readOnly}
        />
        <ActionButton
          icon={<IconLock className="h-3.5 w-3.5" />}
          label={t('files.changePermissions')}
          permission={writePermission}
          onClick={() => onAction('chmod', entry)}
          disabled={readOnly}
        />
        <ActionButton
          className="text-destructive"
          icon={<IconTrash className="h-3.5 w-3.5" />}
          label={t('files.delete')}
          permission={writePermission}
          onClick={() => onAction('delete', entry)}
          disabled={readOnly}
        />
      </div>
      <div className="w-3 shrink-0">
        {entry.isDir && (
          <IconChevronRight className="h-3 w-3 text-muted-foreground opacity-0 group-hover:opacity-100" />
        )}
      </div>
    </div>
  );
}
