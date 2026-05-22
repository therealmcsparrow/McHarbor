// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation, Trans } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { useRemoveContainer } from '../hooks/useContainers';

type ContainerTarget = {
  id: string;
  name: string;
  image: string;
  imageId: string;
  stackName: string | null;
};

type RemoveContainerDialogProps = {
  container: ContainerTarget | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RemoveContainerDialog({
  container,
  open,
  onOpenChange,
  onSuccess,
}: RemoveContainerDialogProps) {
  const { t } = useTranslation('containers');
  const removeMutation = useRemoveContainer();
  const [removeVolumes, setRemoveVolumes] = useState(false);
  const [removeImage, setRemoveImage] = useState(false);
  const [removeStack, setRemoveStack] = useState(false);

  // Reset toggles when dialog opens with a new container
  useEffect(() => {
    if (open) {
      setRemoveVolumes(false);
      setRemoveImage(false);
      setRemoveStack(false);
    }
  }, [open]);

  if (!container) return null;

  const handleRemove = () => {
    removeMutation.mutate(
      {
        id: container.id,
        force: true,
        removeVolumes,
        removeImage,
        removeStack: removeStack && container.stackName !== null,
      },
      {
        onSuccess: () => {
          onOpenChange(false);
          onSuccess?.();
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('removeDialog.title')}</DialogTitle>
          <DialogDescription>
            <Trans
              i18nKey="removeDialog.description"
              ns="containers"
              values={{ name: container.name }}
              components={{ bold: <span className="font-medium text-foreground" /> }}
            />
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 px-4 py-3">
          <div className="rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">{t('removeDialog.image')}</span>
              <span className="font-mono text-foreground">{container.image}</span>
            </div>
            {container.stackName && (
              <div className="mt-1.5 flex items-center justify-between border-t border-border pt-1.5">
                <span className="text-muted-foreground">{t('removeDialog.stack')}</span>
                <span className="font-mono text-foreground">{container.stackName}</span>
              </div>
            )}
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label htmlFor="remove-volumes" className="cursor-pointer">
                <div className="text-sm font-medium">{t('removeDialog.removeVolumes')}</div>
                <div className="text-xs text-muted-foreground">{t('removeDialog.removeVolumesDesc')}</div>
              </Label>
              <Switch id="remove-volumes" checked={removeVolumes} onCheckedChange={setRemoveVolumes} />
            </div>

            <div className="flex items-center justify-between">
              <Label htmlFor="remove-image" className="cursor-pointer">
                <div className="text-sm font-medium">{t('removeDialog.removeImage')}</div>
                <div className="text-xs text-muted-foreground">{t('removeDialog.removeImageDesc')}</div>
              </Label>
              <Switch id="remove-image" checked={removeImage} onCheckedChange={setRemoveImage} />
            </div>

            {container.stackName && (
              <div className="flex items-center justify-between">
                <Label htmlFor="remove-stack" className="cursor-pointer">
                  <div className="text-sm font-medium">{t('removeDialog.removeStack')}</div>
                  <div className="text-xs text-muted-foreground">{t('removeDialog.removeStackDesc')}</div>
                </Label>
                <Switch id="remove-stack" checked={removeStack} onCheckedChange={setRemoveStack} />
              </div>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button variant="destructive" onClick={handleRemove} disabled={removeMutation.isPending}>
            {removeMutation.isPending ? t('removeDialog.removing') : t('actions.remove')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
