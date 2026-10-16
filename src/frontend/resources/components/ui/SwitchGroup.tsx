// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { type ReactNode } from 'react';
import { Switch } from './Switch';
import { Label } from './Label';
import { cn } from '@resources/utils/cn';

type SwitchOption = {
  value: string;
  label: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
};

type SwitchGroupProps = {
  label?: ReactNode;
  description?: ReactNode;
  options: SwitchOption[];
  values: string[];
  onChange: (values: string[]) => void;
  disabled?: boolean;
  orientation?: 'vertical' | 'horizontal';
  gap?: number;
  className?: string;
};

export function SwitchGroup({
  label,
  description,
  options,
  values,
  onChange,
  disabled = false,
  orientation = 'vertical',
  gap = 2,
  className,
}: SwitchGroupProps) {
  const isChecked = (value: string) => values.includes(value);

  const handleToggle = (value: string) => {
    if (disabled) return;
    const newValues = isChecked(value) ? values.filter((v) => v !== value) : [...values, value];
    onChange(newValues);
  };

  return (
    <div className={cn('space-y-1.5', className)}>
      {label && <Label>{label}</Label>}
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      <div
        className={cn('flex gap-(--gap)', orientation === 'vertical' ? 'flex-col' : 'flex-row flex-wrap')}
        style={cssVars({ gap: `${gap * 0.25}rem` })}
      >
        {options.map((option) => (
          <label
            key={option.value}
            className={cn('flex cursor-pointer items-center gap-3', option.disabled && 'cursor-not-allowed opacity-50')}
          >
            <Switch
              checked={isChecked(option.value)}
              onCheckedChange={() => handleToggle(option.value)}
              disabled={disabled || option.disabled}
            />
            <div className="min-w-0">
              <div className="text-sm font-medium text-foreground">{option.label}</div>
              {option.description && <div className="mt-0.5 text-xs text-muted-foreground">{option.description}</div>}
            </div>
          </label>
        ))}
      </div>
    </div>
  );
}
