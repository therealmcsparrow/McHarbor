// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
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
import { useStackCompose } from '../hooks/useStacks';
import type { StackInfo } from '../hooks/useStacks';

type EditStackDialogProps = {
  stack: StackInfo | null;
  onClose: () => void;
  onSave: (compose: string) => void;
  saving: boolean;
};

export function EditStackDialog({ stack, onClose, onSave, saving }: EditStackDialogProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const { data: content, isLoading } = useStackCompose(stack?.name ?? null);
  const [editContent, setEditContent] = useState('');

  useEffect(() => {
    if (content !== undefined) setEditContent(content);
  }, [content]);

  return (
    <Dialog open={stack !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('edit.title', { name: stack?.name })}</DialogTitle>
          <DialogDescription>
            {t('edit.description')}
          </DialogDescription>
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
            className="py-2.5 px-4 block w-full bg-card border border-border rounded-lg font-mono text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary"
          />
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {tc('actions.cancel')}
          </Button>
          <Button onClick={() => onSave(editContent)} disabled={saving || isLoading}>
            {saving ? t('edit.saving') : t('edit.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
