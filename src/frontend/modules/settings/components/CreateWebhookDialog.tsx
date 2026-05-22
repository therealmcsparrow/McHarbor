// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';

type CreateWebhookDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateWebhookDialog({ open, onOpenChange }: CreateWebhookDialogProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const queryClient = useQueryClient();

  const createWebhook = useMutation({
    mutationFn: (data: { name: string; url: string; events: string }) =>
      api.post('/webhooks', data).then(assertSuccess),
    meta: { success: t('toast.webhookCreated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['webhooks'] }),
  });

  const [whName, setWhName] = useState('');
  const [whUrl, setWhUrl] = useState('');
  const [whEvents, setWhEvents] = useState('container.start,container.stop');

  const handleCreate = () => {
    if (!whName.trim() || !whUrl.trim()) return;
    createWebhook.mutate(
      { name: whName.trim(), url: whUrl.trim(), events: whEvents.trim() },
      {
        onSuccess: () => {
          onOpenChange(false);
          setWhName('');
          setWhUrl('');
          setWhEvents('container.start,container.stop');
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('webhooks.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('webhooks.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('webhooks.nameLabel')}</Label>
            <Input
              type="text"
              value={whName}
              onChange={(e) => setWhName(e.target.value)}
              placeholder={t('webhooks.namePlaceholder')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('webhooks.urlLabel')}</Label>
            <Input
              type="text"
              value={whUrl}
              onChange={(e) => setWhUrl(e.target.value)}
              placeholder={t('webhooks.urlPlaceholder')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('webhooks.eventsLabel')}</Label>
            <Input
              type="text"
              value={whEvents}
              onChange={(e) => setWhEvents(e.target.value)}
              placeholder={t('webhooks.eventsPlaceholder')}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button
            onClick={handleCreate}
            disabled={createWebhook.isPending || !whName.trim() || !whUrl.trim()}
          >
            {createWebhook.isPending ? t('webhooks.creating') : t('webhooks.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
