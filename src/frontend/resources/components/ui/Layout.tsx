// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type HTMLAttributes } from 'react';
import { cn } from '@resources/utils/cn';

// Box - A generic div with configurable styling.
// Defaults to `min-w-0` so flex children don't overflow their parent
// when they contain long content (text, code, etc.).
const Box = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(({ className, ...props }, ref) => (
  <div ref={ref} className={cn('min-w-0', className)} {...props} />
));
Box.displayName = 'Box';

// ---------------------------------------------------------------------------
// Static lookup maps
//
// Tailwind's JIT compiler scans source files for full class names. Strings
// assembled at runtime (e.g. `flex-${direction}`) are NOT detected, which
// means dynamic flex / grid classes silently produce no CSS in production
// builds. The maps below enumerate every supported value as a literal so
// the JIT compiler picks them up.
// ---------------------------------------------------------------------------

const FLEX_DIRECTION_CLASSES = {
  row: 'flex-row',
  col: 'flex-col',
  'row-reverse': 'flex-row-reverse',
  'col-reverse': 'flex-col-reverse',
} as const;

const FLEX_ALIGN_CLASSES = {
  start: 'items-start',
  center: 'items-center',
  end: 'items-end',
  stretch: 'items-stretch',
  baseline: 'items-baseline',
} as const;

const FLEX_JUSTIFY_CLASSES = {
  start: 'justify-start',
  center: 'justify-center',
  end: 'justify-end',
  between: 'justify-between',
  around: 'justify-around',
  evenly: 'justify-evenly',
} as const;

const GAP_CLASSES = {
  0: 'gap-0',
  0.5: 'gap-0.5',
  1: 'gap-1',
  1.5: 'gap-1.5',
  2: 'gap-2',
  2.5: 'gap-2.5',
  3: 'gap-3',
  3.5: 'gap-3.5',
  4: 'gap-4',
  5: 'gap-5',
  6: 'gap-6',
  8: 'gap-8',
  10: 'gap-10',
  12: 'gap-12',
} as const;

const GRID_COLUMNS_CLASSES = {
  1: 'grid-cols-1',
  2: 'grid-cols-2',
  3: 'grid-cols-3',
  4: 'grid-cols-4',
  5: 'grid-cols-5',
  6: 'grid-cols-6',
  7: 'grid-cols-7',
  8: 'grid-cols-8',
  9: 'grid-cols-9',
  10: 'grid-cols-10',
  11: 'grid-cols-11',
  12: 'grid-cols-12',
} as const;

// Documentation constants — these intentionally appear as literal strings
// in the source so Tailwind's content scanner registers every supported
// variant. Use the runtime maps above to compose actual class names.
const SAFE_FLEX_DIRECTIONS = Object.keys(FLEX_DIRECTION_CLASSES);
const SAFE_FLEX_ALIGNS = Object.keys(FLEX_ALIGN_CLASSES);
const SAFE_FLEX_JUSTIFIES = Object.keys(FLEX_JUSTIFY_CLASSES);
const SAFE_GAP_VALUES = Object.keys(GAP_CLASSES);
const SAFE_GRID_COLUMNS = Object.keys(GRID_COLUMNS_CLASSES);

// Resolve a `gap` value to its corresponding Tailwind class. Numeric
// values map to the static GAP_CLASSES table (0–12, including half steps
// like 1.5/2.5/3.5). Anything else is passed through unchanged so callers
// can use arbitrary Tailwind values such as "gap-[7px]".
function resolveGapClass(value: unknown): string | undefined {
  if (value === undefined || value === null) return undefined;
  // A numeric key indexes a plain object through its string form, so one
  // String() is enough here — the double cast was standing in for that.
  if ((typeof value === 'number' || typeof value === 'string') && value in GAP_CLASSES) {
    return (GAP_CLASSES as Record<string, string>)[String(value)];
  }
  // Pass through (e.g. "gap-[7px]") unchanged.
  return value as string;
}

function resolveGridColumnsClass(value: unknown): string | undefined {
  if (value === undefined || value === null) return undefined;
  if ((typeof value === 'number' || typeof value === 'string') && value in GRID_COLUMNS_CLASSES) {
    return (GRID_COLUMNS_CLASSES as Record<string, string>)[String(value)];
  }
  return value as string;
}

