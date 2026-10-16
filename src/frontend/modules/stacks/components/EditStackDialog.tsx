// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { useStackCompose } from '@resources/hooks/useStacks';
import type { StackInfo } from '@resources/hooks/useStacks';
import { PERM } from '@core/auth/permissionKeys';

type EditStackDialogProps = {
  stack: StackInfo | null;
  onClose: () => void;
  onSave: (compose: string) => void;
  saving: boolean;
};

export function EditStackDialog({ stack, onClose, onSave, saving }: EditStackDialogProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const cancelLabel = tc('actions.cancel');
  const { data: content, isLoading } = useStackCompose(stack?.name ?? null);
  const [editContent, setEditContent] = useState(content ?? '');
  const [appliedContent, setAppliedContent] = useState(content);
  if (appliedContent !== content) {
    setAppliedContent(content);
    if (content !== undefined) setEditContent(content);
  }

  return (
    <Dialog open={stack !== null} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('edit.title', { name: stack?.name })}</DialogTitle>
          <DialogDescription>{t('edit.description')}</DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            {t('edit.loading')}
          </div>
        ) : (
          <textarea
            value={editContent}
            onChange={(e) => setEditContent(e.target.value)}
            rows={20}
            className="block w-full rounded-lg border border-border bg-card px-4 py-2.5 font-mono text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary"
          />
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {cancelLabel}
          </Button>
          <Button permission={PERM.STACKS_MANAGE} onClick={() => onSave(editContent)} disabled={saving || isLoading}>
            {saving ? t('edit.saving') : t('edit.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
