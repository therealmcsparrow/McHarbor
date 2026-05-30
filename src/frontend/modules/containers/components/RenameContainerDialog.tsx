// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
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
};

export function RenameContainerDialog({
  container,
  open,
  onOpenChange,
}: RenameContainerDialogProps) {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const [name, setName] = useState(container?.name ?? '');
  const renameMutation = useRenameContainer();
  const trimmedName = name.trim().replace(/^\//, '');
  const currentName = container?.name ?? '';
  const unchanged = trimmedName === currentName;

  useEffect(() => {
    if (open) {
      setName(container?.name ?? '');
    }
  }, [container?.name, open]);

  function handleSubmit() {
    if (!container || !trimmedName || unchanged) {
      return;
    }

    renameMutation.mutate(
      { id: container.id, name: trimmedName },
      { onSuccess: () => onOpenChange(false) },
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('rename.title')}</DialogTitle>
          <DialogDescription>
            {t('rename.description', { name: currentName })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2 p-4">
          <Label htmlFor="container-rename-name">{t('rename.nameLabel')}</Label>
          <Input
            id="container-rename-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            onKeyDown={(event) => event.key === 'Enter' && handleSubmit()}
            placeholder={t('rename.namePlaceholder')}
            autoFocus
          />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!trimmedName || unchanged || renameMutation.isPending}
          >
            {renameMutation.isPending ? t('rename.saving') : t('rename.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
