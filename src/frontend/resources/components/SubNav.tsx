// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SubNav — a segmented rail with a pill that slides.
//
// Used for the second level of a grouped tab bar: pick the group above,
// pick the destination here. The pill moves between items rather than
// cross-fading, so the eye can follow where the selection went — one of
// the four sanctioned motions (`mc-segment` in app.css), and it stops
// moving when motion is turned down.

import { PlainButton } from '@resources/components/ui/PlainButton';
import {
  Fragment,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ComponentType,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from 'react';
import { cn } from '@resources/utils/cn';

export type SubNavItem<T extends string> = {
  id: T;
  label: string;
  icon?: ComponentType<{ className?: string }>;
  /** Rendered after the label — a count, a state dot. */
  adornment?: ReactNode;
  /**
   * A destination the viewer may not open. Rendered disabled rather than
   * dropped: a rail that changes width between roles tells the user nothing,
   * while a greyed item plus `disabledReason` tells them what to ask for.
   */
  disabled?: boolean;
  /** Tooltip text for a disabled item — already translated. */
  disabledReason?: string;
};

type SubNavProps<T extends string> = {
  items: readonly SubNavItem<T>[];
  value: T;
  onChange: (next: T) => void;
  /** Accessible name for the rail. Already translated. */
  ariaLabel: string;
  /** Wired through to each item for drag-to-reorder, when the caller wants it. */
  itemProps?: (id: T) => Record<string, unknown>;
  /**
   * Wrap each item — used to attach a context menu without SubNav needing
   * to know what a context menu is.
   */
  itemWrapper?: (id: T, node: ReactNode) => ReactNode;
  /** Prefix for each item's DOM id, so a caller can label its panel. */
  idPrefix?: string;
  /** id of the panel these items control, for aria-controls. */
  controls?: string;
  className?: string;
};

export function SubNav<T extends string>({
  items,
  value,
  onChange,
  ariaLabel,
  itemProps,
  itemWrapper,
  idPrefix = 'subnav-tab',
  controls,
  className,
}: SubNavProps<T>) {
  const listRef = useRef<HTMLDivElement>(null);
  const refs = useRef<Partial<Record<T, HTMLButtonElement>>>({});
  const [pill, setPill] = useState<{
    left: number;
    top: number;
    width: number;
    height: number;
  } | null>(null);

  // Measured with rects, not `offsetLeft`. A caller's `itemWrapper` is free to
  // wrap each button in a positioned element — `TabContextMenu` wraps it in a
  // `relative` span — and that wrapper would become the button's `offsetParent`,
  // pinning `offsetLeft` to 0 for every item and parking the pill on the far
  // left forever. Rect deltas against the rail don't care what sits in between.
  const measure = useCallback(() => {
    const el = refs.current[value];
    const list = listRef.current;
    if (!el || !list) {
      setPill(null);
      return;
    }
    const item = el.getBoundingClientRect();
    const rail = list.getBoundingClientRect();
    setPill({
      left: item.left - rail.left,
      top: item.top - rail.top,
      width: item.width,
      height: item.height,
    });
  }, [value]);

  // Measure after layout so the pill is never a frame behind the selection.
  useLayoutEffect(() => {
    measure();
  }, [measure, items]);

  // Re-measure on resize — the rail wraps, and a wrapped item moves.
  useEffect(() => {
    const list = listRef.current;
    if (!list || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => measure());
    observer.observe(list);
    return () => observer.disconnect();
  }, [measure]);

  const handleKeyDown = (evt: ReactKeyboardEvent<HTMLButtonElement>, id: T) => {
    const index = items.findIndex((item) => item.id === id);
    const direction = evt.key === 'ArrowLeft' ? -1 : evt.key === 'ArrowRight' ? 1 : 0;

    if (direction !== 0) {
      evt.preventDefault();
      // Arrow keys step over a destination the viewer cannot open, the same
      // way a disabled item is skipped by a pointer.
      let next = items[(index + direction + items.length) % items.length];
      for (let step = 1; next?.disabled && step < items.length; step++) {
        next = items[(index + direction * (step + 1) + items.length * (step + 1)) % items.length];
      }
      if (next && !next.disabled) {
        onChange(next.id);
        requestAnimationFrame(() => refs.current[next.id]?.focus());
      }
      return;
    }
    if (evt.key === 'Home' || evt.key === 'End') {
      evt.preventDefault();
      const enabled = items.filter((i) => !i.disabled);
      const next = evt.key === 'Home' ? enabled[0] : enabled.at(-1);
      if (next) {
        onChange(next.id);
        requestAnimationFrame(() => refs.current[next.id]?.focus());
      }
    }
  };

  return (
    <div
      ref={listRef}
      role="tablist"
      aria-label={ariaLabel}
      aria-orientation="horizontal"
      className={cn('relative flex flex-wrap items-center gap-0.5 rounded-lg bg-transparent p-1', className)}
    >
      {pill && (
        // The only thing `style` carries is the measurement — every design
        // value stays in Tailwind, reading the four custom properties. There
        // is no Tailwind class for "wherever the selected item happens to be".
        <span
          aria-hidden="true"
          className={cn(
            'mc-segment pointer-events-none absolute top-0 left-0 border-b-2 border-primary bg-primary/10',
            'h-[var(--mc-pill-h)] w-[var(--mc-pill-w)]',
            'translate-x-[var(--mc-pill-x)] translate-y-[var(--mc-pill-y)]',
          )}
          style={
            {
              '--mc-pill-x': `${pill.left}px`,
              '--mc-pill-y': `${pill.top}px`,
              '--mc-pill-w': `${pill.width}px`,
              '--mc-pill-h': `${pill.height}px`,
            } as CSSProperties
          }
        />
      )}
      {items.map((item) => {
        const Icon = item.icon;
        const active = item.id === value;

        // `itemProps` may carry a className of its own. Spreading it after
        // `className=` would REPLACE the computed one — React keeps the last
        // prop — which silently strips every layout class and collapses the
        // rail into overlapping text. Pull it out and merge instead.
        const { className: extraClassName, ...extraProps } = (itemProps?.(item.id) ?? {}) as Record<string, unknown> & {
          className?: string;
        };

        const button = (
          <PlainButton
            ref={(el) => {
              refs.current[item.id] = el ?? undefined;
            }}
            type="button"
            id={`${idPrefix}-${item.id}`}
            role="tab"
            aria-controls={controls}
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            disabled={item.disabled}
            aria-disabled={item.disabled || undefined}
            title={item.disabled ? item.disabledReason : undefined}
            onClick={() => {
              if (item.disabled) return;
              onChange(item.id);
            }}
            onKeyDown={(evt) => handleKeyDown(evt, item.id)}
            {...extraProps}
            className={cn(
              'relative z-10 inline-flex min-h-9 shrink-0 items-center gap-1.5 whitespace-nowrap',
              'rounded-md px-3 py-1.5 text-sm transition-colors',
              active ? 'font-semibold text-primary' : 'text-muted-foreground hover:bg-accent hover:text-foreground',
              item.disabled && 'cursor-not-allowed opacity-50 hover:text-muted-foreground',
              extraClassName,
            )}
          >
            {Icon && <Icon className="size-4" />}
            {item.label}
            {item.adornment}
          </PlainButton>
        );
        return <Fragment key={item.id}>{itemWrapper ? itemWrapper(item.id, button) : button}</Fragment>;
      })}
    </div>
  );
}
