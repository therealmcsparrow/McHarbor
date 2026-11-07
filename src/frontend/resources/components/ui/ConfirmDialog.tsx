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
  /** Render `disabled` as aria-disabled so Confirm stays in the tab order and its reason can be heard. */
  softDisabled?: boolean;
  /** Id of the element holding the reason Confirm is off. */
  confirmDescribedBy?: string;
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
  softDisabled = false,
  confirmDescribedBy,
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
          <Button
            variant={variant}
            className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
            onClick={() => {
              if (softDisabled && disabled) return;
              onConfirm();
            }}
            disabled={loading || (disabled && !softDisabled)}
            aria-disabled={softDisabled && disabled && !loading ? true : undefined}
            aria-describedby={confirmDescribedBy}
            permission={permission}
          >
            {loading ? t('actions.processing') : resolvedConfirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