// ---------------------------------------------------------------------------

// Flex - A flex container with configurable alignment and spacing.
// `align` defaults to 'stretch' so that vertical stacks (the default
// `direction: 'row'` aside, columns and rows alike) fill the
// cross-axis the way most layouts expect. Call sites that need an
// explicit cross-axis start should pass `align="start"` explicitly.
type FlexProps = HTMLAttributes<HTMLDivElement> & {
  direction?: keyof typeof FLEX_DIRECTION_CLASSES;
  align?: keyof typeof FLEX_ALIGN_CLASSES;
  justify?: keyof typeof FLEX_JUSTIFY_CLASSES;
  wrap?: boolean;
  /**
   * Numeric gaps map to the static GAP_CLASSES table (0–12). For arbitrary
   * spacing, pass a Tailwind class string (e.g. `"gap-[7px]"`) directly.
   */
  gap?: keyof typeof GAP_CLASSES | (string & {}) | (number & {});
  fullWidth?: boolean;
};

const Flex = forwardRef<HTMLDivElement, FlexProps>(
  (
    {
      className,
      direction = 'row',
      align = 'stretch',
      justify = 'start',
      wrap = false,
      gap,
      fullWidth = false,
      ...props
    },
    ref,
  ) => (
    <Box
      ref={ref}
      className={cn(
        'flex',
        FLEX_DIRECTION_CLASSES[direction],
        FLEX_ALIGN_CLASSES[align],
        FLEX_JUSTIFY_CLASSES[justify],
        wrap && 'min-w-0 flex-wrap',
        fullWidth && 'w-full',
        resolveGapClass(gap),
        className,
      )}
      {...props}
    />
  ),
);
Flex.displayName = 'Flex';

// Grid - A grid container with configurable columns and spacing
type GridProps = HTMLAttributes<HTMLDivElement> & {
  /**
   * Numeric column counts map to the static GRID_COLUMNS_CLASSES table
   * (1–12). For arbitrary counts, pass a Tailwind class string directly.
   */
  columns?: keyof typeof GRID_COLUMNS_CLASSES | (string & {}) | (number & {});
  gap?: keyof typeof GAP_CLASSES | (string & {}) | (number & {});
  fullWidth?: boolean;
};

const Grid = forwardRef<HTMLDivElement, GridProps>(({ className, columns, gap, fullWidth = true, ...props }, ref) => (
  <Box
    ref={ref}
    className={cn('grid', resolveGridColumnsClass(columns), resolveGapClass(gap), fullWidth && 'w-full', className)}
    {...props}
  />
));
Grid.displayName = 'Grid';

// Stack - A convenience component for vertical or horizontal stacking.
// `align` defaults to 'stretch' so siblings line up to the full width
// of the container — pass `align="start"` to opt out.
//
// `compact` is a tight-spacing shortcut (`gap=1`) for stacked rows of
// dense text / icons where the default `gap=2` feels too loose.
type StackProps = HTMLAttributes<HTMLDivElement> & {
  direction?: 'vertical' | 'horizontal';
  gap?: keyof typeof GAP_CLASSES | (number & {});
  align?: keyof typeof FLEX_ALIGN_CLASSES;
  compact?: boolean;
  fullWidth?: boolean;
};

const Stack = forwardRef<HTMLDivElement, StackProps>(
  ({ className, direction = 'vertical', gap = 2, align = 'stretch', compact = false, fullWidth, ...props }, ref) => (
    <Flex
      ref={ref}
      direction={direction === 'vertical' ? 'col' : 'row'}
      align={align}
      gap={compact ? 1 : gap}
      className={cn(fullWidth && 'w-full', className)}
      {...props}
    />
  ),
);
Stack.displayName = 'Stack';

export {
  Box,
  Flex,
  Grid,
  Stack,
  SAFE_FLEX_DIRECTIONS,
  SAFE_FLEX_ALIGNS,
  SAFE_FLEX_JUSTIFIES,
  SAFE_GAP_VALUES,
  SAFE_GRID_COLUMNS,
};
export type { FlexProps, GridProps, StackProps };
