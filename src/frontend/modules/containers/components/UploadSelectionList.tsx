// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { formatBytes } from '@resources/utils/format';
import type { UploadSelection } from './upload-dialog-utils';

type UploadSelectionListProps = {
  selection: UploadSelection;
};

export function UploadSelectionList({ selection }: UploadSelectionListProps) {
  const { t } = useTranslation('containers');

  if (selection.files.length === 0) return null;

  return (
    <div className="max-h-32 overflow-auto rounded-md border border-border bg-muted/30 p-2">
      {selection.files.slice(0, 8).map((item) => (
        <div key={item.path} className="flex items-center justify-between gap-3 py-1 text-xs">
          <span className="min-w-0 truncate font-mono text-foreground">{item.path}</span>
          <span className="shrink-0 text-muted-foreground">{formatBytes(item.file.size)}</span>
        </div>
      ))}
      {selection.files.length > 8 && (
        <p className="py-1 text-xs text-muted-foreground">
          {t('files.moreSelected', { count: selection.files.length - 8 })}
        </p>
      )}
    </div>
  );
}
