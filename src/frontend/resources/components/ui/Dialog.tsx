// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  Children,
  forwardRef,
  isValidElement,
  type HTMLAttributes,
  type ComponentPropsWithoutRef,
  type ElementRef,
  type ReactNode,
} from 'react';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';

const Dialog = DialogPrimitive.Root;
const DialogTrigger = DialogPrimitive.Trigger;
const DialogPortal = DialogPrimitive.Portal;
const DialogClose = DialogPrimitive.Close;

const DialogOverlay = forwardRef<
  ElementRef<typeof DialogPrimitive.Overlay>,
  ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Overlay ref={ref} className={cn('mc-scrim fixed inset-0 z-50', className)} {...props} />
));
DialogOverlay.displayName = DialogPrimitive.Overlay.displayName;

const DialogHeader = ({ className, ...props }: HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn(
      // pl/pr rather than px + pr: `padding-inline` and `padding-right` are
      // different properties, and Tailwind orders the emitted CSS by property,
      // not by the order the classes appear in the string.
      //
      // The vertical rhythm is `CardHeader`'s — a dialog is a card that
      // arrived on top of the page, not a heavier component with its own
      // density. `pr-12` clears the 28px close button plus its 12px inset.
      'shrink-0 border-b border-hairline py-3 pr-12 pl-4 sm:text-left md:pl-5',
      className,
    )}
    {...props}
  />
);

const DialogBody = ({ className, ...props }: HTMLAttributes<HTMLDivElement>) => (
  <div className={cn('min-h-0 flex-1 overflow-y-auto p-4 md:p-5', className)} {...props} />
);

const DialogFooter = ({ className, ...props }: HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn(
      // No background tint. Nothing on a page wears one — `CardFooter` is a
      // hairline and the same surface as the body — and the tinted strip was
      // what made a dialog read as a different kind of object than the cards
      // behind it.
      'flex shrink-0 items-center justify-end gap-x-2 border-t border-hairline px-4 py-3 md:px-5',
      className,
    )}
    {...props}
  />
);

function isDialogSlot(child: ReactNode, slot: typeof DialogHeader | typeof DialogBody | typeof DialogFooter) {
  return isValidElement(child) && child.type === slot;
}

const DialogContent = forwardRef<
  ElementRef<typeof DialogPrimitive.Content>,
  ComponentPropsWithoutRef<typeof DialogPrimitive.Content> & {
    hideCloseButton?: boolean;
  }
>(({ className, children, hideCloseButton = false, ...props }, ref) => {
  const { t } = useTranslation('common');
  const childArray = Children.toArray(children);
  const hasExplicitBody = childArray.some((child) => isDialogSlot(child, DialogBody));
  const structuredChildren = hasExplicitBody ? (
    children
  ) : (
    <>
      {childArray.filter((child) => isDialogSlot(child, DialogHeader))}
      {childArray.some((child) => !isDialogSlot(child, DialogHeader) && !isDialogSlot(child, DialogFooter)) ? (
        <DialogBody>
          {childArray.filter((child) => !isDialogSlot(child, DialogHeader) && !isDialogSlot(child, DialogFooter))}
        </DialogBody>
      ) : null}
      {childArray.filter((child) => isDialogSlot(child, DialogFooter))}
    </>
  );

  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        ref={ref}
        className={cn(
          // `rounded-xl` is `Card`'s radius. It was `rounded-2xl` — the only
          // one in the primitive set, with every other floating surface
          // (Popover, Select content, Tooltip) at `rounded-lg`.
          'mc-modal fixed top-[50%] left-[50%] z-50 flex max-h-[85vh] w-full max-w-lg translate-x-[-50%] translate-y-[-50%] flex-col overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-[var(--shadow-3)]',
          className,
        )}
        // Radix dismisses on three separate signals and `onInteractOutside`
        // is only the umbrella for two of them. All three are refused here:
        // a dialog in this product closes through Cancel, the X or Escape,
        // never because a click landed beside it — losing a half-filled
        // Create form to a stray click is not a behaviour worth having.
        // A call site that wants the lighter behaviour passes its own
        // handlers, which the spread below still allows.
        onInteractOutside={(evt) => evt.preventDefault()}
        onPointerDownOutside={(evt) => evt.preventDefault()}
        onFocusOutside={(evt) => evt.preventDefault()}
        {...props}
      >
        {structuredChildren}
        {!hideCloseButton && (
          <DialogPrimitive.Close className="absolute top-2.5 right-3 inline-flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <IconX className="size-4" />
            <span className="sr-only">{t('actions.close')}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
});
DialogContent.displayName = DialogPrimitive.Content.displayName;

const DialogTitle = forwardRef<
  ElementRef<typeof DialogPrimitive.Title>,
  ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Title
    ref={ref}
    className={cn('text-base leading-tight font-semibold tracking-tight text-foreground', className)}
    {...props}
  />
));
DialogTitle.displayName = DialogPrimitive.Title.displayName;

const DialogDescription = forwardRef<
  ElementRef<typeof DialogPrimitive.Description>,
  ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Description ref={ref} className={cn('mt-1 text-sm text-muted-foreground', className)} {...props} />
));
DialogDescription.displayName = DialogPrimitive.Description.displayName;

export {
  Dialog,
  DialogPortal,
  DialogOverlay,
  DialogClose,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogBody,
  DialogFooter,
  DialogTitle,
  DialogDescription,
};
