// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type ElementType, type ReactNode } from 'react';
import { cn } from '@resources/utils/cn';

type TextTone = 'default' | 'muted' | 'subtle' | 'primary' | 'success' | 'warning' | 'destructive';

const toneClasses: Record<TextTone, string> = {
  default: 'text-foreground',
  muted: 'text-muted-foreground',
  subtle: 'text-muted-foreground/80',
  primary: 'text-primary',
  success: 'text-success',
  warning: 'text-warning',
  destructive: 'text-destructive',
};

const sizeClasses = {
  xs: 'text-xs',
  sm: 'text-sm',
  base: 'text-base',
  lg: 'text-lg',
  xl: 'text-xl',
  '2xl': 'text-2xl',
  '3xl': 'text-3xl',
} as const;

const weightClasses = {
  normal: 'font-normal',
  medium: 'font-medium',
  semibold: 'font-semibold',
  bold: 'font-bold',
} as const;

type TextSize = keyof typeof sizeClasses;
type TextWeight = keyof typeof weightClasses;

type TextProps<T extends ElementType> = {
  as?: T;
  tone?: TextTone;
  size?: TextSize;
  weight?: TextWeight;
  truncate?: boolean;
  italic?: boolean;
  underline?: boolean;
  monospace?: boolean;
  /**
   * Render in the display face (Bricolage Grotesque) rather than the
   * interface face. For headings and the wordmark — where type is doing
   * identity work rather than being read in bulk.
   */
  display?: boolean;
  className?: string;
  children: ReactNode;
} & Omit<React.ComponentPropsWithoutRef<T>, 'children' | 'className' | 'display'>;

function createTextComponent(defaultTag: ElementType) {
  const Component = <T extends ElementType = typeof defaultTag>({
    as,
    tone = 'default',
    size = 'sm',
    weight = 'normal',
    truncate = false,
    italic = false,
    underline = false,
    monospace = false,
    display = false,
    className,
    children,
    ...rest
  }: TextProps<T>) => {
    const Tag = (as ?? defaultTag) as ElementType;
    return (
      <Tag
        className={cn(
          toneClasses[tone],
          sizeClasses[size],
          weightClasses[weight],
          truncate && 'truncate',
          italic && 'italic',
          underline && 'underline',
          monospace && 'font-mono',
          display && 'font-display',
          className,
        )}
        {...rest}
      >
        {children}
      </Tag>
    );
  };
  Component.displayName = `Text(${typeof defaultTag === 'string' ? defaultTag : 'component'})`;
  return Component;
}

const TextBlock = createTextComponent('p');
export { TextBlock as Text };
export const Heading = createTextComponent('h2');
export const Subheading = createTextComponent('h3');
export const Title = createTextComponent('h1');
export const Caption = createTextComponent('span');
export const Code = createTextComponent('code');

export type { TextProps, TextTone, TextSize, TextWeight };
