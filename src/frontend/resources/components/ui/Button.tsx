// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type ButtonHTMLAttributes, type ForwardedRef, type ReactElement } from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { Tooltip, TooltipProvider, TooltipTrigger, TooltipContent, useInsideTooltipTrigger } from './Tooltip';
import { TooltipFocusTarget } from './TooltipFocusTarget';

// Hover and press feedback lives here, per variant, rather than in a blanket
// `button:hover { box-shadow: 0 0 8px … }` rule in app.css.
//
// That rule was deleted: with a zero offset it was a halo rather than a
// depth cue, and it fired identically on a ghost icon button in a table row
// and on the primary deploy action — so it told you an element was
// interactive, which the cursor already did, and nothing about how much the
// element mattered.
//
// What replaces it is background only, in two steps: a light shift on hover
// and a deeper one while held. No rise and no shadow — the surfaces around
// these buttons are flat, and a button that lifts off a flat panel reads as
// a different material rather than as the same one responding.
const buttonVariants = cva(
  'inline-flex items-center justify-center gap-x-2 rounded-lg text-sm font-medium whitespace-nowrap transition-[color,background-color,border-color] duration-150 focus:outline-none disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:bg-primary/90 active:bg-primary/80',
        destructive: 'bg-danger/15 text-danger hover:bg-danger/25 active:bg-danger/35',
        caution: 'bg-warning/15 text-warning hover:bg-warning/25 active:bg-warning/35',
        outline: 'mc-glass text-foreground hover:bg-muted active:bg-accent',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80 active:bg-secondary/65',
        ghost: 'text-foreground hover:bg-muted active:bg-accent',
        link: 'text-primary underline-offset-4 hover:underline active:text-primary/80',
      },
      // Height comes from `.mc-control` so buttons line up with selects and
      // inputs; the size variants carry horizontal density and type only.
      // `lg` opts out on purpose — it exists to be larger.
      size: {
        default: 'mc-control px-4',
        sm: 'mc-control px-3 text-xs',
        lg: 'px-5 py-3',
        icon: 'mc-control aspect-square p-0',
        'icon-sm': 'mc-control aspect-square p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
    /**
     * Permission key, or any-of list, required to use this button.
     *
     * When the caller lacks it the button renders disabled and dimmed with a
     * tooltip naming the missing permission, rather than vanishing — a table's
     * action column then keeps its width across roles. Omit the prop and the
     * button behaves exactly as it always has.
     *
     * This is a UX signal, not a security boundary: the endpoint behind the
     * button enforces the same permission on every request.
     */
    permission?: string | string[];
    /**
     * Tooltip text for an icon-only button.
     *
     * An icon button carries its name in `aria-label`, which a screen reader
     * announces and a sighted pointer user never sees — so the default is the
     * `aria-label`, and every icon button gets a tooltip without its call site
     * asking. Pass a string to say something different, or `false` where the
     * icon is genuinely self-evident and a tooltip would only add noise.
     *
     * Ignored on a button with visible text: the label is already on screen.
     */
    tooltip?: string | false;
  };

function BaseButton(
  { className, variant, size, asChild = false, ...props }: Omit<ButtonProps, 'permission'>,
  ref: ForwardedRef<HTMLButtonElement>,
) {
  const Comp = asChild ? Slot : 'button';
  return <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />;
}

const BaseButtonForwarded = forwardRef<HTMLButtonElement, Omit<ButtonProps, 'permission'>>(BaseButton);
BaseButtonForwarded.displayName = 'BaseButton';

/**
 * The gated path. Split into its own component so `usePermissionGate` — and
 * through it TanStack Query — is only ever called when a caller actually asked
 * for a gate. A plain `<Button>` stays a leaf with no hooks, which keeps it
 * renderable outside a QueryClientProvider (Ladle stories, error fallbacks).
 */
const GatedButton = forwardRef<HTMLButtonElement, ButtonProps & { permission: string | string[] }>(
  ({ permission, onClick, asChild, ...props }, ref) => {
    const { denied, reason } = usePermissionGate(permission);

    if (!denied) {
      return <BaseButtonForwarded ref={ref} asChild={asChild} onClick={onClick} {...props} />;
    }

    // A disabled button fires no pointer events (`disabled:pointer-events-none`
    // above), so the tooltip that explains *why* it is dead would never open.
    // The focusable span restores it for pointer and keyboard alike — the same
    // fix HeaderActionButton has carried for its own disabled states.
    //
    // `asChild` is dropped here on purpose: a denied control is rendered as a
    // real disabled <button>, never as the link or menu item it would
    // otherwise become, so there is no href left to follow.
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <TooltipFocusTarget>
            <BaseButtonForwarded ref={ref} {...props} disabled aria-disabled="true" />
          </TooltipFocusTarget>
        </TooltipTrigger>
        <TooltipContent>{reason}</TooltipContent>
      </Tooltip>
    );
  },
);
GatedButton.displayName = 'GatedButton';

/** The icon sizes, which render no visible label. */
const ICON_SIZES = new Set(['icon', 'icon-sm']);

/**
 * TooltipOnIcon gives an icon-only button the tooltip its aria-label implies.
 *
 * It declines in three cases, each for a reason: the caller already wrapped the
 * button in a tooltip of their own (nesting one Radix tooltip inside another's
 * trigger opens neither), `asChild` means the button is being rendered as
 * something else whose own composition owns the wrapper, and `tooltip={false}`
 * is an explicit opt-out.
 */
function TooltipOnIcon({
  size,
  tooltip,
  asChild,
  ariaLabel,
  children,
}: {
  size: ButtonProps['size'];
  tooltip: ButtonProps['tooltip'];
  asChild: boolean;
  ariaLabel: string | undefined;
  children: ReactElement;
}) {
  const alreadyWrapped = useInsideTooltipTrigger();
  const text = tooltip === undefined ? ariaLabel : tooltip;

  if (tooltip === false || asChild || alreadyWrapped || !text || !ICON_SIZES.has(String(size))) {
    return children;
  }
  // Its own provider, because Radix's Tooltip throws without one and a plain
  // <Button> is deliberately renderable anywhere — Ladle stories, error
  // fallbacks, a plugin bundle mounted outside the app shell. A nested provider
  // is supported and wins for its subtree; the only thing given up is
  // cross-button hover chaining, which no call site relied on before because
  // none of these buttons had a tooltip at all.
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{children}</TooltipTrigger>
        <TooltipContent>{text}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(({ permission, tooltip, ...props }, ref) => {
  const inner =
    permission === undefined ? (
      <BaseButtonForwarded ref={ref} {...props} />
    ) : (
      <GatedButton ref={ref} permission={permission} {...props} />
    );
  return (
    <TooltipOnIcon size={props.size} tooltip={tooltip} asChild={props.asChild ?? false} ariaLabel={props['aria-label']}>
      {inner}
    </TooltipOnIcon>
  );
});
Button.displayName = 'Button';

export { Button, buttonVariants };
export type { ButtonProps };
