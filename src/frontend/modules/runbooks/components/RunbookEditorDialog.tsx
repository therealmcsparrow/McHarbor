// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useEnvironmentStore } from '@resources/stores/environment';
import { RunbookAutomationPanel } from './RunbookAutomationPanel';
import { RunbookEditorForm } from './RunbookEditorForm';
import { useRunbookDraft } from '../hooks/useRunbookDraft';
import { useRunbook, useRunbookCatalog, useSaveRunbook } from '@resources/hooks/useRunbooks';
import type { Runbook } from '@resources/types/runbooks';

type RunbookEditorDialogProps = {
  open: boolean;
  runbookId: string | null;
  onClose: () => void;
};

type EditorBodyProps = {
  runbookId: string | null;
  loaded: Runbook | undefined;
  onDirtyChange: (dirty: boolean) => void;
  onCancel: () => void;
  onSaved: () => void;
};

function EditorBody({ runbookId, loaded, onDirtyChange, onCancel, onSaved }: EditorBodyProps) {
  const { t } = useTranslation('runbooks');
  const currentEnv = useEnvironmentStore((s) => s.currentId) ?? '';
  const catalog = useRunbookCatalog();
  const save = useSaveRunbook();
  const editor = useRunbookDraft(loaded, currentEnv);

  useEffect(() => {
    onDirtyChange(editor.dirty);
  }, [editor.dirty, onDirtyChange]);

  return (
    <DialogContent className="max-w-4xl">
      <DialogHeader>
        <DialogTitle>{runbookId ? t('editor.editTitle') : t('editor.createTitle')}</DialogTitle>
        <DialogDescription>{t('editor.dialogDescription')}</DialogDescription>
      </DialogHeader>
      <DialogBody className="flex flex-col gap-4">
        <RunbookEditorForm editor={editor} catalog={catalog.data} fallbackEnvId={currentEnv} />
        {save.isError && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
          >
            {save.error.message}
          </p>
        )}
        {loaded && <RunbookAutomationPanel runbook={loaded} />}
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>
          {t('actions.cancel', { ns: 'common' })}
        </Button>
        <Button
          permission={PERM.RUNBOOKS_MANAGE}
          disabled={save.isPending || editor.draft.name.trim() === ''}
          onClick={() => save.mutate({ id: runbookId ?? undefined, input: editor.draft }, { onSuccess: onSaved })}
        >
          {t('editor.save')}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}

export function RunbookEditorDialog({ open: isOpen, runbookId, onClose }: RunbookEditorDialogProps) {
  const { t } = useTranslation('runbooks');
  const existing = useRunbook(isOpen && runbookId ? runbookId : undefined);
  const waiting = runbookId !== null && !existing.data;
  const [dirty, setDirty] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);

  const closeDialog = () => {
    setDirty(false);
    setConfirmDiscard(false);
    onClose();
  };
  const requestClose = () => (dirty ? setConfirmDiscard(true) : closeDialog());

  return (
    <>
      <Dialog open={isOpen} onOpenChange={(next) => !next && requestClose()}>
        {isOpen && waiting && (
          <DialogContent className="max-w-4xl">
            <DialogHeader>
              <DialogTitle>{t('editor.editTitle')}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              {existing.isError ? (
                <p role="alert" className="text-sm break-words whitespace-pre-wrap text-destructive">
                  {existing.error.message}
                </p>
              ) : (
                <output className="flex justify-center py-10" aria-label={t('editor.loading')}>
                  <Spinner size="lg" />
                </output>
              )}
            </DialogBody>
          </DialogContent>
        )}
        {isOpen && !waiting && (
          <EditorBody
            key={runbookId ?? 'new'}
            runbookId={runbookId}
            loaded={existing.data}
            onDirtyChange={setDirty}
            onCancel={requestClose}
            onSaved={closeDialog}
          />
        )}
      </Dialog>
      <ConfirmDialog
        open={confirmDiscard}
        onOpenChange={setConfirmDiscard}
        title={t('editor.discardTitle')}
        description={t('editor.discardDescription')}
        confirmLabel={t('editor.discard')}
        onConfirm={closeDialog}
      />
    </>
  );
}
