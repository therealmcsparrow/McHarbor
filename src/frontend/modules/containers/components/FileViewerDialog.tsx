// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { CodeEditor } from '@resources/components/CodeEditor';
import { IconDownload } from '@tabler/icons-react';
import { useFileContent } from '../hooks/useContainerFiles';
import { useEnvironmentStore } from '@resources/stores/environment';

type FileViewerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  filePath: string;
  onEdit: () => void;
};

function detectLanguage(path: string): 'yaml' | 'javascript' | 'typescript' {
  const ext = path.split('.').pop()?.toLowerCase() ?? '';
  if (ext === 'js' || ext === 'mjs' || ext === 'cjs') return 'javascript';
  if (ext === 'ts' || ext === 'tsx' || ext === 'mts') return 'typescript';
  return 'yaml';
}

export function FileViewerDialog({
  open,
  onOpenChange,
  containerId,
  filePath,
  onEdit,
}: FileViewerDialogProps) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);
  const { data: content, isLoading, isError } = useFileContent(containerId, filePath, open);

  const handleDownload = () => {
    const params = new URLSearchParams({ path: filePath, download: 'true' });
    if (envId) params.set('env', envId);
    window.open(`/api/containers/${containerId}/files/content?${params}`, '_blank');
  };

  const fileName = filePath.split('/').pop() ?? filePath;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="font-mono text-sm">{fileName}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          {isLoading ? (
            <div className="flex h-48 items-center justify-center">
              <Spinner size="md" />
            </div>
          ) : isError ? (
            <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
              {t('files.fileTooLarge')}
            </div>
          ) : (
            open ? (
              <CodeEditor
                value={content ?? ''}
                readOnly
                language={detectLanguage(filePath)}
                minHeight="200px"
              />
            ) : null
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={handleDownload} aria-label={t('files.download')}>
            <IconDownload className="mr-1.5 h-4 w-4" />
            {t('files.download')}
          </Button>
          <Button onClick={onEdit}>{t('files.edit')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
