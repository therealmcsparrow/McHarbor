// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type HTMLAttributes } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';

type AvatarSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

const sizeClasses = {
  xs: 'size-6 text-xs',
  sm: 'size-8 text-sm',
  md: 'size-10 text-base',
  lg: 'size-12 text-lg',
  xl: 'size-16 text-xl',
};

function getInitials(itemName: string | null | undefined): string {
  if (!itemName) return '?';
  const parts = itemName.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  const first = parts[0]!;
  const last = parts[parts.length - 1]!;
  if (parts.length === 1) return first.slice(0, 2).toUpperCase();
  return (first.charAt(0) + last.charAt(0)).toUpperCase();
}

type AvatarProps = HTMLAttributes<HTMLDivElement> & {
  src?: string;
  alt?: string;
  name?: string;
  size?: AvatarSize;
  className?: string;
};

export const Avatar = forwardRef<HTMLDivElement, AvatarProps>(
  ({ src, alt, name: itemName, size = 'md', className, ...props }, ref) => {
    const { t } = useTranslation('common');
    if (src) {
      return (
        <div
          ref={ref}
          className={cn(
            'relative inline-flex items-center justify-center overflow-hidden rounded-full',
            sizeClasses[size],
            className,
          )}
          {...props}
        >
          <img src={src} alt={alt ?? itemName ?? t('avatar.fallbackAlt')} className="size-full object-cover" />
        </div>
      );
    }

    return (
      <div
        ref={ref}
        aria-label={itemName}
        className={cn(
          'inline-flex items-center justify-center rounded-full bg-primary font-medium text-primary-foreground',
          sizeClasses[size],
          className,
        )}
        {...props}
      >
        {getInitials(itemName)}
      </div>
    );
  },
);
Avatar.displayName = 'Avatar';

export type { AvatarProps, AvatarSize };
export { getInitials };
