// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { FormField } from '@resources/components/ui/FormField';
import { Input } from '@resources/components/ui/Input';
import { useDestroyPRPreview } from '../hooks/useGitOps';
import type { GitOpsPRPreview } from '../types';

type DestroyPreviewDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview: GitOpsPRPreview | null;
};

export function DestroyPreviewDialog({ open: isOpen, onOpenChange, preview }: DestroyPreviewDialogProps) {
  const { t } = useTranslation('gitops');
  const destroy = useDestroyPRPreview(t);

  const [typed, setTyped] = useState('');
  const { reset } = destroy;
  const [wasOpen, setWasOpen] = useState(isOpen);
  if (isOpen !== wasOpen) {
    setWasOpen(isOpen);
    if (!isOpen) setTyped('');
  }
  useEffect(() => {
    if (isOpen) reset();
  }, [isOpen, reset]);

  const expected = preview ? `PR #${preview.prNumber}` : '';
  const confirmed = typed.trim() === expected;

  const handleDestroy = () => {
    if (!preview || !confirmed) return;
    destroy.mutate(preview.id, { onSuccess: () => onOpenChange(false) });
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('destroyPreview.title', { pr: preview?.prNumber ?? '' })}</DialogTitle>
          <DialogDescription>{t('destroyPreview.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Alert variant="destructive" title={t('destroyPreview.warningTitle')}>
            {t('destroyPreview.warningBody')}
          </Alert>
          <FormField
            label={t('destroyPreview.confirmLabel')}
            description={t('destroyPreview.confirmHint', { phrase: expected })}
          >
            <Input
              variant="outline"
              value={typed}
              onChange={(evt) => setTyped(evt.target.value)}
              placeholder={expected}
            />
          </FormField>
          {destroy.isError && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {destroy.error.message}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.GITOPS_DELETE}
            variant="destructive"
            onClick={handleDestroy}
            disabled={!confirmed || destroy.isPending}
          >
            {t('destroyPreview.action')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
