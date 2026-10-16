// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, useState, type InputHTMLAttributes } from 'react';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { PlainButton } from './PlainButton';

type PasswordInputSize = 'sm' | 'md' | 'lg';

const sizeClasses: Record<PasswordInputSize, string> = {
  sm: 'mc-control pl-3 pr-8 text-xs',
  md: 'mc-control pl-4 pr-10 text-sm',
  lg: 'mc-control pl-5 pr-12 text-base',
};

const iconButtonSize: Record<PasswordInputSize, string> = {
  sm: 'size-3 right-2',
  md: 'size-4 right-2.5',
  lg: 'size-5 right-3',
};

const toggleSize: Record<PasswordInputSize, string> = {
  sm: 'size-6 right-1',
  md: 'size-6 right-1',
  lg: 'size-6 right-1',
};

type PasswordInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'type'> & {
  size?: PasswordInputSize;
  /** Default visibility. Defaults to hidden. */
  defaultVisible?: boolean;
  /** Persist the toggle state. */
  visible?: boolean;
  onVisibleChange?: (visible: boolean) => void;
};

const PasswordInput = forwardRef<HTMLInputElement, PasswordInputProps>(
  ({ className, size = 'md', defaultVisible = false, visible: visibleProp, onVisibleChange, ...props }, ref) => {
    const { t } = useTranslation('common');
    const [internalVisible, setInternalVisible] = useState(defaultVisible);
    const isControlled = visibleProp !== undefined;
    const visible = isControlled ? visibleProp : internalVisible;
    const toggle = () => {
      const next = !visible;
      if (!isControlled) setInternalVisible(next);
      onVisibleChange?.(next);
    };

    return (
      <div className={cn('relative w-full', className)}>
        <input
          ref={ref}
          type={visible ? 'text' : 'password'}
          autoComplete={props.autoComplete ?? 'current-password'}
          className={cn(
            'mc-field w-full rounded-lg border text-foreground placeholder:text-muted-foreground',
            'disabled:pointer-events-none disabled:opacity-50',
            sizeClasses[size],
          )}
          {...props}
        />
        <PlainButton
          type="button"
          onClick={toggle}
          aria-label={visible ? t('passwordInput.hide') : t('passwordInput.show')}
          aria-pressed={visible}
          className={cn(
            'absolute top-1/2 inline-flex -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground/70 transition-colors',
            'hover:bg-muted/60 hover:text-foreground',
            'focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
            toggleSize[size],
          )}
        >
          {visible ? <IconEyeOff className={iconButtonSize[size]} /> : <IconEye className={iconButtonSize[size]} />}
        </PlainButton>
      </div>
    );
  },
);
PasswordInput.displayName = 'PasswordInput';

export { PasswordInput };
export type { PasswordInputProps, PasswordInputSize };
