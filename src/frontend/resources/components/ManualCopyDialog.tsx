// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';
import { Button } from './ui/Button';
import { Input } from './ui/Input';

type ManualCopyDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: string;
  title?: string;
  description?: string;
};

/**
 * Shown when the browser refuses programmatic clipboard writes — typically
 * because McHarbor is served over plain HTTP on a non-loopback address
 * (LAN install, homelab IP) where both `navigator.clipboard.writeText` and
 * `document.execCommand('copy')` are unreliable. Renders the value in a
 * read-only input that auto-selects on open so the operator can press
 * Ctrl/Cmd+C. The "Select & copy" button re-attempts the legacy
 * `execCommand` path from this dialog's user gesture, which is the only
 * place it might still succeed.
 */
export function ManualCopyDialog({
  open,
  onOpenChange,
  value,
  title,
  description,
}: ManualCopyDialogProps) {
  const { t } = useTranslation('common');
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open) {
      setCopied(false);
      return;
    }
    const id = window.setTimeout(() => {
      inputRef.current?.select();
    }, 50);
    return () => window.clearTimeout(id);
  }, [open]);

  function handleCopyClick() {
    const input = inputRef.current;
    if (!input) return;
    input.focus();
    input.select();
    input.setSelectionRange(0, value.length);
    try {
      const ok = document.execCommand('copy');
      if (ok) {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      }
    } catch {
      setCopied(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{title ?? t('clipboard.manualCopyTitle')}</DialogTitle>
          <DialogDescription>
            {description ?? t('clipboard.manualCopyDescription')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Input
            ref={inputRef}
            value={value}
            readOnly
            onFocus={(e) => e.currentTarget.select()}
            onClick={(e) => e.currentTarget.select()}
            className="font-mono text-xs"
          />
          <p className="text-xs text-muted-foreground">
            {t('clipboard.manualCopyHint')}
          </p>
        </div>

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={handleCopyClick}>
            {copied ? t('clipboard.copied') : t('clipboard.selectAndCopy')}
          </Button>
          <Button onClick={() => onOpenChange(false)}>
            {t('clipboard.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}