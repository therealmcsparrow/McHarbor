// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, type ReactNode } from 'react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { cn } from '@resources/utils/cn';

type ContextDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  children: ReactNode;
  footer?: ReactNode;
  className?: string;
};

export function ContextDrawer({
  open: isOpen,
  onOpenChange,
  title,
  description,
  children,
  footer,
  className,
}: ContextDrawerProps) {
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent
        onOpenAutoFocus={() => {
          restoreFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        }}
        onCloseAutoFocus={(evt) => {
          evt.preventDefault();
          const target = restoreFocusRef.current;
          if (target?.isConnected) {
            target.focus();
          } else {
            document.getElementById('container-detail-panel')?.focus();
          }
          restoreFocusRef.current = null;
        }}
        className={cn(
          'mc-sheet top-0 right-0 left-auto h-dvh max-h-dvh w-full max-w-none translate-x-0 translate-y-0 rounded-none sm:w-[40vw] sm:max-w-xl sm:min-w-96',
          className,
        )}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogBody>{children}</DialogBody>
        {footer ? <DialogFooter>{footer}</DialogFooter> : null}
      </DialogContent>
    </Dialog>
  );
}
