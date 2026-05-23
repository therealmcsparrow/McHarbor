// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
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
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { formatBytes } from '@resources/utils/format';
import type { FileEntry } from '@core/types/docker';

type FilesTabRowProps = {
  entry: FileEntry;
  isMount: boolean;
  mountLabel: string;
  onAction: (action: string, entry: FileEntry) => void;
  onNavigate: (path: string) => void;
};

export function FilesTabRow({
  entry,
  isMount,
  mountLabel,
  onAction,
  onNavigate,
}: FilesTabRowProps) {
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
          isMount ? <IconDatabase className="h-4 w-4 text-blue-400" /> : <IconFolder className="h-4 w-4 text-yellow-500" />
        ) : (
          <IconFile className="h-4 w-4" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <span
          className={`font-mono text-xs text-foreground ${!entry.isDir ? 'cursor-pointer hover:underline' : ''}`}
          onClick={(event) => {
            if (!entry.isDir) {
              event.stopPropagation();
              onAction('view', entry);
            }
          }}
        >
          {entry.name}
        </span>
        {isMount && (
          <Badge variant="outline" className="ml-2 border-blue-400/30 px-1 py-0 text-[10px] text-blue-400">
            {mountLabel}
          </Badge>
        )}
        {entry.linkTarget && (
          <span className="ml-2 font-mono text-xs text-muted-foreground">-&gt; {entry.linkTarget}</span>
        )}
      </div>
      <div className="hidden w-24 sm:block">
        {!entry.isDir && entry.size > 0 && (
          <div className="h-1.5 w-full rounded-full bg-muted">
            <div className="h-1.5 rounded-full bg-primary/60" style={{ width: `${barWidth}%` }} />
          </div>
        )}
      </div>
      <div className="w-20 text-right font-mono text-xs text-muted-foreground">
        {entry.isDir ? '-' : formatBytes(entry.size)}
      </div>
      <div className="w-20 text-right font-mono text-xs text-muted-foreground">{entry.mode}</div>
      <div className="flex w-24 items-center justify-end gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
        {!entry.isDir && (
          <>
            <ActionButton icon={IconEye} label={t('files.view')} onClick={() => onAction('view', entry)} />
            <ActionButton icon={IconEdit} label={t('files.edit')} onClick={() => onAction('edit', entry)} />
            <ActionButton icon={IconDownload} label={t('files.download')} onClick={() => onAction('download', entry)} />
          </>
        )}
        <ActionButton icon={IconPencil} label={t('files.rename')} onClick={() => onAction('rename', entry)} />
        <ActionButton icon={IconLock} label={t('files.changePermissions')} onClick={() => onAction('chmod', entry)} />
        <ActionButton
          className="text-destructive"
          icon={IconTrash}
          label={t('files.delete')}
          onClick={() => onAction('delete', entry)}
        />
      </div>
      {entry.isDir && <IconChevronRight className="h-3 w-3 text-muted-foreground opacity-0 group-hover:opacity-100" />}
    </div>
  );
}

function ActionButton({
  className,
  icon: Icon,
  label,
  onClick,
}: {
  className?: string;
  icon: typeof IconEye;
  label: string;
  onClick: () => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className={`h-6 w-6 ${className ?? ''}`.trim()}
          aria-label={label}
          onClick={(event) => {
            event.stopPropagation();
            onClick();
          }}
        >
          <Icon className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
