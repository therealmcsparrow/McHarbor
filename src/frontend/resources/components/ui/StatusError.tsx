// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertCircleFilled, IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from './Button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from './Dialog';
import { copyToClipboard } from '@resources/utils/clipboard';
import { useFlash } from '@resources/hooks/useFlash';
import { cn } from '@resources/utils/cn';

export type StatusErrorProps = {
  error?: string | null;
  title?: string;
  description?: string;
  className?: string;
};

export function StatusError({ error, title, description, className }: StatusErrorProps) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);
  const { flashing: copied, flash: flashCopied } = useFlash();

  const message = error?.trim() ?? '';
  if (message === '') return null;

  function handleCopy() {
    void copyToClipboard(message).then((result) => {
      if (!result.ok) return;
      flashCopied();
    });
  }

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        className={cn('size-6 text-destructive hover:text-destructive', className)}
        aria-label={t('statusError.view')}
        onClick={() => setOpen(true)}
      >
        <IconAlertCircleFilled className="size-4" aria-hidden="true" />
      </Button>

      <Dialog open={isOpen} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{title ?? t('statusError.title')}</DialogTitle>
            <DialogDescription>{description ?? t('statusError.description')}</DialogDescription>
          </DialogHeader>
          <pre className="max-h-[50vh] overflow-auto rounded-md border border-border bg-muted p-3 text-xs break-words whitespace-pre-wrap text-foreground">
            {message}
          </pre>
          <DialogFooter>
            <Button variant="outline" onClick={handleCopy}>
              {copied ? (
                <IconCheck className="mr-1 size-3.5" aria-hidden="true" />
              ) : (
                <IconCopy className="mr-1 size-3.5" aria-hidden="true" />
              )}
              {copied ? t('clipboard.copied') : t('clipboard.copy')}
            </Button>
            <Button onClick={() => setOpen(false)}>{t('actions.close')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
