// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
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
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
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

export function FileEditorDialog({ open: isOpen, onOpenChange, containerId, filePath }: FileEditorDialogProps) {
  const { t } = useTranslation('containers');
  const { data: content, isLoading } = useFileContent(containerId, filePath, isOpen);
  const saveMutation = useSaveFile(containerId);
  const [editedContent, setEditedContent] = useState(content ?? '');
  const [hasChanges, setHasChanges] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);

  const [appliedContent, setAppliedContent] = useState(content);
  if (appliedContent !== content) {
    setAppliedContent(content);
    if (content !== undefined) {
      setEditedContent(content);
      setHasChanges(false);
    }
  }

  const handleChange = (value: string) => {
    setEditedContent(value);
    setHasChanges(value !== content);
  };

  const handleSave = () => {
    saveMutation.mutate({ path: filePath, content: editedContent }, { onSuccess: () => setHasChanges(false) });
  };

  const handleClose = (nextOpen: boolean) => {
    if (!nextOpen && hasChanges) {
      setConfirmDiscard(true);
      return;
    }
    onOpenChange(nextOpen);
  };

  const fileName = filePath.split('/').pop() ?? filePath;

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="font-mono text-sm">
            {fileName}
            {hasChanges && <span className="ml-2 text-xs text-warning">*</span>}
          </DialogTitle>
        </DialogHeader>
        <DialogBody>
          {isLoading ? (
            <div className="flex h-48 items-center justify-center">
              <Spinner size="md" />
            </div>
          ) : isOpen ? (
            <CodeEditor
              value={editedContent}
              onChange={handleChange}
              language={detectLanguage(filePath)}
              minHeight="300px"
            />
          ) : null}
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
      <ConfirmDialog
        open={confirmDiscard}
        onOpenChange={setConfirmDiscard}
        title={t('files.discardTitle')}
        description={t('files.unsavedChanges')}
        confirmLabel={t('files.discardConfirm')}
        onConfirm={() => {
          setConfirmDiscard(false);
          setHasChanges(false);
          onOpenChange(false);
        }}
      />
    </Dialog>
  );
}
