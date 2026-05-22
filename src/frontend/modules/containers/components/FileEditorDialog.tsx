// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
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
import { useFileContent, useSaveFile } from '../hooks/useContainerFiles';

type FileEditorDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  filePath: string;
};

function detectLanguage(path: string): 'yaml' | 'javascript' | 'typescript' {
  const ext = path.split('.').pop()?.toLowerCase() ?? '';
  if (ext === 'js' || ext === 'mjs' || ext === 'cjs') return 'javascript';
  if (ext === 'ts' || ext === 'tsx' || ext === 'mts') return 'typescript';
  return 'yaml';
}

export function FileEditorDialog({
  open,
  onOpenChange,
  containerId,
  filePath,
}: FileEditorDialogProps) {
  const { t } = useTranslation('containers');
  const { data: content, isLoading } = useFileContent(containerId, filePath, open);
  const saveMutation = useSaveFile(containerId);
  const [editedContent, setEditedContent] = useState('');
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    if (content !== undefined) {
      setEditedContent(content);
      setHasChanges(false);
    }
  }, [content]);

  const handleChange = (value: string) => {
    setEditedContent(value);
    setHasChanges(value !== content);
  };

  const handleSave = () => {
    saveMutation.mutate(
      { path: filePath, content: editedContent },
      { onSuccess: () => setHasChanges(false) }
    );
  };

  const handleClose = (nextOpen: boolean) => {
    if (!nextOpen && hasChanges) {
      if (!window.confirm(t('files.unsavedChanges'))) return;
    }
    onOpenChange(nextOpen);
  };

  const fileName = filePath.split('/').pop() ?? filePath;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="font-mono text-sm">
            {fileName}
            {hasChanges && <span className="ml-2 text-xs text-yellow-500">*</span>}
          </DialogTitle>
        </DialogHeader>
        <DialogBody>
          {isLoading ? (
            <div className="flex h-48 items-center justify-center">
              <Spinner size="md" />
            </div>
          ) : (
            open ? (
              <CodeEditor
                value={editedContent}
                onChange={handleChange}
                language={detectLanguage(filePath)}
                minHeight="300px"
              />
            ) : null
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(false)}>
            {t('edit.cancelChanges')}
          </Button>
          <Button onClick={handleSave} disabled={!hasChanges || saveMutation.isPending}>
            {saveMutation.isPending ? t('files.saving') : t('files.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
