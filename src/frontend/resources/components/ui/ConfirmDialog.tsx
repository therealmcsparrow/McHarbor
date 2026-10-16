// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useRef, type ReactNode } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from './Dialog';
import { Button } from './Button';

type ConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  confirmLabel?: string;
  variant?: 'destructive' | 'default' | 'caution';
  loading?: boolean;
  /** Disable the confirm button (e.g. while waiting on a form
   *  input or async check before allowing the destructive action). */
  disabled?: boolean;
  permission?: string | string[];
  onConfirm: () => void;
  children?: React.ReactNode;
};

export function ConfirmDialog({
  open: isOpen,
  onOpenChange,
  title,
  description,
  confirmLabel,
  variant = 'destructive',
  loading = false,
  disabled = false,
  permission,
  onConfirm,
  children,
}: ConfirmDialogProps) {
  const { t } = useTranslation('common');
  const resolvedConfirmLabel = confirmLabel ?? t('actions.remove');
  const returnFocusRef = useRef<HTMLElement | null>(null);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent
        onOpenAutoFocus={() => {
          returnFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        }}
        onCloseAutoFocus={(evt) => {
          const target = returnFocusRef.current;
          if (!target?.isConnected) return;
          evt.preventDefault();
          requestAnimationFrame(() => target.focus());
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {children}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button variant={variant} onClick={onConfirm} disabled={disabled || loading} permission={permission}>
            {loading ? t('actions.processing') : resolvedConfirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
