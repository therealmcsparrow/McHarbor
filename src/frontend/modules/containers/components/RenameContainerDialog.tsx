// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { useRenameContainer } from '../hooks/useContainers';

type RenameContainerTarget = {
  id: string;
  name: string;
};

type RenameContainerDialogProps = {
  container: RenameContainerTarget | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  unlockProtected?: boolean;
};

export function RenameContainerDialog({
  container,
  open: isOpen,
  onOpenChange,
  unlockProtected = false,
}: RenameContainerDialogProps) {
  const { t } = useTranslation('containers');
  const [itemName, setName] = useState(container?.name ?? '');
  const renameMutation = useRenameContainer();
  const trimmedName = itemName.trim().replace(/^\//, '');
  const currentName = container?.name ?? '';
  const unchanged = trimmedName === currentName;

  const resetKey = `${isOpen}|${container?.name ?? ''}`;
  const [appliedResetKey, setAppliedResetKey] = useState(resetKey);
  if (appliedResetKey !== resetKey) {
    setAppliedResetKey(resetKey);
    if (isOpen) {
      setName(container?.name ?? '');
    }
  }

  function handleSubmit() {
    if (!container || !trimmedName || unchanged) {
      return;
    }

    renameMutation.mutate(
      { id: container.id, name: trimmedName, unlockProtected },
      { onSuccess: () => onOpenChange(false) },
    );
  }

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('rename.title')}</DialogTitle>
          <DialogDescription>{t('rename.description', { name: currentName })}</DialogDescription>
        </DialogHeader>

        <div className="space-y-2 p-4">
          <Label htmlFor="container-rename-name">{t('rename.nameLabel')}</Label>
          <Input
            id="container-rename-name"
            value={itemName}
            onChange={(changeEvent) => setName(changeEvent.target.value)}
            onKeyDown={(evt) => evt.key === 'Enter' && handleSubmit()}
            placeholder={t('rename.namePlaceholder')}
          />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button onClick={handleSubmit} disabled={!trimmedName || unchanged || renameMutation.isPending}>
            {renameMutation.isPending ? t('rename.saving') : t('rename.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
