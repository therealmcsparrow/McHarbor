// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy, IconEye, IconEyeOff, IconShieldLock } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useFlash } from '@resources/hooks/useFlash';
import { toast } from 'sonner';

// TokenRevealDialog displays a freshly generated SCIM bearer token
// exactly once. Closing the dialog permanently discards the value
// from memory — the server has only the AES-encrypted ciphertext
// plus a stable hash, so we cannot recover it later.
type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  endpointName: string;
  token: string;
};

export function TokenRevealDialog({ open: isOpen, onOpenChange, endpointName, token }: Props) {
  const { t } = useTranslation('scim');
  const [revealed, setRevealed] = useState(false);
  const { flashing: copied, flash: flashCopied } = useFlash();

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(token);
      flashCopied();
      toast.success(t('token.copied'));
    } catch {
      toast.error(t('token.copyFailed'));
    }
  };

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(o) => {
        // Wipe the token from component state when the dialog
        // closes so it doesn't linger in memory.
        if (!o) setRevealed(false);
        onOpenChange(o);
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconShieldLock className="size-5" />
            {t('token.title')}
          </DialogTitle>
          <DialogDescription>{t('token.description', { name: endpointName })}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <div className="rounded-md border border-warning/40 bg-warning/10 p-3 text-xs text-warning-foreground">
            {t('token.warning')}
          </div>
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 p-2 font-mono text-xs">
            <span className="flex-1 break-all">{revealed ? token : '•'.repeat(Math.min(token.length, 48))}</span>
            <Button
              variant="ghost"
              size="icon"
              aria-label={revealed ? t('token.hide') : t('token.reveal')}
              onClick={() => setRevealed((v) => !v)}
            >
              {revealed ? <IconEyeOff className="size-4" /> : <IconEye className="size-4" />}
            </Button>
            <Button variant="ghost" size="icon" aria-label={t('token.copy')} onClick={handleCopy} disabled={!revealed}>
              <IconCopy className="size-4" />
            </Button>
          </div>
          {copied && <p className="text-xs text-success">{t('token.copiedHint')}</p>}
        </DialogBody>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('token.acknowledge')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
