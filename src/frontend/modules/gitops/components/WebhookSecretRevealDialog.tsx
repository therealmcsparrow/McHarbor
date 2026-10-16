// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconCopy, IconEye, IconEyeOff } from '@tabler/icons-react';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
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
import { copyToClipboard } from '@resources/utils/clipboard';

type WebhookSecretRevealDialogProps = {
  repoName: string;
  secret: string | null;
  onClose: () => void;
};

/** The one place a webhook secret is ever rendered; the parent drops the value on close. */
export function WebhookSecretRevealDialog({ repoName, secret, onClose }: WebhookSecretRevealDialogProps) {
  const { t } = useTranslation('gitops');
  const [visible, setVisible] = useState(false);
  const [copied, setCopied] = useState(false);
  const [manualOpen, setManualOpen] = useState(false);

  const dismiss = () => {
    setVisible(false);
    setCopied(false);
    onClose();
  };

  const copy = async () => {
    if (!secret) return;
    const result = await copyToClipboard(secret);
    if (result.ok) setCopied(true);
    else setManualOpen(true);
  };

  return (
    <>
      <Dialog open={secret !== null} onOpenChange={(isOpen) => !isOpen && dismiss()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('webhookSecret.revealTitle', { repo: repoName })}</DialogTitle>
            <DialogDescription>{t('webhookSecret.revealDescription')}</DialogDescription>
          </DialogHeader>
          <Alert variant="warning" title={t('webhookSecret.revealWarningTitle')}>
            {t('webhookSecret.revealWarningBody')}
          </Alert>
          <FormField label={t('webhookSecret.revealLabel')}>
            <div className="flex items-center gap-2">
              <Input
                variant="outline"
                readOnly
                className="font-mono"
                type={visible ? 'text' : 'password'}
                value={secret ?? ''}
                autoComplete="off"
              />
              <Button
                variant="outline"
                size="icon"
                aria-label={visible ? t('webhookSecret.revealHide') : t('webhookSecret.revealShow')}
                onClick={() => setVisible((v) => !v)}
              >
                {visible ? <IconEyeOff className="size-4" /> : <IconEye className="size-4" />}
              </Button>
              <Button variant="outline" size="icon" aria-label={t('webhookSecret.revealCopy')} onClick={copy}>
                {copied ? <IconCheck className="size-4 text-success" /> : <IconCopy className="size-4" />}
              </Button>
            </div>
          </FormField>
          <DialogFooter>
            <Button onClick={dismiss}>{t('actions.close', { ns: 'common' })}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ManualCopyDialog
        open={manualOpen}
        onOpenChange={setManualOpen}
        value={secret ?? ''}
        title={t('clipboard.manualCopyTitle', { ns: 'common' })}
        description={t('clipboard.manualCopyDescription', { ns: 'common' })}
      />
    </>
  );
}
