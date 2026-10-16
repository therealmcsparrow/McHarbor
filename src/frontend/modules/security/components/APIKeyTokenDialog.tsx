// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { useFlash } from '@resources/hooks/useFlash';
import { copyToClipboard } from '@resources/utils/clipboard';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import type { CreateAPIKeyResult } from '../hooks/useAPIKeys';

type APIKeyTokenDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  keyResult: CreateAPIKeyResult;
};

export function APIKeyTokenDialog({ open: isOpen, onOpenChange, keyResult }: APIKeyTokenDialogProps) {
  const { t } = useTranslation('security');
  const { flashing: copied, flash: flashCopied } = useFlash();
  const [manualOpen, setManualOpen] = useState(false);

  const handleCopy = async () => {
    const result = await copyToClipboard(keyResult.key);
    if (!result.ok) {
      setManualOpen(true);
      return;
    }
    flashCopied();
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('apiKeys.createKey')}</DialogTitle>
          <DialogDescription>{t('apiKeys.keyWarning')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-2">
          <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/50 p-3">
            <code className="flex-1 font-mono text-sm break-all">{keyResult.key}</code>
            <Button variant="outline" size="sm" onClick={handleCopy} aria-label={t('apiKeys.copyKey')}>
              <IconCopy className="size-4" />
            </Button>
          </div>
          {copied && <p className="text-sm text-success">{t('apiKeys.keyCopied')}</p>}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('actions.close', { ns: 'common' })}</Button>
        </DialogFooter>
      </DialogContent>

      <ManualCopyDialog open={manualOpen} onOpenChange={setManualOpen} value={keyResult.key} />
    </Dialog>
  );
}
