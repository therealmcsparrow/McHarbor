// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { useAdoptPreview, useAdoptStack } from '@resources/hooks/useTakeOver';

type TakeOverDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  stackName?: string;
  containerId?: string;
  containerName?: string;
};

export function TakeOverDialog({
  open: isOpen,
  onOpenChange,
  stackName,
  containerId,
  containerName,
}: TakeOverDialogProps) {
  const { t } = useTranslation('stacks');
  const preview = useAdoptPreview();
  const adopt = useAdoptStack();
  const [itemName, setName] = useState('');
  const [compose, setCompose] = useState('');
  const [description, setDescription] = useState('');

  const isStandalone = !!containerId;

  useEffect(() => {
    if (!isOpen) return;
    setCompose('');
    setDescription('');

    if (stackName) {
      setName(stackName);
      preview.mutate({ stackName });
    } else if (containerId) {
      setName(containerName ?? '');
      preview.mutate({ containerId });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, stackName, containerId]);

  useEffect(() => {
    if (preview.data) {
      setCompose(preview.data.compose);
      if (!itemName && preview.data.name) {
        setName(preview.data.name);
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preview.data]);

  const handleSubmit = () => {
    if (!itemName.trim() || !compose.trim()) return;

    adopt.mutate(
      {
        name: itemName.trim(),
        compose: compose.trim(),
        description: description.trim() || undefined,
        containerId: containerId || undefined,
      },
      {
        onSuccess: () => {
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('takeOver.title')}</DialogTitle>
          <DialogDescription>
            {isStandalone ? t('takeOver.standaloneDescription') : t('takeOver.description')}
          </DialogDescription>
        </DialogHeader>

        {preview.isPending ? (
          <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
            <Spinner size="sm" />
            {t('takeOver.generating')}
          </div>
        ) : (
          <div className="space-y-3">
            <div>
              <Label className="mb-2">{t('takeOver.name')}</Label>
              <Input
                type="text"
                value={itemName}
                onChange={(evt) => setName(evt.target.value)}
                disabled={!!stackName}
              />
            </div>
            <div>
              <Label className="mb-2">{t('takeOver.compose')}</Label>
              <textarea
                value={compose}
                onChange={(evt) => setCompose(evt.target.value)}
                rows={16}
                className="block w-full rounded-lg border border-border bg-card px-4 py-2.5 font-mono text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary"
              />
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={adopt.isPending || preview.isPending || !itemName.trim() || !compose.trim()}
          >
            {adopt.isPending ? t('takeOver.adopting') : t('takeOver.adopt')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
