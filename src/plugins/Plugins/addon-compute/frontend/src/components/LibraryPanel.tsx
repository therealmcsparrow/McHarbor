// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { formatBytes } from '../format';
import type { ImageLibrary } from '../types';

export function LibraryPanel({ library }: { library?: ImageLibrary }) {
  const { t } = useTranslation('compute');
  if (!library) return null;

  if (!library.writable) {
    return (
      <div className="flex gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-3">
        <IconAlertTriangle className="h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
        <div className="min-w-0 text-xs">
          <p className="font-medium text-foreground">{t('images.library.unwritable')}</p>
          <p className="mt-0.5 break-all text-muted-foreground">{library.path}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-md border border-border bg-muted/40 p-3 text-xs">
      <p className="font-medium text-foreground">{t('images.library.title')}</p>
      <p className="mt-1 break-all text-muted-foreground">{t('images.library.path', { path: library.path })}</p>
      <p className="mt-0.5 break-all text-muted-foreground">
        {t('images.library.agentMount', { path: library.agentMount })}
      </p>
      <p className="mt-1 text-muted-foreground">
        {t('images.library.usage', {
          count: library.fileCount,
          size: formatBytes(library.usedBytes),
        })}
      </p>
    </div>
  );
}
