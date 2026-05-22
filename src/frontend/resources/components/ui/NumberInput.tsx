// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type InputHTMLAttributes } from 'react';
import { IconMinus, IconPlus } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';

type NumberInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'onChange' | 'size'> & {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  step?: number;
  size?: 'default' | 'sm';
};

const sizeConfig = {
  default: {
    input: 'h-[42px] px-4 text-sm',
    button: 'h-[42px] w-9',
    icon: 'size-3.5',
  },
  sm: {
    input: 'h-8 px-3 text-xs',
    button: 'h-8 w-7',
    icon: 'size-3',
  },
};

const NumberInput = forwardRef<HTMLInputElement, NumberInputProps>(
  ({ className, value, onChange, min, max, step = 1, disabled, size = 'default', ...props }, ref) => {
    const s = sizeConfig[size];

    const decrement = () => {
      const next = value - step;
      if (min !== undefined && next < min) return;
      onChange(next);
    };

    const increment = () => {
      const next = value + step;
      if (max !== undefined && next > max) return;
      onChange(next);
    };

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const raw = e.target.value;
      if (raw === '' || raw === '-') return;
      const num = Number(raw);
      if (isNaN(num)) return;
      if (min !== undefined && num < min) return;
      if (max !== undefined && num > max) return;
      onChange(num);
    };

    return (
      <div className={cn('flex items-center', className)}>
        <input
          ref={ref}
          type="number"
          value={value}
          onChange={handleChange}
          min={min}
          max={max}
          step={step}
          disabled={disabled}
          className={cn(
            'w-full min-w-0 rounded-l-lg border border-border bg-card text-foreground [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary disabled:opacity-50',
            s.input,
          )}
          {...props}
        />
        <button
          type="button"
          onClick={decrement}
          disabled={disabled || (min !== undefined && value <= min)}
          aria-label="Decrease"
          className={cn(
            'flex shrink-0 items-center justify-center border border-l-0 border-border bg-card text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground disabled:opacity-50 disabled:pointer-events-none',
            s.button,
          )}
        >
          <IconMinus className={s.icon} />
        </button>
        <button
          type="button"
          onClick={increment}
          disabled={disabled || (max !== undefined && value >= max)}
          aria-label="Increase"
          className={cn(
            'flex shrink-0 items-center justify-center rounded-r-lg border border-l-0 border-border bg-card text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground disabled:opacity-50 disabled:pointer-events-none',
            s.button,
          )}
        >
          <IconPlus className={s.icon} />
        </button>
      </div>
    );
  }
);
NumberInput.displayName = 'NumberInput';

export { NumberInput };
