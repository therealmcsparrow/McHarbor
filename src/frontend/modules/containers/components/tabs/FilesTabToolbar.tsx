// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowUp, IconUpload, IconFilePlus, IconFolderPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

type FilesTabToolbarProps = {
  currentPath: string;
  readOnly: boolean;
  /** Key the write buttons are gated on; host_self.manage on McHarbor's own host. */
  writePermission?: string | string[];
  onNavigateUp: () => void;
  onUpload: () => void;
  onCreate: (mode: 'file' | 'folder') => void;
};

export function FilesTabToolbar({
  currentPath,
  readOnly,
  writePermission,
  onNavigateUp,
  onUpload,
  onCreate,
}: FilesTabToolbarProps) {
  const { t } = useTranslation('containers');

  return (
    <div className="flex items-center gap-3">
      <Button
        variant="outline"
        size="sm"
        onClick={onNavigateUp}
        disabled={currentPath === '/'}
        aria-label={t('files.up')}
      >
        <IconArrowUp className="h-4 w-4" />
      </Button>
      <div className="flex-1 rounded-md bg-muted px-3 py-1.5 font-mono text-xs text-foreground">{currentPath}</div>
      <Button variant="outline" size="sm" onClick={onUpload} permission={writePermission} disabled={readOnly}>
        <IconUpload className="mr-1.5 h-4 w-4" />
        {t('files.upload')}
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={() => onCreate('file')}
        permission={writePermission}
        disabled={readOnly}
      >
        <IconFilePlus className="mr-1.5 h-4 w-4" />
        {t('files.newFile')}
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={() => onCreate('folder')}
        permission={writePermission}
        disabled={readOnly}
      >
        <IconFolderPlus className="mr-1.5 h-4 w-4" />
        {t('files.newFolder')}
      </Button>
    </div>
  );
}
