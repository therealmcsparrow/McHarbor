// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useTestChannel } from '../hooks/useNotificationChannels';

type TestChannelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channelId: string;
  channelName: string;
};

export function TestChannelDialog({ open, onOpenChange, channelId, channelName }: TestChannelDialogProps) {
  const { t } = useTranslation('settings');
  const testChannel = useTestChannel();

  function handleSend() {
    testChannel.mutate(channelId, {
      onSuccess: () => onOpenChange(false),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('communications.testDialogTitle')}</DialogTitle>
          <DialogDescription>
            {t('communications.testDialogDescription', { name: channelName })}
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common:cancel', 'Cancel')}
          </Button>
          <Button
            onClick={handleSend}
            disabled={testChannel.isPending}
          >
            {testChannel.isPending ? t('communications.sending') : t('communications.sendTest')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
